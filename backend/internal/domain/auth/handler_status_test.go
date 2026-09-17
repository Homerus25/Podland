package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuthHandler_Status_AnonymousGetsUnauthenticated pins the
// anonymous branch through the real handler: no user ID in the request
// context is a legal state (the route is served without a token too),
// and the endpoint answers {authenticated: false} with 200 — the
// frontend's first-load probe.
func TestAuthHandler_Status_AnonymousGetsUnauthenticated(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	handler := NewAuthHandler(service, testutils.NewMockUserRepository())

	req := httptest.NewRequest("GET", "/api/auth/status", nil)
	w := httptest.NewRecorder()

	handler.Status(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Anonymous status is a valid answer, not an error")

	var statusResp StatusResponse
	err = json.NewDecoder(resp.Body).Decode(&statusResp)
	require.NoError(t, err, "Should decode status response")

	assert.False(t, statusResp.Authenticated, "A caller with no user ID must report unauthenticated")
	assert.Empty(t, statusResp.UserID, "An anonymous status response carries no user ID")
	assert.Empty(t, statusResp.Role, "An anonymous status response carries no role")
}

// TestAuthHandler_Status_IncludesUserRole asserts that the status
// response reports the authenticated user's role, read from the user
// record rather than assumed. An admin must see "admin".
func TestAuthHandler_Status_IncludesUserRole(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	mockRepo := testutils.NewMockUserRepository()
	handler := NewAuthHandler(service, mockRepo)

	mockRepo.AddUser(&models.User{
		ID:           "user-status-1",
		Username:     "statusadmin",
		PasswordHash: "hash",
		Role:         models.RoleAdmin,
		CreatedAt:    time.Now(),
	})

	req := httptest.NewRequest("GET", "/api/auth/status", nil)
	req = req.WithContext(contextutils.SetUserID(req.Context(), "user-status-1"))
	w := httptest.NewRecorder()

	handler.Status(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Should return 200 OK for authenticated user")

	var statusResp StatusResponse
	err = json.NewDecoder(resp.Body).Decode(&statusResp)
	require.NoError(t, err, "Should decode status response")

	assert.True(t, statusResp.Authenticated, "Should report authenticated")
	assert.Equal(t, "user-status-1", statusResp.UserID, "Should report the caller's user ID")
	assert.Equal(t, models.RoleAdmin, statusResp.Role, "Should report the stored role for an admin user")
}

// TestAuthHandler_Status_RegularUserRole asserts the same lookup path
// reports "user" for a default (non-admin) account, so the field is not
// hardcoded to a single value.
func TestAuthHandler_Status_RegularUserRole(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	mockRepo := testutils.NewMockUserRepository()
	handler := NewAuthHandler(service, mockRepo)

	mockRepo.AddUser(&models.User{
		ID:           "user-status-2",
		Username:     "statususer",
		PasswordHash: "hash",
		Role:         models.RoleUser,
		CreatedAt:    time.Now(),
	})

	req := httptest.NewRequest("GET", "/api/auth/status", nil)
	req = req.WithContext(contextutils.SetUserID(req.Context(), "user-status-2"))
	w := httptest.NewRecorder()

	handler.Status(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Should return 200 OK for authenticated user")

	var statusResp StatusResponse
	err = json.NewDecoder(resp.Body).Decode(&statusResp)
	require.NoError(t, err, "Should decode status response")

	assert.Equal(t, models.RoleUser, statusResp.Role, "Should report user for a default account")
}
