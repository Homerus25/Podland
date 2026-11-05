package images

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAccessRepo is the function-field fake for the permission.PermissionGetter
// seam — the access store the cross-host copy gates the target host
// on. An unarmed lookup panics, so an unexpected gate check fails the
// test loudly.
type fakeAccessRepo struct {
	getPermissionFn func(hostID, userID string) (string, error)
}

func (f *fakeAccessRepo) GetPermission(hostID, userID string) (string, error) {
	if f.getPermissionFn == nil {
		panic("unexpected PermissionGetter.GetPermission call")
	}
	return f.getPermissionFn(hostID, userID)
}

// allowTargetRead answers read on the target host — the minimum the
// preview requires.
func allowTargetRead(hostID, userID string) (string, error) {
	if hostID == "host-b" {
		return models.PermissionRead, nil
	}
	return models.PermissionAdmin, nil
}

// allowTargetWrite answers write on the target host — the minimum the
// copy requires.
func allowTargetWrite(hostID, userID string) (string, error) {
	if hostID == "host-b" {
		return models.PermissionWrite, nil
	}
	return models.PermissionAdmin, nil
}

// newCopyTestService builds the service over per-host fake engines
// (the multi-lease shape the copy exercises) and the given access store.
func newCopyTestService(source, target *fakeImageEngine, access *fakeAccessRepo) *ImageService {
	return NewImageService(fakeEngineProvider{engines: map[string]ImageEngine{
		"host-a": source,
		"host-b": target,
	}}).
		WithAccessRepo(access)
}

// copyImageID is the image the fixtures stream across the splice.
const copyImageID = "sha256:abc123"

// copyArchive is the export payload the source fixture streams.
var copyArchive = bytes.Repeat([]byte("x"), 3<<20)

// copySourceFixture is the source host's engine: the image exists and
// exports its archive.
func copySourceFixture() *fakeImageEngine {
	return &fakeImageEngine{
		existsFn: func(id string) (bool, error) {
			return id == copyImageID, nil
		},
		exportFn: func(ref string, w io.Writer) error {
			_, err := w.Write(copyArchive)
			return err
		},
	}
}

// copyTargetFixture is the target host's engine: it holds nothing,
// loads consume the archive and answer the load's names.
func copyTargetFixture() *fakeImageEngine {
	return &fakeImageEngine{
		existsFn: func(string) (bool, error) {
			return false, nil
		},
		loadFn: func(r io.Reader) ([]string, error) {
			if _, err := io.Copy(io.Discard, r); err != nil {
				return nil, err
			}
			return []string{"registry.example/app:latest"}, nil
		},
	}
}

// --- CopyPreview ----------------------------------------------------------

func TestImageService_CopyPreview_ReportsAbsentTarget(t *testing.T) {
	service := newCopyTestService(copySourceFixture(), copyTargetFixture(),
		&fakeAccessRepo{getPermissionFn: allowTargetRead})

	got, err := service.CopyPreview(context.Background(), "user", "host-a", copyImageID, "host-b")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.False(t, got.Present, "a target without the image answers absent")
}

func TestImageService_CopyPreview_ReportsPresentTarget(t *testing.T) {
	target := copyTargetFixture()
	target.existsFn = func(string) (bool, error) {
		return true, nil
	}
	service := newCopyTestService(copySourceFixture(), target,
		&fakeAccessRepo{getPermissionFn: allowTargetRead})

	got, err := service.CopyPreview(context.Background(), "user", "host-a", copyImageID, "host-b")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Present, "a target holding the image by ID answers present")
}

func TestImageService_CopyPreview_MissingSourceImageIs404(t *testing.T) {
	source := &fakeImageEngine{
		existsFn: func(string) (bool, error) {
			return false, nil
		},
	}
	service := newCopyTestService(source, copyTargetFixture(),
		&fakeAccessRepo{getPermissionFn: allowTargetRead})

	_, err := service.CopyPreview(context.Background(), "user", "host-a", "sha256:gone", "host-b")

	require.Error(t, err)
	assert.Equal(t, appErrors.ErrImageNotFound, err)
}

