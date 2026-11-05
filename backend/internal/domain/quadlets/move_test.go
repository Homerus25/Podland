package quadlets

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/sshexec"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// fakeAccessRepo is the function-field fake for the permission.PermissionGetter
// seam — the access store the cross-host operations (move/copy) gate the
// target host on. An unarmed lookup panics, so an unexpected gate check
// fails the test loudly.
type fakeAccessRepo struct {
	getPermissionFn func(hostID, userID string) (string, error)
}

func (f *fakeAccessRepo) GetPermission(hostID, userID string) (string, error) {
	if f.getPermissionFn == nil {
		panic("unexpected PermissionGetter.GetPermission call")
	}
	return f.getPermissionFn(hostID, userID)
}

// newMovePreviewTestServices builds both quadlet modules over per-host
// fake engines (the two-lease shape MovePreview exercises) and the given
// access store: the real lean service over the same engines, passed to
// the transfer service as its lifecycle seam (architecture review
// 2026-10-19, candidate C2). The source host's quadlet is
// web.container, stopped on host-a; engines map host IDs to their
// fakes so each lease dials its own host's engine.
func newMovePreviewTestServices(sourceEngine, targetEngine *fakeQuadletEngine, access *fakeAccessRepo) (*QuadletService, *QuadletTransferService) {
	provider := fakeEngineProvider{engines: map[string]QuadletEngine{
		"host-a": sourceEngine,
		"host-b": targetEngine,
	}}
	service := NewQuadletService(provider, nil, nil)
	return service, NewQuadletTransferService(provider, nil, access, 0, service)
}

// newMovePreviewTestService builds the transfer service over per-host
// fake engines and the given access store, with the real lean service as
// its seam — the shape the preview suites drive.
func newMovePreviewTestService(sourceEngine, targetEngine *fakeQuadletEngine, access *fakeAccessRepo) *QuadletTransferService {
	_, transfer := newMovePreviewTestServices(sourceEngine, targetEngine, access)
	return transfer
}

// movePreviewSourceFixture is the source lease's answer: one stopped
// container quadlet with printable content.
func movePreviewSourceFixture(content string) *fakeQuadletEngine {
	return &fakeQuadletEngine{
		printFn: func(string) (string, error) {
			return content, nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/etc/containers/systemd/web.container", Status: "stopped"},
			}, nil
		},
	}
}

const movePreviewContent = `[Container]
Image=busybox
Volume=pgdata:/var/lib/postgresql/data:Z
Volume=/srv/backup:/backup:ro
`

// allowTargetRead is the access store answer for a caller holding read
// on the target host — the minimum the preview requires.
func allowTargetRead(hostID, userID string) (string, error) {
	if hostID == "host-b" {
		return models.PermissionRead, nil
	}
	return models.PermissionAdmin, nil
}

func TestQuadletService_MovePreview_ReturnsVolumesCollisionRunning(t *testing.T) {
	service := newMovePreviewTestService(
		movePreviewSourceFixture(movePreviewContent),
		&fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return []*entities.ListQuadlet{}, nil
			},
		},
		&fakeAccessRepo{getPermissionFn: allowTargetRead},
	)

	got, err := service.MovePreview(context.Background(), "user", "host-a", "web.container", "host-b")

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, got.Volumes, 2)
	assert.Equal(t, "pgdata", got.Volumes[0].Source)
	assert.Equal(t, MovePreviewVolumeKindVolume, got.Volumes[0].Kind)
	assert.Equal(t, "/srv/backup", got.Volumes[1].Source)
	assert.Equal(t, MovePreviewVolumeKindHostPath, got.Volumes[1].Kind)
	assert.False(t, got.Collision, "an empty target must not collide")
	assert.False(t, got.Running, "a stopped source quadlet is not running")
}

func TestQuadletService_MovePreview_VolumeKinds(t *testing.T) {
	// The issue-214 enrichment is flag independent: volume-kind entries
	// always resolve their podman volume, and with no source volume behind
	// them (the fake inspects nothing) they answer dangling — migratable
	// false with the reason. Host paths stay out of the volume world and
	// carry the host-path reason instead.
	tests := []struct {
		name    string
		content string
		want    []QuadletMovePreviewVolume
	}{
		{
			name:    "named volume with mount point and options",
			content: "[Container]\nVolume=pgdata:/var/lib/pg:Z\n",
			want:    []QuadletMovePreviewVolume{{Source: "pgdata", Kind: MovePreviewVolumeKindVolume, PodmanVolume: "pgdata", Reason: MovePreviewReasonDangling}},
		},
		{
			name:    "host path with mount point",
			content: "[Container]\nVolume=/srv/data:/data:ro\n",
			want:    []QuadletMovePreviewVolume{{Source: "/srv/data", Kind: MovePreviewVolumeKindHostPath, Reason: MovePreviewReasonHostPath}},
		},
		{
			name:    "bare named volume without mount point",
			content: "[Container]\nVolume=mydata\n",
			want:    []QuadletMovePreviewVolume{{Source: "mydata", Kind: MovePreviewVolumeKindVolume, PodmanVolume: "mydata", Reason: MovePreviewReasonDangling}},
		},
		{
			name:    "bare host path without mount point",
			content: "[Container]\nVolume=/srv/data\n",
			want:    []QuadletMovePreviewVolume{{Source: "/srv/data", Kind: MovePreviewVolumeKindHostPath, Reason: MovePreviewReasonHostPath}},
		},
		{
			name: "every Volume= line counts, in file order, mixed kinds",
			content: "[Container]\nImage=busybox\n" +
				"Volume=alpha:/a\n" +
				"Volume=/host/b:/b\n" +
				"Volume=gamma\n",
			want: []QuadletMovePreviewVolume{
				{Source: "alpha", Kind: MovePreviewVolumeKindVolume, PodmanVolume: "alpha", Reason: MovePreviewReasonDangling},
				{Source: "/host/b", Kind: MovePreviewVolumeKindHostPath, Reason: MovePreviewReasonHostPath},
				{Source: "gamma", Kind: MovePreviewVolumeKindVolume, PodmanVolume: "gamma", Reason: MovePreviewReasonDangling},
			},
		},
		{
			name:    "comments and other keys are ignored, key match is case-insensitive",
			content: "[Container]\n# Volume=commented:/out\n; Volume=also-commented:/out\nImage=busybox\nvolume=lowercase\nNetwork=host\n",
			want:    []QuadletMovePreviewVolume{{Source: "lowercase", Kind: MovePreviewVolumeKindVolume, PodmanVolume: "lowercase", Reason: MovePreviewReasonDangling}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newMovePreviewTestService(
				movePreviewSourceFixture(tt.content),
				&fakeQuadletEngine{listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
					return nil, nil
				}},
				&fakeAccessRepo{getPermissionFn: allowTargetRead},
			)

			got, err := service.MovePreview(context.Background(), "user", "host-a", "web.container", "host-b")

			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Volumes)
		})
	}
}

