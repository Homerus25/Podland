package quadlets

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// Volume data migration service tests (issue 214, architecture-decisions
// §18): plan resolution, preflight refusals, the machine-to-machine data
// phase, the bound's pre-check and counting backstop, and the migrating
// move's stop-before-data ordering.

const moveVolumeContent = `[Container]
Image=busybox
Exec=sh -c "sleep 3600"
Volume=pgdata:/var/lib/pg:Z
`

const moveVolumeDanglingContent = `[Container]
Image=busybox
Volume=missing.volume:/var/lib/gone
`

// buildVolumeTar is what an export fake streams: the volume-root directory
// entry podman always frames, plus one regular file per map entry.
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

// volumeSourceFixture is the source lease's answer for the data-phase
// tests: printable content, one stopped quadlet, and a volume inspect that
// answers the map (an absent name is the production "volume does not
// exist" contract, nil/nil).
func volumeSourceFixture(content string, volumes map[string]*QuadletVolumeInspect) *fakeQuadletEngine {
	return &fakeQuadletEngine{
		printFn: func(string) (string, error) { return content, nil },
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"},
			}, nil
		},
		volumeInspectFn: func(name string) (*QuadletVolumeInspect, error) {
			if inspect, ok := volumes[name]; ok {
				return inspect, nil
			}
			return nil, nil
		},
	}
}

// newMoveVolumeTestService wires the two per-machine fake engines the data
// phase dials, an SSH exec (the post-install reload and move-mode stops run
// through it), and optionally a migration bound (0 keeps the default).
func newMoveVolumeTestService(source, target *fakeQuadletEngine, limit int64) *QuadletService {
	service := NewQuadletService(fakeEngineProvider{engines: map[string]QuadletEngine{
		"machine-a": source,
		"machine-b": target,
	}}, testutils.NewMockSSHExec(), lookupForUser("core")).
		WithAccessRepo(&fakeAccessRepo{getPermissionFn: allowTargetWrite})
	if limit > 0 {
		service = service.WithMaxVolumeUploadBytes(limit)
	}
	return service
}

// migrateCopyRequest opts a copy transfer into volume data migration.
func migrateCopyRequest() QuadletMoveRequest {
	return QuadletMoveRequest{TargetMachineID: "machine-b", Mode: MoveModeCopy, MigrateVolumeData: true}
}

// TestQuadletService_MoveVolumeData_BareVolumeMigrates pins the happy
// splice: the reference resolves to itself, the target volume is created,
// the source export's tar reaches the target import byte-for-byte, and the
// envelope reports one migrated outcome.
func TestQuadletService_MoveVolumeData_BareVolumeMigrates(t *testing.T) {
	payload := buildVolumeTar(t, map[string][]byte{"pgdata/PG_VERSION": []byte("17")})
	var gotCreated, gotImported string
	var gotImportBytes []byte
	source := volumeSourceFixture(moveVolumeContent, map[string]*QuadletVolumeInspect{
		"pgdata": {Name: "pgdata"},
	})
	source.volumeExportFn = func(name string, w io.Writer) error {
		_, err := w.Write(payload)
		return err
	}
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return &entities.QuadletInstallReport{}, nil
		},
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) { return nil, nil },
		volumeCreateFn: func(name string) error {
			gotCreated = name
			return nil
		},
		volumeImportFn: func(name string, r io.Reader) error {
			gotImported = name
			data, err := io.ReadAll(r)
			gotImportBytes = data
			return err
		},
	}

	service := newMoveVolumeTestService(source, target, 0)
	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", migrateCopyRequest())

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, MoveStatusCopied, got.Status)
	assert.Equal(t, "pgdata", gotCreated, "the resolved volume is created if absent on the target")
	assert.Equal(t, "pgdata", gotImported)
	assert.Equal(t, payload, gotImportBytes, "the import receives exactly the export's tar")
	require.NotNil(t, got.VolumeMigration)
	require.Len(t, got.VolumeMigration.Volumes, 1)
	assert.Equal(t, QuadletMoveVolumeOutcome{
		Ref:          "pgdata",
		PodmanVolume: "pgdata",
		Outcome:      MoveVolumeOutcomeMigrated,
	}, got.VolumeMigration.Volumes[0])
}

