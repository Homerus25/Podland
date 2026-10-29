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

// renamedInspect answers the post-rename inspect: the container now
// carries newName.
func renamedInspect(newName string) func(string) ([]*entities.ContainerInspectReport, []error, error) {
	return func(string) ([]*entities.ContainerInspectReport, []error, error) {
		return []*entities.ContainerInspectReport{{
			InspectContainerData: &define.InspectContainerData{
				Name:  "/" + newName,
				State: &define.InspectContainerState{Status: "running", Running: true},
			},
		}}, nil, nil
	}
}

// TestContainerService_Rename_AnswersPostRenameReport pins the happy
// path: the engine is asked to rename the URL's container to the
// request's name, and the service answers the post-rename inspect report
// so the handler can map the same detail DTO the GET answers.
func TestContainerService_Rename_AnswersPostRenameReport(t *testing.T) {
	var gotID, gotName string
	engine := &fakeContainerEngine{
		renameFn: func(id, newName string) error {
			gotID, gotName = id, newName
			return nil
		},
		inspectFn: renamedInspect("renamed"),
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Rename(context.Background(), "user", "machine", "container-123", "renamed")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "container-123", gotID, "the URL param must be the renamed container")
	assert.Equal(t, "renamed", gotName, "the request body must carry the new name")
	assert.Equal(t, "/renamed", got.Name, "the post-rename inspect report must be answered")
}

// TestContainerService_Rename_NoStateGate pins that rename carries no
// state restriction: podman renames running and stopped containers
// alike, so neither state answers a wrong-state terminal.
func TestContainerService_Rename_NoStateGate(t *testing.T) {
	for _, state := range []struct {
		status  string
		running bool
	}{
		{"running", true},
		{"exited", false},
	} {
		t.Run(state.status, func(t *testing.T) {
			engine := &fakeContainerEngine{
				renameFn: func(string, string) error { return nil },
				inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
					return []*entities.ContainerInspectReport{{
						InspectContainerData: &define.InspectContainerData{
							Name:  "/renamed",
							State: &define.InspectContainerState{Status: state.status, Running: state.running},
						},
					}}, nil, nil
				},
			}
			service := NewContainerService(fakeEngineProvider{engine: engine})

			got, err := service.Rename(context.Background(), "user", "machine", "container-123", "renamed")

			require.NoError(t, err, "no state restriction applies to rename")
			require.NotNil(t, got)
		})
	}
}

// TestContainerService_Rename_MapsNotFoundTo404 pins the documented 404:
// podman's not-found answer for a missing container surfaces as
// ErrContainerNotFound, and no post-rename inspect is attempted.
func TestContainerService_Rename_MapsNotFoundTo404(t *testing.T) {
	engine := &fakeContainerEngine{
		renameFn: func(string, string) error {
			return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}
		},
		// inspectFn stays unarmed: a failed rename must not inspect.
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Rename(context.Background(), "user", "machine", "container-123", "renamed")

	require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
	assert.Nil(t, got)
}