func TestQuadletService_MovePreview_NoVolumesIsEmptyList(t *testing.T) {
	service := newMovePreviewTestService(
		movePreviewSourceFixture("[Container]\nImage=busybox\n"),
		&fakeQuadletEngine{listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		}},
		&fakeAccessRepo{getPermissionFn: allowTargetRead},
	)

	got, err := service.MovePreview(context.Background(), "user", "host-a", "web.container", "host-b")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.NotNil(t, got.Volumes, "a quadlet without Volume= lines must report an empty list, not null")
	assert.Empty(t, got.Volumes)
}

func TestQuadletService_MovePreview_CollisionRequiresNameAndResource(t *testing.T) {
	tests := []struct {
		name        string
		targetFiles []*entities.ListQuadlet
		want        bool
	}{
		{
			name:        "same name and resource collides",
			targetFiles: []*entities.ListQuadlet{{Name: "web.container", Path: "/other/root/web.container", Status: "stopped"}},
			want:        true,
		},
		{
			name:        "same base name but different resource does not collide",
			targetFiles: []*entities.ListQuadlet{{Name: "web.pod", Path: "/x/web.pod", Status: "stopped"}},
			want:        false,
		},
		{
			name:        "different name does not collide",
			targetFiles: []*entities.ListQuadlet{{Name: "other.container", Path: "/x/other.container", Status: "stopped"}},
			want:        false,
		},
		{
			name:        "empty target does not collide",
			targetFiles: nil,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newMovePreviewTestService(
				movePreviewSourceFixture("[Container]\nImage=busybox\n"),
				&fakeQuadletEngine{listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
					return tt.targetFiles, nil
				}},
				&fakeAccessRepo{getPermissionFn: allowTargetRead},
			)

			got, err := service.MovePreview(context.Background(), "user", "host-a", "web.container", "host-b")

			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Collision)
		})
	}
}

func TestQuadletService_MovePreview_RunningReflectsSourceState(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{name: "running source", status: "running", want: true},
		{name: "stopped source", status: "stopped", want: false},
		{name: "unknown source", status: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceEngine := &fakeQuadletEngine{
				printFn: func(string) (string, error) { return "[Container]\nImage=busybox\n", nil },
				listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
					return []*entities.ListQuadlet{
						{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: tt.status},
					}, nil
				},
			}
			service := newMovePreviewTestService(
				sourceEngine,
				&fakeQuadletEngine{listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
					return nil, nil
				}},
				&fakeAccessRepo{getPermissionFn: allowTargetRead},
			)

			got, err := service.MovePreview(context.Background(), "user", "host-a", "web.container", "host-b")

			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Running)
		})
	}
}

func TestQuadletService_MovePreview_NoTargetAccessIsAccessDenied(t *testing.T) {
	// Only the source host's engine is armed: a preview that crossed the
	// target's engine despite the missing grant would panic the fake and
	// fail the test — the "no false preview" guarantee (issue 209).
	service := newMovePreviewTestService(
		movePreviewSourceFixture("[Container]\nImage=busybox\n"),
		nil,
		&fakeAccessRepo{getPermissionFn: func(string, string) (string, error) {
			// No access row for the caller anywhere.
			return "", nil
		}},
	)

	got, err := service.MovePreview(context.Background(), "user", "host-a", "web.container", "host-b")

	require.ErrorIs(t, err, appErrors.ErrAccessDenied)
	assert.Nil(t, got)
}

func TestQuadletService_MovePreview_BelowReadTargetAccessIsAccessDenied(t *testing.T) {
	service := newMovePreviewTestService(
		movePreviewSourceFixture("[Container]\nImage=busybox\n"),
		nil,
		&fakeAccessRepo{getPermissionFn: func(hostID, _ string) (string, error) {
			if hostID == "host-b" {
				// A malformed/stale level answers as below-read, like
				// models.HasAtLeast treats any unknown level.
				return "nonsense-level", nil
			}
			return models.PermissionAdmin, nil
		}},
	)

	got, err := service.MovePreview(context.Background(), "user", "host-a", "web.container", "host-b")

	require.ErrorIs(t, err, appErrors.ErrAccessDenied)
	assert.Nil(t, got)
}

func TestQuadletService_MovePreview_AccessLookupFailureIsInternalError(t *testing.T) {
	service := newMovePreviewTestService(
		movePreviewSourceFixture("[Container]\nImage=busybox\n"),
		nil,
		&fakeAccessRepo{getPermissionFn: func(string, string) (string, error) {
			return "", errors.New("db down")
		}},
	)

	got, err := service.MovePreview(context.Background(), "user", "host-a", "web.container", "host-b")

	require.ErrorIs(t, err, appErrors.ErrInternalError)
	assert.Nil(t, got)
}

func TestQuadletService_MovePreview_MissingSourceQuadletIs404(t *testing.T) {
	emptyEngine := &fakeQuadletEngine{
		printFn: func(string) (string, error) { return "", nil },
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
	}
	service := newMovePreviewTestService(
		emptyEngine,
		&fakeQuadletEngine{listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			panic("target engine must not be reached when the source quadlet is missing")
		}},
		&fakeAccessRepo{getPermissionFn: allowTargetRead},
	)

	got, err := service.MovePreview(context.Background(), "user", "host-a", "gone.container", "host-b")

	require.ErrorIs(t, err, appErrors.ErrQuadletNotFound)
	assert.Nil(t, got)
}

// --- The transfer (copy mode, issue 210) ---

// newMoveTransferTestServices builds both quadlet modules over per-host
// fake engines and the given access store, with an SSH exec and host
// lookup wired on the lean service for the post-install manager reload the
// transfer performs on the target and the move-mode stops — both cross
// the lifecycle seam, so the SSH-exec mock pins keep crossing both modules.
func newMoveTransferTestServices(sourceEngine, targetEngine *fakeQuadletEngine, access *fakeAccessRepo, sshExec sshexec.SSHExecInterface) (*QuadletService, *QuadletTransferService) {
	provider := fakeEngineProvider{engines: map[string]QuadletEngine{
		"host-a": sourceEngine,
		"host-b": targetEngine,
	}}
	service := NewQuadletService(provider, sshExec, lookupForUser("core"))
	return service, NewQuadletTransferService(provider, nil, access, 0, service)
}

// newMoveTransferTestService builds the transfer service over per-host
// fake engines and the given access store, with the real lean service as
// its seam — the shape the transfer suites drive.
func newMoveTransferTestService(sourceEngine, targetEngine *fakeQuadletEngine, access *fakeAccessRepo, sshExec sshexec.SSHExecInterface) *QuadletTransferService {
	_, transfer := newMoveTransferTestServices(sourceEngine, targetEngine, access, sshExec)
	return transfer
}