// TestQuadletService_MoveVolumeData_CompanionVolumeNameWins pins the
// resolution order: a companion .volume quadlet's VolumeName= is the
// authoritative volume name, not podman's systemd-<name> default.
func TestQuadletService_MoveVolumeData_CompanionVolumeNameWins(t *testing.T) {
	companionContent := "[Volume]\nVolumeName=custom-name\n"
	content := "[Container]\nImage=busybox\nVolume=app-data.volume:/var/lib/app\n"
	var gotCreated string
	source := volumeSourceFixture(content, map[string]*QuadletVolumeInspect{
		"custom-name": {Name: "custom-name"},
	})
	source.printFn = func(name string) (string, error) {
		if name == "app-data.volume" {
			return companionContent, nil
		}
		return content, nil
	}
	source.listFn = func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
		return []*entities.ListQuadlet{
			{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"},
			{Name: "app-data.volume", Path: "/x/app-data.volume", Status: "stopped"},
		}, nil
	}
	source.volumeExportFn = func(_ string, w io.Writer) error {
		_, err := w.Write(buildVolumeTar(t, nil))
		return err
	}
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return &entities.QuadletInstallReport{}, nil
		},
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) { return nil, nil },
		volumeCreateFn: func(name string) error {
			gotCreated = name
			return nil
		},
		volumeImportFn: func(string, io.Reader) error { return nil },
	}

	service := newMoveVolumeTestService(source, target, 0)
	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", migrateCopyRequest())

	require.NoError(t, err)
	assert.Equal(t, "custom-name", gotCreated)
	require.NotNil(t, got.VolumeMigration)
	assert.Equal(t, "custom-name", got.VolumeMigration.Volumes[0].PodmanVolume)
	assert.Equal(t, "app-data.volume", got.VolumeMigration.Volumes[0].Ref)
}

// TestQuadletService_MoveVolumeData_CompanionDefaultNameWithoutOverride
// pins the fallback: a companion that sets no VolumeName= resolves to
// podman's systemd-<name> default, on both machines.
func TestQuadletService_MoveVolumeData_CompanionDefaultNameWithoutOverride(t *testing.T) {
	companionContent := "[Volume]\nDriver=local\n"
	content := "[Container]\nImage=busybox\nVolume=app-data.volume:/var/lib/app\n"
	var gotCreated string
	source := volumeSourceFixture(content, map[string]*QuadletVolumeInspect{
		"systemd-app-data": {Name: "systemd-app-data"},
	})
	source.printFn = func(name string) (string, error) {
		if name == "app-data.volume" {
			return companionContent, nil
		}
		return content, nil
	}
	source.listFn = func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
		return []*entities.ListQuadlet{
			{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"},
			{Name: "app-data.volume", Path: "/x/app-data.volume", Status: "stopped"},
		}, nil
	}
	source.volumeExportFn = func(_ string, w io.Writer) error {
		_, err := w.Write(buildVolumeTar(t, nil))
		return err
	}
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return &entities.QuadletInstallReport{}, nil
		},
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) { return nil, nil },
		volumeCreateFn: func(name string) error {
			gotCreated = name
			return nil
		},
		volumeImportFn: func(string, io.Reader) error { return nil },
	}

	service := newMoveVolumeTestService(source, target, 0)
	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", migrateCopyRequest())

	require.NoError(t, err)
	assert.Equal(t, "systemd-app-data", gotCreated)
	assert.Equal(t, MoveVolumeOutcomeMigrated, got.VolumeMigration.Volumes[0].Outcome)
}

// TestQuadletService_MoveVolumeData_DanglingReferenceIsSkipped pins the
// honest skip: a .volume reference with no companion file resolves its
// default name, carries no data (the export would panic the fake), and the
// transfer still succeeds with a skipped outcome naming the reason.
func TestQuadletService_MoveVolumeData_DanglingReferenceIsSkipped(t *testing.T) {
	source := volumeSourceFixture(moveVolumeDanglingContent, nil)
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return &entities.QuadletInstallReport{}, nil
		},
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) { return nil, nil },
		volumeCreateFn:  func(string) error { return nil },
	}

	service := newMoveVolumeTestService(source, target, 0)
	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", migrateCopyRequest())

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, MoveStatusCopied, got.Status)
	require.NotNil(t, got.VolumeMigration)
	require.Len(t, got.VolumeMigration.Volumes, 1)
	assert.Equal(t, QuadletMoveVolumeOutcome{
		Ref:          "missing.volume",
		PodmanVolume: "systemd-missing",
		Outcome:      MoveVolumeOutcomeSkipped,
		Reason:       MovePreviewReasonDangling,
	}, got.VolumeMigration.Volumes[0])
}

// TestQuadletService_MoveVolumeData_NonEmptyTargetIs409 pins the
// emptiness preflight: an existing target volume holding content refuses
// the transfer with the migration 409 before anything is stopped or
// installed — the probe read the target's export; no source export, no
// install ever fires.
func TestQuadletService_MoveVolumeData_NonEmptyTargetIs409(t *testing.T) {
	source := volumeSourceFixture(moveVolumeContent, map[string]*QuadletVolumeInspect{
		"pgdata": {Name: "pgdata"},
	})
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) {
			return &QuadletVolumeInspect{Name: "pgdata"}, nil
		},
		volumeExportFn: func(_ string, w io.Writer) error {
			_, err := w.Write(buildVolumeTar(t, map[string][]byte{"existing/file": []byte("keep me")}))
			return err
		},
	}

	service := newMoveVolumeTestService(source, target, 0)
	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", migrateCopyRequest())

	require.Nil(t, got)
	appErr, ok := errors.AsType[*appErrors.AppError](err)
	require.True(t, ok, "the refusal is an AppError, got: %v", err)
	assert.Equal(t, appErrors.ErrVolumeMigrationTargetNotEmpty.Code, appErr.Code)
	assert.Equal(t, appErrors.ErrVolumeMigrationTargetNotEmpty.Status, appErr.Status)
	assert.Contains(t, appErr.Message, "pgdata")
}

