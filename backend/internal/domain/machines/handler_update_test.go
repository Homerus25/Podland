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

// updateRequest serves PUT /api/machines/{id} through the handler over the
// real service (mock repos), mirroring the production wiring: the
// RequireMachineAccess subtree stands in via testutils.Perform, and the
// seeded machine has owner-1 with an admin access row.
func updateRequest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()

	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()
	service := NewMachineHandler(NewMachineService(machineRepo, accessRepo, testutils.NewMockConnectionManager(), testutils.NewMockEncryptionService()))
	machineRepo.AddMachine(&models.Machine{
		ID:                "machine-1",
		Name:              "old-name",
		Hostname:          "old.example.com",
		Port:              22,
		SSHUsername:       "old-user",
		SSHKeyEncrypted:   []byte("old-encrypted"),
		SSHKeyFingerprint: "old-fingerprint",
		OwnerID:           "owner-1",
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	})
	accessRepo.AddAccess("machine-1", "owner-1", models.PermissionAdmin, time.Now().UTC())

	return testutils.Perform(service.Update, testutils.Request{
		Method:    http.MethodPut,
		Path:      "/api/machines/machine-1",
		UserID:    "owner-1",
		URLParams: map[string]string{"id": "machine-1"},
		Body:      strings.NewReader(body),
	})
}

// TestMachineHandler_Update_MalformedBody pins the one contract the
// handler owns on Update: an undecodable JSON body answers 400
// INVALID_REQUEST. Every other rule lives behind the service and is
// covered at the service seam.
func TestMachineHandler_Update_MalformedBody(t *testing.T) {
	w := updateRequest(t, "{not json")

	require.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
	assert.Contains(t, w.Body.String(), "INVALID_REQUEST")
}
