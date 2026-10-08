package networks

import (
	"context"
	"net/http"

	"github.com/Homerus25/podland/backend/internal/web"
)

// NetworkHandler handles network requests
type NetworkHandler struct {
	service *NetworkService
}

// NewNetworkHandler creates a new NetworkHandler
func NewNetworkHandler(service *NetworkService) *NetworkHandler {
	return &NetworkHandler{
		service: service,
	}
}

// List handles listing networks on a machine. Route-level
// RequireMachineAccess (app.go) gates the subtree; the endpoint
// envelope owns extraction, error mapping, and encoding.
func (h *NetworkHandler) List(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) ([]NetworkResponse, error) {
		networks, err := h.service.List(ctx, userID, machineID)
		if err != nil {
			return nil, err
		}
		return ToNetworkListResponse(networks), nil
	})
}