// moveSourceFixture is the source lease's answer: one quadlet with printable
// content. Unarmed operations (Remove above all) panic, so a copy that
// mutated the source fails the test loudly.
func moveSourceFixture(name, path, status, content string) *fakeQuadletEngine {
	return &fakeQuadletEngine{
		printFn: func(string) (string, error) {
			return content, nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: name, UnitName: "web.service", Path: path, Status: status},
			}, nil
		},
	}
}

// emptyTargetEngine is a target lease that holds nothing and installs
// whatever it is asked to, recording the install.
func emptyTargetEngine(t *testing.T) (*fakeQuadletEngine, *string, *string, *bool) {
	t.Helper()
	var gotFile, gotContent string
	var gotReplace bool
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
		installFn: func(fileName string, content []byte, replace bool) (*entities.QuadletInstallReport, error) {
			gotFile, gotContent, gotReplace = fileName, string(content), replace
			return &entities.QuadletInstallReport{
				InstalledQuadlets: map[string]string{"/tmp/web.container": "/etc/containers/systemd/web.container"},
			}, nil
		},
	}
	return engine, &gotFile, &gotContent, &gotReplace
}

// allowTargetWrite is the access store answer for a caller holding write on
// the target host — the minimum the transfer requires.
func allowTargetWrite(hostID, userID string) (string, error) {
	if hostID == "host-b" {
		return models.PermissionWrite, nil
	}
	return models.PermissionAdmin, nil
}

// moveCopyRequest is the body of a copy transfer to host-b.
func moveCopyRequest() QuadletMoveRequest {
	return QuadletMoveRequest{TargetHostID: "host-b", Mode: "copy"}
}

const moveTransferContent = `[Container]
Image=busybox
Exec=sh -c "sleep 3600"
`

func TestQuadletService_Move_CopyInstallsIdenticalFileOnTarget(t *testing.T) {
	target, gotFile, gotContent, gotReplace := emptyTargetEngine(t)
	service := newMoveTransferTestService(
		moveSourceFixture("web.container", "/x/web.container", "stopped", moveTransferContent),
		target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
	)

	got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveCopyRequest())

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "web.container", *gotFile, "the copy must install under the source's file name verbatim")
	assert.Equal(t, moveTransferContent, *gotContent, "the copy must install the source's content verbatim")
	assert.False(t, *gotReplace, "the copy must not overwrite an existing target file")
	assert.Equal(t, "web.container", got.ID)
	assert.Equal(t, "web.container", got.Name)
	assert.Equal(t, "container", got.Resource)
	assert.Equal(t, MoveStatusCopied, got.Status)
	assert.Equal(t, "host-a", got.SourceHostID)
	assert.Equal(t, "host-b", got.TargetHostID)
	assert.Equal(t, "/etc/containers/systemd/web.container", got.Path)
}

// TestQuadletService_Move_CopyReloadsOnlyTheTargetManager pins where the
// post-install reload lands: exactly one daemon-reload, against the TARGET
// host — the copy leaves the source manager (and everything else on the
// source) untouched.
func TestQuadletService_Move_CopyReloadsOnlyTheTargetManager(t *testing.T) {
	target, _, _, _ := emptyTargetEngine(t)
	sshExec := testutils.NewMockSSHExec()
	service := newMoveTransferTestService(
		moveSourceFixture("web.container", "/x/web.container", "stopped", moveTransferContent),
		target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		sshExec,
	)

	_, err := service.Move(context.Background(), "user", "host-a", "web.container", moveCopyRequest())

	require.NoError(t, err)
	calls := sshExec.Calls()
	require.Len(t, calls, 1, "a successful copy must reload exactly one manager")
	assert.Equal(t, "host-b", calls[0].HostID, "the reload must target the target host")
	assert.Equal(t, "systemctl", calls[0].Command)
	assert.Equal(t, []string{"--user", "daemon-reload"}, calls[0].Opts.Args)
}

// TestQuadletService_Move_CopyOfRunningQuadletNeedsNoStop pins copy's
// safety contract: a RUNNING source quadlet copies successfully, and the
// only SSH traffic is the target's daemon-reload — no stop, no force, no
// lifecycle action of any kind.
func TestQuadletService_Move_CopyOfRunningQuadletNeedsNoStop(t *testing.T) {
	target, _, _, _ := emptyTargetEngine(t)
	sshExec := testutils.NewMockSSHExec()
	service := newMoveTransferTestService(
		moveSourceFixture("web.container", "/x/web.container", "running", moveTransferContent),
		target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		sshExec,
	)

	got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveCopyRequest())

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, MoveStatusCopied, got.Status)
	for _, call := range sshExec.Calls() {
		assert.Equal(t, "systemctl", call.Command)
		assert.Equal(t, []string{"--user", "daemon-reload"}, call.Opts.Args,
			"copy must not stop or otherwise act on the source unit")
	}
}

func TestQuadletService_Move_CollisionOnTargetInstallsNothing(t *testing.T) {
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", Path: "/y/web.container", Status: "stopped"},
			}, nil
		},
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			panic("a colliding copy must not install")
		},
	}
	service := newMoveTransferTestService(
		moveSourceFixture("web.container", "/x/web.container", "stopped", moveTransferContent),
		target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
	)

	got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveCopyRequest())

	require.ErrorIs(t, err, appErrors.ErrQuadletAlreadyExists)
	assert.Nil(t, got)
}

func TestQuadletService_Move_SameNameDifferentResourceOnTargetCopies(t *testing.T) {
	// The collision contract is name AND resource: a target file with the
	// same base name but a different resource extension is no collision.
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.pod", Path: "/y/web.pod", Status: "stopped"},
			}, nil
		},
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return &entities.QuadletInstallReport{}, nil
		},
	}
	service := newMoveTransferTestService(
		moveSourceFixture("web.container", "/x/web.container", "stopped", moveTransferContent),
		target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
	)

	got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveCopyRequest())

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, MoveStatusCopied, got.Status)
}

func TestQuadletService_Move_AllFiveResourceTypesCopy(t *testing.T) {
	tests := []struct {
		resource string
		path     string
	}{
		{resource: "container", path: "/x/web.container"},
		{resource: "pod", path: "/x/web.pod"},
		{resource: "image", path: "/x/web.image"},
		{resource: "volume", path: "/x/web.volume"},
		{resource: "network", path: "/x/web.network"},
	}

	for _, tt := range tests {
		t.Run(tt.resource, func(t *testing.T) {
			target, gotFile, _, _ := emptyTargetEngine(t)
			service := newMoveTransferTestService(
				moveSourceFixture("web."+tt.resource, tt.path, "stopped", moveTransferContent),
				target,
				&fakeAccessRepo{getPermissionFn: allowTargetWrite},
				testutils.NewMockSSHExec(),
			)

			got, err := service.Move(context.Background(), "user", "host-a", "web."+tt.resource, moveCopyRequest())

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, "web."+tt.resource, *gotFile, "the file must keep its resource extension")
			assert.Equal(t, tt.resource, got.Resource)
			assert.Equal(t, MoveStatusCopied, got.Status)
		})
	}
}

