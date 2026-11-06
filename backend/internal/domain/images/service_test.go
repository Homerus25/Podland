package images

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/testutils"
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

	got, err := service.List(context.Background(), "user", "host", entities.ImageListOptions{})

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

	got, err := service.List(context.Background(), "user", "host", entities.ImageListOptions{})

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

		err := service.Delete(context.Background(), "user", "host", "image-123", false)

		require.ErrorIs(t, err, appErrors.ErrImageNotFound)
	})

	t.Run("used by containers maps to 409", func(t *testing.T) {
		engine := &fakeImageEngine{
			removeFn: func(string, bool) (*entities.ImageRemoveReport, []error) {
				return nil, []error{&errorhandling.ErrorModel{ResponseCode: http.StatusConflict, Message: "image is in use"}}
			},
		}
		service := NewImageService(fakeEngineProvider{engine: engine})

		err := service.Delete(context.Background(), "user", "host", "image-123", false)

		require.ErrorIs(t, err, appErrors.ErrImageUsedByContainers)
	})

	t.Run("other refusals stay wrapped", func(t *testing.T) {
		engine := &fakeImageEngine{
			removeFn: func(string, bool) (*entities.ImageRemoveReport, []error) {
				return nil, []error{errors.New("boom")}
			},
		}
		service := NewImageService(fakeEngineProvider{engine: engine})

		err := service.Delete(context.Background(), "user", "host", "image-123", false)

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

		err := service.Delete(context.Background(), "user", "host", "image-123", false)

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

	got, err := service.PullWithProgress(context.Background(), "user", "host", "nginx:latest", entities.ImagePullOptions{}, nil)

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

	got, err := service.PullWithProgress(context.Background(), "user", "host", "nginx:latest", entities.ImagePullOptions{}, nil)

	require.ErrorIs(t, err, appErrors.ErrImagePullFailed)
	assert.Nil(t, got)
}

// TestImageService_PullWithProgress_ReturnsPromptlyOnCancel pins the
// release regression: a blocking engine Pull call plus a cancelled
// context must return the operation-cancelled sentinel promptly (the
// shared promptness harness owns the ritual). The pull itself keeps
// running on the host (remote outcome unchanged); the service just
// stops waiting on it.
func TestImageService_PullWithProgress_ReturnsPromptlyOnCancel(t *testing.T) {
	started, block := testutils.CancelStubs(t)
	engine := &fakeImageEngine{
		pullFn: func(string, entities.ImagePullOptions) (*entities.ImagePullReport, error) {
			close(started)
			<-block
			return nil, nil
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	testutils.RequirePromptReturnOnCancel(t, "PullWithProgress", started,
		func(ctx context.Context) error {
			_, err := service.PullWithProgress(ctx, "user", "host", "nginx:latest", entities.ImagePullOptions{}, nil)
			return err
		})
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

	got, err := service.PullWithProgress(context.Background(), "user", "host", "nginx:latest",
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

	got, err := service.Prune(context.Background(), "user", "host")

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

	got, err := service.Prune(context.Background(), "user", "host")

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

	got, err := service.List(context.Background(), "user", "host", entities.ImageListOptions{})

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

func TestImageService_Export_GatesOnExistence(t *testing.T) {
	engine := &fakeImageEngine{
		existsFn: func(string) (bool, error) {
			return false, nil
		},
		// exportFn stays unarmed: it must never be reached for a
		// missing image.
	}
	service := NewImageService(fakeEngineProvider{engine: engine})
	var sink bytes.Buffer

	err := service.Export(context.Background(), "user", "host", "sha256:abc123", &sink)

	require.ErrorIs(t, err, appErrors.ErrImageNotFound)
	assert.Zero(t, sink.Len(), "no byte may be written for a missing image")
}

func TestImageService_Export_StreamsEngineBytesUnchanged(t *testing.T) {
	engine := &fakeImageEngine{
		existsFn: func(string) (bool, error) {
			return true, nil
		},
		exportFn: func(_ string, w io.Writer) error {
			_, err := w.Write([]byte("tar-bytes"))
			return err
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})
	var sink bytes.Buffer

	err := service.Export(context.Background(), "user", "host", "sha256:abc123", &sink)

	require.NoError(t, err)
	assert.Equal(t, "tar-bytes", sink.String())
}

// TestImageService_BulkDelete_MapsBatchReportToEntries pins the batch
// report's per-id mapping: an id the report lists in Deleted or Untagged
// answers success, the ids it does not list answer the batch's failure
// entry error, and the answer keeps the report contract — one entry per
// requested id, in request order.
func TestImageService_BulkDelete_MapsBatchReportToEntries(t *testing.T) {
	engine := &fakeImageEngine{
		removeAllFn: func(ids []string, force bool) (*entities.ImageRemoveReport, []error) {
			assert.False(t, force)
			return &entities.ImageRemoveReport{
				Deleted:  []string{"sha256:gone", "sha256:dup"},
				Untagged: []string{"registry.example.com/app:latest"},
			}, nil
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	entries := service.BulkDelete(context.Background(), "user", "host",
		[]string{"sha256:gone", "sha256:stays", "sha256:dup", "registry.example.com/app:latest"}, false)

	require.Len(t, entries, 4)
	assert.Equal(t, "sha256:gone", entries[0].ID)
	assert.True(t, entries[0].Success, "an id in Deleted answers success")
	assert.Equal(t, "sha256:stays", entries[1].ID)
	require.NotNil(t, entries[1].Error, "an id absent from the report answers a failure entry")
	assert.False(t, entries[1].Success)
	assert.Equal(t, "sha256:dup", entries[2].ID)
	assert.True(t, entries[2].Success, "a duplicate id matches the report's single Deleted entry")
	assert.Equal(t, "registry.example.com/app:latest", entries[3].ID)
	assert.True(t, entries[3].Success, "a tag request matches the report's Untagged entry")
}

// TestImageService_BulkDelete_ShortIDRequestMatchesFullReportEntry pins
// the reference-form parity with the single-item delete: podman resolves
// a short hex ID to the full one, so a removed short-ID request lands in
// Deleted as the full sha256 ID and must answer success, not a false
// failure entry.
func TestImageService_BulkDelete_ShortIDRequestMatchesFullReportEntry(t *testing.T) {
	engine := &fakeImageEngine{
		removeAllFn: func([]string, bool) (*entities.ImageRemoveReport, []error) {
			return &entities.ImageRemoveReport{
				Deleted: []string{"sha256:abcdef0123456789"},
			}, nil
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	entries := service.BulkDelete(context.Background(), "user", "host",
		[]string{"abcdef", "sha256:abcdef"}, false)

	require.Len(t, entries, 2)
	assert.True(t, entries[0].Success, "a short hex ID podman resolved and removed answers success")
	assert.True(t, entries[1].Success, "a short sha256-prefixed ID podman resolved and removed answers success")
}

// TestImageService_BulkDelete_UnanimousFailureAnswersTheSentinel pins the
// attribution rule: podman's batch report carries an unkeyed error slice,
// so when every cause classifies to the same sentinel every failed id
// answers it — the single-item delete's own classification, shared
// through classifyRemoveError.
func TestImageService_BulkDelete_UnanimousFailureAnswersTheSentinel(t *testing.T) {
	tests := []struct {
		name string
		errs []error
		code string
	}{
		{
			name: "every image in use answers IMAGE_USED_BY_CONTAINERS",
			errs: []error{
				&errorhandling.ErrorModel{ResponseCode: http.StatusConflict, Message: "image is in use"},
				errors.New("sha256:other: image used by a container"),
			},
			code: appErrors.ErrImageUsedByContainers.Code,
		},
		{
			name: "every image vanished answers IMAGE_NOT_FOUND",
			errs: []error{
				&errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such image"},
				errors.New("sha256:other: image not known"),
			},
			code: appErrors.ErrImageNotFound.Code,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &fakeImageEngine{
				removeAllFn: func([]string, bool) (*entities.ImageRemoveReport, []error) {
					return &entities.ImageRemoveReport{}, tt.errs
				},
			}
			service := NewImageService(fakeEngineProvider{engine: engine})

			entries := service.BulkDelete(context.Background(), "user", "host", []string{"sha256:a", "sha256:b"}, false)

			require.Len(t, entries, 2)
			for _, entry := range entries {
				require.NotNil(t, entry.Error)
				assert.False(t, entry.Success)
				assert.Equal(t, tt.code, entry.Error.Code)
			}
		})
	}
}

// TestImageService_BulkDelete_MixedFailuresAnswerGenericEntry pins the
// honest fallback: when the batch's causes classify to different
// sentinels, podman's unkeyed error slice cannot attribute one to a
// requested id, so every failed id answers the kit's generic internal
// entry instead of a guessed sentinel.
func TestImageService_BulkDelete_MixedFailuresAnswerGenericEntry(t *testing.T) {
	engine := &fakeImageEngine{
		removeAllFn: func([]string, bool) (*entities.ImageRemoveReport, []error) {
			return &entities.ImageRemoveReport{Deleted: []string{"sha256:gone"}},
				[]error{
					&errorhandling.ErrorModel{ResponseCode: http.StatusConflict, Message: "image is in use"},
					&errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such image"},
				}
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	entries := service.BulkDelete(context.Background(), "user", "host", []string{"sha256:gone", "sha256:busy", "sha256:ghost"}, false)

	require.Len(t, entries, 3)
	assert.True(t, entries[0].Success)
	for _, entry := range entries[1:] {
		require.NotNil(t, entry.Error)
		assert.Equal(t, appErrors.ErrInternalError.Code, entry.Error.Code)
	}
}

// TestImageService_BulkDelete_CallFailureFailsEveryEntry pins the
// call-level failure branch: a nil report (transport or decode failure)
// answers the classified cause on every entry — byte-identical to the
// per-item loop's answer when every single-item call failed the same way.
func TestImageService_BulkDelete_CallFailureFailsEveryEntry(t *testing.T) {
	engine := &fakeImageEngine{
		removeAllFn: func([]string, bool) (*entities.ImageRemoveReport, []error) {
			return nil, []error{appErrors.ErrConnectionFailed}
		},
	}
	service := NewImageService(fakeEngineProvider{engine: engine})

	entries := service.BulkDelete(context.Background(), "user", "host", []string{"sha256:a", "sha256:b"}, false)

	require.Len(t, entries, 2)
	for _, entry := range entries {
		require.NotNil(t, entry.Error)
		assert.False(t, entry.Success)
		assert.Equal(t, appErrors.ErrConnectionFailed.Code, entry.Error.Code)
	}
}

// TestImageService_BulkDelete_AcquisitionFailureFailsEveryEntry pins the
// lease branch: when the engine lease is never granted, every requested
// id answers the classified acquisition error — the report stays 200 with
// all-failed entries, never a status code.
func TestImageService_BulkDelete_AcquisitionFailureFailsEveryEntry(t *testing.T) {
	service := NewImageService(fakeEngineProvider{err: appErrors.ErrConnectionFailed})

	entries := service.BulkDelete(context.Background(), "user", "host", []string{"sha256:a", "sha256:b"}, false)

	require.Len(t, entries, 2)
	for _, entry := range entries {
		require.NotNil(t, entry.Error)
		assert.False(t, entry.Success)
		assert.Equal(t, appErrors.ErrConnectionFailed.Code, entry.Error.Code)
	}
}
