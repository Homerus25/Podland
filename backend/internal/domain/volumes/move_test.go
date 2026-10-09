package volumes

import (
	"archive/tar"
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
	"go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// fakeAccessRepo is the function-field fake for the permission.PermissionGetter
// seam — the access store the cross-machine copy/move gates the target
// machine on. An unarmed lookup panics, so an unexpected gate check fails
// the test loudly.
type fakeAccessRepo struct {
	getPermissionFn func(machineID, userID string) (string, error)
}

func (f *fakeAccessRepo) GetPermission(machineID, userID string) (string, error) {
	if f.getPermissionFn == nil {
		panic("unexpected PermissionGetter.GetPermission call")
	}
	return f.getPermissionFn(machineID, userID)
}

// allowTargetRead answers read on the target machine — the minimum the
// preview requires.
func allowTargetRead(machineID, userID string) (string, error) {
	if machineID == "machine-b" {
		return models.PermissionRead, nil
	}
	return models.PermissionAdmin, nil
}

// allowTargetWrite answers write on the target machine — the minimum the
// transfer requires.
func allowTargetWrite(machineID, userID string) (string, error) {
	if machineID == "machine-b" {
		return models.PermissionWrite, nil
	}
	return models.PermissionAdmin, nil
}

// newMoveTestService builds the service over per-machine fake engines (the
// multi-lease shape the copy/move exercises), a source-machine usage fake,
// and the given access store.
func newMoveTestService(sourceEngine, targetEngine *fakeVolumeEngine, usage *fakeUsageEngine, access *fakeAccessRepo) *VolumeService {
	return NewVolumeService(fakeEngineProvider{engines: map[string]VolumeEngine{
		"machine-a": sourceEngine,
		"machine-b": targetEngine,
	}}).
		WithAccessRepo(access).
		WithUsageEngines(fakeUsageEngineProvider{engines: map[string]VolumeUsageEngine{
			"machine-a": usage,
		}})
}

// buildVolumeTar assembles a small in-memory tar archive — the export
// payload the fakes stream across the splice.
func buildVolumeTar(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// volumeInspectFixture is one existing volume's inspect report.
func volumeInspectFixture(name string) *entities.VolumeInspectReport {
	return &entities.VolumeInspectReport{VolumeConfigResponse: &entities.VolumeConfigResponse{
		InspectVolumeData: define.InspectVolumeData{Name: name, Driver: "local"},
	}}
}

// moveSourceFixture is the source machine's engine: the pgdata volume
// exists and exports a small tar.
func moveSourceFixture(t *testing.T) *fakeVolumeEngine {
	payload := buildVolumeTar(t, map[string][]byte{"pgdata/PG_VERSION": []byte("17")})
	return &fakeVolumeEngine{
		inspectFn: func(name string) (*entities.VolumeInspectReport, error) {
			if name == "pgdata" {
				return volumeInspectFixture("pgdata"), nil
			}
			return nil, nil
		},
		exportFn: func(name string, w io.Writer) error {
			_, err := w.Write(payload)
			return err
		},
	}
}

// emptyTargetFixture is the target machine's engine: nothing exists yet,
// creates succeed, imports consume.
func emptyTargetFixture() *fakeVolumeEngine {
	return &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) { return nil, nil },
		createFn: func(opts entities.VolumeCreateOptions) (*entities.IDOrNameResponse, error) {
			return &entities.IDOrNameResponse{IDOrName: opts.Name}, nil
		},
		importFn: func(name string, r io.Reader) error {
			_, err := io.ReadAll(r)
			return err
		},
	}
}

// idleUsageFixture answers an empty in-use surface: no containers, no
// quadlets.
func idleUsageFixture() *fakeUsageEngine {
	return &fakeUsageEngine{
		containerMountsFn:  func() ([]ContainerMounts, error) { return nil, nil },
		listQuadletNamesFn: func() ([]string, error) { return nil, nil },
	}
}

// --- MovePreview ----------------------------------------------------------

