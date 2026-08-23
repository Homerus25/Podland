package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/Homerus25/podland/backend/internal/testutils"
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

func TestAuthHandler_Login_InvalidJSON(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	mockRepo := testutils.NewMockUserRepository()
	handler := NewAuthHandler(service, mockRepo)

	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader([]byte("invalid json")))
	w := httptest.NewRecorder()

	handler.Login(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should return 400 for invalid JSON")

	var errorResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		RequestID string `json:"requestId"`
	}
	err = json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err, "Should decode error response")
	assert.Equal(t, "INVALID_REQUEST", errorResp.Error.Code, "Should return INVALID_REQUEST error code")
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

