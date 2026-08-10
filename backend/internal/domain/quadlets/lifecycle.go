package quadlets

import (
	"context"
	"fmt"
	"strings"
	"time"

	appErrors "github.com/yourorg/podland/backend/internal/errors"
	"github.com/yourorg/podland/backend/internal/sshexec"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
)

// Lifecycle action constants. Using named constants keeps the action plumbing
// (precondition check, arg building, exit-code interpretation) consistent
// across Start/Stop/Restart and typo-proof.
const (
	actionStart   = "start"
	actionStop    = "stop"
	actionRestart = "restart"
)

// start starts a stopped quadlet-managed resource by running
// `systemctl start <unit>` over SSH (via SSHExecInterface from 070-10).
//
// The flow (doku/description_api.md:988-1010):
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
	unit, err := s.resolveForLifecycle(ctx, userID, machineID, name, actionStart)
	if err != nil {
		return nil, err
	}

	result, err := s.runSystemctl(ctx, machineID, userID, actionStart, unit, LifecycleOptions{})
	if err != nil {
		return nil, err
	}

	if err := interpretLifecycleResult(actionStart, result); err != nil {
		return nil, err
	}

	return buildLifecycleResponse(name, unit, actionStart), nil
}

// Stop stops a running quadlet-managed resource by running
// `systemctl stop <unit>` over SSH (doku/description_api.md:1013-1039).
//
// Honours the contract's `force` and `timeout` query params: force maps to
// `systemctl --force`, timeout maps to `--kill-seconds=<n>`. Refuses a
// non-running resource with QUADLET_NOT_RUNNING (400).
//
// Returns `{id, state: stopped, unit}` on success.
func (s *QuadletService) Stop(ctx context.Context, userID, machineID, name string, opts LifecycleOptions) (*QuadletLifecycleResponse, error) {
	unit, err := s.resolveForLifecycle(ctx, userID, machineID, name, actionStop)
	if err != nil {
		return nil, err
	}

	result, err := s.runSystemctl(ctx, machineID, userID, actionStop, unit, opts)
	if err != nil {
		return nil, err
	}

	if err := interpretLifecycleResult(actionStop, result); err != nil {
		return nil, err
	}

	return buildLifecycleResponse(name, unit, actionStop), nil
}

// Restart restarts a quadlet-managed resource by running
// `systemctl restart <unit>` over SSH (doku/description_api.md:1042-1069).
//
// Supports the same `force`/`timeout` options as Stop. Refuses a non-running
// resource with QUADLET_NOT_RUNNING (400).
//
// Returns `{id, state: running, unit, restartedAt}` on success.
func (s *QuadletService) Restart(ctx context.Context, userID, machineID, name string, opts LifecycleOptions) (*QuadletLifecycleResponse, error) {
	unit, err := s.resolveForLifecycle(ctx, userID, machineID, name, actionRestart)
	if err != nil {
		return nil, err
	}

	result, err := s.runSystemctl(ctx, machineID, userID, actionRestart, unit, opts)
	if err != nil {
		return nil, err
	}

	if err := interpretLifecycleResult(actionRestart, result); err != nil {
		return nil, err
	}

	return buildLifecycleResponse(name, unit, actionRestart), nil
}

