package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestAuthHandler_Login_Success(t *testing.T) {
	mockRepo := seedLoginUser(t, "testuser", "test-password-123")
	service, err := NewTestAuthServiceWithUserRepo(mockRepo)
	require.NoError(t, err, "Should create auth service without error")

	handler := NewAuthHandler(service, mockRepo)

	loginReq := LoginRequest{
		Username: "testuser",
		Password: "test-password-123",
	}

	reqBody, err := json.Marshal(loginReq)
	require.NoError(t, err, "Should marshal request without error")

	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.Login(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Should return 200 OK for correct credentials")

	var loginResp LoginResponse
	err = json.NewDecoder(resp.Body).Decode(&loginResp)
	require.NoError(t, err, "Should decode response without error")
	assert.NotEmpty(t, loginResp.Token, "Response should contain a token")
	assert.Equal(t, "testuser", loginResp.Username, "Response should contain the authenticated user's username")
	assert.Equal(t, models.RoleUser, loginResp.Role, "Response should contain the authenticated user's role")
}

// TestAuthHandler_Login_AdminRoleCarried asserts the login response
// reports "admin" for an admin account, so the client can gate admin
// surfaces straight from the login body without a status round trip.
func TestAuthHandler_Login_AdminRoleCarried(t *testing.T) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("test-password-123"), bcrypt.DefaultCost)
	require.NoError(t, err, "Should hash password without error")

	mockRepo := testutils.NewMockUserRepository()
	mockRepo.AddUser(&models.User{
		ID:           "admin-1",
		Username:     "root",
		PasswordHash: string(hashedPassword),
		Role:         models.RoleAdmin,
		CreatedAt:    time.Now(),
	})

	service, err := NewTestAuthServiceWithUserRepo(mockRepo)
	require.NoError(t, err, "Should create auth service without error")
	handler := NewAuthHandler(service, mockRepo)

	reqBody, err := json.Marshal(LoginRequest{Username: "root", Password: "test-password-123"})
	require.NoError(t, err, "Should marshal request without error")

	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.Login(w, req)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Should return 200 OK for correct credentials")

	var loginResp LoginResponse
	err = json.NewDecoder(resp.Body).Decode(&loginResp)
	require.NoError(t, err, "Should decode response without error")
	assert.Equal(t, models.RoleAdmin, loginResp.Role, "Response should report the admin role")
}

func TestAuthHandler_Login_WrongPassword(t *testing.T) {
	mockRepo := seedLoginUser(t, "testuser", "correct-password-123")
	service, err := NewTestAuthServiceWithUserRepo(mockRepo)
	require.NoError(t, err, "Should create auth service without error")

	handler := NewAuthHandler(service, mockRepo)

	loginReq := LoginRequest{
		Username: "testuser",
		Password: "wrong-password-456",
	}

	reqBody, err := json.Marshal(loginReq)
	require.NoError(t, err, "Should marshal request without error")

	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.Login(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Should return 401 for wrong password")

	var errorResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		RequestID string `json:"requestId"`
	}
	err = json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err, "Should decode error response")
	assert.Equal(t, "INVALID_CREDENTIALS", errorResp.Error.Code, "Should return INVALID_CREDENTIALS error code")
}

func TestAuthHandler_Login_UnknownUser(t *testing.T) {
	mockRepo := testutils.NewMockUserRepository()
	service, err := NewTestAuthServiceWithUserRepo(mockRepo)
	require.NoError(t, err, "Should create auth service without error")

	handler := NewAuthHandler(service, mockRepo)

	loginReq := LoginRequest{
		Username: "nonexistent-user",
		Password: "some-password",
	}

	reqBody, err := json.Marshal(loginReq)
	require.NoError(t, err, "Should marshal request without error")

	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.Login(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Should return 401 for unknown user")

	var errorResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		RequestID string `json:"requestId"`
	}
	err = json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err, "Should decode error response")
	assert.Equal(t, "INVALID_CREDENTIALS", errorResp.Error.Code, "Should return INVALID_CREDENTIALS error code")
}

func TestAuthHandler_Login_EmptyUsername(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	mockRepo := testutils.NewMockUserRepository()
	handler := NewAuthHandler(service, mockRepo)

	loginReq := LoginRequest{
		Username: "",
		Password: "some-password",
	}

	reqBody, err := json.Marshal(loginReq)
	require.NoError(t, err, "Should marshal request without error")

	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.Login(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should return 400 for empty username")

	var errorResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		RequestID string `json:"requestId"`
	}
	err = json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err, "Should decode error response")
	assert.Equal(t, "VALIDATION_FAILED", errorResp.Error.Code, "Should return VALIDATION_FAILED error code")
}

func TestAuthHandler_Login_EmptyPassword(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	mockRepo := testutils.NewMockUserRepository()
	handler := NewAuthHandler(service, mockRepo)

	loginReq := LoginRequest{
		Username: "testuser",
		Password: "",
	}

	reqBody, err := json.Marshal(loginReq)
	require.NoError(t, err, "Should marshal request without error")

	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.Login(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should return 400 for empty password")

	var errorResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		RequestID string `json:"requestId"`
	}
	err = json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err, "Should decode error response")
	assert.Equal(t, "VALIDATION_FAILED", errorResp.Error.Code, "Should return VALIDATION_FAILED error code")
}

func TestAuthHandler_Login_DatabaseError(t *testing.T) {
	dbError := assert.AnError
	mockRepo := testutils.NewMockUserRepositoryWithErrors(nil, nil, dbError, nil, nil, nil)
	service, err := NewTestAuthServiceWithUserRepo(mockRepo)
	require.NoError(t, err, "Should create auth service without error")

	handler := NewAuthHandler(service, mockRepo)

	loginReq := LoginRequest{
		Username: "testuser",
		Password: "some-password",
	}

	reqBody, err := json.Marshal(loginReq)
	require.NoError(t, err, "Should marshal request without error")

	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	handler.Login(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "Should return 500 for database error")

	var errorResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		RequestID string `json:"requestId"`
	}
	err = json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err, "Should decode error response")
	assert.Equal(t, "INTERNAL_ERROR", errorResp.Error.Code, "Should return INTERNAL_ERROR error code")
}