// TestQuadletService_MoveVolumeData_DanglingSourceSkipsTheProbe pins the
// dangling/preflight interaction: a reference whose source volume is absent
// never reaches the target probe — a non-empty target volume at the
// resolved name must not refuse the transfer, because the data phase will
// skip the reference and report it (dangling references are reported, never
// errors).
func TestQuadletService_MoveVolumeData_DanglingSourceSkipsTheProbe(t *testing.T) {
	source := volumeSourceFixture(moveVolumeDanglingContent, nil)
	probed := false
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return &entities.QuadletInstallReport{}, nil
		},
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) {
			return &QuadletVolumeInspect{Name: "systemd-missing"}, nil
		},
		volumeExportFn: func(_ string, w io.Writer) error {
			probed = true
			_, err := w.Write(buildVolumeTar(t, map[string][]byte{"existing/file": []byte("keep me")}))
			return err
		},
	}

	service := newMoveVolumeTestService(source, target, 0)
	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", migrateCopyRequest())

	require.NoError(t, err)
	assert.False(t, probed, "the emptiness probe never fires for a source-dangling reference")
	require.NotNil(t, got.VolumeMigration)
	require.Len(t, got.VolumeMigration.Volumes, 1)
	assert.Equal(t, QuadletMoveVolumeOutcome{
		Ref:          "missing.volume",
		PodmanVolume: "systemd-missing",
		Outcome:      MoveVolumeOutcomeSkipped,
		Reason:       MovePreviewReasonDangling,
	}, got.VolumeMigration.Volumes[0])
}

// TestQuadletService_MoveVolumeData_OverwriteVolumesMerges pins the merge
// opt-in: with overwriteVolumes, an existing target volume skips the
// emptiness probe (the target's export would panic the fake) and the data
// phase imports straight into it.
func TestQuadletService_MoveVolumeData_OverwriteVolumesMerges(t *testing.T) {
	source := volumeSourceFixture(moveVolumeContent, map[string]*QuadletVolumeInspect{
		"pgdata": {Name: "pgdata"},
	})
	source.volumeExportFn = func(_ string, w io.Writer) error {
		_, err := w.Write(buildVolumeTar(t, map[string][]byte{"pgdata/new": []byte("merged")}))
		return err
	}
	var imported bool
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return &entities.QuadletInstallReport{}, nil
		},
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) {
			return &QuadletVolumeInspect{Name: "pgdata"}, nil
		},
		volumeCreateFn: func(string) error { return nil },
		volumeImportFn: func(_ string, r io.Reader) error {
			_, err := io.Copy(io.Discard, r)
			imported = err == nil
			return err
		},
	}

	service := newMoveVolumeTestService(source, target, 0)
	req := migrateCopyRequest()
	req.OverwriteVolumes = true
	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", req)

	require.NoError(t, err)
	assert.True(t, imported)
	assert.Equal(t, MoveVolumeOutcomeMigrated, got.VolumeMigration.Volumes[0].Outcome)
}

// TestQuadletService_MoveVolumeData_KnownOversizeIs413 pins the pre-check:
// when an engine reports an inspect size above the bound, the 413 fires in
// the source lease — before the target is dialed at all (its engine would
// panic the provider) and before anything moves.
func TestQuadletService_MoveVolumeData_KnownOversizeIs413(t *testing.T) {
	source := volumeSourceFixture(moveVolumeContent, map[string]*QuadletVolumeInspect{
		"pgdata": {Name: "pgdata", Size: ptr[int64](2048)},
	})
	// The provider hands this engine out for machine-b: any crossing of
	// the target lease panics the test.
	target := &fakeQuadletEngine{}

	service := newMoveVolumeTestService(source, target, 1024)
	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", migrateCopyRequest())

	require.Nil(t, got)
	appErr, ok := errors.AsType[*appErrors.AppError](err)
	require.True(t, ok, "the refusal is an AppError, got: %v", err)
	assert.Equal(t, appErrors.ErrVolumeUploadTooLarge.Code, appErr.Code)
	assert.Equal(t, appErrors.ErrVolumeUploadTooLarge.Status, appErr.Status)
	assert.Contains(t, appErr.Message, "pgdata")
	assert.Contains(t, appErr.Message, "2048")
}