func TestVolumeService_MovePreview_ReportsFacts(t *testing.T) {
	usage := &fakeUsageEngine{
		containerMountsFn: func() ([]ContainerMounts, error) {
			return []ContainerMounts{
				{Name: "web", Mounts: []string{"pgdata:/var/lib/pg:Z"}},
				{Name: "other", Mounts: []string{"scratch:/tmp"}},
			}, nil
		},
		listQuadletNamesFn: func() ([]string, error) {
			return []string{"web.container", "db.container", "db.volume", "side.container"}, nil
		},
		printQuadletFn: func(name string) (string, error) {
			switch name {
			case "web.container":
				return "[Container]\nImage=busybox\nVolume=pgdata:/var/lib/pg:Z\n", nil
			case "db.container":
				return "[Container]\nImage=postgres\nVolume=db.volume:/var/lib/pg\n", nil
			case "db.volume":
				return "[Volume]\nVolumeName=pgdata\n", nil
			case "side.container":
				return "[Container]\nImage=busybox\nVolume=/srv/host:/srv\nVolume=other:/other\n", nil
			}
			return "", errors.New("no such quadlet")
		},
	}
	service := newMoveTestService(moveSourceFixture(t), emptyTargetFixture(), usage,
		&fakeAccessRepo{getPermissionFn: allowTargetRead})

	got, err := service.MovePreview(context.Background(), "user", "machine-a", "pgdata", "machine-b")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.False(t, got.Collision, "an empty target holds no same-named volume")
	assert.True(t, got.InUse, "a container mounting the volume marks it in use")
	require.Len(t, got.ReferencingQuadlets, 2, "the bare-name and the companion reference both resolve")
	assert.Equal(t, "web.container", got.ReferencingQuadlets[0].Name)
	assert.Equal(t, "db.container", got.ReferencingQuadlets[1].Name,
		"a .volume companion reference resolves through its VolumeName=")
}

func TestVolumeService_MovePreview_Collision(t *testing.T) {
	target := emptyTargetFixture()
	target.inspectFn = func(name string) (*entities.VolumeInspectReport, error) {
		return volumeInspectFixture(name), nil
	}
	service := newMoveTestService(moveSourceFixture(t), target, idleUsageFixture(),
		&fakeAccessRepo{getPermissionFn: allowTargetRead})

	got, err := service.MovePreview(context.Background(), "user", "machine-a", "pgdata", "machine-b")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Collision, "a target holding the same-named volume collides")
}

func TestVolumeService_MovePreview_MissingSourceVolumeIs404(t *testing.T) {
	source := &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) { return nil, nil },
	}
	service := newMoveTestService(source, emptyTargetFixture(), idleUsageFixture(),
		&fakeAccessRepo{getPermissionFn: allowTargetRead})

	_, err := service.MovePreview(context.Background(), "user", "machine-a", "gone", "machine-b")

	require.Error(t, err)
	assert.Equal(t, appErrors.ErrVolumeNotFound, err)
}

func TestVolumeService_MovePreview_TargetAccessDeniedIs403(t *testing.T) {
	// The source engine panics on any call: the target gate must refuse
	// before any podman work.
	source := &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) {
			panic("source engine must not be reached when the target gate refuses")
		},
	}
	service := newMoveTestService(source, emptyTargetFixture(), idleUsageFixture(),
		&fakeAccessRepo{getPermissionFn: func(string, string) (string, error) {
			return "", nil
		}})

	_, err := service.MovePreview(context.Background(), "user", "machine-a", "pgdata", "machine-b")

	require.Error(t, err)
	assert.Equal(t, appErrors.ErrAccessDenied, err)
}

func TestVolumeService_MovePreview_SourceUnreachableIsConnectionFailed(t *testing.T) {
	service := NewVolumeService(fakeEngineProvider{err: appErrors.ErrConnectionFailed}).
		WithAccessRepo(&fakeAccessRepo{getPermissionFn: allowTargetRead})

	_, err := service.MovePreview(context.Background(), "user", "machine-a", "pgdata", "machine-b")

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
}

