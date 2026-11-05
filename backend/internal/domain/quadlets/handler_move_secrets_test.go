package quadlets

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// Handler-level pins for the move/copy secret surface (issue 215): the
// wire shapes — the preview's secrets array, the transfer's
// transferSecrets flag decoding through to the secretTransfer block, and
// the below-admin 403 envelope. The semantics live in the service tests.

// newMoveSecretTestHandler builds the handler over both quadlet modules
// with per-host fake engines, the given access store, and per-host
// fake secret stores — the wiring the secret surface exercises. The real
// lean service (SSH exec and host lookup wired for the seam crossings)
// is passed to the transfer service as its lifecycle seam.
func newMoveSecretTestHandler(sourceEngine, targetEngine *fakeQuadletEngine, access *fakeAccessRepo, secrets map[string]moveSecretEngine) *QuadletHandler {
	provider := fakeEngineProvider{engines: map[string]QuadletEngine{
		"host-a": sourceEngine,
		"host-b": targetEngine,
	}}
	service := NewQuadletService(provider, testutils.NewMockSSHExec(), lookupForUser("core"))
	transfer := NewQuadletTransferService(provider, fakeSecretEngineProvider{engines: secrets}, access, 0, service)
	return NewQuadletHandler(service, transfer)
}

func TestQuadletHandler_MovePreview_RendersSecretsArray(t *testing.T) {
	sourceEngine := movePreviewSourceFixture("Secret=token1\n")
	targetEngine := &fakeQuadletEngine{listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
		return nil, nil
	}}
	handler := newMoveSecretTestHandler(sourceEngine, targetEngine,
		&fakeAccessRepo{getPermissionFn: allowTargetRead},
		map[string]moveSecretEngine{
			"host-a": secretStoreFixture("token1"),
			"host-b": secretStoreFixture("token1"),
		})

	req := movePreviewRequest("/api/hosts/host-a/quadlets/web.container/move-preview?targetHostId=host-b",
		"user123", "host-a", "web.container")

	rr := httptest.NewRecorder()
	handler.MovePreview(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.JSONEq(t, `{
		"volumes": [],
		"secrets": [{"name": "token1", "kind": "secret", "existsOnSource": true, "existsOnTarget": true}],
		"collision": false,
		"running": false
	}`, rr.Body.String(), "the secrets array rides the preview beside volumes")
}

func TestQuadletHandler_Move_TransferSecretsRidesThroughToTheBlock(t *testing.T) {
	targetEngine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return &entities.QuadletInstallReport{}, nil
		},
	}
	sourceStore := &recordingSecretStore{values: map[string]string{"dbpass": "s3cret-value"}, created: map[string]string{}}
	targetStore := &recordingSecretStore{values: map[string]string{}, created: map[string]string{}}
	handler := newMoveSecretTestHandler(
		moveSourceFixture("web.container", "/x/web.container", "stopped", moveSecretTransferContent),
		targetEngine,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite},
		map[string]moveSecretEngine{
			"host-a": sourceStore,
			"host-b": targetStore,
		})

	req := moveRequest(moveEndpoint, `{"targetHostId":"host-b","mode":"copy","transferSecrets":true}`)

	rr := httptest.NewRecorder()
	handler.Move(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, map[string]string{"dbpass": "s3cret-value"}, targetStore.created)
	var got struct {
		Status         string `json:"status"`
		SecretTransfer *struct {
			Secrets []struct {
				Name    string `json:"name"`
				Outcome string `json:"outcome"`
			} `json:"secrets"`
		} `json:"secretTransfer"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, MoveStatusCopied, got.Status)
	require.NotNil(t, got.SecretTransfer, "the secretTransfer block rides the envelope")
	require.Len(t, got.SecretTransfer.Secrets, 1)
	assert.Equal(t, "dbpass", got.SecretTransfer.Secrets[0].Name)
	assert.Equal(t, MoveSecretOutcomeTransferred, got.SecretTransfer.Secrets[0].Outcome)
}

func TestQuadletHandler_Move_TransferSecretsBelowAdminIs403Envelope(t *testing.T) {
	writeBoth := func(string, string) (string, error) {
		return models.PermissionWrite, nil
	}
	targetEngine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
	}
	targetStore := &recordingSecretStore{values: map[string]string{}, created: map[string]string{}}
	handler := newMoveSecretTestHandler(
		moveSourceFixture("web.container", "/x/web.container", "stopped", moveSecretTransferContent),
		targetEngine,
		&fakeAccessRepo{getPermissionFn: writeBoth},
		map[string]moveSecretEngine{
			"host-a": &recordingSecretStore{values: map[string]string{"dbpass": "s3cret-value"}, created: map[string]string{}},
			"host-b": targetStore,
		})

	req := moveRequest(moveEndpoint, `{"targetHostId":"host-b","mode":"copy","transferSecrets":true}`)

	rr := httptest.NewRecorder()
	handler.Move(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)
	var got moveErrorEnvelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, "ACCESS_DENIED", got.Error.Code)
	assert.Empty(t, targetStore.created, "nothing moved")
}
