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
)

// Get service tests run the real ImageService against the fake engine,
// mirroring the Inspect service shapes with the detail DTO vocabulary.

func TestImageService_Get_ReturnsMappedDetail(t *testing.T) {
	engine := &fakeImageEngine{
		inspectFn: func(string) ([]*entities.ImageInspectReport, []error, error) {
			return []*entities.ImageInspectReport{fullInspectReport()}, nil, nil
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	got, err := service.Get(context.Background(), "user", "host", "sha256:abc123")

	require.NoError(t, err)
	assert.Equal(t, "sha256:abc123", got.ID)
	assert.Equal(t, []string{"nginx:latest"}, got.RepoTags)
	assert.Equal(t, "nginx", got.User)
	assert.Equal(t, map[string]string{"PATH": "/usr/bin"}, got.Env)
}

func TestImageService_Get_MissingImageIsAnError(t *testing.T) {
	engine := &fakeImageEngine{
		inspectFn: func(id string) ([]*entities.ImageInspectReport, []error, error) {
			return nil, []error{fmt.Errorf("no such image %q", id)}, nil
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	got, err := service.Get(context.Background(), "user", "host", "image-123")

	require.Error(t, err)
	assert.Equal(t, ImageDetailResponse{}, got)
	assert.ErrorIs(t, err, appErrors.ErrImageNotFound)
}

// TestImageService_Get_AcquireFailureIsThe502Sentinel pins the
// connection-failure contract for Get: the provider's sentinel crosses
// untouched — the service never wraps, maps, or swallows it (the 502
// mapping itself is pinned in the connections package).
func TestImageService_Get_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewImageService(fakeEngineProvider{err: appErrors.ErrConnectionFailed})

	got, err := service.Get(context.Background(), "user", "host", "image-123")

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	assert.Equal(t, ImageDetailResponse{}, got)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadGateway, appErr.Status)
}
