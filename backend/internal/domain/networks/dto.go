package networks

import (
	"slices"
	"strings"
	"time"

	"go.podman.io/common/libnetwork/types"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// NetworkCreateRequest is the POST /api/hosts/{id}/networks body.
// Name is required; containers optionally lists container IDs to attach
// to the new network in the same call.
type NetworkCreateRequest struct {
	// Network name
	Name string `json:"name" validate:"required"`
	// Container IDs to attach after the create (optional)
	Containers []string `json:"containers,omitempty"`
}

// NetworkConnectRequest is the
// POST /api/hosts/{id}/networks/{networkName}/connect body.
type NetworkConnectRequest struct {
	// Container ID to attach to the network
	Container string `json:"container" validate:"required"`
}

// NetworkResponse represents a network in the list API response. It
// carries exactly the rendered fields — name, driver, subnets (CIDR
// strings), createdAt (RFC3339); no id, labels, options, or flags.
type NetworkResponse struct {
	// Network name
	Name string `json:"name"`
	// Network driver (bridge, macvlan, host, …)
	Driver string `json:"driver"`
	// Subnets in CIDR form
	Subnets []string `json:"subnets"`
	// Time when the network was created (RFC3339)
	CreatedAt string `json:"createdAt"`
}

// FromPodmanNetwork converts a podman libnetwork Network to the API
// response
func FromPodmanNetwork(network types.Network) NetworkResponse {
	subnets := make([]string, 0, len(network.Subnets))
	for _, subnet := range network.Subnets {
		subnets = append(subnets, subnet.Subnet.String())
	}
	return NetworkResponse{
		Name:      network.Name,
		Driver:    network.Driver,
		Subnets:   subnets,
		CreatedAt: network.Created.UTC().Format(time.RFC3339),
	}
}

// ToNetworkListResponse converts podman network entities to the API array
func ToNetworkListResponse(networks []types.Network) []NetworkResponse {
	out := make([]NetworkResponse, 0, len(networks))
	for _, network := range networks {
		out = append(out, FromPodmanNetwork(network))
	}
	return out
}

// NetworkContainerRef is one attached container on the network detail
// DTO: the container's ID (the inspect report's Containers map key)
// and name.
type NetworkContainerRef struct {
	// Container ID
	ID string `json:"id"`
	// Container name
	Name string `json:"name"`
}

// NetworkDetailResponse is the GET
// /api/hosts/{id}/networks/{networkName} detail DTO: the fields
// the network detail page renders, mapped from the inspect report.
type NetworkDetailResponse struct {
	// Network name
	Name string `json:"name"`
	// Network ID from the inspect report
	ID string `json:"id"`
	// Network driver (bridge, macvlan, host, …)
	Driver string `json:"driver"`
	// Time when the network was created (RFC3339)
	CreatedAt string `json:"createdAt"`
	// Subnets in CIDR form
	Subnets []string `json:"subnets"`
	// Attached containers (id + name)
	Containers []NetworkContainerRef `json:"containers"`
}

// FromNetworkInspectReport converts a podman network inspect report to
// the detail DTO. A nil report maps to the zero DTO; subnets and
// containers are always present ([] when empty), and the containers
// are ordered by ID so the answer is stable across calls.
func FromNetworkInspectReport(report *entities.NetworkInspectReport) NetworkDetailResponse {
	if report == nil {
		return NetworkDetailResponse{
			Subnets:    []string{},
			Containers: []NetworkContainerRef{},
		}
	}
	subnets := make([]string, 0, len(report.Subnets))
	for _, subnet := range report.Subnets {
		subnets = append(subnets, subnet.Subnet.String())
	}
	containers := make([]NetworkContainerRef, 0, len(report.Containers))
	for id, info := range report.Containers {
		containers = append(containers, NetworkContainerRef{ID: id, Name: info.Name})
	}
	slices.SortFunc(containers, func(a, b NetworkContainerRef) int {
		return strings.Compare(a.ID, b.ID)
	})
	return NetworkDetailResponse{
		Name:       report.Name,
		ID:         report.ID,
		Driver:     report.Driver,
		CreatedAt:  report.Created.UTC().Format(time.RFC3339),
		Subnets:    subnets,
		Containers: containers,
	}
}

// NetworkPruneResponse is one entry in the prune API response: the
// removed network's name, plus any per-network error podman reported
// while removing it. Podman's network prune report carries no size, so
// there is no size field.
type NetworkPruneResponse struct {
	// Name of the removed network
	Name string `json:"name"`
	// Per-network error podman reported, if the removal failed
	Error string `json:"error,omitempty"`
}

// FromPodmanPruneReports converts podman's prune reports to the API
// array. Nil reports are skipped; pruning with nothing to remove
// yields an empty array, not nil.
func FromPodmanPruneReports(reports []*entities.NetworkPruneReport) []NetworkPruneResponse {
	out := make([]NetworkPruneResponse, 0, len(reports))
	for _, report := range reports {
		if report == nil {
			continue
		}
		entry := NetworkPruneResponse{
			Name: report.Name,
		}
		if report.Error != nil {
			entry.Error = report.Error.Error()
		}
		out = append(out, entry)
	}
	return out
}