// TestQuadletService_MoveVolumeData_CountingReaderBackstopsTheBound pins
// the always-on enforcement: an engine that reports no size (production's
// shape today) still cannot stream past the bound — the counting reader
// fails the import at the first byte past it, and the answer is the 413.
func TestQuadletService_MoveVolumeData_CountingReaderBackstopsTheBound(t *testing.T) {
	source := volumeSourceFixture(moveVolumeContent, map[string]*QuadletVolumeInspect{
		"pgdata": {Name: "pgdata"},
	})
	source.volumeExportFn = func(_ string, w io.Writer) error {
		_, err := w.Write(buildVolumeTar(t, map[string][]byte{"pgdata/big": bytes.Repeat([]byte{0}, 2048)}))
		return err
	}
	target := &fakeQuadletEngine{
		listFn:          func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) { return nil, nil },
		volumeCreateFn:  func(string) error { return nil },
		volumeImportFn: func(_ string, r io.Reader) error {
			_, err := io.Copy(io.Discard, r)
			return err
		},
	}

	service := newMoveVolumeTestService(source, target, 1024)
	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", migrateCopyRequest())

	require.Nil(t, got)
	appErr, ok := errors.AsType[*appErrors.AppError](err)
	require.True(t, ok, "the refusal is an AppError, got: %v", err)
	assert.Equal(t, appErrors.ErrVolumeUploadTooLarge.Code, appErr.Code)
	assert.Contains(t, appErr.Message, "pgdata")
}

// TestQuadletService_MoveVolumeData_ImportFailureInstallsNothing pins the
// nothing-installed guarantee: a data-phase failure answers an error and
// the install lease never opens (the install would panic the fake).
func TestQuadletService_MoveVolumeData_ImportFailureInstallsNothing(t *testing.T) {
	source := volumeSourceFixture(moveVolumeContent, map[string]*QuadletVolumeInspect{
		"pgdata": {Name: "pgdata"},
	})
	source.volumeExportFn = func(_ string, w io.Writer) error {
		_, err := w.Write(buildVolumeTar(t, nil))
		return err
	}
	target := &fakeQuadletEngine{
		listFn:          func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) { return nil, nil },
		volumeCreateFn:  func(string) error { return nil },
		volumeImportFn:  func(string, io.Reader) error { return errors.New("disk full") },
	}

	service := newMoveVolumeTestService(source, target, 0)
	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", migrateCopyRequest())

	require.Nil(t, got)
	appErr, ok := errors.AsType[*appErrors.AppError](err)
	require.True(t, ok, "the failure is an AppError, got: %v", err)
	assert.Equal(t, appErrors.ErrVolumeMigrationFailed.Code, appErr.Code)
	assert.Contains(t, appErr.Message, "pgdata")
	assert.Contains(t, appErr.Message, "disk full")
}

// TestQuadletService_MoveVolumeData_MoveStopsBeforeTheDataPhase pins the
// ordering contract: a migrating move of a running quadlet stops the
// source's unit through the lifecycle seam BEFORE the export streams, and
// the remove still comes after everything (the delete flow's shape).
func TestQuadletService_MoveVolumeData_MoveStopsBeforeTheDataPhase(t *testing.T) {
	sequence := []string{}
	sshCallsAtExport := -1
	sshExec := testutils.NewMockSSHExec()
	source := volumeSourceFixture(moveVolumeContent, map[string]*QuadletVolumeInspect{
		"pgdata": {Name: "pgdata"},
	})
	source.listFn = func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
		return []*entities.ListQuadlet{
			{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "running"},
		}, nil
	}
	source.volumeExportFn = func(_ string, w io.Writer) error {
		sshCallsAtExport = len(sshExec.Calls())
		_, err := w.Write(buildVolumeTar(t, nil))
		return err
	}
	source.removeFn = func(string, bool) (*entities.QuadletRemoveReport, error) {
		sequence = append(sequence, "remove")
		return &entities.QuadletRemoveReport{}, nil
	}
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			sequence = append(sequence, "install")
			return &entities.QuadletInstallReport{}, nil
		},
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) { return nil, nil },
		volumeCreateFn:  func(string) error { return nil },
		volumeImportFn: func(_ string, r io.Reader) error {
			sequence = append(sequence, "import")
			_, err := io.Copy(io.Discard, r)
			return err
		},
	}

	service := newMoveTransferTestService(source, target, &fakeAccessRepo{getPermissionFn: allowTargetWrite}, sshExec)
	req := QuadletMoveRequest{TargetMachineID: "machine-b", Mode: MoveModeMove, Stop: true, MigrateVolumeData: true}
	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", req)

	require.NoError(t, err)
	assert.Equal(t, MoveStatusMoved, got.Status)

	// Three SSH calls: the pre-data stop, the target's reload, and the
	// removal lease's stop-before-remove re-run (idempotent — the unit is
	// already stopped, but the ritual re-lists and keeps its flag honored,
	// byte-identical to the non-migration move).
	require.Len(t, sshExec.Calls(), 3)
	assert.Equal(t, "machine-a", sshExec.Calls()[0].MachineID)
	assert.Equal(t, []string{"--user", "stop", "web.service"}, sshExec.Calls()[0].Opts.Args)
	assert.Equal(t, "machine-b", sshExec.Calls()[1].MachineID, "the reload completes the target install before the source removal")
	assert.Equal(t, []string{"--user", "daemon-reload"}, sshExec.Calls()[1].Opts.Args)
	assert.Equal(t, "machine-a", sshExec.Calls()[2].MachineID)
	assert.Equal(t, []string{"--user", "stop", "web.service"}, sshExec.Calls()[2].Opts.Args)
	assert.Equal(t, 1, sshCallsAtExport, "the export must not stream before the source unit is stopped")

	// Full order after the stop: import → install → remove. (The export
	// marker lives in the data-phase goroutine, so it races the import's
	// append here; its ordering is pinned by sshCallsAtExport above and by
	// the byte-for-byte splice test.)
	assert.Equal(t, []string{"import", "install", "remove"}, sequence)
}

