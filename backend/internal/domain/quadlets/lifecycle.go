package quadlets

import (
	"context"
	"fmt"
	"strings"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/sshexec"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
)

// lifecycleAction represents a systemd lifecycle operation on a quadlet.
// Each action knows its own precondition rules, result interpretation,
// and response construction. This eliminates primitive obsession on action
// strings and consolidates switch-case logic into type methods.
type lifecycleAction string

const (
	actionStart   lifecycleAction = "start"
	actionStop    lifecycleAction = "stop"
	actionRestart lifecycleAction = "restart"
)

func (a lifecycleAction) checkPrecondition(state string) error {
	switch a {
	case actionStart:
		if state == "running" {
			return appErrors.ErrQuadletAlreadyRunning
		}
	case actionStop, actionRestart:
		if state != "running" {
			return appErrors.ErrQuadletNotRunning
		}
	}
	return nil
}

func (a lifecycleAction) interpretResult(result sshexec.ExecResult) error {
	if result.ExitCode == 0 {
		return nil
	}
	stderr := strings.ToLower(result.Stderr)

	if result.ExitCode == 5 || containsAny(stderr, "not found", "not loaded", "no such", "not-loaded") {
		return appErrors.ErrQuadletNotFound
	}

	switch a {
	case actionStart:
		if containsAny(stderr, "already", "active", "running") {
			return appErrors.ErrQuadletAlreadyRunning
		}
	case actionStop, actionRestart:
		if containsAny(stderr, "inactive", "not active", "not running", "not-active") {
			return appErrors.ErrQuadletNotRunning
		}
	}

	return fmt.Errorf("systemctl %s failed (exit %d): %s", a, result.ExitCode, result.Stderr)
}

