package quadlets

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/sshexec"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// The quadlet move/copy secret surface (issue 215): every `Secret=` line
// of the quadlet content counts, whatever the resource type; the preview
// always reports the references and their existence in each machine's
// podman secret store; the transfer carries VALUES only when the request
// opts into transferSecrets — admin on the source, never removed from the
// source, same-name skips keep re-runs safe. Through external behavior
// only: parse results, preview entries, statuses, error codes, and the
// envelope's secretTransfer block — never internal call ordering.

// --- preview ----------------------------------------------------------

// newMoveSecretPreviewTestService builds the preview service with the
// secret provider wired: quadlet engines per machine, the access store,
// and per-machine secret engines.
func newMoveSecretPreviewTestService(sourceEngine, targetEngine *fakeQuadletEngine, access *fakeAccessRepo, secrets map[string]moveSecretEngine) *QuadletService {
	return NewQuadletService(fakeEngineProvider{engines: map[string]QuadletEngine{
		"machine-a": sourceEngine,
		"machine-b": targetEngine,
	}}, nil, nil).
		WithAccessRepo(access).
		WithSecretEngines(fakeSecretEngineProvider{engines: secrets})
}

// secretStoreFixture builds a secret engine holding the given names;
// created records every Create call so tests can pin what landed.
func secretStoreFixture(names ...string) *fakeMoveSecretEngine {
	return &fakeMoveSecretEngine{
		namesFn: func() ([]string, error) {
			return append([]string{}, names...), nil
		},
		revealFn: func(string) (string, error) {
			return "", errors.New("preview must never reveal")
		},
		createFn: func(string, string) error {
			return errors.New("preview must never create")
		},
	}
}

const movePreviewSecretContent = `[Container]
Image=busybox
Secret=token1
Secret=/etc/certs/store.pem
`

func TestQuadletService_MovePreview_SecretExistencePerStore(t *testing.T) {
	service := newMoveSecretPreviewTestService(
		movePreviewSourceFixture(movePreviewSecretContent),
		&fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return []*entities.ListQuadlet{}, nil
			},
		},
		&fakeAccessRepo{getPermissionFn: allowTargetRead},
		map[string]moveSecretEngine{
			// token1 exists on the source only; the hostPath entry claims
			// nothing anywhere.
			"machine-a": secretStoreFixture("token1"),
			"machine-b": secretStoreFixture(),
		},
	)

	got, err := service.MovePreview(context.Background(), "user", "machine-a", "web.container", "machine-b")

	require.NoError(t, err)
	require.Len(t, got.Secrets, 2)
	assert.Equal(t, "token1", got.Secrets[0].Name)
	assert.Equal(t, MovePreviewSecretKindSecret, got.Secrets[0].Kind)
	assert.True(t, got.Secrets[0].ExistsOnSource)
	assert.False(t, got.Secrets[0].ExistsOnTarget)
	assert.Equal(t, "/etc/certs/store.pem", got.Secrets[1].Name)
	assert.Equal(t, MovePreviewSecretKindHostPath, got.Secrets[1].Kind)
	assert.False(t, got.Secrets[1].ExistsOnSource, "a hostPath entry carries no existence claims")
	assert.False(t, got.Secrets[1].ExistsOnTarget)
}

func TestQuadletService_MovePreview_SecretOnBothStores(t *testing.T) {
	service := newMoveSecretPreviewTestService(
		movePreviewSourceFixture("Secret=token1\n"),
		&fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return []*entities.ListQuadlet{}, nil
			},
		},
		&fakeAccessRepo{getPermissionFn: allowTargetRead},
		map[string]moveSecretEngine{
			"machine-a": secretStoreFixture("token1"),
			"machine-b": secretStoreFixture("token1"),
		},
	)

	got, err := service.MovePreview(context.Background(), "user", "machine-a", "web.container", "machine-b")

	require.NoError(t, err)
	require.Len(t, got.Secrets, 1)
	assert.True(t, got.Secrets[0].ExistsOnSource)
	assert.True(t, got.Secrets[0].ExistsOnTarget)
}

