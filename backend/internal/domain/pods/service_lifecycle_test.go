package pods

import (
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// TestPodService_Pause_ReachesEngine pins the pause behaviour: the
// URL-param pod reaches the engine and a clean answer succeeds.
func TestPodService_Pause_ReachesEngine(t *testing.T) {
	var gotName string
	engine := &fakePodEngine{
		pauseFn: func(nameOrID string) error {
			gotName = nameOrID
			return nil
		},
	}
	var gotOp string
	service := NewPodService(fakeEngineProvider{engine: engine, opRecorder: func(op string) { gotOp = op }})

	err := service.Pause(t.Context(), "user", "machine", "web")

	require.NoError(t, err)
	assert.Equal(t, "web", gotName, "the URL-param pod must reach the machine")
	assert.Equal(t, "pods.Pause", gotOp, "the op tag must name the pods domain")
}

// TestPodService_Start_ReachesEngine pins the start behaviour: the
// URL-param pod reaches the engine and a clean answer succeeds.
func TestPodService_Start_ReachesEngine(t *testing.T) {
	var gotName string
	engine := &fakePodEngine{
		startFn: func(nameOrID string) error {
			gotName = nameOrID
			return nil
		},
	}
	var gotOp string
	service := NewPodService(fakeEngineProvider{engine: engine, opRecorder: func(op string) { gotOp = op }})

	err := service.Start(t.Context(), "user", "machine", "web")

	require.NoError(t, err)
	assert.Equal(t, "web", gotName, "the URL-param pod must reach the machine")
	assert.Equal(t, "pods.Start", gotOp, "the op tag must name the pods domain")
}

// TestPodService_Stop_ReachesEngine pins the stop behaviour: the
// URL-param pod reaches the engine and a clean answer succeeds.
func TestPodService_Stop_ReachesEngine(t *testing.T) {
	var gotName string
	engine := &fakePodEngine{
		stopFn: func(nameOrID string) error {
			gotName = nameOrID
			return nil
		},
	}
	var gotOp string
	service := NewPodService(fakeEngineProvider{engine: engine, opRecorder: func(op string) { gotOp = op }})

	err := service.Stop(t.Context(), "user", "machine", "web")

	require.NoError(t, err)
	assert.Equal(t, "web", gotName, "the URL-param pod must reach the machine")
	assert.Equal(t, "pods.Stop", gotOp, "the op tag must name the pods domain")
}

// TestPodService_Lifecycle_MissingPodAnswers404 pins the documented 404
// on every lifecycle op: the tunnel's "unable to find pod" answer
// classifies into ErrPodNotFound.
func TestPodService_Lifecycle_MissingPodAnswers404(t *testing.T) {
	notFound := &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "unable to find pod"}
	tests := []struct {
		name  string
		armed func(e *fakePodEngine)
		call  func(s *PodService) error
	}{
		{
			name:  "pause",
			armed: func(e *fakePodEngine) { e.pauseFn = func(string) error { return notFound } },
			call:  func(s *PodService) error { return s.Pause(t.Context(), "user", "machine", "ghost") },
		},
		{
			name:  "unpause",
			armed: func(e *fakePodEngine) { e.unpauseFn = func(string) error { return notFound } },
			call:  func(s *PodService) error { return s.Unpause(t.Context(), "user", "machine", "ghost") },
		},
		{
			name:  "kill",
			armed: func(e *fakePodEngine) { e.killFn = func(string) error { return notFound } },
			call:  func(s *PodService) error { return s.Kill(t.Context(), "user", "machine", "ghost") },
		},
		{
			name:  "restart",
			armed: func(e *fakePodEngine) { e.restartFn = func(string) error { return notFound } },
			call:  func(s *PodService) error { return s.Restart(t.Context(), "user", "machine", "ghost") },
		},
		{
			name:  "start",
			armed: func(e *fakePodEngine) { e.startFn = func(string) error { return notFound } },
			call:  func(s *PodService) error { return s.Start(t.Context(), "user", "machine", "ghost") },
		},
		{
			name:  "stop",
			armed: func(e *fakePodEngine) { e.stopFn = func(string) error { return notFound } },
			call:  func(s *PodService) error { return s.Stop(t.Context(), "user", "machine", "ghost") },
		},
		{
			name:  "remove",
			armed: func(e *fakePodEngine) { e.removeFn = func(string, bool) error { return notFound } },
			call:  func(s *PodService) error { return s.Remove(t.Context(), "user", "machine", "ghost", false) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &fakePodEngine{}
			tt.armed(engine)
			service := NewPodService(fakeEngineProvider{engine: engine})

			err := tt.call(service)

			require.ErrorIs(t, err, appErrors.ErrPodNotFound)
		})
	}
}

