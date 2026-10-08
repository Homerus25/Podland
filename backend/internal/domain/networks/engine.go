package networks

import (
	"context"

	"github.com/Homerus25/podland/backend/internal/connections"
	"go.podman.io/common/libnetwork/types"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
)

// NetworkEngine is the networks domain's view of one machine's podman:
// the exact set of network operations the service performs, in domain
// vocabulary (single names, plain types) and without a context parameter
// — the connection context lives only inside the production adapter
// below.
type NetworkEngine interface {
	// List returns every network podman reports on the machine —
	// including the default bridge network; no filtering.
	List() ([]types.Network, error)
}

// NetworkEngineProvider is the networks domain's slice of the connection
// seam: it hands a closure one machine's NetworkEngine. It delegates to
// connections.Manager.WithConnection, which owns acquisition, the
// warn-and-502-sentinel failure contract, and the op tag.
type NetworkEngineProvider interface {
	WithEngine(ctx context.Context, userID, machineID, op string, fn func(e NetworkEngine) error) error
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

func (p connectionEngines) WithEngine(ctx context.Context, userID, machineID, op string, fn func(e NetworkEngine) error) error {
	return p.conn.WithConnection(ctx, userID, machineID, op, func(connCtx context.Context) error {
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
