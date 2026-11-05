package images

import (
	"context"
	"io"

	"github.com/Homerus25/podland/backend/internal/connections"
	bindingsImages "go.podman.io/podman/v6/pkg/bindings/images"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
)

// ImageEngine is the images domain's view of one host's podman: the
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
	// Exists answers whether the host holds the image by ID or name
	// through podman's dedicated exists endpoint — the existence ritual's
	// width (the export's 404 gate, issue 401, and the copy's source and
	// target presence checks, issue 375), cheaper than an inspect whose
	// report is then discarded.
	Exists(id string) (bool, error)
	// Export streams the image's archive into w by image reference —
	// the source width of the cross-host copy (issue 375, §19).
	Export(ref string, w io.Writer) error
	// Load consumes an image archive from r and answers the names the
	// load applied — the target width of the cross-host copy
	// (issue 375, §19). Idempotent by construction: images are
	// content-addressed, so loading onto a target that already holds
	// the image skips present layers and re-applies tags.
	Load(r io.Reader) ([]string, error)
}

// ImageEngineProvider is the images domain's slice of the connection seam:
// it hands a closure one host's ImageEngine. It delegates to
// connections.Manager.WithConnection, which owns acquisition, the
// warn-and-502-sentinel failure contract, and the op tag.
type ImageEngineProvider interface {
	WithEngine(ctx context.Context, userID, hostID, op string, fn func(e ImageEngine) error) error
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

func (p connectionEngines) WithEngine(ctx context.Context, userID, hostID, op string, fn func(e ImageEngine) error) error {
	return p.conn.WithConnection(ctx, userID, hostID, op, func(connCtx context.Context) error {
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

// Exists answers through podman's dedicated exists endpoint: a missing
// image is the fact (false), not an error.
func (e *podmanImageEngine) Exists(id string) (bool, error) {
	report, err := e.tunnelEngine().Exists(e.ctx, id)
	if err != nil {
		return false, err
	}
	return report.Value, nil
}

// Export streams the image's archive into w through podman's bindings
// export (the volumes adapter's arrangement: the bindings call is the
// one production shape that streams to a writer; the tunnel engine's
// export wants a file path).
func (e *podmanImageEngine) Export(ref string, w io.Writer) error {
	return bindingsImages.Export(e.ctx, []string{ref}, w, nil)
}

// Load consumes the archive through podman's bindings load and answers
// the names the load applied.
func (e *podmanImageEngine) Load(r io.Reader) ([]string, error) {
	report, err := bindingsImages.Load(e.ctx, r)
	if err != nil {
		return nil, err
	}
	if report == nil {
		return nil, nil
	}
	return report.Names, nil
}
