package quadlets

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/sshexec"
	"go.podman.io/podman/v6/pkg/domain/entities"
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
// `systemctl [--user] start <unit>` over SSH (via SSHExecInterface from
// 070-10) — scoped to the SSH user's manager unless the machine's
// registered login is root.
//
// The flow (doku/description_quadlets_api.md:168-192):
//
//  1. Resolve the quadlet via QuadletList — existence check (404 if absent)
//     and unit-name resolution (prefer the quadlet's UnitName; derive
//     `<base>.service` from the quadlet name as a fallback).
//  2. State precondition: if the resource is already running the start is
//     refused with QUADLET_ALREADY_RUNNING (400).
//  3. Run `systemctl [--user] start <unit>` over SSH (user manager
//     unless the SSH login is root).
//  4. Interpret a non-zero exit code defensively (e.g. unit-not-loaded → 404).
//
// Returns `{id, state: running, unit}` on success. Start has no force/timeout
// options per the contract.
func (s *QuadletService) Start(ctx context.Context, userID, machineID, name string) (*QuadletLifecycleResponse, error) {
	return s.runLifecycle(ctx, userID, machineID, name, actionStart, LifecycleOptions{})
}

// Stop stops a running quadlet-managed resource by running
// `systemctl [--user] stop <unit>` over SSH (user manager unless the
// machine's SSH login is root)
// (doku/description_quadlets_api.md:193-231).
//
// Honours the contract's `timeout` query param: it bounds the wait via the
// coreutils timeout wrapper (expiry answers QUADLET_STOP_TIMED_OUT, 504).
// No force option (issue 205 — see LifecycleOptions). Refuses a
// non-running resource with QUADLET_NOT_RUNNING (400).
//
// Returns `{id, state: stopped, unit}` on success.
func (s *QuadletService) Stop(ctx context.Context, userID, machineID, name string, opts LifecycleOptions) (*QuadletLifecycleResponse, error) {
	return s.runLifecycle(ctx, userID, machineID, name, actionStop, opts)
}

// Restart restarts a quadlet-managed resource by running
// `systemctl [--user] restart <unit>` over SSH (user manager unless the
// machine's SSH login is root)
// (doku/description_quadlets_api.md:232-265).
//
// Supports the same `timeout` option as Stop — it bounds the wait for the
// whole restart (one wrapped call). No force option (issue 205 — see
// LifecycleOptions). Refuses a non-running resource with
// QUADLET_NOT_RUNNING (400).
//
// Returns `{id, state: running, unit, restartedAt}` on success.
func (s *QuadletService) Restart(ctx context.Context, userID, machineID, name string, opts LifecycleOptions) (*QuadletLifecycleResponse, error) {
	return s.runLifecycle(ctx, userID, machineID, name, actionRestart, opts)
}

// runLifecycle is the three-step ritual every lifecycle action shares:
// resolve the unit and enforce the action's state precondition
// (resolveUnitNameForLifecycle), execute the action over SSH
// (applyUnitAction), then build the response (lifecycleAction.buildResponse).
// The Start/Stop/Restart wrappers above keep the per-action contract
// comments; the sequencing itself lives here and nowhere else.
func (s *QuadletService) runLifecycle(ctx context.Context, userID, machineID, name string, action lifecycleAction, opts LifecycleOptions) (*QuadletLifecycleResponse, error) {
	unit, err := s.resolveUnitNameForLifecycle(ctx, userID, machineID, name, action)
	if err != nil {
		return nil, err
	}

	if err := s.applyUnitAction(ctx, userID, machineID, action, unit, opts); err != nil {
		return nil, err
	}

	return action.buildResponse(name, unit), nil
}

// resolveUnitNameForLifecycle performs the shared pre-work for every lifecycle action:
// it locates the quadlet (404 if absent), evaluates the wrong-state guard for
// the action (400 if the current state disallows it), and resolves the systemd
// unit name to act on.
//
// The quadlet lookup goes through the domain's engine provider (the
// connection seam) so the existence check and unit-name resolution are
// consistent with the Get/Delete slices.
func (s *QuadletService) resolveUnitNameForLifecycle(ctx context.Context, userID, machineID, name string, action lifecycleAction) (string, error) {
	var unit string
	err := s.engines.WithEngine(ctx, userID, machineID, "quadlets.Lifecycle", func(e QuadletEngine) error {
		target, err := s.resolveTarget(e, name)
		if err != nil {
			return err
		}

		state := parseStateFromStatus(target.Status)
		if err := action.checkPrecondition(state); err != nil {
			return err
		}

		unit = resolveUnitName(target, name)
		return nil
	})
	return unit, err
}

func (s *QuadletService) resolveTarget(e QuadletEngine, name string) (*entities.ListQuadlet, error) {
	list, err := e.List(entities.QuadletListOptions{Filters: []string{"name=" + name}})
	if err != nil {
		return nil, fmt.Errorf("failed to list quadlets: %w", err)
	}

	target := findQuadletByName(list, name)
	if target == nil {
		return nil, appErrors.ErrQuadletNotFound
	}
	return target, nil
}

