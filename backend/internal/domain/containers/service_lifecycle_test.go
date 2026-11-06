package containers

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// lifecycleInspectAnswer builds a pre-action inspect answer for one
// container: it exists (one report), carries the given name and state,
// and answers no errors. The name keeps podman's leading slash so the
// resolver's trim is pinned on every row.
func lifecycleInspectAnswer(name string, state *define.InspectContainerState) func(string) ([]*entities.ContainerInspectReport, []error, error) {
	return func(string) ([]*entities.ContainerInspectReport, []error, error) {
		return []*entities.ContainerInspectReport{{
			InspectContainerData: &define.InspectContainerData{
				Name:  name,
				State: state,
			},
		}}, nil, nil
	}
}

// The inspect states the gate reads: a paused container counts as
// running (podman's State.Running is true for it), and a created
// (never started) container does not.
var (
	lifecycleRunningState = &define.InspectContainerState{Status: "running", Running: true}
	lifecyclePausedState  = &define.InspectContainerState{Status: "paused", Running: true, Paused: true}
	lifecycleStoppedState = &define.InspectContainerState{Status: "exited", Running: false}
	lifecycleCreatedState = &define.InspectContainerState{Status: "created", Running: false}
)

// lifecycleServiceRow is one action declaration under test at the
// service seam: the exported service method, the fake engine's arm the
// ritual dispatches to, the inspect answers whose state passes and
// fails the action's gate, and the per-action pins (op tag, wrap verb,
// wrong-state sentinel). The expected values come from the doku
// contract and the ticket, not from the declarations under test.
type lifecycleServiceRow struct {
	name string
	// call is the exported service method the row exercises.
	call func(s *ContainerService, ctx context.Context, userID, hostID, containerID string) (*ContainerLifecycleResult, error)
	// arm arms the fake engine's action method; the ritual reaches it
	// through the action's declared engine method.
	arm func(e *fakeContainerEngine, fn func(id string) error)
	// passingInspect is the pre-action inspect answer whose state
	// satisfies the action's gate.
	passingInspect func(string) ([]*entities.ContainerInspectReport, []error, error)
	// failingInspects are the answers whose state fails the gate; the
	// engine's action arm stays unarmed for them, so an action call
	// after the gate fails the test loudly.
	failingInspects []lifecycleFailingInspect
	// op is the op tag the ritual must cross the provider with.
	op string
	// wrapVerb prefixes an unclassified engine failure.
	wrapVerb   string
	wrongState error
}

// lifecycleFailingInspect is one gate-failing pre-action inspect answer
// with the subtest label naming the state that fails the gate.
type lifecycleFailingInspect struct {
	label   string
	inspect func(string) ([]*entities.ContainerInspectReport, []error, error)
}

func lifecycleServiceTable() []lifecycleServiceRow {
	return []lifecycleServiceRow{
		{
			name: "start",
			call: func(s *ContainerService, ctx context.Context, userID, hostID, containerID string) (*ContainerLifecycleResult, error) {
				return s.Start(ctx, userID, hostID, containerID)
			},
			arm:            func(e *fakeContainerEngine, fn func(id string) error) { e.startFn = fn },
			passingInspect: lifecycleInspectAnswer("/web", lifecycleStoppedState),
			// A paused container counts as running: podman's
			// State.Running is true for it.
			failingInspects: []lifecycleFailingInspect{
				{"running", lifecycleInspectAnswer("/web", lifecycleRunningState)},
				{"paused counts as running", lifecycleInspectAnswer("/web", lifecyclePausedState)},
			},
			op:         "containers.Start",
			wrapVerb:   "failed to start container",
			wrongState: appErrors.ErrContainerAlreadyRunning,
		},
		{
			name: "stop",
			call: func(s *ContainerService, ctx context.Context, userID, hostID, containerID string) (*ContainerLifecycleResult, error) {
				return s.Stop(ctx, userID, hostID, containerID)
			},
			arm:            func(e *fakeContainerEngine, fn func(id string) error) { e.stopFn = fn },
			passingInspect: lifecycleInspectAnswer("/web", lifecycleRunningState),
			// A created (never started) container is not running
			// either.
			failingInspects: []lifecycleFailingInspect{
				{"stopped", lifecycleInspectAnswer("/web", lifecycleStoppedState)},
				{"created", lifecycleInspectAnswer("/web", lifecycleCreatedState)},
			},
			op:         "containers.Stop",
			wrapVerb:   "failed to stop container",
			wrongState: appErrors.ErrContainerNotRunning,
		},
		{
			name: "restart",
			call: func(s *ContainerService, ctx context.Context, userID, hostID, containerID string) (*ContainerLifecycleResult, error) {
				return s.Restart(ctx, userID, hostID, containerID)
			},
			arm:            func(e *fakeContainerEngine, fn func(id string) error) { e.restartFn = fn },
			passingInspect: lifecycleInspectAnswer("/web", lifecycleRunningState),
			// A stopped container is Start's caller, not this one.
			failingInspects: []lifecycleFailingInspect{
				{"stopped", lifecycleInspectAnswer("/web", lifecycleStoppedState)},
			},
			op:         "containers.Restart",
			wrapVerb:   "failed to restart container",
			wrongState: appErrors.ErrContainerNotRunning,
		},
	}
}

