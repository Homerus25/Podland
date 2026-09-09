package containers

import (
	"context"

	"github.com/Homerus25/podland/backend/internal/connections"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
	"go.podman.io/podman/v6/pkg/specgen"
)

// ContainerEngine is the containers domain's view of one machine's podman:
// the exact set of container operations the service performs, in domain
// vocabulary (single names, plain flags) and without a context parameter —
// the connection context lives only inside the production adapter below.
type ContainerEngine interface {
	List(opts entities.ContainerListOptions) ([]entities.ListContainer, error)
	Create(spec *specgen.SpecGenerator) (*entities.ContainerCreateReport, error)
	Inspect(id string) ([]*entities.ContainerInspectReport, []error, error)
	Start(id string) ([]*entities.ContainerStartReport, error)
	Stop(id string) ([]*entities.StopReport, error)
	Remove(id string, force bool) ([]*reports.RmReport, error)
	// Logs writes the container's logs into the writers carried by opts
	// (follow or one-shot is the caller's choice via opts).
	Logs(name string, opts entities.ContainerLogsOptions) error
}

// ContainerEngineProvider is the containers domain's slice of the
// connection seam: it hands a closure one machine's ContainerEngine. It
// delegates to connections.Manager.WithConnection, which owns acquisition,
// the warn-and-502-sentinel failure contract, and the op tag.
type ContainerEngineProvider interface {
	WithEngine(ctx context.Context, userID, machineID, op string, fn func(e ContainerEngine) error) error
}

// NewContainerEngineProvider returns the production provider: it wraps the
// connection manager and builds the tunnel engine inside the connection
// lease. This file is the only one in the package importing the podman
// tunnel/bindings packages.
func NewContainerEngineProvider(conn connections.Manager) ContainerEngineProvider {
	return connectionEngines{conn: conn}
}

// connectionEngines is the production ContainerEngineProvider.
type connectionEngines struct {
	conn connections.Manager
}

func (p connectionEngines) WithEngine(ctx context.Context, userID, machineID, op string, fn func(e ContainerEngine) error) error {
	return p.conn.WithConnection(ctx, userID, machineID, op, func(connCtx context.Context) error {
		return fn(&podmanContainerEngine{ctx: connCtx})
	})
}

// podmanContainerEngine implements ContainerEngine against one live
// connection.
type podmanContainerEngine struct {
	ctx context.Context
}

func (e *podmanContainerEngine) tunnelEngine() *tunnel.ContainerEngine {
	return &tunnel.ContainerEngine{ClientCtx: e.ctx}
}

func (e *podmanContainerEngine) List(opts entities.ContainerListOptions) ([]entities.ListContainer, error) {
	return e.tunnelEngine().ContainerList(e.ctx, opts)
}

func (e *podmanContainerEngine) Create(spec *specgen.SpecGenerator) (*entities.ContainerCreateReport, error) {
	return e.tunnelEngine().ContainerCreate(e.ctx, spec)
}

func (e *podmanContainerEngine) Inspect(id string) ([]*entities.ContainerInspectReport, []error, error) {
	return e.tunnelEngine().ContainerInspect(e.ctx, []string{id}, entities.InspectOptions{})
}

func (e *podmanContainerEngine) Start(id string) ([]*entities.ContainerStartReport, error) {
	return e.tunnelEngine().ContainerStart(e.ctx, []string{id}, entities.ContainerStartOptions{})
}

func (e *podmanContainerEngine) Stop(id string) ([]*entities.StopReport, error) {
	return e.tunnelEngine().ContainerStop(e.ctx, []string{id}, entities.StopOptions{})
}

func (e *podmanContainerEngine) Remove(id string, force bool) ([]*reports.RmReport, error) {
	return e.tunnelEngine().ContainerRm(e.ctx, []string{id}, entities.RmOptions{Force: force})
}

func (e *podmanContainerEngine) Logs(name string, opts entities.ContainerLogsOptions) error {
	return e.tunnelEngine().ContainerLogs(e.ctx, []string{name}, opts)
}