// TestQuadletService_MoveVolumeData_DataFailureAfterTheStopCarriesTheNote
// pins the honest-failure annotation (issue 207 review): a data-phase
// failure after the migrating move's stop keeps the migration-failure code
// and cause and carries the stop note — the source's unit was stopped and
// stays stopped, and the error says so instead of going silent.
func TestQuadletService_MoveVolumeData_DataFailureAfterTheStopCarriesTheNote(t *testing.T) {
	sshExec := testutils.NewMockSSHExec()
	source := volumeSourceFixture(moveVolumeContent, map[string]*QuadletVolumeInspect{
		"pgdata": {Name: "pgdata"},
	})
	source.listFn = func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
		return []*entities.ListQuadlet{
			{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "running"},
		}, nil
	}
	source.volumeExportFn = func(_ string, w io.Writer) error {
		_, err := w.Write(buildVolumeTar(t, nil))
		return err
	}
	target := &fakeQuadletEngine{
		listFn:          func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) { return nil, nil },
		volumeCreateFn:  func(string) error { return nil },
		volumeImportFn:  func(string, io.Reader) error { return errors.New("disk full") },
	}

	service := newMoveTransferTestService(
		source, target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		sshExec,
	)
	req := QuadletMoveRequest{TargetMachineID: "machine-b", Mode: MoveModeMove, Stop: true, MigrateVolumeData: true}
	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", req)

	require.Nil(t, got)
	appErr, ok := errors.AsType[*appErrors.AppError](err)
	require.True(t, ok, "the failure is an AppError, got: %v", err)
	assert.Equal(t, appErrors.ErrVolumeMigrationFailed.Code, appErr.Code)
	assert.Contains(t, appErr.Message, "pgdata")
	assert.Contains(t, appErr.Message, "disk full")
	assert.Contains(t, appErr.Message, "the source quadlet was stopped before this failure and remains stopped")
	// Exactly one lifecycle call: the pre-data stop. The failure ended the
	// transfer before the install lease's reload and the removal's stop.
	require.Len(t, sshExec.Calls(), 1)
	assert.Equal(t, []string{"--user", "stop", "web.service"}, sshExec.Calls()[0].Opts.Args)
}

// TestQuadletService_MoveVolumeData_EventsFlow pins the progress feed: a
// migrating copy reports start and done volume events around the data, the
// skipped reference gets its skip event, and the events carry no
// operation ID (the streaming handler stamps its own).
func TestQuadletService_MoveVolumeData_EventsFlow(t *testing.T) {
	payload := buildVolumeTar(t, map[string][]byte{"pgdata/PG_VERSION": []byte("17")})
	source := volumeSourceFixture("[Container]\nImage=busybox\nVolume=pgdata:/x\nVolume=missing.volume:/y\n", map[string]*QuadletVolumeInspect{
		"pgdata": {Name: "pgdata"},
	})
	source.volumeExportFn = func(_ string, w io.Writer) error {
		_, err := w.Write(payload)
		return err
	}
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return &entities.QuadletInstallReport{}, nil
		},
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) { return nil, nil },
		volumeCreateFn:  func(string) error { return nil },
		volumeImportFn:  func(_ string, r io.Reader) error { _, err := io.Copy(io.Discard, r); return err },
	}

	events := []QuadletMoveEvent{}
	service := newMoveVolumeTestService(source, target, 0)
	got, err := service.MoveWithProgress(context.Background(), "user", "machine-a", "web.container", migrateCopyRequest(),
		func(event QuadletMoveEvent) { events = append(events, event) })

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, events, 3, "start+done for the migrated volume, skip for the dangling one")

	start := events[0]
	assert.Equal(t, MoveEventTypeVolume, start.Type)
	assert.Equal(t, MoveVolumePhaseStart, start.Phase)
	assert.Equal(t, "pgdata", start.Ref)
	assert.Equal(t, "pgdata", start.PodmanVolume)
	assert.Equal(t, "machine-a", start.MachineID)
	assert.Empty(t, start.OperationID, "the transport owns the operation ID")

	done := events[1]
	assert.Equal(t, MoveVolumePhaseDone, done.Phase)
	assert.Equal(t, "pgdata", done.Ref)
	assert.Equal(t, int64(len(payload)), done.Bytes)

	skip := events[2]
	assert.Equal(t, MoveVolumePhaseSkip, skip.Phase)
	assert.Equal(t, "missing.volume", skip.Ref)
	assert.Equal(t, "systemd-missing", skip.PodmanVolume)
	assert.Equal(t, MovePreviewReasonDangling, skip.Reason)
}