func TestQuadletService_Move_TargetBelowWriteAccessIsAccessDenied(t *testing.T) {
	tests := []struct {
		name   string
		answer string
	}{
		{name: "read-level grant on the target", answer: models.PermissionRead},
		{name: "no access row for the caller", answer: ""},
		{name: "malformed stored level", answer: "nonsense-level"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Only the gate is exercised: the source engine panics on any
			// call, proving the refusal fires before any podman work (the
			// "no install attempt" guarantee, issue 210).
			service := newMoveTransferTestService(
				&fakeQuadletEngine{},
				nil,
				&fakeAccessRepo{getPermissionFn: func(hostID, _ string) (string, error) {
					if hostID == "host-b" {
						return tt.answer, nil
					}
					return models.PermissionAdmin, nil
				}},
				testutils.NewMockSSHExec(),
			)

			got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveCopyRequest())

			require.ErrorIs(t, err, appErrors.ErrAccessDenied)
			assert.Nil(t, got)
		})
	}
}

func TestQuadletService_Move_AccessLookupFailureIsInternalError(t *testing.T) {
	service := newMoveTransferTestService(
		&fakeQuadletEngine{},
		nil,
		&fakeAccessRepo{getPermissionFn: func(string, string) (string, error) {
			return "", errors.New("db down")
		}},
		testutils.NewMockSSHExec(),
	)

	got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveCopyRequest())

	require.ErrorIs(t, err, appErrors.ErrInternalError)
	assert.Nil(t, got)
}

func TestQuadletService_Move_MissingSourceQuadletIs404(t *testing.T) {
	emptyEngine := &fakeQuadletEngine{
		printFn: func(string) (string, error) { return "", nil },
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
	}
	service := newMoveTransferTestService(
		emptyEngine,
		&fakeQuadletEngine{listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			panic("target engine must not be reached when the source quadlet is missing")
		}},
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
	)

	got, err := service.Move(context.Background(), "user", "host-a", "gone.container", moveCopyRequest())

	require.ErrorIs(t, err, appErrors.ErrQuadletNotFound)
	assert.Nil(t, got)
}

// TestQuadletService_Move_InstallRefusalMapsAlreadyExists pins the
// execution-time collision re-check: the pre-install list check and the
// install can race, so podman's own refusal to overwrite must surface as
// the same 409 the list check answers.
func TestQuadletService_Move_InstallRefusalMapsAlreadyExists(t *testing.T) {
	tests := []struct {
		name      string
		installFn func(string, []byte, bool) (*entities.QuadletInstallReport, error)
	}{
		{
			name: "transport-level refusal",
			installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
				return nil, &errorhandling.ErrorModel{
					ResponseCode: http.StatusInternalServerError,
					Message:      "web.container: conflict",
					Because:      "name in use",
				}
			},
		},
		{
			name: "per-file refusal in the install report",
			installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
				return &entities.QuadletInstallReport{
					QuadletErrors: map[string]error{
						"web.container": errors.New("web.container: already exists"),
					},
				}, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := &fakeQuadletEngine{
				listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
					return nil, nil
				},
				installFn: tt.installFn,
			}
			service := newMoveTransferTestService(
				moveSourceFixture("web.container", "/x/web.container", "stopped", moveTransferContent),
				target,
				&fakeAccessRepo{getPermissionFn: allowTargetWrite},
				testutils.NewMockSSHExec(),
			)

			got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveCopyRequest())

			require.ErrorIs(t, err, appErrors.ErrQuadletAlreadyExists)
			assert.Nil(t, got)
		})
	}
}

// TestQuadletService_Move_ReloadFailureFailsCopy pins the same contract
// Create pins: the target file is installed, but a manager that never
// reloaded cannot start it, so the copy fails with CONNECTION_FAILED rather
// than pretending success.
func TestQuadletService_Move_ReloadFailureFailsCopy(t *testing.T) {
	target, _, _, _ := emptyTargetEngine(t)
	service := newMoveTransferTestService(
		moveSourceFixture("web.container", "/x/web.container", "stopped", moveTransferContent),
		target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExecWithResult(sshexec.ExecResult{
			ExitCode: 1,
			Stderr:   "Failed to connect to bus: No such file or directory",
		}),
	)

	got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveCopyRequest())

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	assert.Nil(t, got)
}

// --- Move mode (issue 211) ---

// recordingTargetEngine is a target lease that holds nothing and installs
// whatever it is asked to, appending "install" to the shared op sequence —
// the ordering probe the move-mode removal tests assert against.
func recordingTargetEngine(t *testing.T, sequence *[]string) *fakeQuadletEngine {
	t.Helper()
	return &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			*sequence = append(*sequence, "install")
			return &entities.QuadletInstallReport{
				InstalledQuadlets: map[string]string{"/tmp/web.container": "/etc/containers/systemd/web.container"},
			}, nil
		},
	}
}

// recordingSourceFixture is the source lease's answer: a quadlet with
// printable content and an armed remove that records the force flag and
// appends "remove" to the shared op sequence.
func recordingSourceFixture(status string, sequence *[]string, gotForce *bool) *fakeQuadletEngine {
	return &fakeQuadletEngine{
		printFn: func(string) (string, error) {
			return moveTransferContent, nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: status},
			}, nil
		},
		removeFn: func(_ string, force bool) error {
			*gotForce = force
			*sequence = append(*sequence, "remove")
			return nil
		},
	}
}

// moveModeRequest is the body of a move-mode transfer to host-b.
func moveModeRequest() QuadletMoveRequest {
	return QuadletMoveRequest{TargetHostID: "host-b", Mode: MoveModeMove}
}

// TestQuadletService_Move_MoveRemovesSourceOnlyAfterInstall pins the AC at
// the heart of move mode: the source remove happens, and it happens AFTER
// the target install — the shared op sequence is the probe.
func TestQuadletService_Move_MoveRemovesSourceOnlyAfterInstall(t *testing.T) {
	sequence := []string{}
	var gotForce bool
	source := recordingSourceFixture("stopped", &sequence, &gotForce)
	target := recordingTargetEngine(t, &sequence)
	service := newMoveTransferTestService(
		source, target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
	)

	got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveModeRequest())

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []string{"install", "remove"}, sequence,
		"the source remove must follow the target install")
	assert.False(t, gotForce, "without force the remove must not force over a live resource")
	assert.Equal(t, MoveStatusMoved, got.Status)
	assert.Empty(t, got.Error, "a completed move carries no error detail")
	assert.Equal(t, "web.container", got.ID)
	assert.Equal(t, "container", got.Resource)
	assert.Equal(t, "host-a", got.SourceHostID)
	assert.Equal(t, "host-b", got.TargetHostID)
}

