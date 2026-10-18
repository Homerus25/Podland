package images

import (
	"context"

	"github.com/Homerus25/podland/backend/internal/connections"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
)

// ImageEngine is the images domain's view of one machine's podman: the
// exact set of image operations the service performs, in domain vocabulary
// (single names, plain flags) and without a context parameter — the
// connection context lives only inside the production adapter below.
type ImageEngine interface {
	List(opts entities.ImageListOptions) ([]*entities.ImageSummary, error)
	// Remove reports podman's per-item failures through the error slice,
	// not an error return — the service classifies them.
	Remove(id string, force bool) (*entities.ImageRemoveReport, []error)
	Pull(rawImage string, opts entities.ImagePullOptions) (*entities.ImagePullReport, error)
	// Prune removes every unused image, tagged or not (the equivalent of
	// `podman image prune -a`).
	Prune() ([]*reports.PruneReport, error)
	// Inspect reports podman's per-item failures through the error slice,
	// not an error return — the service classifies them.
	Inspect(id string) ([]*entities.ImageInspectReport, []error, error)
}

// ImageEngineProvider is the images domain's slice of the connection seam:
// it hands a closure one machine's ImageEngine. It delegates to
// connections.Manager.WithConnection, which owns acquisition, the
// warn-and-502-sentinel failure contract, and the op tag.
type ImageEngineProvider interface {
	WithEngine(ctx context.Context, userID, machineID, op string, fn func(e ImageEngine) error) error
}

// NewImageEngineProvider returns the production provider: it wraps the
// connection manager and builds the tunnel engine inside the connection
// lease. This file is the only one in the package importing the podman
// tunnel/bindings packages.
func NewImageEngineProvider(conn connections.Manager) ImageEngineProvider {
	return connectionEngines{conn: conn}
}

// connectionEngines is the production ImageEngineProvider.
type connectionEngines struct {
	conn connections.Manager
}

func (p connectionEngines) WithEngine(ctx context.Context, userID, machineID, op string, fn func(e ImageEngine) error) error {
	return p.conn.WithConnection(ctx, userID, machineID, op, func(connCtx context.Context) error {
		return fn(&podmanImageEngine{ctx: connCtx})
	})
}

// podmanImageEngine implements ImageEngine against one live connection.
type podmanImageEngine struct {
	ctx context.Context
}

func (e *podmanImageEngine) tunnelEngine() *tunnel.ImageEngine {
	return &tunnel.ImageEngine{ClientCtx: e.ctx}
}

func (e *podmanImageEngine) List(opts entities.ImageListOptions) ([]*entities.ImageSummary, error) {
	return e.tunnelEngine().List(e.ctx, opts)
}

func (e *podmanImageEngine) Remove(id string, force bool) (*entities.ImageRemoveReport, []error) {
	return e.tunnelEngine().Remove(e.ctx, []string{id}, entities.ImageRemoveOptions{Force: force})
}

func (e *podmanImageEngine) Pull(rawImage string, opts entities.ImagePullOptions) (*entities.ImagePullReport, error) {
	return e.tunnelEngine().Pull(e.ctx, rawImage, opts)
}

func (e *podmanImageEngine) Prune() ([]*reports.PruneReport, error) {
	return e.tunnelEngine().Prune(e.ctx, entities.ImagePruneOptions{All: true})
}

func (e *podmanImageEngine) Inspect(id string) ([]*entities.ImageInspectReport, []error, error) {
	return e.tunnelEngine().Inspect(e.ctx, []string{id}, entities.InspectOptions{})
}
