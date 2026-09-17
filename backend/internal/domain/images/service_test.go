package images

import (
	"context"
	"errors"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// Service behaviour tests run the real ImageService against the fake
// engine. Connection acquisition (and its 502 mapping) is the provider
// seam: the connections package owns that contract and pins it in its own
// tests.

func TestImageService_List_ReturnsEngineSummaries(t *testing.T) {
	summaries := []*entities.ImageSummary{{ID: "sha256:abc"}}
	engine := &fakeImageEngine{
		listFn: func(entities.ImageListOptions) ([]*entities.ImageSummary, error) {
			return summaries, nil
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	got, err := service.List(context.Background(), "user", "machine", entities.ImageListOptions{})

	require.NoError(t, err)
	assert.Equal(t, summaries, got)
}

func TestImageService_List_WrapsEngineFailure(t *testing.T) {
	engine := &fakeImageEngine{
		listFn: func(entities.ImageListOptions) ([]*entities.ImageSummary, error) {
			return nil, errors.New("boom")
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	got, err := service.List(context.Background(), "user", "machine", entities.ImageListOptions{})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to list images", "engine failures must be wrapped consistently")
}

func TestImageService_Delete_ClassifiesPodmanRefusals(t *testing.T) {
	t.Run("not found maps to 404", func(t *testing.T) {
		engine := &fakeImageEngine{
			removeFn: func(string, bool) (*entities.ImageRemoveReport, []error) {
				return nil, []error{&errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such image"}}
			},
		}
		service := NewImageService(fakeEngineProvider{engine: engine})

		err := service.Delete(context.Background(), "user", "machine", "image-123", false)

		require.ErrorIs(t, err, appErrors.ErrImageNotFound)
	})

	t.Run("used by containers maps to 409", func(t *testing.T) {
		engine := &fakeImageEngine{
			removeFn: func(string, bool) (*entities.ImageRemoveReport, []error) {
				return nil, []error{&errorhandling.ErrorModel{ResponseCode: http.StatusConflict, Message: "image is in use"}}
			},
		}
		service := NewImageService(fakeEngineProvider{engine: engine})

		err := service.Delete(context.Background(), "user", "machine", "image-123", false)

		require.ErrorIs(t, err, appErrors.ErrImageUsedByContainers)
	})

	t.Run("other refusals stay wrapped", func(t *testing.T) {
		engine := &fakeImageEngine{
			removeFn: func(string, bool) (*entities.ImageRemoveReport, []error) {
				return nil, []error{errors.New("boom")}
			},
		}
		service := NewImageService(fakeEngineProvider{engine: engine})

		err := service.Delete(context.Background(), "user", "machine", "image-123", false)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to remove image")
	})

	t.Run("missing report is an error", func(t *testing.T) {
		engine := &fakeImageEngine{
			removeFn: func(string, bool) (*entities.ImageRemoveReport, []error) {
				return nil, nil
			},
		}
		service := NewImageService(fakeEngineProvider{engine: engine})

		err := service.Delete(context.Background(), "user", "machine", "image-123", false)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "no remove report returned for image: image-123")
	})
}

func TestImageService_PullWithProgress_ReturnsEngineReport(t *testing.T) {
	report := &entities.ImagePullReport{ID: "nginx:latest"}
	engine := &fakeImageEngine{
		pullFn: func(rawImage string, opts entities.ImagePullOptions) (*entities.ImagePullReport, error) {
			assert.Equal(t, "nginx:latest", rawImage)
			return report, nil
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	got, err := service.PullWithProgress(context.Background(), "user", "machine", "nginx:latest", entities.ImagePullOptions{}, nil)

	require.NoError(t, err)
	assert.Same(t, report, got)
}

func TestImageService_PullWithProgress_MapsFailureToPullFailed(t *testing.T) {
	engine := &fakeImageEngine{
		pullFn: func(string, entities.ImagePullOptions) (*entities.ImagePullReport, error) {
			return nil, errors.New("registry unreachable")
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	got, err := service.PullWithProgress(context.Background(), "user", "machine", "nginx:latest", entities.ImagePullOptions{}, nil)

	require.ErrorIs(t, err, appErrors.ErrImagePullFailed)
	assert.Nil(t, got)
}

func TestImageService_PullWithProgress_MapsCancellationToOperationCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	engine := &fakeImageEngine{
		pullFn: func(string, entities.ImagePullOptions) (*entities.ImagePullReport, error) {
			return nil, context.Canceled
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	got, err := service.PullWithProgress(ctx, "user", "machine", "nginx:latest", entities.ImagePullOptions{}, nil)

	require.ErrorIs(t, err, appErrors.ErrOperationCancelled, "a cancelled pull must not masquerade as a pull failure")
	assert.Nil(t, got)
}

// TestImageService_PullWithProgress_ArmsTheProgressWriter pins the progress
// seam: a pull with a progress callback hands the engine options carrying a
// writer (podman streams its progress into it), so progress events flow
// without the engine knowing about the NDJSON vocabulary. Events arrive with
// a zero operation ID — the streaming handler owns stamping it.
func TestImageService_PullWithProgress_ArmsTheProgressWriter(t *testing.T) {
	var writerArmed bool
	var events []PullProgressEvent
	engine := &fakeImageEngine{
		pullFn: func(_ string, opts entities.ImagePullOptions) (*entities.ImagePullReport, error) {
			writerArmed = opts.Writer != nil
			if opts.Writer != nil {
				_, _ = opts.Writer.Write([]byte("layer 1 pulling"))
			}
			return &entities.ImagePullReport{ID: "nginx:latest"}, nil
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	got, err := service.PullWithProgress(context.Background(), "user", "machine", "nginx:latest",
		entities.ImagePullOptions{}, func(event PullProgressEvent) { events = append(events, event) })

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, writerArmed, "a progress callback must arm the pull options' writer")
	require.Len(t, events, 1, "the engine's progress bytes must arrive as progress events")
	assert.Empty(t, events[0].OperationID, "the adapter must not stamp the operation ID; the handler owns it")
}

func TestImageService_Prune_ReturnsEngineReports(t *testing.T) {
	prunes := []*reports.PruneReport{{Id: "sha256:abc", Size: 4096}}
	engine := &fakeImageEngine{
		pruneFn: func() ([]*reports.PruneReport, error) {
			return prunes, nil
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	got, err := service.Prune(context.Background(), "user", "machine")

	require.NoError(t, err)
	assert.Equal(t, prunes, got)
}

func TestImageService_Prune_WrapsEngineFailure(t *testing.T) {
	engine := &fakeImageEngine{
		pruneFn: func() ([]*reports.PruneReport, error) {
			return nil, errors.New("boom")
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	got, err := service.Prune(context.Background(), "user", "machine")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to prune images")
}

// TestImageService_AcquireFailureIsThe502Sentinel pins the service-side
// half of the connection-failure contract: the provider's sentinel crosses
// untouched — the service never wraps, maps, or swallows it (the 502
// mapping itself is pinned in the connections package).
func TestImageService_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewImageService(fakeEngineProvider{err: appErrors.ErrConnectionFailed})

	got, err := service.List(context.Background(), "user", "machine", entities.ImageListOptions{})

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	assert.Nil(t, got)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadGateway, appErr.Status)
}

// TestImageService_ParsePlatform tests that ParsePlatform correctly splits platform strings
func TestImageService_ParsePlatform(t *testing.T) {
	t.Run("empty platform", func(t *testing.T) {
		os, arch, variant := ParsePlatform("")
		assert.Equal(t, "", os)
		assert.Equal(t, "", arch)
		assert.Equal(t, "", variant)
	})

	t.Run("simple platform", func(t *testing.T) {
		os, arch, variant := ParsePlatform("linux/amd64")
		assert.Equal(t, "linux", os)
		assert.Equal(t, "amd64", arch)
		assert.Equal(t, "", variant)
	})

	t.Run("platform with variant", func(t *testing.T) {
		os, arch, variant := ParsePlatform("linux/arm64/v8")
		assert.Equal(t, "linux", os)
		assert.Equal(t, "arm64", arch)
		assert.Equal(t, "v8", variant)
	})

	t.Run("platform with multiple slashes", func(t *testing.T) {
		os, arch, variant := ParsePlatform("windows/amd64/10.0.17763.1234")
		assert.Equal(t, "windows", os)
		assert.Equal(t, "amd64", arch)
		assert.Equal(t, "10.0.17763.1234", variant)
	})
}