// TestQuadletService_Move_RunningSourceWithoutStopOrForceInstallsNothing
// pins the delete-precondition carry-over: a running source quadlet refuses
// the move unless stop or force is set — and refuses it BEFORE anything is
// installed.
func TestQuadletService_Move_RunningSourceWithoutStopOrForceInstallsNothing(t *testing.T) {
	sequence := []string{}
	var gotForce bool
	source := recordingSourceFixture("running", &sequence, &gotForce)
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			panic("a refused move must not install on the target")
		},
	}
	service := newMoveTransferTestService(
		source, target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
	)

	got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveModeRequest())

	require.ErrorIs(t, err, appErrors.ErrQuadletResourceRunning)
	assert.Nil(t, got)
	assert.Empty(t, sequence, "a refused move must not reach the source removal either")
}

// TestQuadletService_Move_StopStopsThenMoves pins the stop flag: a running
// source quadlet is stopped through the lifecycle seam BEFORE the remove,
// and the only other SSH traffic is the target's daemon-reload.
func TestQuadletService_Move_StopStopsThenMoves(t *testing.T) {
	sequence := []string{}
	sshCallsAtRemove := -1
	var gotForce bool
	sshExec := testutils.NewMockSSHExec()
	source := &fakeQuadletEngine{
		printFn: func(string) (string, error) {
			return moveTransferContent, nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "running"},
			}, nil
		},
		removeFn: func(_ string, force bool) error {
			sshCallsAtRemove = len(sshExec.Calls())
			gotForce = force
			sequence = append(sequence, "remove")
			return nil
		},
	}
	target := recordingTargetEngine(t, &sequence)
	service := newMoveTransferTestService(
		source, target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		sshExec,
	)

	req := QuadletMoveRequest{TargetHostID: "host-b", Mode: MoveModeMove, Stop: true}
	got, err := service.Move(context.Background(), "user", "host-a", "web.container", req)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, MoveStatusMoved, got.Status)
	assert.False(t, gotForce)
	require.Len(t, sshExec.Calls(), 2, "a stop-then-move performs exactly the target's reload and the stop")
	assert.Equal(t, "host-b", sshExec.Calls()[0].HostID,
		"the reload completes the target install before the source is touched")
	assert.Equal(t, []string{"--user", "daemon-reload"}, sshExec.Calls()[0].Opts.Args)
	assert.Equal(t, "host-a", sshExec.Calls()[1].HostID)
	assert.Equal(t, "systemctl", sshExec.Calls()[1].Command)
	assert.Equal(t, []string{"--user", "stop", "web.service"}, sshExec.Calls()[1].Opts.Args)
	assert.Equal(t, 2, sshCallsAtRemove, "the stop must precede the remove")
}

// TestQuadletService_Move_CollisionAfterTheStopCarriesTheNote pins the
// annotation on the install phase (issue 207 review): a migrating move
// whose post-stop collision re-check refuses with 409 carries the stop
// note — the source's unit was stopped before the refusal and remains
// stopped — beside the collision code.
func TestQuadletService_Move_CollisionAfterTheStopCarriesTheNote(t *testing.T) {
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
		// The install lease's collision re-check finds the quadlet already
		// on the target — same name, same resource.
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container"},
			}, nil
		},
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) { return nil, nil },
		volumeCreateFn:  func(string) error { return nil },
		volumeImportFn: func(_ string, r io.Reader) error {
			_, err := io.Copy(io.Discard, r)
			return err
		},
	}
	service := newMoveTransferTestService(
		source, target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		sshExec,
	)
	req := QuadletMoveRequest{TargetHostID: "host-b", Mode: MoveModeMove, Stop: true, MigrateVolumeData: true}

	got, err := service.Move(context.Background(), "user", "host-a", "web.container", req)

	require.Nil(t, got)
	appErr, ok := errors.AsType[*appErrors.AppError](err)
	require.True(t, ok, "the refusal is an AppError, got: %v", err)
	assert.Equal(t, appErrors.ErrQuadletAlreadyExists.Code, appErr.Code)
	assert.Equal(t, http.StatusConflict, appErr.Status, "the collision code survives the annotation")
	assert.Contains(t, appErr.Message, "the source quadlet was stopped before this failure and remains stopped")
	require.Len(t, sshExec.Calls(), 1, "exactly the pre-data stop — the refusal ended the transfer")
	assert.Equal(t, []string{"--user", "stop", "web.service"}, sshExec.Calls()[0].Opts.Args)
}

// TestQuadletService_Move_ForcePassesThroughToRemoval pins the force flag:
// no stop through the lifecycle seam, and the remove itself receives
// force=true — the delete flow's force over a running resource.
func TestQuadletService_Move_ForcePassesThroughToRemoval(t *testing.T) {
	sequence := []string{}
	var gotForce bool
	source := recordingSourceFixture("running", &sequence, &gotForce)
	sshExec := testutils.NewMockSSHExec()
	target := recordingTargetEngine(t, &sequence)
	service := newMoveTransferTestService(
		source, target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		sshExec,
	)

	req := QuadletMoveRequest{TargetHostID: "host-b", Mode: MoveModeMove, Force: true}
	got, err := service.Move(context.Background(), "user", "host-a", "web.container", req)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, MoveStatusMoved, got.Status)
	assert.True(t, gotForce, "force must pass through to the removal like the delete flow")
	require.Len(t, sshExec.Calls(), 1, "a forced move performs no stop — only the target's reload")
	assert.Equal(t, "host-b", sshExec.Calls()[0].HostID)
	assert.Equal(t, []string{"--user", "daemon-reload"}, sshExec.Calls()[0].Opts.Args)
}

// TestQuadletService_Move_InstallFailureLeavesSourceUntouched pins the
// plain-error half of the failure contract: an install failure answers a
// plain error and the source is never removed.
func TestQuadletService_Move_InstallFailureLeavesSourceUntouched(t *testing.T) {
	source := &fakeQuadletEngine{
		printFn: func(string) (string, error) {
			return moveTransferContent, nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"},
			}, nil
		},
		removeFn: func(string, bool) error {
			panic("a failed install must leave the source untouched")
		},
	}
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return nil, errors.New("target disk full")
		},
	}
	service := newMoveTransferTestService(
		source, target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
	)

	got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveModeRequest())

	require.Error(t, err)
	assert.Nil(t, got, "an install failure answers a plain error, not the transfer envelope")
}