func TestImageService_CopyPreview_TargetAccessDeniedIs403(t *testing.T) {
	// The source engine panics on any call: the target gate must refuse
	// before any podman work.
	source := &fakeImageEngine{
		existsFn: func(string) (bool, error) {
			panic("source engine must not be reached when the target gate refuses")
		},
	}
	service := newCopyTestService(source, copyTargetFixture(),
		&fakeAccessRepo{getPermissionFn: func(string, string) (string, error) {
			return "", nil
		}})

	_, err := service.CopyPreview(context.Background(), "user", "host-a", copyImageID, "host-b")

	require.Error(t, err)
	assert.Equal(t, appErrors.ErrAccessDenied, err)
}

func TestImageService_CopyPreview_SourceUnreachableIsConnectionFailed(t *testing.T) {
	service := NewImageService(fakeEngineProvider{err: appErrors.ErrConnectionFailed}).
		WithAccessRepo(&fakeAccessRepo{getPermissionFn: allowTargetRead})

	_, err := service.CopyPreview(context.Background(), "user", "host-a", copyImageID, "host-b")

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
}

// TestImageService_CopyPreview_TargetUnreachableIsConnectionFailed pins
// doku's "(source or target unreachable)" branch: the source lease
// stands, the target lease fails, and the 502 sentinel crosses — the
// preview's answer is the connection failure, never a false fact.
func TestImageService_CopyPreview_TargetUnreachableIsConnectionFailed(t *testing.T) {
	service := NewImageService(fakeEngineProvider{
		engines: map[string]ImageEngine{"host-a": copySourceFixture()},
		engineErrs: map[string]error{
			"host-b": appErrors.ErrConnectionFailed,
		},
	}).WithAccessRepo(&fakeAccessRepo{getPermissionFn: allowTargetRead})

	_, err := service.CopyPreview(context.Background(), "user", "host-a", copyImageID, "host-b")

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
}

// --- Copy -----------------------------------------------------------------

func TestImageService_Copy_StreamsArchiveAndReportsNames(t *testing.T) {
	target := copyTargetFixture()
	loaded := int64(0)
	target.loadFn = func(r io.Reader) ([]string, error) {
		data, err := io.ReadAll(r)
		loaded = int64(len(data))
		if err != nil {
			return nil, err
		}
		return []string{"registry.example/app:latest"}, nil
	}
	service := newCopyTestService(copySourceFixture(), target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite})

	var phases []string
	got, err := service.Copy(context.Background(), "user", "host-a", copyImageID, "host-b",
		func(event ImageCopyEvent) { phases = append(phases, event.Phase) })

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(len(copyArchive)), loaded, "the archive crossed the splice")
	assert.Equal(t, copyImageID, got.ID)
	assert.Equal(t, ImageCopyStatusCopied, got.Status)
	assert.Equal(t, "host-a", got.SourceHostID)
	assert.Equal(t, "host-b", got.TargetHostID)
	assert.Equal(t, int64(len(copyArchive)), got.Bytes, "the envelope reports the bytes streamed")
	assert.Equal(t, []string{"registry.example/app:latest"}, got.Names, "the load's names ride the envelope")
	require.Equal(t, ImageCopyPhaseStart, phases[0], "the stream opens with start")
	require.Equal(t, ImageCopyPhaseDone, phases[len(phases)-1], "the stream closes with done")
	for _, phase := range phases[1 : len(phases)-1] {
		assert.Equal(t, ImageCopyPhaseBytes, phase, "the mid-stream phases are byte counts at the kit's cadence")
	}
}

// TestImageService_Copy_TargetAlreadyHoldingImageSucceeds pins the
// idempotence: images are content-addressed, so a target that already
// holds the image is never a collision — the load skips present layers
// and re-applies tags.
func TestImageService_Copy_TargetAlreadyHoldingImageSucceeds(t *testing.T) {
	target := copyTargetFixture()
	target.existsFn = func(string) (bool, error) {
		return true, nil
	}
	service := newCopyTestService(copySourceFixture(), target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite})

	got, err := service.Copy(context.Background(), "user", "host-a", copyImageID, "host-b", nil)

	require.NoError(t, err, "an already-present target image is never a refusal")
	require.NotNil(t, got)
	assert.Equal(t, ImageCopyStatusCopied, got.Status)
}

