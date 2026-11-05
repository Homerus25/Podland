package networks

import (
	"context"

	"github.com/Homerus25/podland/backend/internal/connections"
	"go.podman.io/common/libnetwork/types"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
)

// NetworkEngine is the networks domain's view of one host's podman:
// the exact set of network operations the service performs, in domain
// vocabulary (single names, plain types) and without a context parameter
// — the connection context lives only inside the production adapter
// below.
type NetworkEngine interface {
	// List returns every network podman reports on the host —
	// including the default bridge network; no filtering.
	List() ([]types.Network, error)
	// Create creates a network by name with podman's defaults (bridge
	// driver, auto-assigned subnet) and returns the created network.
	Create(name string) (*types.Network, error)
	// Connect attaches a container (by ID) to a network by name.
	Connect(networkName, containerID string) error
	// Inspect returns podman's raw inspect reports for the named
	// network, plus the tunnel's per-item errors.
	Inspect(name string) ([]entities.NetworkInspectReport, []error, error)
	// Prune removes every unused network (podman's own selection).
	Prune() ([]*entities.NetworkPruneReport, error)
}

// NetworkEngineProvider is the networks domain's slice of the connection
// seam: it hands a closure one host's NetworkEngine. It delegates to
// connections.Manager.WithConnection, which owns acquisition, the
// warn-and-502-sentinel failure contract, and the op tag.
type NetworkEngineProvider interface {
	WithEngine(ctx context.Context, userID, hostID, op string, fn func(e NetworkEngine) error) error
}

// NewNetworkEngineProvider returns the production provider: it wraps the
// connection manager and builds the tunnel engine inside the connection
// lease. This file is the only one in the package importing the podman
// tunnel/bindings packages.
func NewNetworkEngineProvider(conn connections.Manager) NetworkEngineProvider {
	return connectionEngines{conn: conn}
}

// connectionEngines is the production NetworkEngineProvider.
type connectionEngines struct {
	conn connections.Manager
}

func (p connectionEngines) WithEngine(ctx context.Context, userID, hostID, op string, fn func(e NetworkEngine) error) error {
	return p.conn.WithConnection(ctx, userID, hostID, op, func(connCtx context.Context) error {
		return fn(&podmanNetworkEngine{ctx: connCtx})
	})
}

// podmanNetworkEngine implements NetworkEngine against one live
// connection.
type podmanNetworkEngine struct {
	ctx context.Context
}

func (e *podmanNetworkEngine) tunnelEngine() *tunnel.ContainerEngine {
	return &tunnel.ContainerEngine{ClientCtx: e.ctx}
}

func (e *podmanNetworkEngine) List() ([]types.Network, error) {
	return e.tunnelEngine().NetworkList(e.ctx, entities.NetworkListOptions{})
}

func (e *podmanNetworkEngine) Create(name string) (*types.Network, error) {
	return e.tunnelEngine().NetworkCreate(e.ctx, types.Network{Name: name}, &types.NetworkCreateOptions{})
}

func (e *podmanNetworkEngine) Connect(networkName, containerID string) error {
	return e.tunnelEngine().NetworkConnect(e.ctx, networkName, entities.NetworkConnectOptions{Container: containerID})
}

func (e *podmanNetworkEngine) Inspect(name string) ([]entities.NetworkInspectReport, []error, error) {
	return e.tunnelEngine().NetworkInspect(e.ctx, []string{name}, entities.InspectOptions{})
}

func (e *podmanNetworkEngine) Prune() ([]*entities.NetworkPruneReport, error) {
	return e.tunnelEngine().NetworkPrune(e.ctx, entities.NetworkPruneOptions{})
}
