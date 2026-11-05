package hosts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// enrichedRow mirrors the wire shape of the enrichment endpoints' body:
// one GET /api/hosts element, or the GET /api/hosts/{id} body.
// IsOwner is a pointer so the decode distinguishes "present, false"
// from "absent": the field must be present on every response, with an
// explicit false for grantees.
type enrichedRow struct {
	HostResponse
	IsOwner *bool `json:"isOwner"`
}

// listRequest serves GET /api/hosts as userID over the real list
// handler and service, with mock repositories seeded by seed.
func listRequest(t *testing.T, userID string, seed func(repo *testutils.MockHostRepository)) *httptest.ResponseRecorder {
	t.Helper()

	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	if seed != nil {
		seed(hostRepo)
	}
	service := NewHostService(hostRepo, accessRepo, testutils.NewMockConnectionManager(), testutils.NewMockEncryptionService(), newFakeSSHInvalidator())

	return testutils.Perform(NewHostHandler(service).List, testutils.Request{
		Method: http.MethodGet,
		Path:   "/api/hosts",
		UserID: userID,
	})
}

func newListedHost(id, ownerID string) *models.Host {
	return &models.Host{
		ID:        id,
		Name:      "m-" + id,
		OwnerID:   ownerID,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
}

func decodeListRows(t *testing.T, body string) []enrichedRow {
	t.Helper()
	var rows []enrichedRow
	require.NoError(t, json.Unmarshal([]byte(body), &rows))
	return rows
}

// getRequest serves GET /api/hosts/{id} as userID over the real get
// handler and service, with mock repositories seeded by seed.
func getRequest(t *testing.T, userID string, seed func(hostRepo *testutils.MockHostRepository, accessRepo *testutils.MockHostAccessRepository)) *httptest.ResponseRecorder {
	t.Helper()

	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	if seed != nil {
		seed(hostRepo, accessRepo)
	}
	service := NewHostService(hostRepo, accessRepo, testutils.NewMockConnectionManager(), testutils.NewMockEncryptionService(), newFakeSSHInvalidator())

	return testutils.Perform(NewHostHandler(service).Get, testutils.Request{
		Method:    http.MethodGet,
		Path:      "/api/hosts/host-1",
		UserID:    userID,
		URLParams: map[string]string{"id": "host-1"},
	})
}

// TestHostHandler_Get_CallerFacts pins the detail endpoint's
// per-caller enrichment: the response carries the caller's stored
// permission and isOwner exactly as the list rows do, so gating facts
// are always present on the host object.
func TestHostHandler_Get_CallerFacts(t *testing.T) {
	t.Run("owner reads their admin row and isOwner true", func(t *testing.T) {
		w := getRequest(t, "owner-1", func(hostRepo *testutils.MockHostRepository, accessRepo *testutils.MockHostAccessRepository) {
			hostRepo.AddHost(newListedHost("host-1", "owner-1"))
			accessRepo.AddAccess("host-1", "owner-1", models.PermissionAdmin, time.Now().UTC())
		})

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		var row enrichedRow
		require.NoError(t, json.Unmarshal([]byte(w.Body.String()), &row))
		require.NotNil(t, row.IsOwner, "isOwner must be present on the detail response")
		assert.True(t, *row.IsOwner)
		assert.Equal(t, models.PermissionAdmin, row.Permission)
	})

	t.Run("grantees read the granted level and an explicit isOwner false", func(t *testing.T) {
		for _, level := range []string{models.PermissionRead, models.PermissionWrite, models.PermissionAdmin} {
			t.Run(level, func(t *testing.T) {
				w := getRequest(t, "grantee-1", func(hostRepo *testutils.MockHostRepository, accessRepo *testutils.MockHostAccessRepository) {
					hostRepo.AddHost(newListedHost("host-1", "owner-1"))
					accessRepo.AddAccess("host-1", "grantee-1", level, time.Now().UTC())
				})

				require.Equal(t, http.StatusOK, w.Result().StatusCode)
				var row enrichedRow
				require.NoError(t, json.Unmarshal([]byte(w.Body.String()), &row))
				require.NotNil(t, row.IsOwner, "isOwner must be present on the detail response")
				assert.False(t, *row.IsOwner)
				assert.Equal(t, level, row.Permission)
			})
		}
	})
}

// TestHostHandler_List_IsOwner pins the list endpoint's ownership
// enrichment: isOwner is present on every row and true exactly when the
// caller is the host's owner.
func TestHostHandler_List_IsOwner(t *testing.T) {
	t.Run("owner sees isOwner true on their host", func(t *testing.T) {
		w := listRequest(t, "owner-1", func(repo *testutils.MockHostRepository) {
			repo.AddHost(newListedHost("host-1", "owner-1"))
		})

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		rows := decodeListRows(t, w.Body.String())
		require.Len(t, rows, 1)
		require.NotNil(t, rows[0].IsOwner, "isOwner must be present on every row")
		assert.True(t, *rows[0].IsOwner)
	})

	t.Run("grantees see an explicit isOwner false at every level", func(t *testing.T) {
		for _, level := range []string{models.PermissionRead, models.PermissionWrite, models.PermissionAdmin} {
			t.Run(level, func(t *testing.T) {
				w := listRequest(t, "grantee-1", func(repo *testutils.MockHostRepository) {
					repo.AddAccessibleHost(models.AccessibleHost{
						Host:       newListedHost("host-1", "owner-1"),
						Permission: level,
					})
				})

				require.Equal(t, http.StatusOK, w.Result().StatusCode)
				rows := decodeListRows(t, w.Body.String())
				require.Len(t, rows, 1)
				require.NotNil(t, rows[0].IsOwner, "isOwner must be present on every row")
				assert.False(t, *rows[0].IsOwner)
				assert.Equal(t, level, rows[0].Permission)
			})
		}
	})

	t.Run("one list answers per host", func(t *testing.T) {
		w := listRequest(t, "caller-1", func(repo *testutils.MockHostRepository) {
			repo.AddHost(newListedHost("owned-1", "caller-1"))
			repo.AddAccessibleHost(models.AccessibleHost{
				Host:       newListedHost("shared-1", "someone-else"),
				Permission: models.PermissionWrite,
			})
		})

		require.Equal(t, http.StatusOK, w.Result().StatusCode)
		rows := decodeListRows(t, w.Body.String())
		require.Len(t, rows, 2)

		byID := make(map[string]enrichedRow, len(rows))
		for _, row := range rows {
			byID[row.ID] = row
		}
		require.Contains(t, byID, "owned-1")
		require.Contains(t, byID, "shared-1")
		require.NotNil(t, byID["owned-1"].IsOwner)
		require.NotNil(t, byID["shared-1"].IsOwner)
		assert.True(t, *byID["owned-1"].IsOwner, "the caller's own host must read isOwner true")
		assert.False(t, *byID["shared-1"].IsOwner, "a granted host must read isOwner false")
	})
}
