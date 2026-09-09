package volumes

import (
	"context"
	"io"

	"github.com/Homerus25/podland/backend/internal/connections"
	bindingsVolumes "go.podman.io/podman/v6/pkg/bindings/volumes"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
)

// VolumeEngine is the volumes domain's view of one machine's podman: the
// exact set of volume operations the service performs, in domain vocabulary
// (single names, plain flags) and without a context parameter — the
// connection context lives only inside the production adapter below.
type VolumeEngine interface {
	List(opts entities.VolumeListOptions) ([]*entities.VolumeListReport, error)
	// Inspect resolves one volume by exact name. A missing volume comes
	// back as a nil report with a nil error; the 404 mapping is the
	// service's.
	Inspect(name string) (*entities.VolumeInspectReport, error)
	Create(opts entities.VolumeCreateOptions) (*entities.IDOrNameResponse, error)
	Remove(name string, force bool) ([]*entities.VolumeRmReport, error)
	// Prune removes every unused volume (podman's own selection).
	Prune() ([]*reports.PruneReport, error)
	Export(name string, w io.Writer) error
	Import(name string, r io.Reader) error
}

// VolumeEngineProvider is the volumes domain's slice of the connection
// seam: it hands a closure one machine's VolumeEngine. It delegates to
// connections.Manager.WithConnection, which owns acquisition, the
// warn-and-502-sentinel failure contract, and the op tag.
type VolumeEngineProvider interface {
	WithEngine(ctx context.Context, userID, machineID, op string, fn func(e VolumeEngine) error) error
}

// NewVolumeEngineProvider returns the production provider: it wraps the
// connection manager and builds the tunnel engine inside the connection
// lease. This file is the only one in the package importing the podman
// tunnel/bindings packages.
func NewVolumeEngineProvider(conn connections.Manager) VolumeEngineProvider {
	return connectionEngines{conn: conn}
}

// connectionEngines is the production VolumeEngineProvider.
type connectionEngines struct {
	conn connections.Manager
}

func (p connectionEngines) WithEngine(ctx context.Context, userID, machineID, op string, fn func(e VolumeEngine) error) error {
	return p.conn.WithConnection(ctx, userID, machineID, op, func(connCtx context.Context) error {
		return fn(&podmanVolumeEngine{ctx: connCtx})
	})
}

// podmanVolumeEngine implements VolumeEngine against one live connection.
type podmanVolumeEngine struct {
	ctx context.Context
}

func (e *podmanVolumeEngine) tunnelEngine() *tunnel.ContainerEngine {
	return &tunnel.ContainerEngine{ClientCtx: e.ctx}
}

func (e *podmanVolumeEngine) List(opts entities.VolumeListOptions) ([]*entities.VolumeListReport, error) {
	return e.tunnelEngine().VolumeList(e.ctx, opts)
}

// Inspect narrows podman's slice-shaped inspect (transport error,
// per-item errors, reports) to the single-name width every caller wants:
// a transport error or the item's own error becomes the error, an absent
// volume becomes a nil report with a nil error.
func (e *podmanVolumeEngine) Inspect(name string) (*entities.VolumeInspectReport, error) {
	inspected, itemErrs, err := e.tunnelEngine().VolumeInspect(e.ctx, []string{name}, entities.InspectOptions{})
	if err != nil {
		return nil, err
	}
	for _, itemErr := range itemErrs {
		if itemErr != nil {
			return nil, itemErr
		}
	}
	if len(inspected) == 0 {
		return nil, nil
	}
	return inspected[0], nil
}

func (e *podmanVolumeEngine) Create(opts entities.VolumeCreateOptions) (*entities.IDOrNameResponse, error) {
	return e.tunnelEngine().VolumeCreate(e.ctx, opts)
}

func (e *podmanVolumeEngine) Remove(name string, force bool) ([]*entities.VolumeRmReport, error) {
	return e.tunnelEngine().VolumeRm(e.ctx, []string{name}, entities.VolumeRmOptions{Force: force})
}

func (e *podmanVolumeEngine) Prune() ([]*reports.PruneReport, error) {
	return e.tunnelEngine().VolumePrune(e.ctx, entities.VolumePruneOptions{})
}

func (e *podmanVolumeEngine) Export(name string, w io.Writer) error {
	return bindingsVolumes.Export(e.ctx, name, w)
}

func (e *podmanVolumeEngine) Import(name string, r io.Reader) error {
	return bindingsVolumes.Import(e.ctx, name, r)
}
