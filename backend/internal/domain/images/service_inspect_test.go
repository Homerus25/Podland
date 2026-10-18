package images

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
	"go.podman.io/podman/v6/pkg/inspect"
)

// Inspect service tests run the real ImageService against the fake
// engine, mirroring the containers domain's Get shapes with the images
// vocabulary.

func TestImageService_Inspect_ReturnsEngineReport(t *testing.T) {
	report := &entities.ImageInspectReport{ImageData: &inspect.ImageData{ID: "sha256:abc123", RepoTags: []string{"nginx:latest"}}}
	engine := &fakeImageEngine{
		inspectFn: func(string) ([]*entities.ImageInspectReport, []error, error) {
			return []*entities.ImageInspectReport{report}, nil, nil
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	got, err := service.Inspect(context.Background(), "user", "machine", "image-123")

	require.NoError(t, err)
	assert.Same(t, report, got)
}

func TestImageService_Inspect_MissingImageIsAnError(t *testing.T) {
	notFound := &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such image"}

	t.Run("transport-level refusal", func(t *testing.T) {
		engine := &fakeImageEngine{
			inspectFn: func(string) ([]*entities.ImageInspectReport, []error, error) {
				return nil, nil, notFound
			},
		}
		service := NewImageService(fakeEngineProvider{engine: engine})

		got, err := service.Inspect(context.Background(), "user", "machine", "image-123")

		require.Error(t, err)
		assert.Nil(t, got)
		assert.ErrorIs(t, err, appErrors.ErrImageNotFound)
	})

	t.Run("per-item refusal", func(t *testing.T) {
		engine := &fakeImageEngine{
			inspectFn: func(id string) ([]*entities.ImageInspectReport, []error, error) {
				return nil, []error{fmt.Errorf("no such image %q", id)}, nil
			},
		}
		service := NewImageService(fakeEngineProvider{engine: engine})

		got, err := service.Inspect(context.Background(), "user", "machine", "image-123")

		require.Error(t, err)
		assert.Nil(t, got)
		assert.ErrorIs(t, err, appErrors.ErrImageNotFound)
	})
}

func TestImageService_Inspect_WrapsEngineFailure(t *testing.T) {
	engine := &fakeImageEngine{
		inspectFn: func(string) ([]*entities.ImageInspectReport, []error, error) {
			return nil, nil, assert.AnError
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	got, err := service.Inspect(context.Background(), "user", "machine", "image-123")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to inspect image")
}

func TestImageService_Inspect_ZeroReportsWithPerItemErrorWrapsCause(t *testing.T) {
	engine := &fakeImageEngine{
		inspectFn: func(string) ([]*entities.ImageInspectReport, []error, error) {
			return nil, []error{assert.AnError}, nil
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	got, err := service.Inspect(context.Background(), "user", "machine", "image-123")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, assert.AnError)
	assert.NotErrorIs(t, err, appErrors.ErrImageNotFound)
	assert.Contains(t, err.Error(), "failed to inspect image")
}

func TestImageService_Inspect_ZeroReportsWithoutErrorsIsAnomaly(t *testing.T) {
	engine := &fakeImageEngine{
		inspectFn: func(string) ([]*entities.ImageInspectReport, []error, error) {
			return nil, nil, nil
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	got, err := service.Inspect(context.Background(), "user", "machine", "image-123")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "no inspect report returned for image")
}

// TestImageService_Inspect_AcquireFailureIsThe502Sentinel pins the
// connection-failure contract for Inspect: the provider's sentinel crosses
// untouched — the service never wraps, maps, or swallows it (the 502
// mapping itself is pinned in the connections package).
func TestImageService_Inspect_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewImageService(fakeEngineProvider{err: appErrors.ErrConnectionFailed})

	got, err := service.Inspect(context.Background(), "user", "machine", "image-123")

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	assert.Nil(t, got)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadGateway, appErr.Status)
}