// TestQuadletService_Move_RemoveFailureAfterInstallReturnsPartial pins the
// partial envelope: a removal failure after a successful install answers
// the transfer envelope with status "partial" plus error detail, and the
// target install stays intact.
func TestQuadletService_Move_RemoveFailureAfterInstallReturnsPartial(t *testing.T) {
	sequence := []string{}
	source := &fakeQuadletEngine{
		printFn: func(string) (string, error) {
			return moveTransferContent, nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"},
			}, nil
		},
		removeFn: func(string, bool) error {
			sequence = append(sequence, "remove")
			return errors.New("source read-only")
		},
	}
	target := recordingTargetEngine(t, &sequence)
	service := newMoveTransferTestService(
		source, target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
	)

	got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveModeRequest())

	require.NoError(t, err, "a partial transfer is a success-shaped envelope, not an error")
	require.NotNil(t, got)
	assert.Equal(t, []string{"install", "remove"}, sequence)
	assert.Equal(t, MoveStatusPartial, got.Status)
	assert.NotEmpty(t, got.Error, "partial must carry the removal failure's detail")
	assert.Contains(t, got.Error, "source read-only")
	assert.Equal(t, "/etc/containers/systemd/web.container", got.Path,
		"the target install remains intact and reported")
}

// TestQuadletService_Move_SourceVanishedBeforeRemovalIsMoved pins the
// race answer: when the source quadlet vanished between the install and
// the removal, the transfer's goal is already met and the status is
// "moved" — never a false "partial" claiming the quadlet exists on both
// hosts.
func TestQuadletService_Move_SourceVanishedBeforeRemovalIsMoved(t *testing.T) {
	sequence := []string{}
	listCalls := 0
	source := &fakeQuadletEngine{
		printFn: func(string) (string, error) {
			return moveTransferContent, nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			listCalls++
			if listCalls == 1 {
				return []*entities.ListQuadlet{
					{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"},
				}, nil
			}
			return nil, nil
		},
		removeFn: func(string, bool) error {
			panic("a vanished source quadlet needs no removal")
		},
	}
	target := recordingTargetEngine(t, &sequence)
	service := newMoveTransferTestService(
		source, target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
	)

	got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveModeRequest())

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, MoveStatusMoved, got.Status)
	assert.Empty(t, got.Error)
	assert.Equal(t, []string{"install"}, sequence)
}

// TestQuadletService_Move_CollisionOnTargetRemovesNothing extends the copy
// collision contract to move mode: the 409 fires before any removal, so the
// source is untouched.
func TestQuadletService_Move_CollisionOnTargetRemovesNothing(t *testing.T) {
	source := &fakeQuadletEngine{
		printFn: func(string) (string, error) {
			return moveTransferContent, nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"},
			}, nil
		},
		removeFn: func(string, bool) error {
			panic("a colliding move must not remove the source")
		},
	}
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", Path: "/y/web.container", Status: "stopped"},
			}, nil
		},
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			panic("a colliding move must not install")
		},
	}
	service := newMoveTransferTestService(
		source, target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
	)

	got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveModeRequest())

	require.ErrorIs(t, err, appErrors.ErrQuadletAlreadyExists)
	assert.Nil(t, got)
}

// TestQuadletService_Move_CopyIgnoresStopAndForce pins that the flags are
// move-mode vocabulary: a copy with stop/force set neither stops nor
// removes anything on the source.
func TestQuadletService_Move_CopyIgnoresStopAndForce(t *testing.T) {
	sequence := []string{}
	source := &fakeQuadletEngine{
		printFn: func(string) (string, error) {
			return moveTransferContent, nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "running"},
			}, nil
		},
		removeFn: func(string, bool) error {
			panic("a copy must never remove the source")
		},
	}
	sshExec := testutils.NewMockSSHExec()
	target := recordingTargetEngine(t, &sequence)
	service := newMoveTransferTestService(
		source, target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		sshExec,
	)

	req := QuadletMoveRequest{TargetHostID: "host-b", Mode: MoveModeCopy, Stop: true, Force: true}
	got, err := service.Move(context.Background(), "user", "host-a", "web.container", req)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, MoveStatusCopied, got.Status)
	assert.Equal(t, []string{"install"}, sequence)
	require.Len(t, sshExec.Calls(), 1, "copy performs no stop — only the target's reload")
	assert.Equal(t, []string{"--user", "daemon-reload"}, sshExec.Calls()[0].Opts.Args)
}

// --- Companion .volume quadlets (issue 217) ---

// companionContent is a main quadlet whose Volume= source names a .volume
// quadlet file — the suffix form podman resolves to the same-named unit
// file.
const companionContent = `[Container]
Image=busybox
Exec=sh -c "sleep 3600"
Volume=app-data.volume:/var/lib/app:Z
`

// appDataVolumeContent is the companion .volume quadlet's content on the
// source host.
const appDataVolumeContent = `[Volume]
Driver=local
Label=org.example.kind=app-data
`

// companionSourceFixture is the source lease's answer for companion
// scenarios: the main container quadlet plus the app-data.volume companion
// it references, both printable.
func companionSourceFixture(mainContent string) *fakeQuadletEngine {
	return &fakeQuadletEngine{
		printFn: func(name string) (string, error) {
			switch name {
			case "web.container":
				return mainContent, nil
			case "app-data.volume":
				return appDataVolumeContent, nil
			}
			panic("unexpected QuadletEngine.Print call: " + name)
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"},
				{Name: "app-data.volume", Path: "/x/app-data.volume", Status: "stopped"},
			}, nil
		},
	}
}

// targetEngineHoldingCompanion is a target lease that already holds the
// companion .volume quadlet with the given printed content.
func targetEngineHoldingCompanion(companionContent string, installs *[]string) *fakeQuadletEngine {
	return &fakeQuadletEngine{
		printFn: func(name string) (string, error) {
			if name != "app-data.volume" {
				panic("unexpected QuadletEngine.Print call: " + name)
			}
			return companionContent, nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "app-data.volume", Path: "/y/app-data.volume", Status: "stopped"},
			}, nil
		},
		installFn: func(fileName string, content []byte, replace bool) (*entities.QuadletInstallReport, error) {
			if installs != nil {
				*installs = append(*installs, fileName)
			}
			return &entities.QuadletInstallReport{}, nil
		},
	}
}

