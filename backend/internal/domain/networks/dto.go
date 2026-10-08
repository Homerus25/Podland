package networks

import (
	"time"

	"go.podman.io/common/libnetwork/types"
)

// NetworkCreateRequest is the POST /api/machines/{id}/networks body.
// Name is required; containers optionally lists container IDs to attach
// to the new network in the same call.
type NetworkCreateRequest struct {
	// Network name
	Name string `json:"name" validate:"required"`
	// Container IDs to attach after the create (optional)
	Containers []string `json:"containers,omitempty"`
}

// NetworkConnectRequest is the
// POST /api/machines/{id}/networks/{networkName}/connect body.
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
