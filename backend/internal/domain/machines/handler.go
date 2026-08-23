package machines

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
)

// MachineHandler handles machine requests
type MachineHandler struct {
	service *MachineService
}

// NewMachineHandler creates a new MachineHandler
func NewMachineHandler(service *MachineService) *MachineHandler {
	return &MachineHandler{
		service: service,
	}
}

// Create handles machine creation
func (h *MachineHandler) Create(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context
	userID := contextutils.GetUserIDFromContext(r.Context())
	if userID == "" {
		errors.RespondWithError(w, errors.ErrInvalidCredentials, contextutils.GetRequestID(r.Context()))
		return
	}

	// Parse request body
	var req CreateMachineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), contextutils.GetRequestID(r.Context()))
		return
	}

	// Create machine through service
	machine, err := h.service.Create(r.Context(), userID, &req)
	if err != nil {
		// Handle specific error types
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		// Handle generic errors
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(FromModel(machine))
}

// Get handles getting a machine by ID
func (h *MachineHandler) Get(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context
	userID := contextutils.GetUserIDFromContext(r.Context())
	if userID == "" {
		errors.RespondWithError(w, errors.ErrInvalidCredentials, contextutils.GetRequestID(r.Context()))
		return
	}

	// Get machine ID from URL parameter
	machineID := chi.URLParam(r, "id")
	if machineID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_MACHINE_ID", "Machine ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	// Get machine through service
	machine, err := h.service.GetMachineByID(userID, machineID)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(FromModel(machine))
}

// List handles listing machines
func (h *MachineHandler) List(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context
	userID := contextutils.GetUserIDFromContext(r.Context())
	if userID == "" {
		errors.RespondWithError(w, errors.ErrInvalidCredentials, contextutils.GetRequestID(r.Context()))
		return
	}

	// List machines accessible to user
	machines, err := h.service.ListAllMachines(userID)
	if err != nil {
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		return
	}

	// Convert to responses, carrying the caller's permission on each
	responses := make([]MachineResponse, len(machines))
	for i, machine := range machines {
		responses[i] = FromModel(machine.Machine)
		responses[i].Permission = machine.Permission
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses)
}

// Update handles updating a machine
func (h *MachineHandler) Update(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context
	userID := contextutils.GetUserIDFromContext(r.Context())
	if userID == "" {
		errors.RespondWithError(w, errors.ErrInvalidCredentials, contextutils.GetRequestID(r.Context()))
		return
	}

	// Get machine ID from URL parameter
	machineID := chi.URLParam(r, "id")
	if machineID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_MACHINE_ID", "Machine ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	// Parse request body
	var req CreateMachineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), contextutils.GetRequestID(r.Context()))
		return
	}

	// Get existing machine
	machine, err := h.service.GetMachineByID(userID, machineID)
	if err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		return
	}

	// Update machine fields
	if req.Name != "" {
		machine.Name = req.Name
	}
	if req.Description != "" {
		machine.Description = req.Description
	}
	if req.Hostname != "" {
		machine.Hostname = req.Hostname
	}
	if req.Port != 0 {
		machine.Port = req.Port
	}
	if req.SSHUsername != "" {
		machine.SSHUsername = req.SSHUsername
	}
	if req.SSHPrivateKey != "" {
		// Validate SSH key
		if !IsValidPEM(req.SSHPrivateKey) {
			errors.RespondWithError(w, errors.ErrInvalidSSHKey, contextutils.GetRequestID(r.Context()))
			return
		}
		// Encrypt new SSH key
		encryptedKey, fingerprint, err := h.service.encryption.EncryptSSHKey(req.SSHPrivateKey)
		if err != nil {
			errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
			return
		}
		machine.SSHKeyEncrypted = encryptedKey
		machine.SSHKeyFingerprint = fingerprint
	}
	if req.PodmanSocketPath != "" {
		machine.PodmanSocketPath = req.PodmanSocketPath
	}
	machine.UpdatedAt = time.Now().UTC()

	// Update machine in database
	if err := h.service.machineRepo.Update(machine); err != nil {
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(FromModel(machine))
}

// Delete handles deleting a machine
func (h *MachineHandler) Delete(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context
	userID := contextutils.GetUserIDFromContext(r.Context())
	if userID == "" {
		errors.RespondWithError(w, errors.ErrInvalidCredentials, contextutils.GetRequestID(r.Context()))
		return
	}

	// Get machine ID from URL parameter
	machineID := chi.URLParam(r, "id")
	if machineID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_MACHINE_ID", "Machine ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	// Delete machine through service
	if err := h.service.Delete(r.Context(), userID, machineID); err != nil {
		if appErr, ok := err.(*errors.AppError); ok {
			errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
			return
		}
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		return
	}

	// Return success response
	w.WriteHeader(http.StatusNoContent)
}

// AccessResponse represents machine access in API responses
type AccessResponse struct {
	UserID     string `json:"userId"`
	Permission string `json:"permission"`
	GrantedAt  string `json:"grantedAt"`
}

