package quadlets

import (
	"context"
	"fmt"

	"github.com/Homerus25/podland/backend/internal/removal"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// removeTerminals carries the two caller-owned terminal errors of the
// stop-and-remove ritual (issues 231 and 264). The ritual's shape is
// identical for both callers; what a vanished quadlet or a
// running-resource refusal MEANS is the caller's vocabulary, so the
// terminals are the caller's to pass in. nil is a legal missing
// terminal — the move source half answers nil for a vanished quadlet
// because the transfer's goal is met. The running terminal is a dropping
// closure: the cause (podman's refusal text) is dropped today in both
// callers, so the wire answers the bare terminal — the fact stays
// visible here, and the up-front gate calls the closure with no cause
// text.
type removeTerminals struct {
	// missing answers a quadlet that is not there — at the locate and at
	// the remove call (the adapter folds the report's error map into the
	// call's single error, issue 263).
	missing error
	// running answers the managed resource being up where the flags
	// refuse the removal — up front (no force, no stop) and in the
	// remove's own refusal mapping.
	running func(cause string) error
}

// stopAndRemove is the one stop-and-remove ritual both quadlet removal
// callers cross (issue 231): QuadletService.Delete and the move source half
// (removeSourceAfterInstall in move.go). It owns the whole engine lease —
// list, locate, the caller's terminals, the running refusal, the
// stop-before-remove gate, and the remove — parameterised
// by the op tag (so the two callers stay distinct in logs) and the
// terminals (so both stay byte-identical on the wire):
//
//   - A quadlet missing from the list answers terminals.missing before
//     anything is touched.
//   - A running resource with neither force nor the stop waiver answers
//     terminals.running up front. For the move source half this is the
//     accepted tightening of issue 231: a quadlet that started between the
//     preflight and the removal with stop=false now refuses here instead of
//     asking podman to refuse at the remove — same partial status,
//     byte-identical detail. No existing test pinned the old path.
//   - A running resource with stop set and no force is stopped through the
//     lifecycle seam (applyUnitAction — `systemctl [--user] stop`, scoped
//     to the SSH login's manager) before the remove; a stop failure
//     propagates and nothing is removed. For Delete this gate is
//     behaviour-identical to its old bare !force gate: the refusal above
//     already proved stop was set whenever a running quadlet gets here
//     without force.
//   - The remove is called with the caller's force (systemd reload
//     included) so systemd picks up the removal; the remove-and-classify
//     tail composes internal/removal (issue 264) — its not-found and
//     running-refusal answers map to the terminals, everything else to a
//     "failed to remove quadlet" wrap. The adapter folds the remove
//     report's error map for the named quadlet into the call's single
//     error (issue 263), so a race where the quadlet vanished or its
//     resource started between the locate and the remove maps the same
//     way.
func (s *QuadletService) stopAndRemove(ctx context.Context, userID, hostID, name string, force, stop bool, op string, terminals removeTerminals) error {
	return s.engines.WithEngine(ctx, userID, hostID, op, func(e QuadletEngine) error {
		list, err := e.List(entities.QuadletListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list quadlets: %w", err)
		}

		target := findQuadletByName(list, name)
		if target == nil {
			return terminals.missing
		}

		running := quadletResourceRunning(target)

		if running && !force && !stop {
			// The gate's refusal carries no podman text; the closure
			// drops the cause anyway.
			return terminals.running("")
		}

		if running && stop && !force {
			if err := s.applyUnitAction(ctx, userID, hostID, actionStop, resolveUnitName(target, name), LifecycleOptions{}); err != nil {
				return fmt.Errorf("failed to stop quadlet before remove: %w", err)
			}
		}

		// The remove-and-classify tail rides the shared removal module
		// (issue 264): the adapter's folded single error maps to the
		// caller's terminals — not-found to missing, the running refusal
		// (quadlet-local phrases, passed as the predicate) to running,
		// anything else to the wrap.
		return removal.Run(func() error {
			return e.Remove(name, force)
		}, removal.Terminals{
			Missing:   terminals.missing,
			Refusal:   terminals.running,
			IsRefusal: func(err error) bool { return !force && isResourceRunningError(err) },
		}, "failed to remove quadlet")
	})
}

// quadletResourceRunning is the one derivation of the running predicate
// (issue 231): the delete preconditions and the stop-and-remove body both
// evaluate it, so the ritual and its preflight cannot drift apart.
func quadletResourceRunning(q *entities.ListQuadlet) bool {
	return parseStateFromStatus(q.Status) == "running"
}