func TestQuadletService_MovePreview_DanglingSecretIsHonestEntry(t *testing.T) {
	service := newMoveSecretPreviewTestService(
		movePreviewSourceFixture("Secret=missing\n"),
		&fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return []*entities.ListQuadlet{}, nil
			},
		},
		&fakeAccessRepo{getPermissionFn: allowTargetRead},
		map[string]moveSecretEngine{
			"machine-a": secretStoreFixture(),
			"machine-b": secretStoreFixture(),
		},
	)

	got, err := service.MovePreview(context.Background(), "user", "machine-a", "web.container", "machine-b")

	require.NoError(t, err, "a dangling reference is an honest entry, not an error")
	require.Len(t, got.Secrets, 1)
	assert.False(t, got.Secrets[0].ExistsOnSource)
	assert.False(t, got.Secrets[0].ExistsOnTarget)
}

func TestQuadletService_MovePreview_NoSecretLinesIsEmptyArray(t *testing.T) {
	service := newMoveSecretPreviewTestService(
		movePreviewSourceFixture(movePreviewContent),
		&fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return []*entities.ListQuadlet{}, nil
			},
		},
		&fakeAccessRepo{getPermissionFn: allowTargetRead},
		map[string]moveSecretEngine{
			// No engine may be dialed for a content without Secret= lines.
			"machine-a": secretStoreFixture("token1"),
			"machine-b": secretStoreFixture(),
		},
	)

	got, err := service.MovePreview(context.Background(), "user", "machine-a", "web.container", "machine-b")

	require.NoError(t, err)
	require.NotNil(t, got.Secrets, "the secrets array renders empty, never null")
	assert.Empty(t, got.Secrets)
}

func TestQuadletService_MovePreview_HostPathOnlyNeverDialsTheStores(t *testing.T) {
	// No secret provider wired at all: a store dial would panic at the
	// nil seam and fail the test — the shape that pins that hostPath
	// references never consult a store.
	service := newMovePreviewTestService(
		movePreviewSourceFixture("Secret=/etc/certs/store.pem\n"),
		&fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return []*entities.ListQuadlet{}, nil
			},
		},
		&fakeAccessRepo{getPermissionFn: allowTargetRead},
	)

	got, err := service.MovePreview(context.Background(), "user", "machine-a", "web.container", "machine-b")

	require.NoError(t, err)
	require.Len(t, got.Secrets, 1)
	assert.Equal(t, MovePreviewSecretKindHostPath, got.Secrets[0].Kind)
	assert.False(t, got.Secrets[0].ExistsOnSource)
	assert.False(t, got.Secrets[0].ExistsOnTarget)
}

// --- transfer ---------------------------------------------------------

// newMoveSecretTransferTestService builds the transfer service with the
// secret provider wired, mirroring newMoveTransferTestService.
func newMoveSecretTransferTestService(sourceEngine, targetEngine *fakeQuadletEngine, access *fakeAccessRepo, sshExec sshexec.SSHExecInterface, secrets map[string]moveSecretEngine) *QuadletService {
	return NewQuadletService(fakeEngineProvider{engines: map[string]QuadletEngine{
		"machine-a": sourceEngine,
		"machine-b": targetEngine,
	}}, sshExec, lookupForUser("core")).
		WithAccessRepo(access).
		WithSecretEngines(fakeSecretEngineProvider{engines: secrets})
}

// moveSecretTransferContent is the secret-bearing source content the
// transfer tests share.
const moveSecretTransferContent = `[Container]
Image=busybox
Secret=dbpass
`

// secretTransferRequest is a copy to machine-b with transferSecrets set.
func secretTransferRequest(mode string) QuadletMoveRequest {
	return QuadletMoveRequest{TargetMachineID: "machine-b", Mode: mode, TransferSecrets: true}
}

// recordingSecretStore builds a secret engine holding the given names,
// answering reveals from values, and recording creates; reveals and
// creates are recorded so tests can pin what was read and what landed.
type recordingSecretStore struct {
	names   []string
	values  map[string]string
	created map[string]string
	fails   map[string]error
}

func newRecordingSecretStore(names []string, values map[string]string) *recordingSecretStore {
	if values == nil {
		values = map[string]string{}
	}
	return &recordingSecretStore{
		names:   names,
		values:  values,
		created: map[string]string{},
		fails:   map[string]error{},
	}
}

