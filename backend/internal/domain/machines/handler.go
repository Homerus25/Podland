package machines

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/web"
	"github.com/go-chi/chi/v5"
)

// MachineHandler handles machine requests
//
// The subtree handlers ({id}-scoped routes, behind RequireMachineAccess)
// cross the endpoint envelope (internal/web). Create and List sit
// outside the subtree with no machine ID and keep their own shape.
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
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (MachineResponse, error) {
		machine, err := h.service.GetMachineByID(userID, machineID)
		if err != nil {
			return MachineResponse{}, err
		}
		return FromModel(machine), nil
	})
}

// MachineStatusResponse is the payload of GET /api/machines/{id}/status.
// checkedAt is RFC3339 UTC. There is no third state: any probe failure is
// reported as "offline" and the underlying connection error is never
// included (machines are shared; SSH-level errors describe the owner's
// infrastructure).
type MachineStatusResponse struct {
	Status    string `json:"status"`
	CheckedAt string `json:"checkedAt"`
}

// Status handles the live status check for a machine
func (h *MachineHandler) Status(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (MachineStatusResponse, error) {
		status, checkedAt, err := h.service.GetMachineStatus(ctx, userID, machineID)
		if err != nil {
			return MachineStatusResponse{}, err
		}
		return MachineStatusResponse{
			Status:    status,
			CheckedAt: checkedAt.Format(time.RFC3339),
		}, nil
	})
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
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (MachineResponse, error) {
		var req CreateMachineRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return MachineResponse{}, errors.NewAppError("INVALID_REQUEST", err.Error(), http.StatusBadRequest)
		}

		// Get existing machine
		machine, err := h.service.GetMachineByID(userID, machineID)
		if err != nil {
			return MachineResponse{}, err
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
			if !IsValidPEM(req.SSHPrivateKey) {
				return MachineResponse{}, errors.ErrInvalidSSHKey
			}
			// Encrypt new SSH key
			encryptedKey, fingerprint, err := h.service.encryption.EncryptSSHKey(req.SSHPrivateKey)
			if err != nil {
				return MachineResponse{}, err
			}
			machine.SSHKeyEncrypted = encryptedKey
			machine.SSHKeyFingerprint = fingerprint
		}
		if req.PodmanSocketPath != "" {
			machine.PodmanSocketPath = req.PodmanSocketPath
		}
		machine.UpdatedAt = time.Now().UTC()

		if err := h.service.machineRepo.Update(machine); err != nil {
			return MachineResponse{}, err
		}

		return FromModel(machine), nil
	})
}

// Delete handles deleting a machine
func (h *MachineHandler) Delete(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusNoContent, func(ctx context.Context, userID, machineID string) (struct{}, error) {
		return struct{}{}, h.service.Delete(ctx, userID, machineID)
	})
}

// AccessResponse represents machine access in API responses
type AccessResponse struct {
	UserID     string `json:"userId"`
	Permission string `json:"permission"`
	GrantedAt  string `json:"grantedAt"`
}

// ListAccess handles listing access for a machine. Beyond the subtree
// gate, the service re-checks the caller's access row.
func (h *MachineHandler) ListAccess(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) ([]AccessResponse, error) {
		if !h.service.UserHasAccess(userID, machineID) {
			return nil, errors.ErrAccessDenied
		}

		accesses, err := h.service.accessRepo.ListByMachine(machineID)
		if err != nil {
			return nil, err
		}

		responses := make([]AccessResponse, len(accesses))
		for i, access := range accesses {
			responses[i] = AccessResponse{
				UserID:     access.UserID,
				Permission: access.Permission,
				GrantedAt:  access.GrantedAt.Format(time.RFC3339),
			}
		}
		return responses, nil
	})
}

// GrantAccessRequest represents a request to grant access to a machine
type GrantAccessRequest struct {
	UserID     string `json:"userId"`
	Permission string `json:"permission"`
}

// GrantAccess handles granting access to a user. Only the owner may
// grant, and an existing access row is a client error.
func (h *MachineHandler) GrantAccess(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusCreated, func(ctx context.Context, grantorID, machineID string) (AccessResponse, error) {
		var req GrantAccessRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return AccessResponse{}, errors.NewAppError("INVALID_REQUEST", err.Error(), http.StatusBadRequest)
		}

		if req.Permission != models.PermissionRead && req.Permission != models.PermissionWrite && req.Permission != models.PermissionAdmin {
			return AccessResponse{}, errors.NewAppError("INVALID_PERMISSION", "Permission must be read, write, or admin", http.StatusBadRequest)
		}

		machine, err := h.service.machineRepo.GetByID(machineID)
		if err != nil {
			return AccessResponse{}, err
		}
		if machine == nil {
			return AccessResponse{}, errors.ErrMachineNotFound
		}

		if machine.OwnerID != grantorID {
			return AccessResponse{}, errors.ErrAccessDenied
		}

		existingAccess, err := h.service.accessRepo.GetByMachineAndUser(machineID, req.UserID)
		if err != nil {
			return AccessResponse{}, err
		}
		if existingAccess != nil {
			return AccessResponse{}, errors.ErrAccessAlreadyGranted
		}

		access := &models.MachineAccess{
			MachineID:  machineID,
			UserID:     req.UserID,
			Permission: req.Permission,
			GrantedAt:  time.Now().UTC(),
		}

		if err := h.service.accessRepo.Create(access); err != nil {
			return AccessResponse{}, err
		}

		return AccessResponse{
			UserID:     access.UserID,
			Permission: access.Permission,
			GrantedAt:  access.GrantedAt.Format(time.RFC3339),
		}, nil
	})
}

// RevokeAccess handles revoking access from a user. Only the owner may
// revoke, and the owner's own access cannot be revoked.
func (h *MachineHandler) RevokeAccess(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusNoContent, func(ctx context.Context, revokerID, machineID string) (struct{}, error) {
		targetUserID := chi.URLParam(r, "userId")
		if targetUserID == "" {
			return struct{}{}, errors.NewAppError("MISSING_USER_ID", "User ID is required", http.StatusBadRequest)
		}

		machine, err := h.service.machineRepo.GetByID(machineID)
		if err != nil {
			return struct{}{}, err
		}
		if machine == nil {
			return struct{}{}, errors.ErrMachineNotFound
		}

		if machine.OwnerID != revokerID {
			return struct{}{}, errors.ErrAccessDenied
		}
		if targetUserID == revokerID {
			return struct{}{}, errors.ErrCannotRevokeOwner
		}

		return struct{}{}, h.service.accessRepo.Delete(machineID, targetUserID)
	})
}
