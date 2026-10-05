package secrets

import (
	"context"
	"errors"
	"testing"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// fakeAccessRepo is the function-field fake for the
// permission.PermissionGetter seam — the access store the cross-machine
// copy/move gates the source reveal and the target machine on. An
// unarmed lookup panics, so an unexpected gate check fails the test
// loudly.
type fakeAccessRepo struct {
	getPermissionFn func(machineID, userID string) (string, error)
}

func (f *fakeAccessRepo) GetPermission(machineID, userID string) (string, error) {
	if f.getPermissionFn == nil {
		panic("unexpected PermissionGetter.GetPermission call")
	}
	return f.getPermissionFn(machineID, userID)
}

// allowSourceAdminTargetWrite answers admin on the source machine and
// write on the target — the minimum the transfer requires of both.
func allowSourceAdminTargetWrite(machineID, userID string) (string, error) {
	if machineID == "machine-b" {
		return models.PermissionWrite, nil
	}
	return models.PermissionAdmin, nil
}

// allowSourceAdminTargetRead answers admin on the source machine and
// read on the target — the minimum the preview requires of the target.
func allowSourceAdminTargetRead(machineID, userID string) (string, error) {
	if machineID == "machine-b" {
		return models.PermissionRead, nil
	}
	return models.PermissionAdmin, nil
}

// newMoveTestService builds the service over per-machine fake engines
// (the multi-lease shape the copy/move exercises), a source-machine
// usage fake, and the given access store.
func newMoveTestService(sourceEngine, targetEngine *fakeSecretEngine, usage *fakeUsageEngine, access *fakeAccessRepo) *SecretService {
	return NewSecretService(fakeEngineProvider{engines: map[string]SecretEngine{
		"machine-a": sourceEngine,
		"machine-b": targetEngine,
	}}, access).
		WithUsageEngines(fakeUsageEngineProvider{engines: map[string]SecretUsageEngine{
			"machine-a": usage,
		}})
}

// moveSourceFixture is the source machine's engine: the db-password
// secret exists, reveals its value on the admin-gated inspect, and
// removes cleanly.
func moveSourceFixture() *fakeSecretEngine {
	return &fakeSecretEngine{
		inspectFn: func(name string, reveal bool) (*entities.SecretInfoReport, error) {
			if name == "db-password" {
				return secretFixture("3f2e1d0c", "db-password", time.Time{}, "hunter2\n"), nil
			}
			return nil, errors.New("no such secret")
		},
		removeFn: func(name string) error {
			if name == "db-password" {
				return nil
			}
			return errors.New("no such secret")
		},
	}
}

// emptyTargetFixture is the target machine's engine: no secrets yet (the
// transfer's same-name check inspects and finds nothing), creates succeed
// and record what they installed.
func emptyTargetFixture() *fakeSecretEngine {
	return &fakeSecretEngine{
		listFn: func() ([]*entities.SecretInfoReport, error) { return nil, nil },
		inspectFn: func(name string, reveal bool) (*entities.SecretInfoReport, error) {
			return nil, errors.New("no such secret")
		},
		createFn: func(name, value string) (*entities.SecretCreateReport, error) {
			return &entities.SecretCreateReport{ID: "new-on-target"}, nil
		},
	}
}

// idleUsageFixture answers an empty quadlet surface: no quadlet files.
func idleUsageFixture() *fakeUsageEngine {
	return &fakeUsageEngine{
		listQuadletNamesFn: func() ([]string, error) { return nil, nil },
	}
}

// --- MovePreview ----------------------------------------------------------

func TestSecretService_MovePreview_ReportsFacts(t *testing.T) {
	usage := &fakeUsageEngine{
		listQuadletNamesFn: func() ([]string, error) {
			return []string{"web.container", "db.container"}, nil
		},
		printQuadletFn: func(name string) (string, error) {
			switch name {
			case "web.container":
				return "[Container]\nImage=busybox\nSecret=db-password\n", nil
			case "db.container":
				return "[Container]\nImage=busybox\nSecret=other-secret\n", nil
			}
			return "", errors.New("no such quadlet")
		},
	}
	service := newMoveTestService(moveSourceFixture(), emptyTargetFixture(), usage, &fakeAccessRepo{getPermissionFn: allowSourceAdminTargetRead})

	preview, err := service.MovePreview(context.Background(), "user", "machine-a", "db-password", "machine-b")

	require.NoError(t, err)
	assert.False(t, preview.Collision, "the target holds no secret of this name")
	require.Len(t, preview.ReferencingQuadlets, 1)
	assert.Equal(t, "web.container", preview.ReferencingQuadlets[0].Name)
}

func TestSecretService_MovePreview_Collision(t *testing.T) {
	target := emptyTargetFixture()
	target.listFn = func() ([]*entities.SecretInfoReport, error) {
		return []*entities.SecretInfoReport{
			secretFixture("b-1", "db-password", time.Time{}, ""),
		}, nil
	}
	service := newMoveTestService(moveSourceFixture(), target, idleUsageFixture(), &fakeAccessRepo{getPermissionFn: allowSourceAdminTargetRead})

	preview, err := service.MovePreview(context.Background(), "user", "machine-a", "db-password", "machine-b")

	require.NoError(t, err)
	assert.True(t, preview.Collision, "a same-named target secret is a collision")
}

func TestSecretService_MovePreview_MissingSourceSecretIs404(t *testing.T) {
	service := newMoveTestService(moveSourceFixture(), emptyTargetFixture(), idleUsageFixture(), &fakeAccessRepo{getPermissionFn: allowSourceAdminTargetRead})

	preview, err := service.MovePreview(context.Background(), "user", "machine-a", "no-such-secret", "machine-b")

	assert.ErrorIs(t, err, appErrors.ErrSecretNotFound)
	assert.Nil(t, preview)
}

func TestSecretService_MovePreview_TargetAccessDeniedIs403(t *testing.T) {
	service := newMoveTestService(moveSourceFixture(), emptyTargetFixture(), idleUsageFixture(),
		&fakeAccessRepo{getPermissionFn: func(machineID, userID string) (string, error) {
			if machineID == "machine-b" {
				return "", nil // no access row on the target
			}
			return models.PermissionAdmin, nil
		}})

	preview, err := service.MovePreview(context.Background(), "user", "machine-a", "db-password", "machine-b")

	assert.ErrorIs(t, err, appErrors.ErrAccessDenied)
	assert.Nil(t, preview)
}

func TestSecretService_MovePreview_SourceUnreachableIsConnectionFailed(t *testing.T) {
	service := NewSecretService(fakeEngineProvider{err: appErrors.ErrConnectionFailed}, &fakeAccessRepo{getPermissionFn: allowSourceAdminTargetRead}).
		WithUsageEngines(fakeUsageEngineProvider{engines: map[string]SecretUsageEngine{}})

	preview, err := service.MovePreview(context.Background(), "user", "machine-a", "db-password", "machine-b")

	assert.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	assert.Nil(t, preview)
}

// --- Move -----------------------------------------------------------------

func TestSecretService_Move_CopyRevealsAndCreates(t *testing.T) {
	source := moveSourceFixture()
	target := emptyTargetFixture()
	var revealed bool
	var createdName, createdValue string
	source.inspectFn = func(name string, reveal bool) (*entities.SecretInfoReport, error) {
		if name != "db-password" {
			return nil, errors.New("no such secret")
		}
		revealed = revealed || reveal
		return secretFixture("3f2e1d0c", "db-password", time.Time{}, "hunter2\n"), nil
	}
	target.createFn = func(name, value string) (*entities.SecretCreateReport, error) {
		createdName, createdValue = name, value
		return &entities.SecretCreateReport{ID: "new-on-target"}, nil
	}
	service := newMoveTestService(source, target, idleUsageFixture(), &fakeAccessRepo{getPermissionFn: allowSourceAdminTargetWrite})

	response, err := service.Move(context.Background(), "user", "machine-a", "db-password",
		SecretMoveRequest{TargetMachineID: "machine-b", Mode: SecretMoveModeCopy})

	require.NoError(t, err)
	assert.Equal(t, SecretMoveStatusCopied, response.Status)
	assert.Equal(t, "db-password", response.Name)
	assert.Equal(t, "machine-a", response.SourceMachineID)
	assert.Equal(t, "machine-b", response.TargetMachineID)
	assert.True(t, revealed, "the transfer must reveal the value on the source")
	assert.Equal(t, "db-password", createdName, "the name is preserved")
	assert.Equal(t, "hunter2\n", createdValue, "the value must cross byte-identical")
}

func TestSecretService_Move_MoveRemovesSourceAfterInstall(t *testing.T) {
	source := moveSourceFixture()
	removed := false
	source.removeFn = func(name string) error {
		removed = true
		return nil
	}
	service := newMoveTestService(source, emptyTargetFixture(), idleUsageFixture(), &fakeAccessRepo{getPermissionFn: allowSourceAdminTargetWrite})

	response, err := service.Move(context.Background(), "user", "machine-a", "db-password",
		SecretMoveRequest{TargetMachineID: "machine-b", Mode: SecretMoveModeMove})

	require.NoError(t, err)
	assert.Equal(t, SecretMoveStatusMoved, response.Status)
	assert.True(t, removed, "a move removes the source secret after the install")
	assert.Empty(t, response.Error)
}

func TestSecretService_Move_CollisionIs409(t *testing.T) {
	source := moveSourceFixture()
	target := emptyTargetFixture()
	target.inspectFn = func(name string, reveal bool) (*entities.SecretInfoReport, error) {
		return secretFixture("b-1", "db-password", time.Time{}, ""), nil
	}
	revealReached := false
	source.inspectFn = func(name string, reveal bool) (*entities.SecretInfoReport, error) {
		revealReached = revealReached || reveal
		return secretFixture("3f2e1d0c", "db-password", time.Time{}, "hunter2\n"), nil
	}
	service := newMoveTestService(source, target, idleUsageFixture(), &fakeAccessRepo{getPermissionFn: allowSourceAdminTargetWrite})

	response, err := service.Move(context.Background(), "user", "machine-a", "db-password",
		SecretMoveRequest{TargetMachineID: "machine-b", Mode: SecretMoveModeCopy})

	assert.ErrorIs(t, err, appErrors.ErrSecretAlreadyExists)
	assert.Nil(t, response)
	assert.False(t, revealReached, "the kit's same-name skip fires BEFORE the reveal — the value is never read when nothing needs it")
}

func TestSecretService_Move_MissingSourceSecretIs404(t *testing.T) {
	service := newMoveTestService(moveSourceFixture(), emptyTargetFixture(), idleUsageFixture(), &fakeAccessRepo{getPermissionFn: allowSourceAdminTargetWrite})

	response, err := service.Move(context.Background(), "user", "machine-a", "no-such-secret",
		SecretMoveRequest{TargetMachineID: "machine-b", Mode: SecretMoveModeCopy})

	assert.ErrorIs(t, err, appErrors.ErrSecretNotFound)
	assert.Nil(t, response)
}

func TestSecretService_Move_BelowAdminSourceIs403(t *testing.T) {
	// The engines stay unarmed: a below-admin caller must be refused
	// before any connection lease, so any engine call panics the test.
	service := newMoveTestService(&fakeSecretEngine{}, &fakeSecretEngine{}, &fakeUsageEngine{},
		&fakeAccessRepo{getPermissionFn: func(machineID, userID string) (string, error) {
			return models.PermissionWrite, nil
		}})

	response, err := service.Move(context.Background(), "user", "machine-a", "db-password",
		SecretMoveRequest{TargetMachineID: "machine-b", Mode: SecretMoveModeCopy})

	assert.ErrorIs(t, err, appErrors.ErrAccessDenied)
	assert.Nil(t, response)
}

func TestSecretService_Move_TargetAccessDeniedIs403(t *testing.T) {
	service := newMoveTestService(moveSourceFixture(), emptyTargetFixture(), idleUsageFixture(),
		&fakeAccessRepo{getPermissionFn: func(machineID, userID string) (string, error) {
			if machineID == "machine-b" {
				return models.PermissionRead, nil
			}
			return models.PermissionAdmin, nil
		}})

	response, err := service.Move(context.Background(), "user", "machine-a", "db-password",
		SecretMoveRequest{TargetMachineID: "machine-b", Mode: SecretMoveModeCopy})

	assert.ErrorIs(t, err, appErrors.ErrAccessDenied)
	assert.Nil(t, response)
}

func TestSecretService_Move_RevealFailureIsTransferFailed(t *testing.T) {
	source := moveSourceFixture()
	source.inspectFn = func(name string, reveal bool) (*entities.SecretInfoReport, error) {
		if reveal {
			return nil, errors.New("podman exploded")
		}
		return secretFixture("3f2e1d0c", "db-password", time.Time{}, ""), nil
	}
	service := newMoveTestService(source, emptyTargetFixture(), idleUsageFixture(), &fakeAccessRepo{getPermissionFn: allowSourceAdminTargetWrite})

	response, err := service.Move(context.Background(), "user", "machine-a", "db-password",
		SecretMoveRequest{TargetMachineID: "machine-b", Mode: SecretMoveModeCopy})

	appErr, ok := err.(*appErrors.AppError)
	require.True(t, ok)
	assert.Equal(t, appErrors.ErrSecretTransferFailed.Code, appErr.Code)
	assert.Contains(t, appErr.Message, "db-password", "the failure must name the secret")
	assert.Nil(t, response)
}

func TestSecretService_Move_RemoveFailureIsPartial(t *testing.T) {
	source := moveSourceFixture()
	source.removeFn = func(name string) error {
		return errors.New("secret \"db-password\" is in use by container 3f2e1d0c")
	}
	service := newMoveTestService(source, emptyTargetFixture(), idleUsageFixture(), &fakeAccessRepo{getPermissionFn: allowSourceAdminTargetWrite})

	response, err := service.Move(context.Background(), "user", "machine-a", "db-password",
		SecretMoveRequest{TargetMachineID: "machine-b", Mode: SecretMoveModeMove})

	require.NoError(t, err, "a failed source removal does not fail the request")
	assert.Equal(t, SecretMoveStatusPartial, response.Status)
	assert.Contains(t, response.Error, "in use by container", "the partial detail carries the removal failure's cause")
}

func TestSecretService_Move_VanishedSourceIsMoved(t *testing.T) {
	source := moveSourceFixture()
	source.removeFn = func(name string) error {
		return errors.New("no such secret")
	}
	service := newMoveTestService(source, emptyTargetFixture(), idleUsageFixture(), &fakeAccessRepo{getPermissionFn: allowSourceAdminTargetWrite})

	response, err := service.Move(context.Background(), "user", "machine-a", "db-password",
		SecretMoveRequest{TargetMachineID: "machine-b", Mode: SecretMoveModeMove})

	require.NoError(t, err)
	assert.Equal(t, SecretMoveStatusMoved, response.Status,
		"a source that vanished between transfer and removal needs no removal")
	assert.Empty(t, response.Error)
}

func TestSecretService_Move_SourceUnreachableIsConnectionFailed(t *testing.T) {
	service := NewSecretService(fakeEngineProvider{err: appErrors.ErrConnectionFailed}, &fakeAccessRepo{getPermissionFn: allowSourceAdminTargetWrite}).
		WithUsageEngines(fakeUsageEngineProvider{engines: map[string]SecretUsageEngine{}})

	response, err := service.Move(context.Background(), "user", "machine-a", "db-password",
		SecretMoveRequest{TargetMachineID: "machine-b", Mode: SecretMoveModeCopy})

	assert.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	assert.Nil(t, response)
}

// --- the `Secret=` parse ritual -------------------------------------------

func TestQuadletSecretReferences(t *testing.T) {
	content := `[Container]
Image=busybox
Secret=db-password
Secret=db-password,target=/run/secrets/db
Secret=source=api-token
Secret=src=not-the-option
Secret=/etc/certs/store.pem
# Secret=commented-out
; Secret=semicolon-commented
Volume=db-password:/data
secret=lowercase-key
Secret=
`
	references := quadletSecretReferences(content)

	assert.Equal(t, []string{"db-password", "db-password", "api-token", "lowercase-key"}, references,
		"the name is the token before the first comma; source= governs the all-options form (any other "+
			"option key names nothing); host paths and comment lines never match; key matching is "+
			"case-insensitive")
}
