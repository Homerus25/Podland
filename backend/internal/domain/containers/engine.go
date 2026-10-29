package containers

import (
	"context"

	"github.com/Homerus25/podland/backend/internal/connections"
	"go.podman.io/podman/v6/libpod/define"
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
	// Start starts the container and answers one error: the adapter
	// folds podman's one-entry report slice into the single error the
	// service classifies.
	Start(id string) error
	// Stop stops the container and answers one error, like Start.
	Stop(id string) error
	// Restart restarts the container (podman's default timeout applies,
	// no force) and answers one error, like Start.
	Restart(id string) error
	// Rename renames the container to newName and answers one error.
	// Podman renames running and stopped containers alike, so there is
	// no state restriction here.
	Rename(id, newName string) error
	// Update changes the container's resource limits and healthcheck
	// configuration through podman's container update and answers one
	// error, like Rename (podman's update answers a bare error, no
	// report slice, so nothing is folded). Patch semantics: the
	// options carry only the fields the request presented, nil means
	// unchanged.
	Update(id string, opts *entities.ContainerUpdateOptions) error
	// Remove removes the container (force also removes a running one)
	// and answers one error, like Start.
	Remove(id string, force bool) error
	// Logs writes the container's logs into the writers carried by opts
	// (follow or one-shot is the caller's choice via opts).
	Logs(name string, opts entities.ContainerLogsOptions) error
	// Prune removes every stopped container (podman's own selection,
	// no filters) and answers podman's per-container prune reports.
	Prune() ([]*reports.PruneReport, error)
	// Stats answers a one-shot stats snapshot for the target containers
	// (every running container on the machine when all is set) and one
	// error. The adapter reads podman's single stats report inside the
	// connection lease and folds its error into the returned error —
	// the report channel never crosses the seam.
	Stats(ids []string, all bool) ([]define.ContainerStats, error)
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

// narrowReports narrows podman's report-slice lifecycle answer to the
// single error the service classifies, mirroring the pods engine's
// Remove fold: a transport error becomes the error, and the one-entry
// report slice's per-container error — the race window where the
// container's state changed between the call's precondition inspect
// and its execution — becomes the error too; a clean (or empty)
// answer is nil.
func narrowReports[T any](reports []T, err error, errOf func(T) error) error {
	if err != nil {
		return err
	}
	for _, report := range reports {
		if reportErr := errOf(report); reportErr != nil {
			return reportErr
		}
	}
	return nil
}

// startReportErr reads one start report's per-container error, nil-safe
// like the pods engine's Remove fold.
func startReportErr(report *entities.ContainerStartReport) error {
	if report == nil {
		return nil
	}
	return report.Err
}

// stopReportErr reads one stop report's per-container error like
// startReportErr.
func stopReportErr(report *entities.StopReport) error {
	if report == nil {
		return nil
	}
	return report.Err
}

// restartReportErr reads one restart report's per-container error like
// startReportErr.
func restartReportErr(report *entities.RestartReport) error {
	if report == nil {
		return nil
	}
	return report.Err
}

// removeReportErr reads one remove report's per-container error like
// startReportErr.
func removeReportErr(report *reports.RmReport) error {
	if report == nil {
		return nil
	}
	return report.Err
}

// Start narrows podman's report-slice start to the single error the
// service classifies.
func (e *podmanContainerEngine) Start(id string) error {
	reports, err := e.tunnelEngine().ContainerStart(e.ctx, []string{id}, entities.ContainerStartOptions{})
	return narrowReports(reports, err, startReportErr)
}

// Stop narrows podman's report-slice stop like Start above.
func (e *podmanContainerEngine) Stop(id string) error {
	reports, err := e.tunnelEngine().ContainerStop(e.ctx, []string{id}, entities.StopOptions{})
	return narrowReports(reports, err, stopReportErr)
}

// Restart narrows podman's report-slice restart like Start above.
func (e *podmanContainerEngine) Restart(id string) error {
	reports, err := e.tunnelEngine().ContainerRestart(e.ctx, []string{id}, entities.RestartOptions{})
	return narrowReports(reports, err, restartReportErr)
}

// Remove narrows podman's report-slice remove like Start above.
func (e *podmanContainerEngine) Remove(id string, force bool) error {
	reports, err := e.tunnelEngine().ContainerRm(e.ctx, []string{id}, entities.RmOptions{Force: force})
	return narrowReports(reports, err, removeReportErr)
}

// Rename renames the container. Podman's rename answers a bare error (no
// report slice), so nothing is folded here.
func (e *podmanContainerEngine) Rename(id, newName string) error {
	return e.tunnelEngine().ContainerRename(e.ctx, id, entities.ContainerRenameOptions{NewName: newName})
}

// Update changes the container's resource limits and healthcheck
// configuration. Podman's update answers a bare error like rename (no
// report slice), so nothing is folded here; the tunnel impl runs the
// deprecated specgen processing itself, which is a no-op with a nil
// specgen.
func (e *podmanContainerEngine) Update(id string, opts *entities.ContainerUpdateOptions) error {
	opts.NameOrID = id
	_, err := e.tunnelEngine().ContainerUpdate(e.ctx, opts)
	return err
}

func (e *podmanContainerEngine) Logs(name string, opts entities.ContainerLogsOptions) error {
	return e.tunnelEngine().ContainerLogs(e.ctx, []string{name}, opts)
}

// Prune removes every stopped container like the volumes prune adapter:
// podman's own selection, no filters, per-container reports back.
func (e *podmanContainerEngine) Prune() ([]*reports.PruneReport, error) {
	return e.tunnelEngine().ContainerPrune(e.ctx, entities.ContainerPruneOptions{})
}

// Stats answers a one-shot stats snapshot. Podman's tunnel stats answers a
// report channel; with streaming disabled exactly one report arrives, so
// the adapter reads it here — inside the connection lease — and folds its
// error into the single returned error. The channel never crosses the
// seam, mirroring the report-slice folds above. Interval must be >= 1 or
// the libpod server rejects the request ("invalid interval"), so the
// one-shot snapshot pins it to the smallest legal value.
func (e *podmanContainerEngine) Stats(ids []string, all bool) ([]define.ContainerStats, error) {
	statsChan, err := e.tunnelEngine().ContainerStats(e.ctx, ids, entities.ContainerStatsOptions{All: all, Interval: 1})
	if err != nil {
		return nil, err
	}
	report, ok := <-statsChan
	if !ok {
		// A closed channel without a report cannot happen with
		// streaming disabled (the bindings goroutine always sends
		// one report before closing); an empty snapshot is the
		// honest fallback if it ever does.
		return nil, nil
	}
	if report.Error != nil {
		return nil, report.Error
	}
	return report.Stats, nil
}