// --- Move: copy mode ------------------------------------------------------

func TestVolumeService_Move_CopyCreatesTargetAndCarriesData(t *testing.T) {
	target := emptyTargetFixture()
	created := ""
	target.createFn = func(opts entities.VolumeCreateOptions) (*entities.IDOrNameResponse, error) {
		created = opts.Name
		return &entities.IDOrNameResponse{IDOrName: opts.Name}, nil
	}
	imported := ""
	target.importFn = func(name string, r io.Reader) error {
		data, err := io.ReadAll(r)
		imported = string(data)
		return err
	}
	// The usage engine panics on any call: a copy never scans in-use state.
	usage := &fakeUsageEngine{}
	service := newMoveTestService(moveSourceFixture(t), target, usage,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite})

	got, err := service.Move(context.Background(), "user", "machine-a", "pgdata",
		VolumeMoveRequest{TargetMachineID: "machine-b", Mode: VolumeMoveModeCopy}, nil)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "pgdata", created, "the target volume carries the source's name")
	assert.NotEmpty(t, imported, "the data crossed the splice")
	assert.Equal(t, VolumeMoveStatusCopied, got.Status)
	assert.Equal(t, "machine-a", got.SourceMachineID)
	assert.Equal(t, "machine-b", got.TargetMachineID)
	assert.Greater(t, got.Bytes, int64(0), "the envelope reports the bytes streamed")
	assert.Empty(t, got.Error)
}

func TestVolumeService_Move_CollisionIs409(t *testing.T) {
	target := emptyTargetFixture()
	target.inspectFn = func(name string) (*entities.VolumeInspectReport, error) {
		return volumeInspectFixture(name), nil
	}
	target.createFn = func(entities.VolumeCreateOptions) (*entities.IDOrNameResponse, error) {
		panic("a colliding transfer must not create on the target")
	}
	service := newMoveTestService(moveSourceFixture(t), target, idleUsageFixture(),
		&fakeAccessRepo{getPermissionFn: allowTargetWrite})

	_, err := service.Move(context.Background(), "user", "machine-a", "pgdata",
		VolumeMoveRequest{TargetMachineID: "machine-b", Mode: VolumeMoveModeCopy}, nil)

	require.Error(t, err)
	assert.Equal(t, appErrors.ErrVolumeAlreadyExists, err)
}

func TestVolumeService_Move_MissingSourceVolumeIs404(t *testing.T) {
	source := &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) { return nil, nil },
	}
	service := newMoveTestService(source, emptyTargetFixture(), idleUsageFixture(),
		&fakeAccessRepo{getPermissionFn: allowTargetWrite})

	_, err := service.Move(context.Background(), "user", "machine-a", "gone",
		VolumeMoveRequest{TargetMachineID: "machine-b", Mode: VolumeMoveModeCopy}, nil)

	require.Error(t, err)
	assert.Equal(t, appErrors.ErrVolumeNotFound, err)
}

func TestVolumeService_Move_TargetAccessDeniedIs403(t *testing.T) {
	// Read on the target is below the write the transfer needs; the
	// source engine panics on any call: the gate refuses before any
	// podman work.
	source := &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) {
			panic("source engine must not be reached when the target gate refuses")
		},
	}
	service := newMoveTestService(source, emptyTargetFixture(), idleUsageFixture(),
		&fakeAccessRepo{getPermissionFn: allowTargetRead})

	_, err := service.Move(context.Background(), "user", "machine-a", "pgdata",
		VolumeMoveRequest{TargetMachineID: "machine-b", Mode: VolumeMoveModeCopy}, nil)

	require.Error(t, err)
	assert.Equal(t, appErrors.ErrAccessDenied, err)
}

func TestVolumeService_Move_SourceUnreachableIsConnectionFailed(t *testing.T) {
	service := NewVolumeService(fakeEngineProvider{err: appErrors.ErrConnectionFailed}).
		WithAccessRepo(&fakeAccessRepo{getPermissionFn: allowTargetWrite})

	_, err := service.Move(context.Background(), "user", "machine-a", "pgdata",
		VolumeMoveRequest{TargetMachineID: "machine-b", Mode: VolumeMoveModeCopy}, nil)

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
}