// TestPodService_Lifecycle_ConflictRefusalAnswersStateConflict pins the
// refusal classification: podman's conflict answers (409 ErrorModels)
// classify into the domain's 409 POD_STATE_CONFLICT carrying podman's
// message.
func TestPodService_Lifecycle_ConflictRefusalAnswersStateConflict(t *testing.T) {
	conflict := &errorhandling.ErrorModel{ResponseCode: http.StatusConflict, Message: "pod is paused"}
	tests := []struct {
		name  string
		armed func(e *fakePodEngine)
		call  func(s *PodService) error
	}{
		{
			name:  "pause",
			armed: func(e *fakePodEngine) { e.pauseFn = func(string) error { return conflict } },
			call:  func(s *PodService) error { return s.Pause(t.Context(), "user", "machine", "web") },
		},
		{
			name:  "unpause",
			armed: func(e *fakePodEngine) { e.unpauseFn = func(string) error { return conflict } },
			call:  func(s *PodService) error { return s.Unpause(t.Context(), "user", "machine", "web") },
		},
		{
			name:  "kill",
			armed: func(e *fakePodEngine) { e.killFn = func(string) error { return conflict } },
			call:  func(s *PodService) error { return s.Kill(t.Context(), "user", "machine", "web") },
		},
		{
			name:  "restart",
			armed: func(e *fakePodEngine) { e.restartFn = func(string) error { return conflict } },
			call:  func(s *PodService) error { return s.Restart(t.Context(), "user", "machine", "web") },
		},
		{
			name:  "start",
			armed: func(e *fakePodEngine) { e.startFn = func(string) error { return conflict } },
			call:  func(s *PodService) error { return s.Start(t.Context(), "user", "machine", "web") },
		},
		{
			name:  "stop",
			armed: func(e *fakePodEngine) { e.stopFn = func(string) error { return conflict } },
			call:  func(s *PodService) error { return s.Stop(t.Context(), "user", "machine", "web") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &fakePodEngine{}
			tt.armed(engine)
			service := NewPodService(fakeEngineProvider{engine: engine})

			err := tt.call(service)

			var appErr *appErrors.AppError
			require.ErrorAs(t, err, &appErr, "a conflict refusal must map to an AppError")
			assert.Equal(t, "POD_STATE_CONFLICT", appErr.Code)
			assert.Equal(t, http.StatusConflict, appErr.Status)
			assert.Contains(t, appErr.Message, "pod is paused", "the message must carry podman's refusal text")
		})
	}
}

// TestPodService_Remove_RunningRefusalAnswersStateConflict pins the
// remove refusal classification: podman's running-refusal text (which
// arrives as an untyped 500) classifies into the domain's 409
// POD_STATE_CONFLICT carrying podman's message.
func TestPodService_Remove_RunningRefusalAnswersStateConflict(t *testing.T) {
	engine := &fakePodEngine{
		removeFn: func(string, bool) error {
			return &errorhandling.ErrorModel{
				ResponseCode: http.StatusInternalServerError,
				Message:      "cannot remove container ctr-1 as it is running - running or paused containers cannot be removed without force",
			}
		},
	}
	service := NewPodService(fakeEngineProvider{engine: engine})

	err := service.Remove(t.Context(), "user", "machine", "web", false)

	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr, "the running refusal must map to an AppError")
	assert.Equal(t, "POD_STATE_CONFLICT", appErr.Code)
	assert.Equal(t, http.StatusConflict, appErr.Status)
	assert.Contains(t, appErr.Message, "cannot be removed without force", "the message must carry podman's refusal text")
}

// TestPodService_Remove_ForceReachesEngine pins the force flag: it
// crosses to the engine untouched.
func TestPodService_Remove_ForceReachesEngine(t *testing.T) {
	var gotForce bool
	engine := &fakePodEngine{
		removeFn: func(nameOrID string, force bool) error {
			gotForce = force
			return nil
		},
	}
	service := NewPodService(fakeEngineProvider{engine: engine})

	err := service.Remove(t.Context(), "user", "machine", "web", true)

	require.NoError(t, err)
	assert.True(t, gotForce, "the force flag must reach the machine")
}

// TestPodService_Prune_ReturnsPodmanReports pins the prune service
// behaviour: podman's per-pod reports pass through untouched.
func TestPodService_Prune_ReturnsPodmanReports(t *testing.T) {
	reports := []*entities.PodPruneReport{
		{Id: "stopped-a"},
		{Id: "stopped-b", Err: assert.AnError},
	}
	engine := &fakePodEngine{
		pruneFn: func() ([]*entities.PodPruneReport, error) {
			return reports, nil
		},
	}
	service := NewPodService(fakeEngineProvider{engine: engine})

	got, err := service.Prune(t.Context(), "user", "machine")

	require.NoError(t, err)
	assert.Equal(t, reports, got, "the engine's reports must pass through untouched")
}

// TestFromPodmanPruneReports_MapsEntries pins the prune DTO mapping: one
// entry per removed pod, per-pod errors carried, nil reports skipped.
func TestFromPodmanPruneReports_MapsEntries(t *testing.T) {
	got := FromPodmanPruneReports([]*entities.PodPruneReport{
		{Id: "stopped-a"},
		nil,
		{Id: "stopped-b", Err: assert.AnError},
	})

	require.Len(t, got, 2, "nil reports must be skipped")
	assert.Equal(t, "stopped-a", got[0].ID)
	assert.Empty(t, got[0].Error)
	assert.Equal(t, "stopped-b", got[1].ID)
	assert.NotEmpty(t, got[1].Error, "the per-pod error must be carried")
}

// TestFromPodmanPruneReports_Empty pins that pruning with nothing to
// remove still yields an empty array, not nil.
func TestFromPodmanPruneReports_Empty(t *testing.T) {
	got := FromPodmanPruneReports(nil)

	assert.NotNil(t, got)
	assert.Empty(t, got)
}
