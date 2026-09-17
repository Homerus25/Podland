package machines

import (
	"context"
	"net/http"
	"time"

	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/web"
)

// MachineHandler handles machine requests
//
// Every handler crosses the endpoint envelope (internal/web): the
// {id}-scoped routes web.Handle, Create and List web.HandleUser (outside
// the subtree, no machine ID).
type MachineHandler struct {
	service *MachineService
}

// NewMachineHandler creates a new MachineHandler
func NewMachineHandler(service *MachineService) *MachineHandler {
	return &MachineHandler{
		service: service,
	}
}

// Create handles machine creation. The registration rules — field
// validation, the SSH key check, the synchronous connection probe — live
// in the service; the handler decodes the body and crosses the envelope.
func (h *MachineHandler) Create(w http.ResponseWriter, r *http.Request) {
	web.HandleUser(w, r, http.StatusCreated, func(ctx context.Context, userID string) (MachineResponse, error) {
		var req CreateMachineRequest
		if appErr := web.Decode(r, &req); appErr != nil {
			return MachineResponse{}, appErr
		}

		machine, err := h.service.Create(ctx, userID, &req)
		if err != nil {
			return MachineResponse{}, err
		}
		return FromModel(machine), nil
	})
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

// List handles listing the machines the caller can access
func (h *MachineHandler) List(w http.ResponseWriter, r *http.Request) {
	web.HandleUser(w, r, http.StatusOK, func(ctx context.Context, userID string) ([]MachineResponse, error) {
		machines, err := h.service.ListAllMachines(userID)
		if err != nil {
			return nil, err
		}

		// Convert to responses, carrying the caller's permission on each
		responses := make([]MachineResponse, len(machines))
		for i, machine := range machines {
			responses[i] = FromModel(machine.Machine)
			responses[i].Permission = machine.Permission
		}
		return responses, nil
	})
}

// Update handles updating a machine. The update rules — owner-only
// permission, partial merge, PEM validation, key encryption, the write —
// live in the service; the handler decodes the body and maps the response.
func (h *MachineHandler) Update(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (MachineResponse, error) {
		var req CreateMachineRequest
		if appErr := web.Decode(r, &req); appErr != nil {
			return MachineResponse{}, appErr
		}

		machine, err := h.service.Update(ctx, userID, machineID, &req)
		if err != nil {
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
		accesses, err := h.service.ListAccess(userID, machineID)
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

// GrantAccess handles granting access to a user. The grant rules —
// owner-only, valid permission, no duplicates — live in the service; the
// handler decodes the body and maps the response.
func (h *MachineHandler) GrantAccess(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusCreated, func(ctx context.Context, grantorID, machineID string) (AccessResponse, error) {
		var req GrantAccessRequest
		if appErr := web.Decode(r, &req); appErr != nil {
			return AccessResponse{}, appErr
		}

		access, err := h.service.GrantAccess(grantorID, machineID, &req)
		if err != nil {
			return AccessResponse{}, err
		}

		return AccessResponse{
			UserID:     access.UserID,
			Permission: access.Permission,
			GrantedAt:  access.GrantedAt.Format(time.RFC3339),
		}, nil
	})
}

// RevokeAccess handles revoking access from a user. The revoke rules —
// owner-only, no self-revoke — live in the service; the handler reads
// the URL param and crosses the envelope.
func (h *MachineHandler) RevokeAccess(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusNoContent, func(ctx context.Context, revokerID, machineID string) (struct{}, error) {
		targetUserID, guard := web.URLParam(r, "userId", "MISSING_USER_ID")
		if guard != nil {
			return struct{}{}, guard
		}

		return struct{}{}, h.service.RevokeAccess(revokerID, machineID, targetUserID)
	})
}