// ListAccess handles listing access for a machine
func (h *MachineHandler) ListAccess(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context
	userID := contextutils.GetUserIDFromContext(r.Context())
	if userID == "" {
		errors.RespondWithError(w, errors.ErrInvalidCredentials, contextutils.GetRequestID(r.Context()))
		return
	}

	// Get machine ID from URL parameter
	machineID := chi.URLParam(r, "id")
	if machineID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_MACHINE_ID", "Machine ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	// Verify user has access to machine
	if !h.service.UserHasAccess(userID, machineID) {
		errors.RespondWithError(w, errors.ErrAccessDenied, contextutils.GetRequestID(r.Context()))
		return
	}

	// Get machine access entries
	accesses, err := h.service.accessRepo.ListByMachine(machineID)
	if err != nil {
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		return
	}

	// Convert to responses
	responses := make([]AccessResponse, len(accesses))
	for i, access := range accesses {
		responses[i] = AccessResponse{
			UserID:     access.UserID,
			Permission: access.Permission,
			GrantedAt:  access.GrantedAt.Format(time.RFC3339),
		}
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses)
}

// GrantAccessRequest represents a request to grant access to a machine
type GrantAccessRequest struct {
	UserID     string `json:"userId"`
	Permission string `json:"permission"`
}

// GrantAccess handles granting access to a user
func (h *MachineHandler) GrantAccess(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context (current user - the grantor)
	grantorID := contextutils.GetUserIDFromContext(r.Context())
	if grantorID == "" {
		errors.RespondWithError(w, errors.ErrInvalidCredentials, contextutils.GetRequestID(r.Context()))
		return
	}

	// Get machine ID from URL parameter
	machineID := chi.URLParam(r, "id")
	if machineID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_MACHINE_ID", "Machine ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	// Parse request body
	var req GrantAccessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), contextutils.GetRequestID(r.Context()))
		return
	}

	// Validate permission
	if req.Permission != models.PermissionRead && req.Permission != models.PermissionWrite && req.Permission != models.PermissionAdmin {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "INVALID_PERMISSION", "Permission must be read, write, or admin", contextutils.GetRequestID(r.Context()))
		return
	}

	// Get machine
	machine, err := h.service.machineRepo.GetByID(machineID)
	if err != nil {
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		return
	}
	if machine == nil {
		errors.RespondWithError(w, errors.ErrMachineNotFound, contextutils.GetRequestID(r.Context()))
		return
	}

	// Only owner can grant access
	if machine.OwnerID != grantorID {
		errors.RespondWithError(w, errors.ErrAccessDenied, contextutils.GetRequestID(r.Context()))
		return
	}

	// Check if access already exists
	existingAccess, err := h.service.accessRepo.GetByMachineAndUser(machineID, req.UserID)
	if err != nil {
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		return
	}
	if existingAccess != nil {
		errors.RespondWithError(w, errors.ErrAccessAlreadyGranted, contextutils.GetRequestID(r.Context()))
		return
	}

	// Create access entry
	access := &models.MachineAccess{
		MachineID:  machineID,
		UserID:     req.UserID,
		Permission: req.Permission,
		GrantedAt:  time.Now().UTC(),
	}

	if err := h.service.accessRepo.Create(access); err != nil {
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(AccessResponse{
		UserID:     access.UserID,
		Permission: access.Permission,
		GrantedAt:  access.GrantedAt.Format(time.RFC3339),
	})
}

// RevokeAccess handles revoking access from a user
func (h *MachineHandler) RevokeAccess(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context (current user - the revoker)
	revokerID := contextutils.GetUserIDFromContext(r.Context())
	if revokerID == "" {
		errors.RespondWithError(w, errors.ErrInvalidCredentials, contextutils.GetRequestID(r.Context()))
		return
	}

	// Get machine ID from URL parameter
	machineID := chi.URLParam(r, "id")
	if machineID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_MACHINE_ID", "Machine ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	// Get user ID to revoke from URL parameter
	targetUserID := chi.URLParam(r, "userId")
	if targetUserID == "" {
		errors.RespondWithHTTPError(w, http.StatusBadRequest, "MISSING_USER_ID", "User ID is required", contextutils.GetRequestID(r.Context()))
		return
	}

	// Get machine
	machine, err := h.service.machineRepo.GetByID(machineID)
	if err != nil {
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		return
	}
	if machine == nil {
		errors.RespondWithError(w, errors.ErrMachineNotFound, contextutils.GetRequestID(r.Context()))
		return
	}

	// Only owner can revoke access, and cannot revoke their own access
	if machine.OwnerID != revokerID {
		errors.RespondWithError(w, errors.ErrAccessDenied, contextutils.GetRequestID(r.Context()))
		return
	}
	if targetUserID == revokerID {
		errors.RespondWithError(w, errors.ErrCannotRevokeOwner, contextutils.GetRequestID(r.Context()))
		return
	}

	// Revoke access
	if err := h.service.accessRepo.Delete(machineID, targetUserID); err != nil {
		errors.RespondWithError(w, errors.ErrInternalError, contextutils.GetRequestID(r.Context()))
		return
	}

	// Return success response
	w.WriteHeader(http.StatusNoContent)
}