func (r *recordingSecretStore) Names() ([]string, error) {
	return append([]string{}, r.names...), nil
}

// Has answers the same-name check over what the store holds — including
// what an earlier run's Create installed, so the re-run skip keeps
// working.
func (r *recordingSecretStore) Has(name string) (bool, error) {
	for _, existing := range r.names {
		if existing == name {
			return true, nil
		}
	}
	return false, nil
}

func (r *recordingSecretStore) Reveal(name string) (string, error) {
	if err, failed := r.fails[name]; failed {
		return "", err
	}
	value, ok := r.values[name]
	if !ok {
		return "", &fakeNotFoundError{}
	}
	return value, nil
}

func (r *recordingSecretStore) Create(name, value string) error {
	if err, failed := r.fails[name]; failed {
		return err
	}
	r.created[name] = value
	r.names = append(r.names, name)
	return nil
}

// fakeNotFoundError is the podman not-found shape the phase's dangling
// branch classifies through podmanerr.IsNotFound.
type fakeNotFoundError struct{}

func (e *fakeNotFoundError) Error() string { return "no such secret" }

func (e *fakeNotFoundError) NotFound() {}

func TestQuadletService_Move_CopyWithSecretsCreatesOnTarget(t *testing.T) {
	target, _, _, _ := emptyTargetEngine(t)
	sourceStore := newRecordingSecretStore(nil, map[string]string{"dbpass": "s3cret-value"})
	targetStore := newRecordingSecretStore(nil, nil)
	service := newMoveSecretTransferTestService(
		moveSourceFixture("web.container", "/x/web.container", "stopped", moveSecretTransferContent),
		target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
		map[string]moveSecretEngine{
			"machine-a": sourceStore,
			"machine-b": targetStore,
		},
	)

	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", secretTransferRequest(MoveModeCopy))

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, MoveStatusCopied, got.Status)
	assert.Equal(t, map[string]string{"dbpass": "s3cret-value"}, targetStore.created,
		"the value must reach the target's store byte-identical")
	require.NotNil(t, got.SecretTransfer, "the envelope carries the secretTransfer block")
	require.Len(t, got.SecretTransfer.Secrets, 1)
	assert.Equal(t, QuadletMoveSecretOutcome{Name: "dbpass", Outcome: MoveSecretOutcomeTransferred}, got.SecretTransfer.Secrets[0])
	assert.Empty(t, got.SecretTransfer.Secrets[0].Reason, "a transferred outcome carries no reason")
	assert.Empty(t, sourceStore.created, "the source store is never written")
}

func TestQuadletService_Move_MoveKeepsSourceSecretsAndRemovesOnlyTheQuadlet(t *testing.T) {
	var removed []string
	target, _, _, _ := emptyTargetEngine(t)
	source := &fakeQuadletEngine{
		printFn: func(string) (string, error) {
			return moveSecretTransferContent, nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"},
			}, nil
		},
		removeFn: func(name string, force bool) (*entities.QuadletRemoveReport, error) {
			removed = append(removed, name)
			return &entities.QuadletRemoveReport{}, nil
		},
	}
	sourceStore := newRecordingSecretStore(nil, map[string]string{"dbpass": "s3cret-value"})
	targetStore := newRecordingSecretStore(nil, nil)
	service := newMoveSecretTransferTestService(
		source,
		target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
		map[string]moveSecretEngine{
			"machine-a": sourceStore,
			"machine-b": targetStore,
		},
	)

	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", secretTransferRequest(MoveModeMove))

	require.NoError(t, err)
	assert.Equal(t, MoveStatusMoved, got.Status)
	assert.Equal(t, []string{"web.container"}, removed,
		"a move removes ONLY the main quadlet — the source secret survives (secrets are copy-only)")
	assert.Equal(t, map[string]string{"dbpass": "s3cret-value"}, targetStore.created)
}