// TestQuadletService_MoveVolumeData_BytesEventsThrottle pins the cadence:
// the counting reader reports at most one bytes event per MiB crossed.
func TestQuadletService_MoveVolumeData_BytesEventsThrottle(t *testing.T) {
	payload := buildVolumeTar(t, map[string][]byte{"pgdata/big": bytes.Repeat([]byte{0}, (2<<20)+17)})
	source := volumeSourceFixture(moveVolumeContent, map[string]*QuadletVolumeInspect{
		"pgdata": {Name: "pgdata"},
	})
	source.volumeExportFn = func(_ string, w io.Writer) error {
		_, err := w.Write(payload)
		return err
	}
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return &entities.QuadletInstallReport{}, nil
		},
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) { return nil, nil },
		volumeCreateFn:  func(string) error { return nil },
		volumeImportFn:  func(_ string, r io.Reader) error { _, err := io.Copy(io.Discard, r); return err },
	}

	events := []QuadletMoveEvent{}
	service := newMoveVolumeTestService(source, target, 0)
	_, err := service.MoveWithProgress(context.Background(), "user", "machine-a", "web.container", migrateCopyRequest(),
		func(event QuadletMoveEvent) { events = append(events, event) })

	require.NoError(t, err)
	bytesEvents := []QuadletMoveEvent{}
	for _, event := range events {
		if event.Phase == MoveVolumePhaseBytes {
			bytesEvents = append(bytesEvents, event)
		}
	}
	require.Len(t, bytesEvents, 2, "one report per MiB crossed, not one per Read")
	assert.GreaterOrEqual(t, bytesEvents[0].Bytes, int64(1<<20))
	assert.GreaterOrEqual(t, bytesEvents[1].Bytes, int64(2<<20))
}

// TestQuadletService_MoveVolumeData_EmptyVolumeStillMigrates pins the
// framing edge: an empty volume exports as a directory-entry-only tar, and
// that is data worth carrying — the import receives it and the outcome is
// migrated, not skipped.
func TestQuadletService_MoveVolumeData_EmptyVolumeStillMigrates(t *testing.T) {
	payload := buildVolumeTar(t, nil)
	source := volumeSourceFixture(moveVolumeContent, map[string]*QuadletVolumeInspect{
		"pgdata": {Name: "pgdata"},
	})
	source.volumeExportFn = func(_ string, w io.Writer) error {
		_, err := w.Write(payload)
		return err
	}
	var gotImport []byte
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return &entities.QuadletInstallReport{}, nil
		},
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) { return nil, nil },
		volumeCreateFn:  func(string) error { return nil },
		volumeImportFn: func(_ string, r io.Reader) error {
			data, err := io.ReadAll(r)
			gotImport = data
			return err
		},
	}

	service := newMoveVolumeTestService(source, target, 0)
	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", migrateCopyRequest())

	require.NoError(t, err)
	assert.Equal(t, payload, gotImport)
	assert.Equal(t, MoveVolumeOutcomeMigrated, got.VolumeMigration.Volumes[0].Outcome)
}