func TestImageService_Copy_MissingSourceImageIs404(t *testing.T) {
	source := &fakeImageEngine{
		existsFn: func(string) (bool, error) {
			return false, nil
		},
	}
	service := newCopyTestService(source, copyTargetFixture(),
		&fakeAccessRepo{getPermissionFn: allowTargetWrite})

	_, err := service.Copy(context.Background(), "user", "host-a", "sha256:gone", "host-b", nil)

	require.Error(t, err)
	assert.Equal(t, appErrors.ErrImageNotFound, err)
}

func TestImageService_Copy_TargetAccessDeniedIs403(t *testing.T) {
	// Read on the target is below the write the copy needs; the source
	// engine panics on any call: the gate refuses before any podman work.
	source := &fakeImageEngine{
		existsFn: func(string) (bool, error) {
			panic("source engine must not be reached when the target gate refuses")
		},
	}
	service := newCopyTestService(source, copyTargetFixture(),
		&fakeAccessRepo{getPermissionFn: allowTargetRead})

	_, err := service.Copy(context.Background(), "user", "host-a", copyImageID, "host-b", nil)

	require.Error(t, err)
	assert.Equal(t, appErrors.ErrAccessDenied, err)
}

func TestImageService_Copy_SourceUnreachableIsConnectionFailed(t *testing.T) {
	service := NewImageService(fakeEngineProvider{err: appErrors.ErrConnectionFailed}).
		WithAccessRepo(&fakeAccessRepo{getPermissionFn: allowTargetWrite})

	_, err := service.Copy(context.Background(), "user", "host-a", copyImageID, "host-b", nil)

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
}

// TestImageService_Copy_BoundTripsIs413 pins the bound: a stream crossing
// the wired bound fails with the too-large sentinel — the kit's
// bound-exceeded error mapped onto the images domain's 413 vocabulary.
func TestImageService_Copy_BoundTripsIs413(t *testing.T) {
	service := newCopyTestService(copySourceFixture(), copyTargetFixture(),
		&fakeAccessRepo{getPermissionFn: allowTargetWrite}).
		WithMaxImageTransferBytes(1 << 20)

	_, err := service.Copy(context.Background(), "user", "host-a", copyImageID, "host-b", nil)

	require.Error(t, err)
	appErr, ok := err.(*appErrors.AppError)
	require.True(t, ok, "the bound maps onto the 413 sentinel")
	assert.Equal(t, appErrors.ErrImageTransferTooLarge.Code, appErr.Code)
	assert.Equal(t, http.StatusRequestEntityTooLarge, appErr.Status)
}

// TestImageService_Copy_LoadFailureIsTransferFailed pins the transfer
// failure vocabulary: a load that fails mid-stream answers the
// transfer-failed sentinel, and no rollback exists — the comment in
// copy.go's failure branch says why (content-addressed idempotence).
func TestImageService_Copy_LoadFailureIsTransferFailed(t *testing.T) {
	target := copyTargetFixture()
	target.loadFn = func(io.Reader) ([]string, error) {
		return nil, errors.New("load refused")
	}
	service := newCopyTestService(copySourceFixture(), target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite})

	_, err := service.Copy(context.Background(), "user", "host-a", copyImageID, "host-b", nil)

	require.Error(t, err)
	appErr, ok := err.(*appErrors.AppError)
	require.True(t, ok, "the load failure maps onto the transfer-failed sentinel")
	assert.Equal(t, appErrors.ErrImageTransferFailed.Code, appErr.Code)
}

// TestImageService_Copy_CancelledContextRidesBare pins the cancellation
// path: the bare ctx.Err() rides back — the streaming kit's
// cancelled-wins rule answers every cancellation with the generic
// OPERATION_CANCELLED terminal event, so no narration could reach the
// wire anyway.
func TestImageService_Copy_CancelledContextRidesBare(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	target := copyTargetFixture()
	target.loadFn = func(io.Reader) ([]string, error) {
		cancel()
		return nil, errors.New("load refused")
	}
	service := newCopyTestService(copySourceFixture(), target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite})

	_, err := service.Copy(ctx, "user", "host-a", copyImageID, "host-b", nil)

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err, "the cancellation rides bare — no narration wrapped")
}