// TestVolumeService_Move_ImportFailureRollsTargetBack pins the rollback: a
// failure after the target volume's creation removes the half-populated
// volume, and the error says so — a partial volume must not survive to
// block a retry with a false collision.
func TestVolumeService_Move_ImportFailureRollsTargetBack(t *testing.T) {
	target := emptyTargetFixture()
	removed := ""
	target.importFn = func(string, io.Reader) error {
		return errors.New("import refused")
	}
	target.removeFn = func(name string, force bool) error {
		removed = name
		return nil
	}
	service := newMoveTestService(moveSourceFixture(t), target, idleUsageFixture(),
		&fakeAccessRepo{getPermissionFn: allowTargetWrite})

	_, err := service.Move(context.Background(), "user", "machine-a", "pgdata",
		VolumeMoveRequest{TargetMachineID: "machine-b", Mode: VolumeMoveModeCopy}, nil)

	require.Error(t, err)
	assert.Equal(t, "pgdata", removed, "the half-populated target volume is removed")
	assert.Contains(t, err.Error(), "partially transferred target volume was removed",
		"the error states the rollback")
}

// TestVolumeService_Move_BoundTripsAndRollsBack pins the 413: a stream
// crossing the wired bound fails with the upload sentinel, and the
// half-populated target volume is removed.
func TestVolumeService_Move_BoundTripsAndRollsBack(t *testing.T) {
	source := moveSourceFixture(t)
	source.exportFn = func(name string, w io.Writer) error {
		_, err := w.Write(bytes.Repeat([]byte("x"), 3<<20))
		return err
	}
	target := emptyTargetFixture()
	removed := ""
	target.removeFn = func(name string, force bool) error {
		removed = name
		return nil
	}
	service := NewVolumeService(fakeEngineProvider{engines: map[string]VolumeEngine{
		"machine-a": source,
		"machine-b": target,
	}}).
		WithAccessRepo(&fakeAccessRepo{getPermissionFn: allowTargetWrite}).
		WithUsageEngines(fakeUsageEngineProvider{engines: map[string]VolumeUsageEngine{
			"machine-a": idleUsageFixture(),
		}}).
		WithMaxVolumeUploadBytes(1 << 20)

	_, err := service.Move(context.Background(), "user", "machine-a", "pgdata",
		VolumeMoveRequest{TargetMachineID: "machine-b", Mode: VolumeMoveModeCopy}, nil)

	require.Error(t, err)
	appErr, ok := err.(*appErrors.AppError)
	require.True(t, ok, "the bound maps onto the 413 sentinel")
	assert.Equal(t, appErrors.ErrVolumeUploadTooLarge.Code, appErr.Code)
	assert.Equal(t, "pgdata", removed, "the half-populated target volume is removed")
}

// TestVolumeService_Move_BoundWithFailedRollbackNamesBoth pins that a
// rollback failure is never narrated as a success on the bound path: the
// 413 still answers with the upload sentinel, but its message carries the
// removal failure instead of claiming the partial volume was removed.
func TestVolumeService_Move_BoundWithFailedRollbackNamesBoth(t *testing.T) {
	source := moveSourceFixture(t)
	source.exportFn = func(name string, w io.Writer) error {
		_, err := w.Write(bytes.Repeat([]byte("x"), 3<<20))
		return err
	}
	target := emptyTargetFixture()
	target.removeFn = func(string, bool) error {
		return errors.New("remove refused")
	}
	service := NewVolumeService(fakeEngineProvider{engines: map[string]VolumeEngine{
		"machine-a": source,
		"machine-b": target,
	}}).
		WithAccessRepo(&fakeAccessRepo{getPermissionFn: allowTargetWrite}).
		WithUsageEngines(fakeUsageEngineProvider{engines: map[string]VolumeUsageEngine{
			"machine-a": idleUsageFixture(),
		}}).
		WithMaxVolumeUploadBytes(1 << 20)

	_, err := service.Move(context.Background(), "user", "machine-a", "pgdata",
		VolumeMoveRequest{TargetMachineID: "machine-b", Mode: VolumeMoveModeCopy}, nil)

	require.Error(t, err)
	appErr, ok := err.(*appErrors.AppError)
	require.True(t, ok, "the bound still maps onto the 413 sentinel")
	assert.Equal(t, appErrors.ErrVolumeUploadTooLarge.Code, appErr.Code)
	assert.Contains(t, err.Error(), "exceeded the transfer bound", "the message names the bound")
	assert.Contains(t, err.Error(), "remove refused", "the message names the rollback failure")
	assert.NotContains(t, err.Error(), "was removed", "a failed rollback is not narrated as a success")
}