func TestQuadletService_Move_SameNameOnTargetIsSkippedNotOverwritten(t *testing.T) {
	target, _, _, _ := emptyTargetEngine(t)
	sourceStore := newRecordingSecretStore(nil, map[string]string{"dbpass": "source-value"})
	targetStore := newRecordingSecretStore([]string{"dbpass"}, map[string]string{"dbpass": "target-value"})
	service := newMoveSecretTransferTestService(
		moveSourceFixture("web.container", "/x/web.container", "stopped", moveSecretTransferContent),
		target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
		map[string]moveSecretEngine{
			"machine-a": sourceStore,
			"machine-b": targetStore,
		},
	)

	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", secretTransferRequest(MoveModeCopy))

	require.NoError(t, err)
	assert.Empty(t, targetStore.created, "nothing on the target is overwritten")
	assert.Equal(t, "target-value", targetStore.values["dbpass"])
	require.NotNil(t, got.SecretTransfer)
	require.Len(t, got.SecretTransfer.Secrets, 1)
	assert.Equal(t, MoveSecretOutcomeSkipped, got.SecretTransfer.Secrets[0].Outcome)
	assert.Equal(t, MoveSecretSkipOnTarget, got.SecretTransfer.Secrets[0].Reason)
}

func TestQuadletService_Move_DanglingAndHostPathReferencesAreSkipped(t *testing.T) {
	content := `[Container]
Image=busybox
Secret=missing
Secret=/etc/certs/store.pem
`
	target, _, _, _ := emptyTargetEngine(t)
	sourceStore := newRecordingSecretStore(nil, nil) // reveals nothing: "missing" dangles
	targetStore := newRecordingSecretStore(nil, nil)
	service := newMoveSecretTransferTestService(
		moveSourceFixture("web.container", "/x/web.container", "stopped", content),
		target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
		map[string]moveSecretEngine{
			"machine-a": sourceStore,
			"machine-b": targetStore,
		},
	)

	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", secretTransferRequest(MoveModeCopy))

	require.NoError(t, err, "dangling references are reported, not errors")
	assert.Empty(t, targetStore.created)
	require.NotNil(t, got.SecretTransfer)
	require.Len(t, got.SecretTransfer.Secrets, 2)
	assert.Equal(t, "missing", got.SecretTransfer.Secrets[0].Name)
	assert.Equal(t, MoveSecretOutcomeSkipped, got.SecretTransfer.Secrets[0].Outcome)
	assert.Equal(t, MoveSecretSkipDangling, got.SecretTransfer.Secrets[0].Reason)
	assert.Equal(t, "/etc/certs/store.pem", got.SecretTransfer.Secrets[1].Name)
	assert.Equal(t, MoveSecretOutcomeSkipped, got.SecretTransfer.Secrets[1].Outcome)
	assert.Equal(t, MoveSecretSkipHostPath, got.SecretTransfer.Secrets[1].Reason)
}

func TestQuadletService_Move_BelowAdminOnSourceIsRefusedInPreflight(t *testing.T) {
	// The caller holds write on BOTH machines — enough for a plain
	// transfer, not enough to move a value off the source.
	writeBoth := func(machineID, userID string) (string, error) {
		return models.PermissionWrite, nil
	}
	target, _, _, _ := emptyTargetEngine(t)
	sourceStore := newRecordingSecretStore(nil, map[string]string{"dbpass": "s3cret-value"})
	targetStore := newRecordingSecretStore(nil, nil)
	service := newMoveSecretTransferTestService(
		moveSourceFixture("web.container", "/x/web.container", "stopped", moveSecretTransferContent),
		target,
		&fakeAccessRepo{getPermissionFn: writeBoth},
		testutils.NewMockSSHExec(),
		map[string]moveSecretEngine{
			"machine-a": sourceStore,
			"machine-b": targetStore,
		},
	)

	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", secretTransferRequest(MoveModeCopy))

	require.Error(t, err)
	assert.Nil(t, got, "a refused transfer answers no envelope")
	var accessErr *appErrors.AppError
	require.ErrorAs(t, err, &accessErr)
	assert.Equal(t, appErrors.ErrAccessDenied.Code, accessErr.Code, "the refusal is the standard 403 ACCESS_DENIED")
	assert.Equal(t, http.StatusForbidden, accessErr.Status)
	assert.Empty(t, targetStore.created, "nothing moved")
}

