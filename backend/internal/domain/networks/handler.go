package networks

import (
	"context"
	"net/http"

	"github.com/Homerus25/podland/backend/internal/web"
	"go.podman.io/podman/v6/pkg/domain/entities"
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

// Create handles creating a network on a machine. Route-level
// RequireMachinePermission(write) (app.go) gates it at write. The body
// carries the network's name (required) and optionally the IDs of
// containers to attach after the create; an attach failure names the
// failing container and the network stays created (no rollback).
func (h *NetworkHandler) Create(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusCreated, func(ctx context.Context, userID, machineID string) (NetworkResponse, error) {
		var request NetworkCreateRequest
		if appErr := web.Decode(r, &request); appErr != nil {
			return NetworkResponse{}, appErr
		}

		network, err := h.service.Create(ctx, userID, machineID, request)
		if err != nil {
			return NetworkResponse{}, err
		}
		return FromPodmanNetwork(*network), nil
	})
}

// Connect handles attaching a container to a network on a machine.
// Route-level RequireMachinePermission(write) (app.go) gates it at
// write. Answers 200 with an empty JSON object ({}): the envelope
// encodes the handler's struct{} result, so the body is {} rather than
// empty.
func (h *NetworkHandler) Connect(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (struct{}, error) {
		networkName, guard := web.URLParam(r, "networkName", "MISSING_NETWORK_NAME")
		if guard != nil {
			return struct{}{}, guard
		}

		var request NetworkConnectRequest
		if appErr := web.Decode(r, &request); appErr != nil {
			return struct{}{}, appErr
		}

		return struct{}{}, h.service.Connect(ctx, userID, machineID, networkName, request.Container)
	})
}

// Prune handles removing all unused networks on a machine. Route-level
// RequireMachinePermission(write) (app.go) gates it at write; podman
// leaves in-use networks alone. Returns 200 with one report entry per
// removed network (name, any per-network error; podman's network
// prune report carries no size).
func (h *NetworkHandler) Prune(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) ([]NetworkPruneResponse, error) {
		prunes, err := h.service.Prune(ctx, userID, machineID)
		if err != nil {
			return nil, err
		}
		return FromPodmanPruneReports(prunes), nil
	})
}

// Get handles returning one network's detail DTO on a machine. Route-level
// RequireMachineAccess (app.go) gates the subtree at read; the endpoint
// envelope owns extraction, error mapping, and encoding. The DTO is
// mapped from podman's inspect report: name, id, driver, createdAt,
// subnets, and the attached containers.
func (h *NetworkHandler) Get(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (NetworkDetailResponse, error) {
		networkName, guard := web.URLParam(r, "networkName", "MISSING_NETWORK_NAME")
		if guard != nil {
			return NetworkDetailResponse{}, guard
		}

		return h.service.Get(ctx, userID, machineID, networkName)
	})
}

// Inspect handles returning one network's raw podman inspect report on
// a machine. Route-level RequireMachineAccess (app.go) gates the
// subtree at read; the endpoint envelope owns extraction, error
// mapping, and encoding. The report is returned unmapped: the body is
// podman's raw inspect JSON.
func (h *NetworkHandler) Inspect(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (entities.NetworkInspectReport, error) {
		networkName, guard := web.URLParam(r, "networkName", "MISSING_NETWORK_NAME")
		if guard != nil {
			return entities.NetworkInspectReport{}, guard
		}

		return h.service.Inspect(ctx, userID, machineID, networkName)
	})
}