// TestVolumeService_Move_CancelledContextRollsTargetBack pins the
// cancellation path's half of decision 8: the rollback runs even when
// the request context died mid-transfer — the engine calls carry no
// ctx, so the still-open target lease performs the removal — while the
// error rides bare: the streaming kit's cancelled-wins rule answers
// every cancellation with the generic OPERATION_CANCELLED terminal
// event, so no rollback narration could reach the wire (the branch's
// comment in move.go says why).
func TestVolumeService_Move_CancelledContextRollsTargetBack(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	target := emptyTargetFixture()
	removed := ""
	target.importFn = func(string, io.Reader) error {
		// The import dies first; the transfer kit's cancelled-context-
		// wins rule maps the failure to ctx.Err() once the context is
		// dead.
		cancel()
		return errors.New("import refused")
	}
	target.removeFn = func(name string, force bool) error {
		removed = name
		return nil
	}
	service := newMoveTestService(moveSourceFixture(t), target, idleUsageFixture(),
		&fakeAccessRepo{getPermissionFn: allowTargetWrite})

	_, err := service.Move(ctx, "user", "machine-a", "pgdata",
		VolumeMoveRequest{TargetMachineID: "machine-b", Mode: VolumeMoveModeCopy}, nil)

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err, "the cancellation rides bare — no narration wrapped")
	assert.Equal(t, "pgdata", removed, "the half-populated target volume is removed despite the cancellation")
}

// --- Move: move mode ------------------------------------------------------

// movableSourceFixture arms the source's remove: a successful removal.
func movableSourceFixture(t *testing.T) *fakeVolumeEngine {
	source := moveSourceFixture(t)
	source.removeFn = func(string, bool) error {
		return nil
	}
	return source
}

func TestVolumeService_Move_MoveRemovesSourceAfterCopy(t *testing.T) {
	removedWith := ""
	source := movableSourceFixture(t)
	source.removeFn = func(name string, force bool) error {
		removedWith = name
		return nil
	}
	service := newMoveTestService(source, emptyTargetFixture(), idleUsageFixture(),
		&fakeAccessRepo{getPermissionFn: allowTargetWrite})

	got, err := service.Move(context.Background(), "user", "machine-a", "pgdata",
		VolumeMoveRequest{TargetMachineID: "machine-b", Mode: VolumeMoveModeMove}, nil)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, VolumeMoveStatusMoved, got.Status)
	assert.Equal(t, "pgdata", removedWith, "the source volume is removed after the copy")
	assert.Empty(t, got.Error)
}

func TestVolumeService_Move_InUseSourceRefusesWithoutForce(t *testing.T) {
	usage := &fakeUsageEngine{
		containerMountsFn: func() ([]ContainerMounts, error) {
			return []ContainerMounts{{Name: "web", Mounts: []string{"pgdata:/var/lib/pg:Z"}}}, nil
		},
	}
	target := emptyTargetFixture()
	target.createFn = func(entities.VolumeCreateOptions) (*entities.IDOrNameResponse, error) {
		panic("a refused move must not create on the target")
	}
	service := newMoveTestService(moveSourceFixture(t), target, usage,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite})

	_, err := service.Move(context.Background(), "user", "machine-a", "pgdata",
		VolumeMoveRequest{TargetMachineID: "machine-b", Mode: VolumeMoveModeMove}, nil)

	require.Error(t, err)
	assert.Equal(t, appErrors.ErrVolumeInUse, err)
}

