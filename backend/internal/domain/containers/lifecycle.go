package containers

import (
	"context"
	"fmt"
	"strings"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// This module owns the containers domain's lifecycle-action ritual once
// (issue #474): Start, Stop, and Restart previously hand-wrote the same
// pre-action sequence three times over, and the triplication echoed
// through every tier — three result structs, three response DTOs,
// three mappers, six test files. The module is domain-local, mirroring
// the quadlets domain's lifecycle.go placement precedent: the
// cross-domain extraction threshold is not crossed (pods' action
// helper has no pre-inspect and no identity result; quadlets'
// runLifecycle rides SSH/systemctl), so this is not a shared kit.

// lifecycleAction declares one lifecycle action's differences as pure
// data. The three actions differ only in these fields — the ritual
// itself (pre-action inspect, state gate, engine call, not-found
// classification, identity result) lives once in runLifecycleAction —
// so the value table below reads as one line per action and a fourth
// action would be one more value, nothing else. The engine method
// rides a method expression on the ContainerEngine interface, keeping
// the dispatch in the type system rather than a switch.
type lifecycleAction struct {
	// op is the op tag the ritual crosses the engine provider with
	// (containers.Start etc.) — the connection seam logs it.
	op string
	// wrapVerb prefixes an unclassified engine failure ("failed to
	// start container" etc.); the wrap text is the action's own.
	wrapVerb string
	// requiresRunning is the state gate: the action requires the
	// container to be running (stop, restart) or not running (start).
	// The gate is requiresRunning != containerRunning(report).
	requiresRunning bool
	// wrongState is the sentinel answered when the gate fails.
	wrongState error
	// run is the engine method the action dispatches to.
	run func(e ContainerEngine, id string) error
	// postState is the wire state the response DTO stamps at mapping
	// time (running for start/restart, stopped for stop).
	postState string
}

// The action table: one value per lifecycle action, no switches
// anywhere in the domain.
var (
	actionStart = lifecycleAction{
		op:              "containers.Start",
		wrapVerb:        "failed to start container",
		requiresRunning: false,
		wrongState:      appErrors.ErrContainerAlreadyRunning,
		run:             ContainerEngine.Start,
		postState:       "running",
	}
	actionStop = lifecycleAction{
		op:              "containers.Stop",
		wrapVerb:        "failed to stop container",
		requiresRunning: true,
		wrongState:      appErrors.ErrContainerNotRunning,
		run:             ContainerEngine.Stop,
		postState:       "stopped",
	}
	actionRestart = lifecycleAction{
		op:              "containers.Restart",
		wrapVerb:        "failed to restart container",
		requiresRunning: true,
		wrongState:      appErrors.ErrContainerNotRunning,
		run:             ContainerEngine.Restart,
		postState:       "running",
	}
)

// runLifecycleAction owns the whole pre-action ritual every lifecycle
// action shares: inspect once for existence, state, and name (through
// the existing single-report inspect resolution), answer the action's
// wrong-state terminal before any action call — podman's refusals
// produce no error shape through the tunnel (verified live, issue
// #270), so the precondition is state-based, mirroring the quadlets
// checkPrecondition — run the engine call, classify podman's
// not-found, and answer the container's identity. The Start/Stop/
// Restart wrappers keep their per-action contract comments; the
// sequencing itself lives here and nowhere else.
func (s *ContainerService) runLifecycleAction(ctx context.Context, userID, hostID, containerID string, action lifecycleAction) (*ContainerLifecycleResult, error) {
	var result *ContainerLifecycleResult
	err := s.engines.WithEngine(ctx, userID, hostID, action.op, func(e ContainerEngine) error {
		report, err := inspectContainer(e, containerID)
		if err != nil {
			return err
		}

		if action.requiresRunning != containerRunning(report) {
			return action.wrongState
		}

		if err := action.run(e, containerID); err != nil {
			if podmanerr.IsNotFound(err) {
				return appErrors.ErrContainerNotFound
			}
			return fmt.Errorf("%s: %w", action.wrapVerb, err)
		}

		result = &ContainerLifecycleResult{
			ID:   containerID,
			Name: containerName(report, containerID),
		}
		return nil
	})
	return result, err
}

// hasInspectData is the domain's one nil-guard for podman's inspect
// report: the report embeds its inspect data as a pointer, so every
// promoted field access (ID, Name, State, …) must first clear this
// predicate or panic. The lifecycle module owns it; the detail DTO's
// readers and mapper (dto.go) delegate through it.
func hasInspectData(report *entities.ContainerInspectReport) bool {
	return report != nil && report.InspectContainerData != nil
}

// containerRunning is the domain's one nil-safe running check: a
// paused container counts as running (podman's State.Running is true
// for it), and a report without embedded inspect data or state is not
// running. The detail DTO's state and exited readers delegate here.
func containerRunning(report *entities.ContainerInspectReport) bool {
	return hasInspectData(report) && report.State != nil && report.State.Running
}

// containerName is the domain's one nil-safe name resolver: it trims
// podman's leading slash and falls back to the caller-supplied
// fallback — the lifecycle actions pass the container ID, the detail
// DTO passes the report's ID. The guard covers a report without
// embedded inspect data (podman's report embeds it as a pointer), so
// the promoted field access cannot panic; the detail DTO's reader
// delegates here, which kills its latent nil-panic.
func containerName(report *entities.ContainerInspectReport, fallback string) string {
	if hasInspectData(report) && len(report.Name) > 0 {
		return strings.TrimPrefix(report.Name, "/")
	}
	return fallback
}