// TestContainerService_Rename_MapsNameCollisionTo409 pins the documented
// 409. Podman answers a rename onto a taken name as a 500 sqlite UNIQUE
// constraint failure rather than a typed conflict (RenameContainer →
// RewriteContainerConfig in podman's source), so the collision is
// resolved by asking whether the name is taken — the follow-up inspect
// resolving a container is the collision.
func TestContainerService_Rename_MapsNameCollisionTo409(t *testing.T) {
	engine := &fakeContainerEngine{
		renameFn: func(string, string) error {
			return &errorhandling.ErrorModel{
				ResponseCode: http.StatusInternalServerError,
				Message:      "renaming container abc: updating container config table with new configuration for container abc: UNIQUE constraint failed: ContainerConfig.Name",
			}
		},
		inspectFn: func(name string) ([]*entities.ContainerInspectReport, []error, error) {
			require.Equal(t, "taken", name, "the collision check must inspect the requested name")
			return []*entities.ContainerInspectReport{{
				InspectContainerData: &define.InspectContainerData{Name: "/taken"},
			}}, nil, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Rename(context.Background(), "user", "machine", "container-123", "taken")

	require.ErrorIs(t, err, appErrors.ErrContainerAlreadyExists)
	assert.Nil(t, got)
}

// TestContainerService_Rename_MapsAlreadyExistsTo409 pins the typed
// collision answer: podman's own "already exists" refusal maps through
// the shared predicate without the follow-up inspect.
func TestContainerService_Rename_MapsAlreadyExistsTo409(t *testing.T) {
	engine := &fakeContainerEngine{
		renameFn: func(string, string) error {
			return &errorhandling.ErrorModel{ResponseCode: http.StatusConflict, Message: "container name taken already exists"}
		},
		// inspectFn stays unarmed: the predicate answers first.
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Rename(context.Background(), "user", "machine", "container-123", "taken")

	require.ErrorIs(t, err, appErrors.ErrContainerAlreadyExists)
	assert.Nil(t, got)
}

// TestContainerService_Rename_MapsInvalidNameTo400 pins that the name's
// shape is podman's to validate: its "names must match" rejection is a
// client error and answers 400 INVALID_REQUEST carrying podman's
// message, never a 500.
func TestContainerService_Rename_MapsInvalidNameTo400(t *testing.T) {
	engine := &fakeContainerEngine{
		renameFn: func(string, string) error {
			return fmt.Errorf("names must match [a-zA-Z0-9][a-zA-Z0-9_.-]*: %w", define.ErrInvalidArg)
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Rename(context.Background(), "user", "machine", "container-123", "bad name!")

	require.Error(t, err)
	assert.Nil(t, got)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, appErrors.ErrInvalidRequest.Code, appErr.Code)
	assert.Equal(t, appErrors.ErrInvalidRequest.Status, appErr.Status)
	assert.Contains(t, appErr.Message, "names must match", "podman's message must ride along")
}

// TestContainerService_Rename_AcquireFailureIsThe502Sentinel pins the
// documented 502: the provider's connection sentinel crosses untouched,
// so a broken tunnel never masquerades as a collision or a bad name.
func TestContainerService_Rename_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewContainerService(fakeEngineProvider{err: appErrors.ErrConnectionFailed})

	got, err := service.Rename(context.Background(), "user", "machine", "container-123", "renamed")

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	assert.Nil(t, got)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadGateway, appErr.Status)
}

// TestContainerService_Rename_SameNameSucceeds pins that a no-op rename
// is not a collision: the requested name resolves the container itself,
// but the rename succeeds before any collision check runs, so the
// heuristic never fires on it.
func TestContainerService_Rename_SameNameSucceeds(t *testing.T) {
	engine := &fakeContainerEngine{
		renameFn:  func(string, string) error { return nil },
		inspectFn: renamedInspect("same"),
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Rename(context.Background(), "user", "machine", "container-123", "same")

	require.NoError(t, err, "renaming a container to its own name is not a collision")
	require.NotNil(t, got)
	assert.Equal(t, "/same", got.Name)
}

// TestContainerService_Rename_WrapsEngineFailure pins that an unrelated
// failure with a free name is wrapped, never answered as a collision.
func TestContainerService_Rename_WrapsEngineFailure(t *testing.T) {
	engine := &fakeContainerEngine{
		renameFn: func(string, string) error { return assert.AnError },
		inspectFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
			return nil, []error{fmt.Errorf("no such container %q", "free")}, nil
		},
	}
	service := NewContainerService(fakeEngineProvider{engine: engine})

	got, err := service.Rename(context.Background(), "user", "machine", "container-123", "free")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.NotErrorIs(t, err, appErrors.ErrContainerAlreadyExists)
	assert.Contains(t, err.Error(), "failed to rename container")
}