// applyUnitAction is the service's systemctl seam for one lifecycle action:
// it builds the argv from the typed action and options (the action × options
// table lives here and nowhere else), executes it over SSH, and interprets a
// non-zero exit into the domain's error vocabulary. Callers supply a resolved
// unit (resolveUnitNameForLifecycle) and finish with buildResponse.
//
// The argv reads `systemctl [--user] <action> <unit>`: the action verb and
// unit come last, and `--user` scopes the call to the SSH user's manager
// unless the machine's registered login is root (root has no meaningful
// user manager; its units live in the system manager — systemctlUserScope).
// There is no force flag in the vocabulary — systemd ignores --force
// for stop/restart (issue 205; the full ruling lives on LifecycleOptions),
// so aggressive teardown stays with DELETE's force.
// A positive timeout wraps the whole call in the coreutils `timeout` wrapper —
// `timeout <n> systemctl [--user] <action> <unit>` — because no systemd version
// has a per-stop-call timeout flag. Expiry exits 124 and maps to
// ErrQuadletStopTimedOut (the WAIT was given up; the stop job may still
// complete in systemd). The wrapper's own failures (125/126/127: timeout
// itself failed, command not executable / not found) keep the generic 500
// mapping — they are transport problems, not quadlet states.
//
// An SSH-layer failure (dial, session, network) maps to ErrConnectionFailed —
// fail-fast, no retry, per docs/architecture-decisions.md §4.
func (s *QuadletService) applyUnitAction(ctx context.Context, userID, machineID string, action lifecycleAction, unit string, opts LifecycleOptions) error {
	userScope, err := s.systemctlUserScope(machineID)
	if err != nil {
		return err
	}

	command := "systemctl"
	args := []string{string(action), unit}
	if userScope {
		args = append([]string{"--user"}, args...)
	}
	if opts.Timeout > 0 {
		command = "timeout"
		args = append([]string{strconv.Itoa(opts.Timeout), "systemctl"}, args...)
	}

	result, err := s.execSystemctl(ctx, machineID, userID, command, args)
	if err != nil {
		return err
	}

	if opts.Timeout > 0 {
		switch result.ExitCode {
		case 124:
			return appErrors.ErrQuadletStopTimedOut
		case 125, 126, 127:
			return fmt.Errorf("timeout wrapper failed for systemctl %s (exit %d): %s",
				action, result.ExitCode, result.Stderr)
		}
	}

	return action.interpretResult(result)
}

// execSystemctl executes a command with its argument vector over SSH via the
// SSHExecInterface. It owns the nil-check and the ErrConnectionFailed mapping
// so every caller (the lifecycle actions and reloadUserSystemd) shares one
// connection-failure contract.
func (s *QuadletService) execSystemctl(ctx context.Context, machineID, userID, command string, args []string) (sshexec.ExecResult, error) {
	if s.sshExec == nil {
		// Defensive: production wiring always supplies an SSHExecInterface
		// (cmd/server/main.go). A nil dependency is a programming error.
		return sshexec.ExecResult{}, appErrors.ErrInternalError
	}

	result, err := s.sshExec.Exec(ctx, machineID, userID, command, sshexec.ExecOptions{
		Args: args,
	})
	if err != nil {
		return sshexec.ExecResult{}, appErrors.ErrConnectionFailed
	}
	return result, nil
}

// systemctlUserScope resolves whether systemctl on the given machine must
// address the user manager (`--user`): true unless the machine's registered
// SSH login is root. A machine row that vanished (nil, nil from GetByID —
// e.g. deleted concurrently with the request) is a 404, mirroring the
// machines domain's mapping; a store failure is an internal error.
func (s *QuadletService) systemctlUserScope(machineID string) (bool, error) {
	if s.machines == nil {
		// Defensive: production wiring always supplies the machine lookup
		// (internal/app). A nil dependency is a programming error.
		return false, appErrors.ErrInternalError
	}

	machine, err := s.machines.GetByID(machineID)
	if err != nil {
		return false, fmt.Errorf("%w: failed to resolve machine %q for systemctl scoping: %v",
			appErrors.ErrInternalError, machineID, err)
	}
	if machine == nil {
		return false, appErrors.ErrMachineNotFound
	}
	// Trim + case-insensitive: registration never trims the username, and a
	// uid-0 login spelled "Root" is still root.
	return !strings.EqualFold(strings.TrimSpace(machine.SSHUsername), "root"), nil
}

// reloadUserSystemd runs `systemctl daemon-reload` over the SSH seam,
// against the same manager the lifecycle actions act on — the SSH user's
// manager (`--user`) unless the registered login is root, in which case the
// system manager owns the units and no flag is passed.
//
// Podman's install/remove primitives reload systemd themselves, but over DBus
// from the podman service process — which reaches whichever manager that
// process can see (the system manager for a rootful socket, the service
// user's manager otherwise), not necessarily the manager the lifecycle's
// systemctl over SSH addresses. Without an explicit reload, a freshly
// installed quadlet still lists (podman reads the files from disk) but its
// unit was never generated into the target manager, so Start fails with
// QUADLET_NOT_FOUND. The install paths (Create/Update/Import) call this
// after a successful install to close that gap.
//
// An SSH-layer failure and a non-zero systemctl exit both map to
// ErrConnectionFailed: either way the manager the lifecycle depends on
// could not be reached.
func (s *QuadletService) reloadUserSystemd(ctx context.Context, machineID, userID string) error {
	userScope, err := s.systemctlUserScope(machineID)
	if err != nil {
		return err
	}

	args := []string{"daemon-reload"}
	cmd := "systemctl daemon-reload"
	if userScope {
		args = []string{"--user", "daemon-reload"}
		cmd = "systemctl --user daemon-reload"
	}

	result, err := s.execSystemctl(ctx, machineID, userID, "systemctl", args)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%w: %s failed (exit %d): %s",
			appErrors.ErrConnectionFailed, cmd, result.ExitCode, result.Stderr)
	}
	return nil
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