// TestContainerService_Lifecycle_ResolvesContainerName pins the
// identity result through the gate-passing path: the service answers
// the container's ID and its inspect-resolved name — podman's leading
// slash trimmed — and crosses the provider with the action's own op
// tag.
func TestContainerService_Lifecycle_ResolvesContainerName(t *testing.T) {
	for _, row := range lifecycleServiceTable() {
		t.Run(row.name, func(t *testing.T) {
			var gotID string
			engine := &fakeContainerEngine{
				inspectFn: row.passingInspect,
			}
			row.arm(engine, func(id string) error {
				gotID = id
				return nil
			})
			var crossedOps []string
			service := NewContainerService(fakeEngineProvider{engine: engine, opRecorder: func(op string) { crossedOps = append(crossedOps, op) }})

			got, err := row.call(service, context.Background(), "user", "host", "container-123")

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, "container-123", gotID, "the action must receive the requested container")
			assert.Equal(t, []string{row.op}, crossedOps, "the ritual must cross the provider with the action's op tag")
			assert.Equal(t, "container-123", got.ID)
			assert.Equal(t, "web", got.Name, "the leading slash must be trimmed from podman's name")
		})
	}
}

// TestContainerService_Lifecycle_WrongStateAnswersSentinel pins the
// documented 400 terminals: the gate answers the action's wrong-state
// sentinel before any action call — podman's refusals produce no error
// shape through the tunnel (verified live, issue #270), so the
// precondition is state-based. The engine's action arm stays unarmed:
// an unexpected call panics.
func TestContainerService_Lifecycle_WrongStateAnswersSentinel(t *testing.T) {
	for _, row := range lifecycleServiceTable() {
		for _, failing := range row.failingInspects {
			t.Run(row.name+" / "+failing.label, func(t *testing.T) {
				engine := &fakeContainerEngine{inspectFn: failing.inspect}
				service := NewContainerService(fakeEngineProvider{engine: engine})

				got, err := row.call(service, context.Background(), "user", "host", "container-123")

				require.ErrorIs(t, err, row.wrongState)
				assert.Nil(t, got)
			})
		}
	}
}

// TestContainerService_Lifecycle_MapsNotFoundTo404 pins the 404
// terminal from all three sources: the inspect transport-level refusal,
// the inspect per-item refusal (the tunnel's not-found answer for
// inspect arrives as a per-item error with zero reports), and the
// action call's own refusal — the race window where the container
// vanished between the precondition inspect and the action's
// execution.
func TestContainerService_Lifecycle_MapsNotFoundTo404(t *testing.T) {
	notFound := &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}

	sources := []struct {
		name string
		arm  func(e *fakeContainerEngine, row lifecycleServiceRow)
	}{
		{
			name: "inspect transport-level refusal",
			arm: func(e *fakeContainerEngine, row lifecycleServiceRow) {
				e.inspectFn = func(string) ([]*entities.ContainerInspectReport, []error, error) {
					return nil, nil, notFound
				}
			},
		},
		{
			name: "inspect per-item refusal",
			arm: func(e *fakeContainerEngine, row lifecycleServiceRow) {
				e.inspectFn = func(string) ([]*entities.ContainerInspectReport, []error, error) {
					return nil, []error{fmt.Errorf("no such container %q", "container-123")}, nil
				}
			},
		},
		{
			name: "action refusal",
			arm: func(e *fakeContainerEngine, row lifecycleServiceRow) {
				e.inspectFn = row.passingInspect
				row.arm(e, func(string) error { return notFound })
			},
		},
	}

	for _, row := range lifecycleServiceTable() {
		for _, source := range sources {
			t.Run(row.name+" / "+source.name, func(t *testing.T) {
				engine := &fakeContainerEngine{}
				source.arm(engine, row)
				service := NewContainerService(fakeEngineProvider{engine: engine})

				got, err := row.call(service, context.Background(), "user", "host", "container-123")

				require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
				assert.Nil(t, got)
			})
		}
	}
}

// TestContainerService_Lifecycle_WrapsEngineFailure pins the wrap
// verb: an unclassified engine failure is wrapped with the action's
// own verb, never answered as a success and never misclassified as the
// 404 sentinel.
func TestContainerService_Lifecycle_WrapsEngineFailure(t *testing.T) {
	for _, row := range lifecycleServiceTable() {
		t.Run(row.name, func(t *testing.T) {
			engine := &fakeContainerEngine{inspectFn: row.passingInspect}
			row.arm(engine, func(string) error { return assert.AnError })
			service := NewContainerService(fakeEngineProvider{engine: engine})

			got, err := row.call(service, context.Background(), "user", "host", "container-123")

			require.Error(t, err)
			assert.Nil(t, got)
			assert.NotErrorIs(t, err, appErrors.ErrContainerNotFound)
			assert.Contains(t, err.Error(), row.wrapVerb)
		})
	}
}