func TestQuadletService_MovePreview_CompanionFieldsPerTargetState(t *testing.T) {
	tests := []struct {
		name            string
		targetCompanion *string // nil: absent; else its printed content on the target
		wantTargetState string
		wantQuadletFile string
	}{
		{
			name:            "no companion on target is absent",
			targetCompanion: nil,
			wantTargetState: MovePreviewCompanionAbsent,
			wantQuadletFile: "app-data.volume",
		},
		{
			name:            "byte-identical companion on target is identical",
			targetCompanion: ptr(appDataVolumeContent),
			wantTargetState: MovePreviewCompanionIdentical,
			wantQuadletFile: "app-data.volume",
		},
		{
			name:            "differing companion on target is differs",
			targetCompanion: ptr("[Volume]\nDriver=local\n"),
			wantTargetState: MovePreviewCompanionDiffers,
			wantQuadletFile: "app-data.volume",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var target *fakeQuadletEngine
			if tt.targetCompanion == nil {
				// Absent: the target's list holds nothing, so the
				// companion's file is never printed (podman's print
				// answers empty, not an error, for an unknown file).
				target = &fakeQuadletEngine{
					printFn: func(string) (string, error) {
						panic("an unlisted companion must not be printed")
					},
					listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
						return nil, nil
					},
				}
			} else {
				target = &fakeQuadletEngine{
					printFn: func(name string) (string, error) {
						return *tt.targetCompanion, nil
					},
					listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
						return []*entities.ListQuadlet{
							{Name: "app-data.volume", Path: "/y/app-data.volume", Status: "stopped"},
						}, nil
					},
				}
			}

			service := newMovePreviewTestService(
				companionSourceFixture(companionContent),
				target,
				&fakeAccessRepo{getPermissionFn: allowTargetRead},
			)

			got, err := service.MovePreview(context.Background(), "user", "host-a", "web.container", "host-b")

			require.NoError(t, err)
			require.Len(t, got.Volumes, 1)
			assert.Equal(t, "app-data.volume", got.Volumes[0].Source)
			assert.Equal(t, MovePreviewVolumeKindVolume, got.Volumes[0].Kind)
			assert.Equal(t, tt.wantQuadletFile, got.Volumes[0].QuadletFile,
				"a suffix reference with a matching source quadlet names the companion file")
			assert.Equal(t, tt.wantTargetState, got.Volumes[0].TargetState)
		})
	}
}

// TestQuadletService_MovePreview_BareAndDanglingReferencesStayOrdinary
// pins the discovery rule's negative space: a bare volume name (podman's
// generated volume, not a file reference) and a .volume reference with no
// matching source file both surface as ordinary entries — no companion
// fields.
func TestQuadletService_MovePreview_BareAndDanglingReferencesStayOrdinary(t *testing.T) {
	content := `[Container]
Image=busybox
Volume=plain-data:/var/lib/plain
Volume=missing.volume:/var/lib/gone
Volume=/srv/host.volume:/host
`
	source := &fakeQuadletEngine{
		printFn: func(name string) (string, error) {
			if name != "web.container" {
				panic("a dangling or bare reference must not print anything: " + name)
			}
			return content, nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			// Only the main quadlet: missing.volume does not exist on the
			// source host.
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"},
			}, nil
		},
	}
	service := newMovePreviewTestService(
		source,
		&fakeQuadletEngine{
			printFn: func(string) (string, error) {
				panic("no companion resolution means no target print")
			},
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return nil, nil
			},
		},
		&fakeAccessRepo{getPermissionFn: allowTargetRead},
	)

	got, err := service.MovePreview(context.Background(), "user", "host-a", "web.container", "host-b")

	require.NoError(t, err)
	require.Len(t, got.Volumes, 3)
	for _, volume := range got.Volumes {
		assert.Empty(t, volume.QuadletFile,
			"a bare, dangling, or host-path reference must carry no companion fields: %+v", volume)
		assert.Empty(t, volume.TargetState)
	}
	assert.Equal(t, MovePreviewVolumeKindVolume, got.Volumes[0].Kind)
	assert.Equal(t, MovePreviewVolumeKindVolume, got.Volumes[1].Kind,
		"a dangling suffix reference still labels its kind like any named volume")
	assert.Equal(t, MovePreviewVolumeKindHostPath, got.Volumes[2].Kind,
		"a host path ending in .volume is a host bind path, never a quadlet reference")
}

// TestQuadletService_MovePreview_TwoRefsOneVolumeSharesPodmanVolume pins
// §18's per-reference rows through the service (issue 230): two references
// resolving to one podman volume — here a companion whose VolumeName=
// collides with a bare name — stay two entries sharing podmanVolume, both
// migratable. The same fixture drives the migration suite's one-outcome
// pin. The print count pins the issue-230 fetch discipline: the main
// quadlet and each companion are Printed exactly once per source lease —
// the companion's single print feeds both the companion set and the
// resolution's VolumeName= extraction.
func TestQuadletService_MovePreview_TwoRefsOneVolumeSharesPodmanVolume(t *testing.T) {
	content := `[Container]
Image=busybox
Volume=app-data.volume:/var/lib/app
Volume=custom-name:/var/lib/other
`
	prints := 0
	source := &fakeQuadletEngine{
		printFn: func(name string) (string, error) {
			prints++
			switch name {
			case "web.container":
				return content, nil
			case "app-data.volume":
				return "[Volume]\nVolumeName=custom-name\n", nil
			}
			panic("unexpected QuadletEngine.Print call: " + name)
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"},
				{Name: "app-data.volume", Path: "/x/app-data.volume", Status: "stopped"},
			}, nil
		},
		volumeInspectFn: func(name string) (*QuadletVolumeInspect, error) {
			if name == "custom-name" {
				return &QuadletVolumeInspect{Name: "custom-name"}, nil
			}
			return nil, nil
		},
	}
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
	}
	service := newMovePreviewTestService(source, target, &fakeAccessRepo{getPermissionFn: allowTargetRead})

	got, err := service.MovePreview(context.Background(), "user", "host-a", "web.container", "host-b")

	require.NoError(t, err)
	require.Len(t, got.Volumes, 2, "per-reference rows: two references stay two entries")
	assert.Equal(t, "app-data.volume", got.Volumes[0].Source)
	assert.Equal(t, "custom-name", got.Volumes[0].PodmanVolume)
	assert.Equal(t, "custom-name", got.Volumes[1].PodmanVolume,
		"two references resolving to one volume share podmanVolume")
	assert.True(t, got.Volumes[0].Migratable)
	assert.True(t, got.Volumes[1].Migratable)
	assert.Equal(t, 2, prints,
		"the main quadlet and the companion print once each per source lease (issue 230)")
}

// TestQuadletService_Move_CopyInstallsCompanionsBeforeMain pins the
// dependency-first order: the companion .volume quadlet installs before the
// main quadlet, both verbatim from the source, and the answer stays the
// plain copy envelope.
func TestQuadletService_Move_CopyInstallsCompanionsBeforeMain(t *testing.T) {
	installs := []namedInstall{}
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
		installFn: func(fileName string, content []byte, replace bool) (*entities.QuadletInstallReport, error) {
			installs = append(installs, namedInstall{file: fileName, content: string(content), replace: replace})
			return &entities.QuadletInstallReport{}, nil
		},
	}
	service := newMoveTransferTestService(
		companionSourceFixture(companionContent),
		target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
	)

	got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveCopyRequest())

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, installs, 2, "companion and main quadlet both install")
	assert.Equal(t, "app-data.volume", installs[0].file,
		"the companion must install before the quadlet that references it")
	assert.Equal(t, appDataVolumeContent, installs[0].content)
	assert.Equal(t, "web.container", installs[1].file)
	assert.Equal(t, companionContent, installs[1].content)
	assert.False(t, installs[0].replace, "a companion must not overwrite an existing target file")
	assert.False(t, installs[1].replace)
	assert.Equal(t, MoveStatusCopied, got.Status)
}

