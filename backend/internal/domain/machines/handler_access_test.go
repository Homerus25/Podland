package machines

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// grantRequest serves POST /api/machines/{id}/access as owner-1.
func grantRequest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()

	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()
	service := NewMachineHandler(NewMachineService(machineRepo, accessRepo, testutils.NewMockConnectionManager(), testutils.NewMockEncryptionService(), newFakeSSHInvalidator()))
	machineRepo.AddMachine(&models.Machine{
		ID:        "machine-1",
		Name:      "shared-machine",
		OwnerID:   "owner-1",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	accessRepo.AddAccess("machine-1", "owner-1", models.PermissionAdmin, time.Now().UTC())

	return testutils.Perform(service.GrantAccess, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/machines/machine-1/access",
		UserID:    "owner-1",
		URLParams: map[string]string{"id": "machine-1"},
		Body:      strings.NewReader(body),
	})
}

// TestMachineHandler_GrantAccess pins what the handler owns on the grant
// endpoint: an invalid permission value crosses to the service's 400
// INVALID_PERMISSION. Every other grant rule (including body decoding)
// lives behind the service and web.Decode.
func TestMachineHandler_GrantAccess(t *testing.T) {
	t.Run("invalid permission value", func(t *testing.T) {
		w := grantRequest(t, `{"userId":"reader-1","permission":"superuser"}`)

		require.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
		assert.Contains(t, w.Body.String(), "INVALID_PERMISSION")
	})
}