func (a lifecycleAction) buildResponse(name, unit string) *QuadletLifecycleResponse {
	resp := &QuadletLifecycleResponse{
		ID:   name,
		Unit: unit,
	}
	switch a {
	case actionStart, actionRestart:
		resp.State = "running"
	case actionStop:
		resp.State = "stopped"
	}
	if a == actionRestart {
		resp.RestartedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return resp
}

// start starts a stopped quadlet-managed resource by running
// `systemctl start <unit>` over SSH (via SSHExecInterface from 070-10).
//
// The flow (doku/description_quadlets_api.md:163-185):
//
//  1. Resolve the quadlet via QuadletList — existence check (404 if absent)
//     and unit-name resolution (prefer the quadlet's UnitName; derive
//     `<base>.service` from the quadlet name as a fallback).
//  2. State precondition: if the resource is already running the start is
//     refused with QUADLET_ALREADY_RUNNING (400).
//  3. Run `systemctl start <unit>` over SSH.
//  4. Interpret a non-zero exit code defensively (e.g. unit-not-loaded → 404).
//
// Returns `{id, state: running, unit}` on success. Start has no force/timeout
// options per the contract.
func (s *QuadletService) Start(ctx context.Context, userID, machineID, name string) (*QuadletLifecycleResponse, error) {
	unit, err := s.resolveUnitNameForLifecycle(ctx, userID, machineID, name, actionStart)
	if err != nil {
		return nil, err
	}

	result, err := s.runSystemctl(ctx, machineID, userID, string(actionStart), unit, LifecycleOptions{})
	if err != nil {
		return nil, err
	}

	if err := actionStart.interpretResult(result); err != nil {
		return nil, err
	}

	return actionStart.buildResponse(name, unit), nil
}

// Stop stops a running quadlet-managed resource by running
// `systemctl stop <unit>` over SSH (doku/description_quadlets_api.md:188-215).
//
// Honours the contract's `force` and `timeout` query params: force maps to
// `systemctl --force`, timeout maps to `--kill-seconds=<n>`. Refuses a
// non-running resource with QUADLET_NOT_RUNNING (400).
//
// Returns `{id, state: stopped, unit}` on success.
func (s *QuadletService) Stop(ctx context.Context, userID, machineID, name string, opts LifecycleOptions) (*QuadletLifecycleResponse, error) {
	unit, err := s.resolveUnitNameForLifecycle(ctx, userID, machineID, name, actionStop)
	if err != nil {
		return nil, err
	}

	result, err := s.runSystemctl(ctx, machineID, userID, string(actionStop), unit, opts)
	if err != nil {
		return nil, err
	}

	if err := actionStop.interpretResult(result); err != nil {
		return nil, err
	}

	return actionStop.buildResponse(name, unit), nil
}

// Restart restarts a quadlet-managed resource by running
// `systemctl restart <unit>` over SSH (doku/description_quadlets_api.md:217-244).
//
// Supports the same `force`/`timeout` options as Stop. Refuses a non-running
// resource with QUADLET_NOT_RUNNING (400).
//
// Returns `{id, state: running, unit, restartedAt}` on success.
func (s *QuadletService) Restart(ctx context.Context, userID, machineID, name string, opts LifecycleOptions) (*QuadletLifecycleResponse, error) {
	unit, err := s.resolveUnitNameForLifecycle(ctx, userID, machineID, name, actionRestart)
	if err != nil {
		return nil, err
	}

	result, err := s.runSystemctl(ctx, machineID, userID, string(actionRestart), unit, opts)
	if err != nil {
		return nil, err
	}

	if err := actionRestart.interpretResult(result); err != nil {
		return nil, err
	}

	return actionRestart.buildResponse(name, unit), nil
}

// resolveUnitNameForLifecycle performs the shared pre-work for every lifecycle action:
// it locates the quadlet (404 if absent), evaluates the wrong-state guard for
// the action (400 if the current state disallows it), and resolves the systemd
// unit name to act on.
//
// The quadlet lookup goes through the podman QuadletList primitive (built
// inline, matching the rest of the service) so the existence check and
// unit-name resolution are consistent with the Get/Delete slices.
func (s *QuadletService) resolveUnitNameForLifecycle(ctx context.Context, userID, machineID, name string, action lifecycleAction) (string, error) {
	connCtx, err := s.connManager.GetConnection(machineID, userID)
	if err != nil {
		return "", appErrors.ErrConnectionFailed
	}
	defer s.connManager.Release(machineID)

	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	target, err := s.resolveTarget(containerEngine, name)
	if err != nil {
		return "", err
	}

	state := parseStateFromStatus(target.Status)
	if err := action.checkPrecondition(state); err != nil {
		return "", err
	}

	return resolveUnitName(target, name), nil
}

func (s *QuadletService) resolveTarget(containerEngine *tunnel.ContainerEngine, name string) (*entities.ListQuadlet, error) {
	list, err := containerEngine.QuadletList(context.Background(), entities.QuadletListOptions{Filters: []string{"name=" + name}})
	if err != nil {
		return nil, fmt.Errorf("failed to list quadlets: %w", err)
	}

	target := findQuadletByName(list, name)
	if target == nil {
		return nil, appErrors.ErrQuadletNotFound
	}
	return target, nil
}

// runSystemctl executes `systemctl <action> [flags] <unit>` over SSH via the
// SSHExecInterface. An SSH-layer failure (dial, session, network) maps to
// ErrConnectionFailed — fail-fast, no retry, per CONTEXT.md §4.
func (s *QuadletService) runSystemctl(ctx context.Context, machineID, userID, action, unit string, opts LifecycleOptions) (sshexec.ExecResult, error) {
	if s.sshExec == nil {
		// Defensive: production wiring always supplies an SSHExecInterface
		// (cmd/server/main.go). A nil dependency is a programming error.
		return sshexec.ExecResult{}, appErrors.ErrInternalError
	}

	result, err := s.sshExec.Exec(ctx, machineID, userID, "systemctl", sshexec.ExecOptions{
		Args: buildLifecycleArgs(action, unit, opts),
	})
	if err != nil {
		return sshexec.ExecResult{}, appErrors.ErrConnectionFailed
	}
	return result, nil
}

// buildLifecycleArgs builds the systemctl argument vector for a lifecycle
// action. force prepends the global `--force` flag; a positive timeout adds
// `--kill-seconds=<n>` (per the issue's resolved mechanism decision). The
// action verb and unit are always appended last so the result reads as
// `systemctl [--force] [--kill-seconds=N] <action> <unit>`.
func buildLifecycleArgs(action string, unit string, opts LifecycleOptions) []string {
	var args []string
	if opts.Force {
		args = append(args, "--force")
	}
	if opts.Timeout > 0 {
		args = append(args, fmt.Sprintf("--kill-seconds=%d", opts.Timeout))
	}
	args = append(args, "--user", action, unit)
	return args
}

// --- Pure lifecycle helpers ---
//
// These carry the real domain logic for the lifecycle slice and are unit-tested
// directly (see lifecycle_test.go). Extracting them keeps the service methods
// thin and preserves coverage of the meaningful rules — the QuadletList lookup
// they sit behind cannot be exercised under the mock connection, matching the
// established service-seam convention in this package.

// resolveUnitName returns the systemd unit name for a quadlet, preferring the
// UnitName reported by QuadletList and falling back to deriving `<base>.service`
// from the quadlet name (stripping the resource extension).
func resolveUnitName(quadlet *entities.ListQuadlet, name string) string {
	if quadlet.UnitName != "" {
		return quadlet.UnitName
	}
	return deriveServiceUnit(name)
}

// deriveServiceUnit derives a systemd service unit name from a quadlet name by
// stripping the trailing resource extension (`.container`, `.pod`, ...) and
// appending `.service`. Every quadlet resource type produces a `.service` unit.
func deriveServiceUnit(name string) string {
	base := name
	if idx := strings.LastIndex(name, "."); idx > 0 {
		base = name[:idx]
	}
	return base + ".service"
}

// containsAny reports whether s contains any of the substrings. Used by the
// exit-code interpreter's heuristic stderr matching.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
