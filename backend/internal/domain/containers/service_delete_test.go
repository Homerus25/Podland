package containers

import (
	"context"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

func TestContainerService_Delete_Succeeds(t *testing.T) {
	var gotForce bool
	engine := &fakeContainerEngine{
		removeFn: func(id string, force bool) error {
			gotForce = force
			return nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	err := service.Delete(context.Background(), "user", "host", "container-123", true)

	require.NoError(t, err)
	assert.True(t, gotForce, "the force flag must reach the engine")
}

func TestContainerService_Delete_MapsNotFoundTo404(t *testing.T) {
	notFound := &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}

	t.Run("remove refusal", func(t *testing.T) {
		engine := &fakeContainerEngine{
			removeFn: func(string, bool) error { return notFound },
		}
		service := NewContainerService(fakeEngineProvider{engine: engine})

		err := service.Delete(context.Background(), "user", "host", "container-123", false)

		require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
	})
}

func TestContainerService_Delete_WrapsEngineFailure(t *testing.T) {
	engine := &fakeContainerEngine{
		removeFn: func(string, bool) error { return assert.AnError },
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	err := service.Delete(context.Background(), "user", "host", "container-123", false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove container")
}

// TestContainerService_Delete_MapsRunningRefusalTo409 pins the running
// container refusal: removing a running container without force answers 409
// CONTAINER_RUNNING carrying podman's refusal text, so the UI can explain
// the failure and offer force. The error shape is podman's real one — the
// compat handler answers a 500 whose message is the running-removal refusal,
// not a 409 — so this test fails if the containers-only refusal predicate
// (podmanerr.IsRunningContainerRefusal) stops recognising "cannot be
// removed without force".
func TestContainerService_Delete_MapsRunningRefusalTo409(t *testing.T) {
	const refusal = "cannot remove container 3f2e1d0c as it is running - running or paused containers cannot be removed without force: container state improper"
	engine := &fakeContainerEngine{
		removeFn: func(string, bool) error {
			return &errorhandling.ErrorModel{
				ResponseCode: http.StatusInternalServerError,
				Message:      refusal,
				Because:      "container state improper",
			}
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	err := service.Delete(context.Background(), "user", "host", "container-123", false)

	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusConflict, appErr.Status)
	assert.Equal(t, "CONTAINER_RUNNING", appErr.Code)
	assert.Contains(t, appErr.Message, refusal,
		"podman's refusal text must reach the frontend")
}

// TestContainerService_Delete_MapsPausedRefusalTo409 pins the paused
// variant of the same refusal: podman says "as it is paused" instead of
// "as it is running", and the predicate must cover both — a paused
// container is exactly as unremovable without force.
func TestContainerService_Delete_MapsPausedRefusalTo409(t *testing.T) {
	const refusal = "cannot remove container 3f2e1d0c as it is paused - running or paused containers cannot be removed without force: container state improper"
	engine := &fakeContainerEngine{
		removeFn: func(string, bool) error {
			return &errorhandling.ErrorModel{
				ResponseCode: http.StatusInternalServerError,
				Message:      refusal,
				Because:      "container state improper",
			}
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	err := service.Delete(context.Background(), "user", "host", "container-123", false)

	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusConflict, appErr.Status)
	assert.Equal(t, "CONTAINER_RUNNING", appErr.Code)
	assert.Contains(t, appErr.Message, refusal,
		"podman's refusal text must reach the frontend")
}

// TestContainerService_Delete_ForceReachesEngineOnRunningContainer pins the
// force escape hatch: the same running container removes cleanly when force
// is requested, so the refusal above is a flag decision, not a state wall.
func TestContainerService_Delete_ForceReachesEngineOnRunningContainer(t *testing.T) {
	var gotForce bool
	engine := &fakeContainerEngine{
		removeFn: func(_ string, force bool) error {
			gotForce = force
			return nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	err := service.Delete(context.Background(), "user", "host", "container-123", true)

	require.NoError(t, err)
	assert.True(t, gotForce, "force=true must reach the engine")
}