// TestQuadletService_MoveVolumeData_TwoRefsOneVolumeMigratesOnce pins the
// plan-side projection of the same two-refs-one-volume fixture the preview
// test uses (issue 230, §18): two references resolving to one podman
// volume carry its data once and report one outcome, the first reference
// winning the entry. The print count pins the issue-230 fetch discipline:
// the main quadlet and the companion print once each per source lease —
// the companion's single print feeds both the companion install set and
// the resolution's VolumeName= extraction.
func TestQuadletService_MoveVolumeData_TwoRefsOneVolumeMigratesOnce(t *testing.T) {
	content := "[Container]\nImage=busybox\nVolume=app-data.volume:/a\nVolume=custom-name:/b\n"
	prints := 0
	source := volumeSourceFixture(content, map[string]*QuadletVolumeInspect{
		"custom-name": {Name: "custom-name"},
	})
	source.printFn = func(name string) (string, error) {
		prints++
		switch name {
		case "web.container":
			return content, nil
		case "app-data.volume":
			return "[Volume]\nVolumeName=custom-name\n", nil
		}
		panic("unexpected QuadletEngine.Print call: " + name)
	}
	source.listFn = func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
		return []*entities.ListQuadlet{
			{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"},
			{Name: "app-data.volume", Path: "/x/app-data.volume", Status: "stopped"},
		}, nil
	}
	source.volumeExportFn = func(_ string, w io.Writer) error {
		_, err := w.Write(buildVolumeTar(t, map[string][]byte{"custom-name/data": []byte("x")}))
		return err
	}
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return &entities.QuadletInstallReport{}, nil
		},
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) { return nil, nil },
		volumeCreateFn:  func(string) error { return nil },
		volumeImportFn:  func(_ string, r io.Reader) error { _, err := io.Copy(io.Discard, r); return err },
	}

	service := newMoveVolumeTestService(source, target, 0)
	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", migrateCopyRequest())

	require.NoError(t, err)
	require.NotNil(t, got.VolumeMigration)
	require.Len(t, got.VolumeMigration.Volumes, 1,
		"two references resolving to one volume migrate its data once and report one outcome")
	assert.Equal(t, QuadletMoveVolumeOutcome{
		Ref:          "app-data.volume",
		PodmanVolume: "custom-name",
		Outcome:      MoveVolumeOutcomeMigrated,
	}, got.VolumeMigration.Volumes[0])
	assert.Equal(t, 2, prints,
		"the main quadlet and the companion print once each per source lease (issue 230)")
}

// TestQuadletService_MovePreview_VolumeMigrationFacts pins the preview's
// flag-independent enrichment: an existing source volume is migratable
// with its inspect facts, the target's facts ride along (existence, size),
// and the entries stay honest when the engines report no sizes.
func TestQuadletService_MovePreview_VolumeMigrationFacts(t *testing.T) {
	content := "[Container]\nImage=busybox\nVolume=pgdata:/x\nVolume=/srv/data:/data\n"
	source := volumeSourceFixture(content, map[string]*QuadletVolumeInspect{
		"pgdata": {Name: "pgdata", Size: ptr[int64](42)},
	})
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		volumeInspectFn: func(name string) (*QuadletVolumeInspect, error) {
			if name == "pgdata" {
				return &QuadletVolumeInspect{Name: "pgdata", Size: ptr[int64](7)}, nil
			}
			return nil, nil
		},
	}

	service := NewQuadletService(fakeEngineProvider{engines: map[string]QuadletEngine{
		"machine-a": source,
		"machine-b": target,
	}}, nil, nil).WithAccessRepo(&fakeAccessRepo{getPermissionFn: allowTargetRead})

	got, err := service.MovePreview(context.Background(), "user", "machine-a", "web.container", "machine-b")

	require.NoError(t, err)
	require.Len(t, got.Volumes, 2)

	pg := got.Volumes[0]
	assert.Equal(t, "pgdata", pg.PodmanVolume)
	require.NotNil(t, pg.SizeBytes)
	assert.Equal(t, int64(42), *pg.SizeBytes)
	assert.True(t, pg.ExistsOnTarget)
	require.NotNil(t, pg.TargetSizeBytes)
	assert.Equal(t, int64(7), *pg.TargetSizeBytes)
	assert.True(t, pg.Migratable)
	assert.Empty(t, pg.Reason)

	host := got.Volumes[1]
	assert.Equal(t, MovePreviewVolumeKindHostPath, host.Kind)
	assert.False(t, host.Migratable)
	assert.Equal(t, MovePreviewReasonHostPath, host.Reason)
	assert.Empty(t, host.PodmanVolume)
	assert.False(t, host.ExistsOnTarget)
}

// --- The one derivation (issue 230, §18) ---

