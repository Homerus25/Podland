package quadlets

import (
	"context"
	"fmt"

	"github.com/Homerus25/podland/backend/internal/bulkremove"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
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
			if err := s.stopQuadletResource(ctx, userID, hostID, name, target); err != nil {
				return err
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

// stopQuadletResource is the one stop-before-remove gate both removal
// shapes cross (issue 231) — the single-item ritual and the batch
// preflight — through the lifecycle seam, so a stop edit cannot drift
// between them. The wrap keeps the ritual's message: the single-item
// surface returns it, the batch classifies it.
func (s *QuadletService) stopQuadletResource(ctx context.Context, userID, hostID, name string, target *entities.ListQuadlet) error {
	if err := s.applyUnitAction(ctx, userID, hostID, actionStop, resolveUnitName(target, name), LifecycleOptions{}); err != nil {
		return fmt.Errorf("failed to stop quadlet before remove: %w", err)
	}
	return nil
}

// BulkDelete removes several quadlets through ONE podman batch call —
// the batch remove endpoint accepts the whole array, so the bulk surface
// stops paying one podman call per id. The ritual keeps the
// stop-and-remove shape (issue 231) at batch width: one list locates
// every target, a missing id answers its QUADLET_NOT_FOUND entry, and a
// running resource is stopped through the lifecycle seam before the
// removal (the silent stop=true default, spec 461 user story 14 — the
// request has no stop field, so a client cannot turn it off). The
// removals then cross as one call whose report answers per name.
//
// A quadlet that fails AT the call answers the kit's generic internal
// entry: podman's batch endpoint marshals the report's error values as
// empty objects, so the wire names the failed quadlet but never the
// cause (the adapter owns that fact) — the running-refusal race window
// (a resource started between the list and the remove) answers the
// generic entry where the single-item delete answers
// QUADLET_RESOURCE_RUNNING. An acquisition or list failure answers the
// classified cause on every entry — byte-identical to the per-item
// loop's answer when every single-item call failed the same way.
func (s *QuadletService) BulkDelete(ctx context.Context, userID, hostID string, ids []string, force bool) []bulkremove.Entry {
	entries := make([]bulkremove.Entry, len(ids))
	for i, id := range ids {
		entries[i] = bulkremove.Entry{ID: id}
	}

	err := s.engines.WithEngine(ctx, userID, hostID, "quadlets.BulkRemove", func(e QuadletEngine) error {
		list, err := e.List(entities.QuadletListOptions{})
		if err != nil {
			// The only early exit: it runs before any entry is
			// answered, so the acquisition branch below may safely
			// answer every entry.
			return fmt.Errorf("failed to list quadlets: %w", err)
		}

		// pending carries the entry indices the batch call answers.
		var pending []int
		for i, id := range ids {
			target := findQuadletByName(list, id)
			if target == nil {
				entries[i].Error = bulkremove.Classify(ctx, id, appErrors.ErrQuadletNotFound)
				continue
			}
			if !force && quadletResourceRunning(target) {
				if err := s.stopQuadletResource(ctx, userID, hostID, id, target); err != nil {
					entries[i].Error = bulkremove.Classify(ctx, id, err)
					continue
				}
			}
			pending = append(pending, i)
		}
		if len(pending) == 0 {
			return nil
		}

		names := make([]string, len(pending))
		for i, idx := range pending {
			names[i] = ids[idx]
		}
		report, err := e.RemoveAll(names, force)
		if err != nil {
			for _, idx := range pending {
				entries[idx].Error = bulkremove.Classify(ctx, ids[idx], err)
			}
			return nil
		}

		removed := make(map[string]bool, len(report.Removed))
		for _, name := range report.Removed {
			removed[name] = true
		}
		failed := make(map[string]bool, len(report.Failed))
		for _, name := range report.Failed {
			failed[name] = true
		}
		for _, idx := range pending {
			name := ids[idx]
			switch {
			case removed[name]:
				entries[idx].Success = true
			case failed[name]:
				// The cause does not cross the wire (see the adapter):
				// the entry answers the kit's generic internal error,
				// with the cause logged once and never surfaced.
				entries[idx].Error = bulkremove.Classify(ctx, name,
					fmt.Errorf("quadlet removal failed: podman's batch report carries no cause"))
			default:
				entries[idx].Error = bulkremove.Classify(ctx, name,
					fmt.Errorf("quadlet absent from the batch remove report"))
			}
		}
		return nil
	})
	if err != nil {
		for i := range entries {
			entries[i].Error = bulkremove.Classify(ctx, ids[i], err)
		}
	}
	return entries
}
