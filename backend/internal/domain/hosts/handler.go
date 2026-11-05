package hosts

import (
	"context"
	"net/http"
	"time"

	"github.com/Homerus25/podland/backend/internal/web"
)

// HostHandler handles host requests
//
// Every handler crosses the endpoint envelope (internal/web): the
// {id}-scoped routes web.Handle, Create and List web.HandleUser (outside
// the subtree, no host ID).
type HostHandler struct {
	service *HostService
}

// NewHostHandler creates a new HostHandler
func NewHostHandler(service *HostService) *HostHandler {
	return &HostHandler{
		service: service,
	}
}

// Create handles host creation. The registration rules — field
// validation, the SSH key check, the synchronous connection probe — live
// in the service; the handler decodes the body and crosses the envelope.
func (h *HostHandler) Create(w http.ResponseWriter, r *http.Request) {
	web.HandleUser(w, r, http.StatusCreated, func(ctx context.Context, userID string) (HostResponse, error) {
		var req CreateHostRequest
		if appErr := web.Decode(w, r, &req); appErr != nil {
			return HostResponse{}, appErr
		}

		host, err := h.service.Create(ctx, userID, &req)
		if err != nil {
			return HostResponse{}, err
		}
		return FromModel(host), nil
	})
}

// Get handles getting a host by ID. The service returns the host
// with the caller's stored permission; the response carries the same
// per-caller enrichment as the list (FromAccessibleHost), so the
// client's gating facts are always present on the host object.
func (h *HostHandler) Get(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (AccessibleHostResponse, error) {
		host, err := h.service.GetHostByID(userID, hostID)
		if err != nil {
			return AccessibleHostResponse{}, err
		}
		return FromAccessibleHost(*host, userID), nil
	})
}

// HostStatusResponse is the payload of GET /api/hosts/{id}/status.
// checkedAt is RFC3339 UTC. There is no third state: any probe failure is
// reported as "offline" and the underlying connection error is never
// included (hosts are shared; SSH-level errors describe the owner's
// infrastructure).
type HostStatusResponse struct {
	Status    string `json:"status"`
	CheckedAt string `json:"checkedAt"`
}

// Status handles the live status check for a host
func (h *HostHandler) Status(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (HostStatusResponse, error) {
		status, checkedAt, err := h.service.GetHostStatus(ctx, userID, hostID)
		if err != nil {
			return HostStatusResponse{}, err
		}
		return HostStatusResponse{
			Status:    status,
			CheckedAt: checkedAt.Format(time.RFC3339),
		}, nil
	})
}

// HostVersionResponse is the payload of GET /api/hosts/{id}/version.
// Both fields are null when the remote Podman could not be asked — offline
// is a valid answer, not an error, and the underlying failure is logged
// server-side only (the status endpoint's rule: SSH-level errors describe
// the owner's infrastructure).
type HostVersionResponse struct {
	PodmanVersion *string `json:"podmanVersion"`
	APIVersion    *string `json:"apiVersion"`
}

// Version handles the live version check for a host
func (h *HostHandler) Version(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (HostVersionResponse, error) {
		podmanVersion, apiVersion, err := h.service.GetHostVersion(ctx, userID, hostID)
		if err != nil {
			return HostVersionResponse{}, err
		}
		return HostVersionResponse{
			PodmanVersion: podmanVersion,
			APIVersion:    apiVersion,
		}, nil
	})
}

// List handles listing the hosts the caller can access
func (h *HostHandler) List(w http.ResponseWriter, r *http.Request) {
	web.HandleUser(w, r, http.StatusOK, func(ctx context.Context, userID string) ([]AccessibleHostResponse, error) {
		hosts, err := h.service.ListAllHosts(userID)
		if err != nil {
			return nil, err
		}
		return ToHostListResponse(hosts, userID), nil
	})
}

// Update handles updating a host. The update rules — owner-only
// permission, partial merge, PEM validation, key encryption, the write —
// live in the service; the handler decodes the body and maps the response.
func (h *HostHandler) Update(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (HostResponse, error) {
		var req CreateHostRequest
		if appErr := web.Decode(w, r, &req); appErr != nil {
			return HostResponse{}, appErr
		}

		host, err := h.service.Update(ctx, userID, hostID, &req)
		if err != nil {
			return HostResponse{}, err
		}
		return FromModel(host), nil
	})
}

// Delete handles deleting a host
func (h *HostHandler) Delete(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusNoContent, func(ctx context.Context, userID, hostID string) (struct{}, error) {
		return struct{}{}, h.service.Delete(ctx, userID, hostID)
	})
}

// AccessResponse represents host access in API responses
type AccessResponse struct {
	UserID     string `json:"userId"`
	Permission string `json:"permission"`
	GrantedAt  string `json:"grantedAt"`
}

// ListAccess handles listing access for a host. Beyond the subtree
// gate, the service re-checks the caller's access row.
func (h *HostHandler) ListAccess(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) ([]AccessResponse, error) {
		accesses, err := h.service.ListAccess(userID, hostID)
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
func (h *HostHandler) GrantAccess(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusCreated, func(ctx context.Context, grantorID, hostID string) (AccessResponse, error) {
		var req GrantAccessRequest
		if appErr := web.Decode(w, r, &req); appErr != nil {
			return AccessResponse{}, appErr
		}

		access, err := h.service.GrantAccess(grantorID, hostID, &req)
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
func (h *HostHandler) RevokeAccess(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusNoContent, func(ctx context.Context, revokerID, hostID string) (struct{}, error) {
		targetUserID, guard := web.URLParam(r, "userId", "MISSING_USER_ID")
		if guard != nil {
			return struct{}{}, guard
		}

		return struct{}{}, h.service.RevokeAccess(revokerID, hostID, targetUserID)
	})
}