func TestQuadletService_Move_FlagAbsentReadsNoValueAndAnswersNoBlock(t *testing.T) {
	target, gotFile, _, _ := emptyTargetEngine(t)
	targetStore := newRecordingSecretStore(nil, nil)
	service := newMoveSecretTransferTestService(
		moveSourceFixture("web.container", "/x/web.container", "stopped", moveSecretTransferContent),
		target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
		map[string]moveSecretEngine{
			// The source store's Reveal is armed to fail loudly through the
			// recording fixture only when called — with the flag absent the
			// reveal map is never consulted... so arm an explicit tripwire
			// instead: a reveal here means the value was read without the
			// flag asking for it.
			"machine-a": &fakeMoveSecretEngine{
				revealFn: func(string) (string, error) {
					return "", errors.New("no value may be read without transferSecrets")
				},
			},
			"machine-b": targetStore,
		},
	)

	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", moveCopyRequest())

	require.NoError(t, err)
	require.NotNil(t, gotFile, "the transfer behaves exactly as today")
	assert.Nil(t, got.SecretTransfer, "no secretTransfer block without the flag")
	assert.Empty(t, targetStore.created)
}

func TestQuadletService_Move_SecretFailureIs500NamesTheSecretAndInstallsNothing(t *testing.T) {
	var installs int
	target, _, _, _ := emptyTargetEngine(t)
	target.installFn = func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
		installs++
		return &entities.QuadletInstallReport{}, nil
	}
	sourceStore := newRecordingSecretStore(nil, map[string]string{"dbpass": "s3cret-value"})
	sourceStore.fails["apikey"] = errors.New("store refused the reveal")
	targetStore := newRecordingSecretStore(nil, nil)
	service := newMoveSecretTransferTestService(
		moveSourceFixture("web.container", "/x/web.container", "stopped",
			"Secret=dbpass\nSecret=apikey\n"),
		target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
		map[string]moveSecretEngine{
			"machine-a": sourceStore,
			"machine-b": targetStore,
		},
	)

	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", secretTransferRequest(MoveModeCopy))

	require.Error(t, err)
	assert.Nil(t, got)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, appErrors.ErrSecretTransferFailed.Code, appErr.Code, "the phase failure is the secret sentinel")
	assert.Equal(t, http.StatusInternalServerError, appErr.Status)
	assert.Contains(t, appErr.Message, "apikey", "the message names the failing secret")
	assert.Contains(t, appErr.Message, "store refused the reveal", "the message names the cause")
	assert.Zero(t, installs, "nothing is installed — the phase aborts before the target quadlet lands")
	assert.Equal(t, map[string]string{"dbpass": "s3cret-value"}, targetStore.created,
		"the secret delivered before the failure stays on the target")
}

func TestQuadletService_Move_ReRunSkipsTheDeliveredSecret(t *testing.T) {
	target, _, _, _ := emptyTargetEngine(t)
	sourceStore := newRecordingSecretStore(nil, map[string]string{"dbpass": "s3cret-value"})
	targetStore := newRecordingSecretStore(nil, nil)
	secrets := map[string]moveSecretEngine{
		"machine-a": sourceStore,
		"machine-b": targetStore,
	}
	service := newMoveSecretTransferTestService(
		moveSourceFixture("web.container", "/x/web.container", "stopped", moveSecretTransferContent),
		target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
		secrets,
	)

	_, err := service.Move(context.Background(), "user", "machine-a", "web.container", secretTransferRequest(MoveModeCopy))
	require.NoError(t, err)

	// The quadlet vanished from the target between runs (cleanup after a
	// partial failure); the secret it delivered is still there and must be
	// skipped, not overwritten.
	target.listFn = func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
		return []*entities.ListQuadlet{}, nil
	}
	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", secretTransferRequest(MoveModeCopy))

	require.NoError(t, err)
	assert.Equal(t, map[string]string{"dbpass": "s3cret-value"}, targetStore.created,
		"the re-run must not create again — the delivered secret is skipped")
	require.NotNil(t, got.SecretTransfer)
	require.Len(t, got.SecretTransfer.Secrets, 1)
	assert.Equal(t, MoveSecretOutcomeSkipped, got.SecretTransfer.Secrets[0].Outcome)
}

