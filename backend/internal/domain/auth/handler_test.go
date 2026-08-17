package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/podland/backend/internal/models"
	"github.com/yourorg/podland/backend/internal/testutils"
	"golang.org/x/crypto/bcrypt"
)

func TestAuthHandler_Login_Success(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	mockRepo := testutils.NewMockUserRepository()
	handler := NewAuthHandler(service, mockRepo)

	password := "test-password-123"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err, "Should hash password without error")

	testUser := &models.User{
		ID:           "user-123",
		Username:     "testuser",
		PasswordHash: string(hashedPassword),
		CreatedAt:    time.Now(),
	}
	mockRepo.AddUser(testUser)

	loginReq := LoginRequest{
		Username: "testuser",
		Password: password,
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
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	mockRepo := testutils.NewMockUserRepository()
	handler := NewAuthHandler(service, mockRepo)

	correctPassword := "correct-password-123"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(correctPassword), bcrypt.DefaultCost)
	require.NoError(t, err, "Should hash password without error")

	testUser := &models.User{
		ID:           "user-123",
		Username:     "testuser",
		PasswordHash: string(hashedPassword),
		CreatedAt:    time.Now(),
	}
	mockRepo.AddUser(testUser)

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
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	mockRepo := testutils.NewMockUserRepository()
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
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	dbError := assert.AnError
	mockRepo := testutils.NewMockUserRepositoryWithErrors(nil, nil, dbError, nil, nil, nil)
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

func TestAuthHandler_Login_TokenGenerationError(t *testing.T) {
	service, err := NewTestAuthService()
	require.NoError(t, err, "Should create auth service without error")

	mockRepo := testutils.NewMockUserRepository()
	handler := NewAuthHandler(service, mockRepo)

	password := "test-password-123"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err, "Should hash password without error")

	testUser := &models.User{
		ID:           "user-123",
		Username:     "testuser",
		PasswordHash: string(hashedPassword),
		CreatedAt:    time.Now(),
	}
	mockRepo.AddUser(testUser)

	loginReq := LoginRequest{
		Username: "testuser",
		Password: password,
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
