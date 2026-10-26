package auth

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Homerus25/podland/backend/internal/failurebudget"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decodeErrorEnvelope decodes the structured error envelope.
func decodeErrorEnvelope(t *testing.T, resp *http.Response) (code string) {
	t.Helper()
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope), "Should decode the error envelope")
	return envelope.Error.Code
}

// TestAuthHandler_Login_FailureBudget429CarriesHeaders pins the wire
// contract of the lockout: once the budget is spent, the login — even
// with the correct password — answers 429 RATE_LIMIT_EXCEEDED with the
// X-RateLimit trio of the username budget, while in-budget failures
// stay plain 401s without rate-limit headers.
func TestAuthHandler_Login_FailureBudget429CarriesHeaders(t *testing.T) {
	mockRepo := seedLoginUser(t, "testuser", "right-password")
	budget := newFakeBudget(1)
	// One surface value handed to the service and the handler, the way
	// internal/app wires it.
	loginSurface := failurebudget.NewSurface("login", budget)
	service, err := NewTestAuthServiceWithFailureBudget(mockRepo, testutils.NewMockSessionRepository(), loginSurface, nil)
	require.NoError(t, err, "Should create auth service with failure budget")
	handler := NewAuthHandler(service, mockRepo).WithLoginBudget(loginSurface)

	// In-budget failure: a plain 401, no rate-limit advertisement.
	resp := performLogin(handler, map[string]any{"username": "testuser", "password": "wrong-password"})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "The in-budget failure should stay a 401")
	assert.Empty(t, resp.Header.Get("X-RateLimit-Limit"), "A plain 401 must not carry rate-limit headers")
	require.NoError(t, resp.Body.Close(), "Should close the 401 body")

	// The budget is spent: even the correct password is locked out.
	resp = performLogin(handler, map[string]any{"username": "testuser", "password": "right-password"})
	defer resp.Body.Close()
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "The locked-out login should answer 429")
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", decodeErrorEnvelope(t, resp), "The 429 should carry the rate-limit code")
	assert.Equal(t, "1", resp.Header.Get("X-RateLimit-Limit"), "The 429 should advertise the configured budget")
	assert.Equal(t, "0", resp.Header.Get("X-RateLimit-Remaining"), "The 429 should report no remaining budget")
	assert.Equal(t, "300", resp.Header.Get("X-RateLimit-Reset"), "The 429 should report the window reset in seconds")
}

// TestAuthHandler_ChangePassword_FailureBudget429CarriesHeaders pins
// the same wire contract on the password-change path, keyed by the
// caller.
func TestAuthHandler_ChangePassword_FailureBudget429CarriesHeaders(t *testing.T) {
	repo, sessionRepo, _, userID := seedPasswordChangeUser(t, "testuser", "old-password-123")
	budget := newFakeBudget(1)
	// One surface value handed to the service and the handler, the way
	// internal/app wires it.
	changeSurface := failurebudget.NewSurface("change-password", budget)
	service, err := NewTestAuthServiceWithFailureBudget(repo, sessionRepo, nil, changeSurface)
	require.NoError(t, err, "Should create auth service with failure budget")
	handler := NewAuthHandler(service, repo).WithChangeBudget(changeSurface)

	resp := performChangePassword(t, handler, userID, changePasswordBody{
		CurrentPassword: "wrong-current",
		NewPassword:     "new-pass-8",
	})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "The in-budget wrong attempt should stay a 401")
	assert.Empty(t, resp.Header.Get("X-RateLimit-Limit"), "A plain 401 must not carry rate-limit headers")
	require.NoError(t, resp.Body.Close(), "Should close the 401 body")

	resp = performChangePassword(t, handler, userID, changePasswordBody{
		CurrentPassword: "old-password-123",
		NewPassword:     "new-pass-8",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "The locked-out change should answer 429")
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", decodeErrorEnvelope(t, resp), "The 429 should carry the rate-limit code")
	assert.Equal(t, "1", resp.Header.Get("X-RateLimit-Limit"), "The 429 should advertise the configured budget")
	assert.Equal(t, "0", resp.Header.Get("X-RateLimit-Remaining"), "The 429 should report no remaining budget")
	assert.Equal(t, "300", resp.Header.Get("X-RateLimit-Reset"), "The 429 should report the window reset in seconds")
}