func TestQuadletService_Move_SecretsAndVolumeDataBlocksRideTogether(t *testing.T) {
	target, _, _, _ := emptyTargetEngine(t)
	sourceStore := newRecordingSecretStore(nil, map[string]string{"dbpass": "s3cret-value"})
	targetStore := newRecordingSecretStore(nil, nil)
	service := newMoveSecretTransferTestService(
		moveSourceFixture("web.container", "/x/web.container", "stopped", moveSecretTransferContent),
		target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
		map[string]moveSecretEngine{
			"machine-a": sourceStore,
			"machine-b": targetStore,
		},
	)
	req := secretTransferRequest(MoveModeCopy)
	req.MigrateVolumeData = true

	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", req)

	require.NoError(t, err)
	require.NotNil(t, got.SecretTransfer, "the secretTransfer block rides the envelope beside volumeMigration")
	require.NotNil(t, got.VolumeMigration, "the volumeMigration block rides beside secretTransfer")
}

// TestQuadletService_Move_SecretFailureAbortsWithQuadletsUntouched pins
// the phase-order consequence through external behavior: a transfer whose
// secret phase fails answers the error sentinel and leaves the target
// quadlet list exactly as it was.
func TestQuadletService_Move_SecretFailureLeavesTargetQuadletSetUntouched(t *testing.T) {
	var installed []string
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{}, nil
		},
		installFn: func(fileName string, content []byte, replace bool) (*entities.QuadletInstallReport, error) {
			installed = append(installed, fileName)
			return &entities.QuadletInstallReport{}, nil
		},
	}
	sourceStore := &fakeMoveSecretEngine{
		revealFn: func(string) (string, error) {
			return "", errors.New("store unreachable")
		},
	}
	targetStore := newRecordingSecretStore(nil, nil)
	service := newMoveSecretTransferTestService(
		moveSourceFixture("web.container", "/x/web.container", "stopped", moveSecretTransferContent),
		target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		testutils.NewMockSSHExec(),
		map[string]moveSecretEngine{
			"machine-a": sourceStore,
			"machine-b": targetStore,
		},
	)

	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", secretTransferRequest(MoveModeCopy))

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Empty(t, installed, "the quadlet installs nothing when the secret phase fails")
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, appErrors.ErrSecretTransferFailed.Code, appErr.Code)
	assert.True(t, strings.Contains(err.Error(), "dbpass"))
}

// TestQuadletService_Move_SecretFailureBeforeTheStopStopsNothing pins the
// phase order (issue 207 review): on a migrating move the secret phase
// runs BEFORE the stop, so a cheap secret failure aborts while the
// source's unit was never touched — no lifecycle stop call at all, nothing
// installed, and no stop note in the message.
func TestQuadletService_Move_SecretFailureBeforeTheStopStopsNothing(t *testing.T) {
	sshExec := testutils.NewMockSSHExec()
	var installs int
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{}, nil
		},
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			installs++
			return &entities.QuadletInstallReport{}, nil
		},
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) { return nil, nil },
	}
	source := &fakeQuadletEngine{
		printFn: func(string) (string, error) {
			return "[Container]\nImage=busybox\nVolume=pgdata:/var/lib/pg:Z\nSecret=dbpass\n", nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "running"},
			}, nil
		},
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) {
			return &QuadletVolumeInspect{Name: "pgdata"}, nil
		},
	}
	sourceStore := &fakeMoveSecretEngine{
		revealFn: func(string) (string, error) {
			return "", errors.New("store unreachable")
		},
	}
	targetStore := newRecordingSecretStore(nil, nil)
	service := newMoveSecretTransferTestService(
		source, target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		sshExec,
		map[string]moveSecretEngine{
			"machine-a": sourceStore,
			"machine-b": targetStore,
		},
	)
	req := secretTransferRequest(MoveModeMove)
	req.Stop = true
	req.MigrateVolumeData = true

	got, err := service.Move(context.Background(), "user", "machine-a", "web.container", req)

	require.Error(t, err)
	assert.Nil(t, got)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, appErrors.ErrSecretTransferFailed.Code, appErr.Code)
	assert.Zero(t, installs, "nothing is installed")
	assert.Empty(t, sshExec.Calls(),
		"the secret failure aborts before the stop — the unit was never touched")
	assert.NotContains(t, appErr.Message, "remains stopped",
		"no stop note: nothing had been stopped when the phase failed")
}