// resolveForLifecycle performs the shared pre-work for every lifecycle action:
// it locates the quadlet (404 if absent), evaluates the wrong-state guard for
// the action (400 if the current state disallows it), and resolves the systemd
// unit name to act on.
//
// The quadlet lookup goes through the podman QuadletList primitive (built
// inline, matching the rest of the service) so the existence check and
// unit-name resolution are consistent with the Get/Delete slices.
func (s *QuadletService) resolveForLifecycle(ctx context.Context, userID, machineID, name, action string) (string, error) {
	connCtx, err := s.connManager.GetConnection(machineID, userID)
	if err != nil {
		return "", appErrors.ErrConnectionFailed
	}
	defer s.connManager.Release(machineID)

	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	list, err := containerEngine.QuadletList(connCtx, entities.QuadletListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list quadlets: %w", err)
	}

	target := findQuadletByName(list, name)
	if target == nil {
		return "", appErrors.ErrQuadletNotFound
	}

	state := parseStateFromStatus(target.Status)
	if err := checkLifecyclePrecondition(action, state); err != nil {
		return "", err
	}

	return resolveUnitName(target, name), nil
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

// --- Pure lifecycle helpers ---
//
// These carry the real domain logic for the lifecycle slice and are unit-tested
// directly (see lifecycle_test.go). Extracting them keeps the service methods
// thin and preserves coverage of the meaningful rules — the QuadletList lookup
// they sit behind cannot be exercised under the mock connection, matching the
// established service-seam convention in this package.

// resolveUnitName returns the systemd unit name for a quadlet, preferring the
// UnitName reported by QuadletList and falling back to deriving `<base>.service`
// from the quadlet name (stripping the resource extension). A nil quadlet is
// tolerated — the fallback derives from the name alone.
func resolveUnitName(quadlet *entities.ListQuadlet, name string) string {
	if quadlet != nil && quadlet.UnitName != "" {
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

// checkLifecyclePrecondition evaluates the wrong-state guard for a lifecycle
// action against the quadlet's current normalized state. It returns:
//
//   - start:    ErrQuadletAlreadyRunning when already running
//   - stop:     ErrQuadletNotRunning      when not running
//   - restart:  ErrQuadletNotRunning      when not running
//
// "Not running" means any state other than "running" (stopped, unknown, error).
// Starting from an error/unknown state is permitted so a failed unit can be
// recovered.
func checkLifecyclePrecondition(action, state string) error {
	switch action {
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

// buildLifecycleArgs builds the systemctl argument vector for a lifecycle
// action. force prepends the global `--force` flag; a positive timeout adds
// `--kill-seconds=<n>` (per the issue's resolved mechanism decision). The
// action verb and unit are always appended last so the result reads as
// `systemctl [--force] [--kill-seconds=N] <action> <unit>`.
func buildLifecycleArgs(action, unit string, opts LifecycleOptions) []string {
	var args []string
	if opts.Force {
		args = append(args, "--force")
	}
	if opts.Timeout > 0 {
		args = append(args, fmt.Sprintf("--kill-seconds=%d", opts.Timeout))
	}
	args = append(args, action, unit)
	return args
}

// interpretLifecycleResult maps a non-zero systemctl exit to the appropriate
// AppError, based on the exit code and stderr content. Returns nil on success
// (exit 0).
//
// This is defensive: the QuadletList pre-check in resolveForLifecycle already
// rejects wrong-state/not-found before systemctl runs. These mappings catch
// races (the state changed between the pre-check and the exec) so callers still
// see the right error code rather than a generic 500.
func interpretLifecycleResult(action string, result sshexec.ExecResult) error {
	if result.ExitCode == 0 {
		return nil
	}
	stderr := strings.ToLower(result.Stderr)

	// systemctl exit 5 = "unit ... not found" (unit not loaded). Also match
	// common phrasings in stderr so a non-5 exit code with a clear not-found
	// message still maps correctly.
	if result.ExitCode == 5 || containsAny(stderr, "not found", "not loaded", "no such", "not-loaded") {
		return appErrors.ErrQuadletNotFound
	}

	// Wrong-state races: the unit's state shifted between the pre-check and
	// the exec. Map the systemctl-reported state back to the action's
	// specific wrong-state error.
	switch action {
	case actionStart:
		if containsAny(stderr, "already", "active", "running") {
			return appErrors.ErrQuadletAlreadyRunning
		}
	case actionStop, actionRestart:
		if containsAny(stderr, "inactive", "not active", "not running", "not-active") {
			return appErrors.ErrQuadletNotRunning
		}
	}

	return fmt.Errorf("systemctl %s failed (exit %d): %s", action, result.ExitCode, result.Stderr)
}

// buildLifecycleResponse assembles the success body for a lifecycle action.
// Start/restart report state "running"; stop reports "stopped". Restart also
// carries a restartedAt timestamp (ISO8601, per doku/description_api.md:1058).
func buildLifecycleResponse(name, unit, action string) *QuadletLifecycleResponse {
	resp := &QuadletLifecycleResponse{
		ID:   name,
		Unit: unit,
	}
	switch action {
	case actionStart, actionRestart:
		resp.State = "running"
	case actionStop:
		resp.State = "stopped"
	}
	if action == actionRestart {
		resp.RestartedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return resp
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
