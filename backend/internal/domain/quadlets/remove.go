package quadlets

import (
	"context"
	"fmt"

	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// removeTerminals carries the two caller-owned terminal errors of the
// stop-and-remove ritual (issue 231). The ritual's shape is identical for
// both callers; what a vanished quadlet or a running-resource refusal MEANS
// is the caller's vocabulary, so the terminals are plain values the caller
// passes in. nil is a legal terminal — the move source half answers nil for
// a vanished quadlet because the transfer's goal is met — and the running
// cause is dropped today in both callers, so a value, not a builder.
type removeTerminals struct {
	// missing answers a quadlet that is not there — at the locate, at the
	// remove call, and in the report scan.
	missing error
	// running answers the managed resource being up where the flags refuse
	// the removal — up front (no force, no stop) and in the remove's own
	// refusal mapping.
	running error
}

// stopAndRemove is the one stop-and-remove ritual both quadlet removal
// callers cross (issue 231): QuadletService.Delete and the move source half
// (removeSourceAfterInstall in move.go). It owns the whole engine lease —
// list, locate, the caller's terminals, the running refusal, the
// stop-before-remove gate, the remove, and the report scan — parameterised
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
//     included) so systemd picks up the removal; its not-found and
//     running-refusal answers map to the terminals, everything else to a
//     "failed to remove quadlet" wrap. A failure that surfaces in the
//     report's Errors map (a race where the quadlet vanished or its
//     resource started between the locate and the remove) maps the same
//     way.
func (s *QuadletService) stopAndRemove(ctx context.Context, userID, machineID, name string, force, stop bool, op string, terminals removeTerminals) error {
	return s.engines.WithEngine(ctx, userID, machineID, op, func(e QuadletEngine) error {
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
			return terminals.running
		}

		if running && stop && !force {
			if err := s.applyUnitAction(ctx, userID, machineID, actionStop, resolveUnitName(target, name), LifecycleOptions{}); err != nil {
				return fmt.Errorf("failed to stop quadlet before remove: %w", err)
			}
		}

		report, err := e.Remove(name, force)
		if err != nil {
			if podmanerr.IsNotFound(err) {
				return terminals.missing
			}
			if !force && isResourceRunningError(err) {
				return terminals.running
			}
			return fmt.Errorf("failed to remove quadlet: %w", err)
		}

		if report != nil {
			if qErr, ok := report.Errors[name]; ok && qErr != nil {
				if podmanerr.IsNotFound(qErr) {
					return terminals.missing
				}
				if !force && isResourceRunningError(qErr) {
					return terminals.running
				}
				return fmt.Errorf("failed to remove quadlet: %w", qErr)
			}
		}

		return nil
	})
}

// quadletResourceRunning is the one derivation of the running predicate
// (issue 231): the delete preconditions and the stop-and-remove body both
// evaluate it, so the ritual and its preflight cannot drift apart.
func quadletResourceRunning(q *entities.ListQuadlet) bool {
	return parseStateFromStatus(q.Status) == "running"
}