// TestDeriveVolumeResolution pins the one derivation both the preview's
// per-reference rows and the plan's per-volume entries read (issue 230,
// §18): refs stay in file order without dedup — two references resolving
// to one volume stay two refs — while the volume facts dedup by resolved
// name, first reference winning. Resolution consumes the companion
// contents the caller fetched once (no engine Print), so the table feeds
// companions directly.
func TestDeriveVolumeResolution(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		list        []*entities.ListQuadlet
		companions  []companionVolumeQuadlet
		inspects    map[string]*QuadletVolumeInspect
		wantRefs    []volumeRef
		wantVolumes []resolvedVolume
	}{
		{
			name: "two refs resolving to one volume stay two refs and one volume fact",
			content: "[Container]\n" +
				"Volume=app-data.volume:/a\n" +
				"Volume=custom-name:/b\n",
			companions: []companionVolumeQuadlet{
				{File: "app-data.volume", Content: "[Volume]\nVolumeName=custom-name\n"},
			},
			inspects: map[string]*QuadletVolumeInspect{
				"custom-name": {Name: "custom-name"},
			},
			wantRefs: []volumeRef{
				{Source: "app-data.volume", Kind: MovePreviewVolumeKindVolume, PodmanVolume: "custom-name", QuadletFile: "app-data.volume"},
				{Source: "custom-name", Kind: MovePreviewVolumeKindVolume, PodmanVolume: "custom-name"},
			},
			wantVolumes: []resolvedVolume{
				{Name: "custom-name", Ref: "app-data.volume", QuadletFile: "app-data.volume", Inspect: &QuadletVolumeInspect{Name: "custom-name"}},
			},
		},
		{
			name: "the same reference twice keeps both refs and one first-ref-wins volume fact",
			content: "[Container]\n" +
				"Volume=pgdata:/a\n" +
				"Volume=pgdata:/b\n",
			inspects: map[string]*QuadletVolumeInspect{
				"pgdata": {Name: "pgdata"},
			},
			wantRefs: []volumeRef{
				{Source: "pgdata", Kind: MovePreviewVolumeKindVolume, PodmanVolume: "pgdata"},
				{Source: "pgdata", Kind: MovePreviewVolumeKindVolume, PodmanVolume: "pgdata"},
			},
			wantVolumes: []resolvedVolume{
				{Name: "pgdata", Ref: "pgdata", Inspect: &QuadletVolumeInspect{Name: "pgdata"}},
			},
		},
		{
			name:    "host bind paths stay unresolved and out of the volume facts",
			content: "[Container]\nVolume=/srv/data:/data\n",
			wantRefs: []volumeRef{
				{Source: "/srv/data", Kind: MovePreviewVolumeKindHostPath},
			},
			wantVolumes: []resolvedVolume{},
		},
		{
			name:    "a dangling companion resolves the default name with no volume fact",
			content: "[Container]\nVolume=missing.volume:/gone\n",
			wantRefs: []volumeRef{
				{Source: "missing.volume", Kind: MovePreviewVolumeKindVolume, PodmanVolume: "systemd-missing", QuadletFile: "missing.volume"},
			},
			wantVolumes: []resolvedVolume{
				{Name: "systemd-missing", Ref: "missing.volume", QuadletFile: "missing.volume"},
			},
		},
		{
			name:    "a companion without VolumeName= resolves podman's systemd default",
			content: "[Container]\nVolume=app-data.volume:/a\n",
			companions: []companionVolumeQuadlet{
				{File: "app-data.volume", Content: "[Volume]\nDriver=local\n"},
			},
			wantRefs: []volumeRef{
				{Source: "app-data.volume", Kind: MovePreviewVolumeKindVolume, PodmanVolume: "systemd-app-data", QuadletFile: "app-data.volume"},
			},
			wantVolumes: []resolvedVolume{
				{Name: "systemd-app-data", Ref: "app-data.volume", QuadletFile: "app-data.volume"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &fakeQuadletEngine{volumeInspectFn: func(name string) (*QuadletVolumeInspect, error) {
				return tt.inspects[name], nil
			}}

			got, err := deriveVolumeResolution(parseQuadletVolumes(tt.content), tt.companions, e)

			require.NoError(t, err)
			assert.Equal(t, tt.wantRefs, got.Refs, "per-reference facts, file order, no dedup")
			assert.Equal(t, tt.wantVolumes, got.Volumes, "per-volume facts, deduped by resolved name, first ref wins")
		})
	}
}

// TestQuadletService_MoveVolumeData_ProbeEmptyTargetProceeds pins the
// probe's empty answer: a target volume whose export is a directory-only
// tar counts as empty, so the transfer proceeds without overwriteVolumes
// and imports into the existing volume.
func TestQuadletService_MoveVolumeData_ProbeEmptyTargetProceeds(t *testing.T) {
	source := volumeSourceFixture(moveVolumeContent, map[string]*QuadletVolumeInspect{
		"pgdata": {Name: "pgdata"},
	})
	source.volumeExportFn = func(_ string, w io.Writer) error {
		_, err := w.Write(buildVolumeTar(t, map[string][]byte{"pgdata/fresh": []byte("data")}))
		return err
	}
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return &entities.QuadletInstallReport{}, nil
		},
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) {
			return &QuadletVolumeInspect{Name: "pgdata"}, nil
		},
		// The PROBE export streams framing only; the data-phase export is
		// the source's. The second target export call would panic the fake.
		volumeExportFn: func(_ string, w io.Writer) error {
			_, err := w.Write(buildVolumeTar(t, nil))
			return err
		},
		volumeCreateFn: func(string) error { return nil },
		volumeImportFn: func(_ string, r io.Reader) error { _, err := io.Copy(io.Discard, r); return err },
	}

	service := newMoveVolumeTestService(source, target, 0)
	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", migrateCopyRequest())

	require.NoError(t, err)
	assert.True(t, strings.Contains(got.ID, "web.container"))
	assert.Equal(t, MoveVolumeOutcomeMigrated, got.VolumeMigration.Volumes[0].Outcome)
}