func TestVolumeService_Move_InUseSourceForceProceeds(t *testing.T) {
	usage := &fakeUsageEngine{
		containerMountsFn: func() ([]ContainerMounts, error) {
			return []ContainerMounts{{Name: "web", Mounts: []string{"pgdata:/var/lib/pg:Z"}}}, nil
		},
	}
	service := newMoveTestService(movableSourceFixture(t), emptyTargetFixture(), usage,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite})

	got, err := service.Move(context.Background(), "user", "machine-a", "pgdata",
		VolumeMoveRequest{TargetMachineID: "machine-b", Mode: VolumeMoveModeMove, Force: true}, nil)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, VolumeMoveStatusMoved, got.Status)
}

func TestVolumeService_Move_RemoveFailureIsPartial(t *testing.T) {
	source := movableSourceFixture(t)
	source.removeFn = func(string, bool) error {
		return errors.New("source read-only")
	}
	service := newMoveTestService(source, emptyTargetFixture(), idleUsageFixture(),
		&fakeAccessRepo{getPermissionFn: allowTargetWrite})

	got, err := service.Move(context.Background(), "user", "machine-a", "pgdata",
		VolumeMoveRequest{TargetMachineID: "machine-b", Mode: VolumeMoveModeMove}, nil)

	require.NoError(t, err, "a partial transfer is not a request failure")
	require.NotNil(t, got)
	assert.Equal(t, VolumeMoveStatusPartial, got.Status)
	assert.Contains(t, got.Error, "source read-only", "partial carries the removal failure's detail")
}

// TestVolumeService_Move_InUseSourceRemovalIsPartial pins the doku's move
// sentence (doku/description_volumes_api.md): an in-use refusal at the
// source removal answers status "partial" with the 409 VOLUME_IN_USE
// detail. The removal module's conflict classification (issue 264) maps
// podman's 409 onto ErrVolumeInUse and threads the refusal text, which the
// envelope carries as the partial detail.
func TestVolumeService_Move_InUseSourceRemovalIsPartial(t *testing.T) {
	source := movableSourceFixture(t)
	source.removeFn = func(string, bool) error {
		return &errorhandling.ErrorModel{
			ResponseCode: http.StatusConflict,
			Message:      "volume is in use by container web",
		}
	}
	service := newMoveTestService(source, emptyTargetFixture(), idleUsageFixture(),
		&fakeAccessRepo{getPermissionFn: allowTargetWrite})

	got, err := service.Move(context.Background(), "user", "machine-a", "pgdata",
		VolumeMoveRequest{TargetMachineID: "machine-b", Mode: VolumeMoveModeMove}, nil)

	require.NoError(t, err, "a partial transfer is not a request failure")
	require.NotNil(t, got)
	assert.Equal(t, VolumeMoveStatusPartial, got.Status)
	assert.Contains(t, got.Error, "VOLUME_IN_USE", "partial carries the 409 detail's code")
	assert.Contains(t, got.Error, "volume is in use by container web",
		"partial carries podman's refusal text")
}

func TestVolumeService_Move_VanishedSourceIsMoved(t *testing.T) {
	source := movableSourceFixture(t)
	source.removeFn = func(string, bool) error {
		// The adapter folds podman's per-item not-found into the call's
		// error.
		return errors.New("no such volume")
	}
	service := newMoveTestService(source, emptyTargetFixture(), idleUsageFixture(),
		&fakeAccessRepo{getPermissionFn: allowTargetWrite})

	got, err := service.Move(context.Background(), "user", "machine-a", "pgdata",
		VolumeMoveRequest{TargetMachineID: "machine-b", Mode: VolumeMoveModeMove}, nil)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, VolumeMoveStatusMoved, got.Status,
		"a vanished source needs no removal — the transfer's goal is met")
	assert.Empty(t, got.Error)
}