// namedInstall records one Install call's arguments.
type namedInstall struct {
	file    string
	content string
	replace bool
}

// TestQuadletService_Move_IdenticalCompanionOnTargetIsSkipped pins the
// idempotence contract: a companion already on the target byte-identical
// installs nothing and raises no error — the transfer proceeds to the main
// quadlet alone.
func TestQuadletService_Move_IdenticalCompanionOnTargetIsSkipped(t *testing.T) {
	installs := []string{}
	target := targetEngineHoldingCompanion(appDataVolumeContent, &installs)
	service := newMoveTransferTestService(
		companionSourceFixture(companionContent),
		target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
	)

	got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveCopyRequest())

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []string{"web.container"}, installs,
		"an identical companion installs nothing; only the main quadlet installs")
	assert.Equal(t, MoveStatusCopied, got.Status)
}

// TestQuadletService_Move_DifferingCompanionOnTargetIs409 pins the refusal:
// a same-name companion with different content fails the transfer with 409
// QUADLET_ALREADY_EXISTS BEFORE the main quadlet installs.
func TestQuadletService_Move_DifferingCompanionOnTargetIs409(t *testing.T) {
	target := targetEngineHoldingCompanion("[Volume]\nDriver=local\n", nil)
	target.installFn = func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
		panic("a differing companion must stop the transfer before anything installs")
	}
	// A move-mode request whose source removal must never fire: the 409
	// precedes it.
	source := companionSourceFixture(companionContent)
	source.removeFn = func(string, bool) error {
		panic("a refused transfer must not remove the source")
	}
	service := newMoveTransferTestService(
		source, target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
	)

	got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveModeRequest())

	require.Error(t, err)
	assert.Nil(t, got)
	appErr, ok := err.(*appErrors.AppError)
	require.True(t, ok, "the refusal is an AppError")
	assert.Equal(t, appErrors.ErrQuadletAlreadyExists.Code, appErr.Code)
	assert.Equal(t, appErrors.ErrQuadletAlreadyExists.Status, appErr.Status)
	assert.Contains(t, appErr.Message, "app-data.volume",
		"the refusal names the companion file")
}

// TestQuadletService_Move_CompanionInstallFailureNamesFile pins the
// mid-set contract: the first failing companion stops the set with an
// error naming the failed file, earlier companions stay installed, the
// main quadlet never installs, and the source is untouched.
func TestQuadletService_Move_CompanionInstallFailureNamesFile(t *testing.T) {
	const twoCompanions = `[Container]
Image=busybox
Volume=alpha.volume:/a
Volume=beta.volume:/b
`
	source := &fakeQuadletEngine{
		printFn: func(name string) (string, error) {
			switch name {
			case "web.container":
				return twoCompanions, nil
			case "alpha.volume", "beta.volume":
				return "[Volume]\nDriver=local\n", nil
			}
			panic("unexpected QuadletEngine.Print call: " + name)
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"},
				{Name: "alpha.volume", Path: "/x/alpha.volume", Status: "stopped"},
				{Name: "beta.volume", Path: "/x/beta.volume", Status: "stopped"},
			}, nil
		},
		removeFn: func(string, bool) error {
			panic("a failed transfer must leave the source untouched")
		},
	}
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
		installFn: func(fileName string, _ []byte, _ bool) (*entities.QuadletInstallReport, error) {
			if fileName == "beta.volume" {
				return nil, errors.New("target disk full")
			}
			return &entities.QuadletInstallReport{}, nil
		},
	}
	service := newMoveTransferTestService(
		source, target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
	)

	got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveModeRequest())

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "beta.volume",
		"the error must name the failed companion file")
	assert.Contains(t, err.Error(), "target disk full")
}

// TestQuadletService_Move_FlagFalseSkipsCompanions pins the opt-out: with
// transferVolumeQuadlets=false the transfer moves only the main quadlet —
// no companion discovery print on the source, no companion install on the
// target.
func TestQuadletService_Move_FlagFalseSkipsCompanions(t *testing.T) {
	installs := []string{}
	source := &fakeQuadletEngine{
		printFn: func(name string) (string, error) {
			if name != "web.container" {
				panic("flag false must not print companions: " + name)
			}
			return companionContent, nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"},
				{Name: "app-data.volume", Path: "/x/app-data.volume", Status: "stopped"},
			}, nil
		},
	}
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
		installFn: func(fileName string, _ []byte, _ bool) (*entities.QuadletInstallReport, error) {
			installs = append(installs, fileName)
			return &entities.QuadletInstallReport{}, nil
		},
	}
	service := newMoveTransferTestService(
		source, target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
	)

	req := QuadletMoveRequest{TargetHostID: "host-b", Mode: MoveModeCopy, TransferVolumeQuadlets: ptr(false)}
	got, err := service.Move(context.Background(), "user", "host-a", "web.container", req)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []string{"web.container"}, installs,
		"transferVolumeQuadlets=false must install the main quadlet only")
	assert.Equal(t, MoveStatusCopied, got.Status)
}

// TestQuadletService_Move_MoveRemovesOnlyMainQuadlet pins that companions
// are copy-only: a move removes the main quadlet from the source and leaves
// the companion .volume file there — a shared definition other source
// quadlets may reference, whose volume data stays behind regardless (#214).
func TestQuadletService_Move_MoveRemovesOnlyMainQuadlet(t *testing.T) {
	removed := []string{}
	installs := []string{}
	source := companionSourceFixture(companionContent)
	source.removeFn = func(name string, _ bool) error {
		removed = append(removed, name)
		return nil
	}
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
		installFn: func(fileName string, _ []byte, _ bool) (*entities.QuadletInstallReport, error) {
			installs = append(installs, fileName)
			return &entities.QuadletInstallReport{}, nil
		},
	}
	service := newMoveTransferTestService(
		source, target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
	)

	got, err := service.Move(context.Background(), "user", "host-a", "web.container", moveModeRequest())

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []string{"app-data.volume", "web.container"}, installs)
	assert.Equal(t, []string{"web.container"}, removed,
		"a move removes only the main quadlet — companions stay on the source")
	assert.Equal(t, MoveStatusMoved, got.Status)
}

// ptr is the literal helper for the pointer-typed request flag and the
// table-driven companion fixtures.
func ptr[T any](v T) *T {
	return &v
}
