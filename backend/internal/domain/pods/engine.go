package pods

import (
	"context"

	"github.com/Homerus25/podland/backend/internal/connections"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
)

// PodEngine is the pods domain's view of one machine's podman: the exact
// set of pod operations the service performs, in domain vocabulary
// (single names, plain flags) and without a context parameter — the
// connection context lives only inside the production adapter below.
type PodEngine interface {
	// List returns every pod podman reports on the machine.
	List(opts entities.PodPSOptions) ([]*entities.ListPodsReport, error)
	// Create creates a pod from the spec and reports its ID.
	Create(spec entities.PodSpec) (*entities.PodCreateReport, error)
	// Inspect returns podman's raw inspect reports for the named pod,
	// plus the tunnel's per-item errors.
	Inspect(nameOrID string) ([]*entities.PodInspectReport, []error, error)
	// Pause pauses every running container in the pod and answers one
	// error: the adapter folds podman's one-entry report slice into the
	// single error the service classifies.
	Pause(nameOrID string) error
	// Unpause unpauses every paused container in the pod.
	Unpause(nameOrID string) error
	// Kill sends podman's default signal to the pod's running containers.
	Kill(nameOrID string) error
	// Restart restarts the pod's containers.
	Restart(nameOrID string) error
	// Remove removes the pod; force also removes running containers.
	Remove(nameOrID string, force bool) error
	// Prune removes every stopped pod (podman's own selection).
	Prune() ([]*entities.PodPruneReport, error)
}

// PodEngineProvider is the pods domain's slice of the connection seam:
// it hands a closure one machine's PodEngine. It delegates to
// connections.Manager.WithConnection, which owns acquisition, the
// warn-and-502-sentinel failure contract, and the op tag.
type PodEngineProvider interface {
	WithEngine(ctx context.Context, userID, machineID, op string, fn func(e PodEngine) error) error
}

// NewPodEngineProvider returns the production provider: it wraps the
// connection manager and builds the tunnel engine inside the connection
// lease. This file is the only one in the package importing the podman
// tunnel/bindings packages.
func NewPodEngineProvider(conn connections.Manager) PodEngineProvider {
	return connectionEngines{conn: conn}
}

// connectionEngines is the production PodEngineProvider.
type connectionEngines struct {
	conn connections.Manager
}

func (p connectionEngines) WithEngine(ctx context.Context, userID, machineID, op string, fn func(e PodEngine) error) error {
	return p.conn.WithConnection(ctx, userID, machineID, op, func(connCtx context.Context) error {
		return fn(&podmanPodEngine{ctx: connCtx})
	})
}

// podmanPodEngine implements PodEngine against one live connection.
type podmanPodEngine struct {
	ctx context.Context
}

func (e *podmanPodEngine) tunnelEngine() *tunnel.ContainerEngine {
	return &tunnel.ContainerEngine{ClientCtx: e.ctx}
}

func (e *podmanPodEngine) List(opts entities.PodPSOptions) ([]*entities.ListPodsReport, error) {
	return e.tunnelEngine().PodPs(e.ctx, opts)
}

func (e *podmanPodEngine) Create(spec entities.PodSpec) (*entities.PodCreateReport, error) {
	return e.tunnelEngine().PodCreate(e.ctx, spec)
}

func (e *podmanPodEngine) Inspect(nameOrID string) ([]*entities.PodInspectReport, []error, error) {
	return e.tunnelEngine().PodInspect(e.ctx, []string{nameOrID}, entities.InspectOptions{})
}

// foldErrs narrows a report's per-container error slice to the single
// error the service classifies: the first per-container error — the race
// window where a container's state changed between the call's checks and
// its execution — becomes the error; a clean answer is nil.
func foldErrs(errs []error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// narrowReports narrows podman's report-slice lifecycle answer to the
// single error the service classifies: a transport error becomes the
// error, and each report's per-container errors fold through foldErrs —
// the first one, the race window where a container's state changed
// between the call's checks and its execution, becomes the error; a
// clean answer is nil.
func narrowReports[T any](reports []T, err error, errsOf func(T) []error) error {
	if err != nil {
		return err
	}
	for _, report := range reports {
		if err := foldErrs(errsOf(report)); err != nil {
			return err
		}
	}
	return nil
}

// Pause narrows podman's report-slice pause to the single error the
// service classifies, mirroring the volumes engine's Remove narrowing: a
// transport error becomes the error, and the one-entry report slice's
// per-container errors become the error too; a clean pause answers nil.
func (e *podmanPodEngine) Pause(nameOrID string) error {
	reports, err := e.tunnelEngine().PodPause(e.ctx, []string{nameOrID}, entities.PodPauseOptions{})
	return narrowReports(reports, err, func(report *entities.PodPauseReport) []error { return report.Errs })
}

// Unpause narrows podman's report-slice unpause like Pause above.
func (e *podmanPodEngine) Unpause(nameOrID string) error {
	reports, err := e.tunnelEngine().PodUnpause(e.ctx, []string{nameOrID}, entities.PodunpauseOptions{})
	return narrowReports(reports, err, func(report *entities.PodUnpauseReport) []error { return report.Errs })
}

// Kill narrows podman's report-slice kill like Pause above.
func (e *podmanPodEngine) Kill(nameOrID string) error {
	reports, err := e.tunnelEngine().PodKill(e.ctx, []string{nameOrID}, entities.PodKillOptions{})
	return narrowReports(reports, err, func(report *entities.PodKillReport) []error { return report.Errs })
}

// Restart narrows podman's report-slice restart like Pause above.
func (e *podmanPodEngine) Restart(nameOrID string) error {
	reports, err := e.tunnelEngine().PodRestart(e.ctx, []string{nameOrID}, entities.PodRestartOptions{})
	return narrowReports(reports, err, func(report *entities.PodRestartReport) []error { return report.Errs })
}

// Remove narrows podman's report-slice remove to the single error the
// service classifies: a transport error, or the one-entry report slice's
// own per-pod error — the race window where the pod's containers changed
// state between the call's checks and its execution — becomes the error;
// a clean removal answers nil.
func (e *podmanPodEngine) Remove(nameOrID string, force bool) error {
	reports, err := e.tunnelEngine().PodRm(e.ctx, []string{nameOrID}, entities.PodRmOptions{Force: force})
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

func (e *podmanPodEngine) Prune() ([]*entities.PodPruneReport, error) {
	return e.tunnelEngine().PodPrune(e.ctx, entities.PodPruneOptions{})
}
