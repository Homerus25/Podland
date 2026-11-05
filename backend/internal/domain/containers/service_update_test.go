package containers

import (
	"context"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// updatedInspect answers the post-update inspect — the container now
// carries the requested limits — for the service update suite.
func updatedInspect() func(string) ([]*entities.ContainerInspectReport, []error, error) {
	return func(string) ([]*entities.ContainerInspectReport, []error, error) {
		return []*entities.ContainerInspectReport{{
			InspectContainerData: &define.InspectContainerData{
				ID:   "container-123",
				Name: "/web",
				HostConfig: &define.InspectContainerHostConfig{
					Memory:    512 * 1024 * 1024,
					CpuPeriod: 100000,
					CpuQuota:  150000,
				},
			},
		}}, nil, nil
	}
}

// TestContainerService_Update_ForwardsOptionsAndAnswersPostUpdateReport
// pins the happy path: the engine is asked to update the URL's container
// with the options built from the request, and the service answers the
// post-update inspect report so the handler can map the same detail DTO
// the detail GET does.
func TestContainerService_Update_ForwardsOptionsAndAnswersPostUpdateReport(t *testing.T) {
	var gotID string
	var gotOpts *entities.ContainerUpdateOptions
	engine := &fakeContainerEngine{
		updateFn: func(id string, opts *entities.ContainerUpdateOptions) error {
			gotID, gotOpts = id, opts
			return nil
		},
		inspectFn: updatedInspect(),
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Update(context.Background(), "user", "host", "container-123", ContainerUpdateRequest{
		CPUs:   1.5,
		Memory: "512m",
	})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "container-123", gotID, "the URL param must be the updated container")
	require.NotNil(t, gotOpts)
	require.NotNil(t, gotOpts.Resources, "the built options must carry the requested limits")
	require.NotNil(t, gotOpts.Resources.CPU)
	assert.Equal(t, int64(150000), *gotOpts.Resources.CPU.Quota, "the request's cpus must reach the engine")
	require.NotNil(t, gotOpts.Resources.Memory)
	assert.Equal(t, int64(512*1024*1024), *gotOpts.Resources.Memory.Limit, "the request's memory must reach the engine")
	assert.Equal(t, "container-123", got.ID, "the post-update inspect report must be answered")
}

// TestContainerService_Update_NoStateGate pins that update carries no
// state restriction: podman applies limit changes live to a running
// container and rewrites a stopped one's config for the next start, so
// neither state answers a wrong-state terminal.
func TestContainerService_Update_NoStateGate(t *testing.T) {
	for _, state := range []struct {
		status  string
		running bool
	}{
		{"running", true},
		{"exited", false},
	} {
		t.Run(state.status, func(t *testing.T) {
			engine := &fakeContainerEngine{
				updateFn: func(string, *entities.ContainerUpdateOptions) error { return nil },
				inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
					return []*entities.ContainerInspectReport{{
						InspectContainerData: &define.InspectContainerData{
							Name:  "/web",
							State: &define.InspectContainerState{Status: state.status, Running: state.running},
						},
					}}, nil, nil
				},
			}
			service := NewContainerService(fakeEngineProvider{engine: engine})

			got, err := service.Update(context.Background(), "user", "host", "container-123", ContainerUpdateRequest{Memory: "512m"})

			require.NoError(t, err, "no state restriction applies to update")
			require.NotNil(t, got)
		})
	}
}

// TestContainerService_Update_ValidationFailureNeverAcquiresTheLease
// pins the thin-lease shape: the options are built before the lease is
// acquired, so a request-local validation failure never costs an engine
// lease.
func TestContainerService_Update_ValidationFailureNeverAcquiresTheLease(t *testing.T) {
	var ops []string
	provider := fakeEngineProvider{
		engine:     &fakeContainerEngine{},
		opRecorder: func(op string) { ops = append(ops, op) },
	}
	service := NewContainerService(provider)

	got, err := service.Update(context.Background(), "user", "host", "container-123", ContainerUpdateRequest{Memory: "not-a-size"})

	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "VALIDATION_FAILED", appErr.Code)
	assert.Nil(t, got)
	assert.Empty(t, ops, "a validation failure must not acquire the engine lease")
}

// TestContainerService_Update_MapsNotFoundTo404 pins the documented 404:
// podman's not-found answer for a missing container surfaces as
// ErrContainerNotFound — the same envelope as start/stop — and no
// post-update inspect is attempted.
func TestContainerService_Update_MapsNotFoundTo404(t *testing.T) {
	engine := &fakeContainerEngine{
		updateFn: func(string, *entities.ContainerUpdateOptions) error {
			return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}
		},
		// inspectFn stays unarmed: a failed update must not inspect.
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Update(context.Background(), "user", "host", "container-123", ContainerUpdateRequest{Memory: "512m"})

	require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
	assert.Nil(t, got)
}

// TestContainerService_Update_WrapsEngineFailure pins that an unrelated
// failure is wrapped, never answered as a not-found: classification
// reads the failure itself, so no follow-up probe runs.
func TestContainerService_Update_WrapsEngineFailure(t *testing.T) {
	engine := &fakeContainerEngine{
		updateFn: func(string, *entities.ContainerUpdateOptions) error { return assert.AnError },
		// inspectFn stays unarmed: an unrelated failure must not probe.
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Update(context.Background(), "user", "host", "container-123", ContainerUpdateRequest{Memory: "512m"})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.NotErrorIs(t, err, appErrors.ErrContainerNotFound)
	assert.Contains(t, err.Error(), "failed to update container")
}

// TestContainerService_Update_AcquireFailureIsThe502Sentinel pins the
// documented 502: the provider's connection sentinel crosses untouched,
// so a broken tunnel never masquerades as a missing container.
func TestContainerService_Update_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewContainerService(fakeEngineProvider{err: appErrors.ErrConnectionFailed})

	got, err := service.Update(context.Background(), "user", "host", "container-123", ContainerUpdateRequest{Memory: "512m"})

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	assert.Nil(t, got)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadGateway, appErr.Status)
}

// TestContainerService_Update_PostUpdateInspectFailureSurfaces pins
// that a failed post-update inspect surfaces instead of answering a
// stale report: the refreshed detail DTO is the endpoint's contract.
func TestContainerService_Update_PostUpdateInspectFailureSurfaces(t *testing.T) {
	engine := &fakeContainerEngine{
		updateFn: func(string, *entities.ContainerUpdateOptions) error { return nil },
		inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
			return nil, nil, &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Update(context.Background(), "user", "host", "container-123", ContainerUpdateRequest{Memory: "512m"})

	require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
	assert.Nil(t, got)
}
