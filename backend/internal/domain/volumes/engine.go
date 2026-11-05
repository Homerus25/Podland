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

// VolumeEngine is the volumes domain's view of one host's podman: the
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
	// Remove uninstalls one volume by exact name and answers one error:
	// the adapter folds podman's one-entry report slice into the single
	// error the service classifies — a transport error, or the report's
	// own per-volume error (the race window where the volume vanished or
	// came into use between the call's checks and its execution), becomes
	// the error; a clean removal answers nil.
	Remove(name string, force bool) error
	// Prune removes every unused volume (podman's own selection).
	Prune() ([]*reports.PruneReport, error)
	Export(name string, w io.Writer) error
	Import(name string, r io.Reader) error
}

// VolumeUsageEngine is the volumes domain's view of one host's
// in-use surface (issue 238): the exact reads the move/copy preview and
// transfer need to decide whether a volume is in use and which quadlets
// reference it. Podman reports no in-use flag on volume inspect and no
// volume filter on container list, so in-use is derived here from the
// container list's mounts alone; the quadlet files' `Volume=` lines feed
// only the preview's referencing-quadlets warning, never the in-use
// decision. Deliberately narrow: no container lifecycle, no quadlet
// install — the move/copy never mutates either surface. The two quadlet
// reads satisfy the transfer kit's QuadletReader structurally (§19,
// issue 259): the move preview hands the engine to the kit's scan
// directly — no adapter.
type VolumeUsageEngine interface {
	// ContainerMounts lists every container (all states) with its
	// user-declared volume mounts, each as written (`name:/dest:opts`).
	ContainerMounts() ([]ContainerMounts, error)
	// ListQuadletNames lists the host's quadlet file names.
	ListQuadletNames() ([]string, error)
	// PrintQuadlet returns one quadlet file's verbatim content.
	PrintQuadlet(name string) (string, error)
}

// ContainerMounts is one listed container's name and its user-declared
// volume mounts, each string the `Volume=` source as written (the part
// before the first ":" is the volume name or host path).
type ContainerMounts struct {
	Name   string
	Mounts []string
}

// VolumeUsageEngineProvider is the volumes domain's slice of the
// connection seam for the usage surface (issue 238): it hands a closure
// one host's VolumeUsageEngine, delegating to connections.Manager.
type VolumeUsageEngineProvider interface {
	WithEngine(ctx context.Context, userID, hostID, op string, fn func(e VolumeUsageEngine) error) error
}

// NewVolumeUsageEngineProvider returns the production usage provider: it
// wraps the connection manager and builds the tunnel engine inside the
// connection lease, like NewVolumeEngineProvider does for volumes.
func NewVolumeUsageEngineProvider(conn connections.Manager) VolumeUsageEngineProvider {
	return connectionUsageEngines{conn: conn}
}

// connectionUsageEngines is the production VolumeUsageEngineProvider.
type connectionUsageEngines struct {
	conn connections.Manager
}

func (p connectionUsageEngines) WithEngine(ctx context.Context, userID, hostID, op string, fn func(e VolumeUsageEngine) error) error {
	return p.conn.WithConnection(ctx, userID, hostID, op, func(connCtx context.Context) error {
		return fn(&podmanVolumeUsageEngine{ctx: connCtx})
	})
}

// podmanVolumeUsageEngine implements VolumeUsageEngine against one live
// connection.
type podmanVolumeUsageEngine struct {
	ctx context.Context
}

func (e *podmanVolumeUsageEngine) tunnelEngine() *tunnel.ContainerEngine {
	return &tunnel.ContainerEngine{ClientCtx: e.ctx}
}

func (e *podmanVolumeUsageEngine) ContainerMounts() ([]ContainerMounts, error) {
	list, err := e.tunnelEngine().ContainerList(e.ctx, entities.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	out := make([]ContainerMounts, 0, len(list))
	for _, container := range list {
		name := ""
		if len(container.Names) > 0 {
			name = container.Names[0]
		}
		out = append(out, ContainerMounts{Name: name, Mounts: container.Mounts})
	}
	return out, nil
}

func (e *podmanVolumeUsageEngine) ListQuadletNames() ([]string, error) {
	list, err := e.tunnelEngine().QuadletList(e.ctx, entities.QuadletListOptions{})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(list))
	for _, quadlet := range list {
		names = append(names, quadlet.Name)
	}
	return names, nil
}

func (e *podmanVolumeUsageEngine) PrintQuadlet(name string) (string, error) {
	return e.tunnelEngine().QuadletPrint(e.ctx, name)
}

// seam: it hands a closure one host's VolumeEngine. It delegates to
// connections.Manager.WithConnection, which owns acquisition, the
// warn-and-502-sentinel failure contract, and the op tag.
type VolumeEngineProvider interface {
	WithEngine(ctx context.Context, userID, hostID, op string, fn func(e VolumeEngine) error) error
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

func (p connectionEngines) WithEngine(ctx context.Context, userID, hostID, op string, fn func(e VolumeEngine) error) error {
	return p.conn.WithConnection(ctx, userID, hostID, op, func(connCtx context.Context) error {
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

// Remove narrows podman's report-slice remove to the single error the
// service classifies, mirroring the Inspect narrowing above: a transport
// error becomes the error, and the one-entry report slice's per-volume
// error — the race window where the volume vanished or came into use
// between the call's checks and its execution — becomes the error too; a
// clean removal answers nil.
func (e *podmanVolumeEngine) Remove(name string, force bool) error {
	reports, err := e.tunnelEngine().VolumeRm(e.ctx, []string{name}, entities.VolumeRmOptions{Force: force})
	if err != nil {
		return err
	}
	for _, report := range reports {
		if report != nil && report.Err != nil {
			return report.Err
		}
	}
	return nil
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
