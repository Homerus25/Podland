package containers

import (
	"context"
	"fmt"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/inspect"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"github.com/Homerus25/podland/backend/internal/removal"
	"github.com/Homerus25/podland/backend/internal/streaming"
	libpoddefine "go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
)

// ContainerService provides business logic for containers. Every
// host-scoped podman call crosses the ContainerEngineProvider (the
// connection seam); the service itself owns only domain mapping,
// composition, and dispatch to the streaming kit's log rituals.
type ContainerService struct {
	engines ContainerEngineProvider
}

// NewContainerService creates a new ContainerService
func NewContainerService(engines ContainerEngineProvider) *ContainerService {
	return &ContainerService{
		engines: engines,
	}
}

// List returns a list of containers on the specified host
func (s *ContainerService) List(ctx context.Context, userID, hostID string, opts entities.ContainerListOptions) ([]entities.ListContainer, error) {
	var containers []entities.ListContainer
	err := s.engines.WithEngine(ctx, userID, hostID, "containers.List", func(e ContainerEngine) error {
		list, err := e.List(opts)
		if err != nil {
			return fmt.Errorf("failed to list containers: %w", err)
		}
		containers = list
		return nil
	})
	return containers, err
}

// Create creates a new container on the specified host. The
// request → specgen mapping lives in the pure builder (buildCreateSpec,
// create_spec.go — the per-domain create builder settlement,
// backend-conventions); the spec is built before the lease is acquired,
// so a request-local validation failure (the unparseable memory) never
// costs an engine lease, and this closure is a thin lease: the engine
// call plus the not-found classification. The service returns podman's
// create report; the handler maps the response DTO.
func (s *ContainerService) Create(ctx context.Context, userID, hostID string, request ContainerCreateRequest) (*entities.ContainerCreateReport, error) {
	spec, err := buildCreateSpec(request)
	if err != nil {
		return nil, err
	}

	var report *entities.ContainerCreateReport
	err = s.engines.WithEngine(ctx, userID, hostID, "containers.Create", func(e ContainerEngine) error {
		// Create the container using Podman API. A not-found answer is
		// the missing-image terminal — unless the request asked for a
		// pod, in which case the missing pod is the terminal: the doku
		// documents 404 IMAGE_NOT_FOUND, plus 404 POD_NOT_FOUND when a
		// pod was requested, so the classification happens here, not in
		// the envelope's 500 fallback. The classification is ambiguous
		// (podman's not-found covers the image, the pod, and anything
		// else in the spec), so podman's own message travels through
		// WithMessage as the only disambiguator — the pods domain's
		// create precedent.
		createReport, err := e.Create(spec)
		if err != nil {
			if podmanerr.IsNotFound(err) {
				if request.Pod != "" {
					return appErrors.ErrPodNotFound.WithMessage(podmanerr.Message(err))
				}
				return appErrors.ErrImageNotFound.WithMessage(podmanerr.Message(err))
			}
			return fmt.Errorf("failed to create container: %w", err)
		}

		report = createReport
		return nil
	})
	return report, err
}

// Get returns detailed information about a specific container
func (s *ContainerService) Get(ctx context.Context, userID, hostID, containerID string) (*entities.ContainerInspectReport, error) {
	var report *entities.ContainerInspectReport
	err := s.engines.WithEngine(ctx, userID, hostID, "containers.Get", func(e ContainerEngine) error {
		inspectReport, err := inspectContainer(e, containerID)
		if err != nil {
			return err
		}

		report = inspectReport
		return nil
	})
	return report, err
}

// Inspect returns the container's raw podman inspect report. Unlike
// Get, the report crosses the wire unmapped: this endpoint is the raw
// inspect JSON window, so no DTO mapping happens here or in the handler.
func (s *ContainerService) Inspect(ctx context.Context, userID, hostID, containerID string) (*entities.ContainerInspectReport, error) {
	var report *entities.ContainerInspectReport
	err := s.engines.WithEngine(ctx, userID, hostID, "containers.Inspect", func(e ContainerEngine) error {
		inspectReport, err := inspectContainer(e, containerID)
		if err != nil {
			return err
		}

		report = inspectReport
		return nil
	})
	return report, err
}

// Start starts a container by ID on the specified host. The whole
// pre-action ritual — inspect for existence, state, and name, the
// already-running terminal before any start call (podman's refusal
// produces no error shape through the tunnel, verified live, issue
// #270, so the precondition is state-based), the engine call, podman's
// not-found classification, and the identity result — lives once in
// the domain's lifecycle module (lifecycle.go); this wrapper and its
// siblings are one-line declarations of their differences as data.
func (s *ContainerService) Start(ctx context.Context, userID, hostID, containerID string) (*ContainerLifecycleResult, error) {
	return s.runLifecycleAction(ctx, userID, hostID, containerID, actionStart)
}

// Stop stops a container by ID on the specified host. The ritual is
// the lifecycle module's (lifecycle.go): one inspect serves existence,
// state, and name; the not-running terminal answers before any stop
// call (podman's refusal produces no error shape through the tunnel,
// verified live, issue #270, so the precondition is state-based); the
// engine folds podman's report slice into one error; a missing
// container answers ErrContainerNotFound, anything else is wrapped,
// never answered as a stopped success.
func (s *ContainerService) Stop(ctx context.Context, userID, hostID, containerID string) (*ContainerLifecycleResult, error) {
	return s.runLifecycleAction(ctx, userID, hostID, containerID, actionStop)
}

// Restart restarts a running container by ID on the specified host.
// Like Stop it requires a running container — a stopped container is
// Start's caller, not this one — and no timeout or force parameters
// travel: podman's default timeout applies (the quadlets restart's
// no-force precedent, issue 205). The ritual itself is the lifecycle
// module's (lifecycle.go).
func (s *ContainerService) Restart(ctx context.Context, userID, hostID, containerID string) (*ContainerLifecycleResult, error) {
	return s.runLifecycleAction(ctx, userID, hostID, containerID, actionRestart)
}

// inspectContainer resolves one container's inspect report through the
// shared single-report ritual, classifying podman's not-found shapes
// into ErrContainerNotFound. It is the pre-action read for Start, Stop,
// and Restart: one inspect serves existence, state, and name.
func inspectContainer(e ContainerEngine, containerID string) (*entities.ContainerInspectReport, error) {
	reports, inspectErrs, err := e.Inspect(containerID)
	return inspect.Resolve(reports, inspectErrs, err, appErrors.ErrContainerNotFound, "container", containerID)
}

// Rename renames a container on the specified host and answers the
// container's post-rename inspect report, so the handler maps the same
// detail DTO the detail GET does. Podman renames running and stopped
// containers alike — no state restriction, and no quadlet-owned
// restriction either (podman's CLI imposes neither) — so unlike
// Start/Stop/Restart there is no pre-action state gate. The name's
// shape is podman's to validate: an invalid name is a client error and
// answers 400 INVALID_REQUEST carrying podman's message, while a name
// already in use answers the domain's 409 collision sentinel.
func (s *ContainerService) Rename(ctx context.Context, userID, hostID, containerID, newName string) (*entities.ContainerInspectReport, error) {
	var report *entities.ContainerInspectReport
	err := s.engines.WithEngine(ctx, userID, hostID, "containers.Rename", func(e ContainerEngine) error {
		if err := e.Rename(containerID, newName); err != nil {
			if podmanerr.IsAlreadyExists(err) {
				return appErrors.ErrContainerAlreadyExists
			}
			if podmanerr.IsNotFound(err) {
				return appErrors.ErrContainerNotFound
			}
			// Podman answers a rename onto a taken name as a 500 sqlite
			// UNIQUE constraint failure rather than a typed conflict
			// (RenameContainer → RewriteContainerConfig in podman's
			// source), so the constraint phrase is the collision's
			// signature, classified through the shared adapter — never
			// a follow-up probe, which could attribute an unrelated
			// failure to a name it did not refuse.
			if podmanerr.IsUniqueConstraintViolation(err) {
				return appErrors.ErrContainerAlreadyExists
			}
			if podmanerr.IsClientError(err) {
				return appErrors.ErrInvalidRequest.WithMessage(podmanerr.Message(err))
			}
			return fmt.Errorf("failed to rename container: %w", err)
		}

		inspectReport, err := inspectContainer(e, containerID)
		if err != nil {
			return err
		}

		report = inspectReport
		return nil
	})
	return report, err
}

// Update changes a container's resource limits and healthcheck
// configuration on the specified host and answers the container's
// post-update inspect report, so the handler maps the same detail DTO
// the detail GET does — one round trip carries the new state. The
// request → podman update options mapping lives in the pure update
// builder (buildUpdateOptions, update_spec.go — the update twin of the
// create builder settlement, backend-conventions); the options are
// built before the lease is acquired, so a request-local validation
// failure (the unparseable memory, a malformed duration, a
// non-positive cpus, the disable-flag exclusivity) never costs an
// engine lease. Patch semantics are podman's own contract: only the
// fields the request carries are forwarded, and podman merges resource
// fields. Like Rename there is no state gate — podman applies limit
// changes live to a running container and rewrites a stopped one's
// config for the next start — and a missing container answers
// ErrContainerNotFound, the same envelope as start/stop.
func (s *ContainerService) Update(ctx context.Context, userID, hostID, containerID string, request ContainerUpdateRequest) (*entities.ContainerInspectReport, error) {
	opts, err := buildUpdateOptions(request)
	if err != nil {
		return nil, err
	}

	var report *entities.ContainerInspectReport
	err = s.engines.WithEngine(ctx, userID, hostID, "containers.Update", func(e ContainerEngine) error {
		if err := e.Update(containerID, opts); err != nil {
			if podmanerr.IsNotFound(err) {
				return appErrors.ErrContainerNotFound
			}
			return fmt.Errorf("failed to update container: %w", err)
		}

		inspectReport, err := inspectContainer(e, containerID)
		if err != nil {
			return err
		}

		report = inspectReport
		return nil
	})
	return report, err
}

// Delete removes a container by ID on the specified host. Without force,
// podman refuses to remove a running (or paused) container — that refusal
// surfaces as 409 CONTAINER_RUNNING carrying podman's message; with force,
// podman removes it regardless of state. Unknown containers surface as the
// clean 404 CONTAINER_NOT_FOUND.
//
// The classification covers the remove call's own failure too, like the
// networks delete: a remove that errors at the call rather than in podman's
// report — the race window where the container vanished or started between
// the request and podman's execution — answers the same terminals (404
// CONTAINER_NOT_FOUND / 409 CONTAINER_RUNNING) instead of a wrapped
// unclassified error. The remove-and-classify ritual itself rides
// internal/removal (issue 264).
func (s *ContainerService) Delete(ctx context.Context, userID, hostID, containerID string, force bool) error {
	return s.engines.WithEngine(ctx, userID, hostID, "containers.Delete", func(e ContainerEngine) error {
		return removal.Run(func() error {
			return e.Remove(containerID, force)
		}, removal.Terminals{
			Missing: appErrors.ErrContainerNotFound,
			// Podman's compat layer answers the running-container refusal as
			// a 500 whose message ends in the ErrCtrStateInvalid text, not a
			// 409; the containers-only predicate owns that shape (the shared
			// podmanerr.IsConflict deliberately does not, so volumes,
			// networks and secrets keep their own in-use refusals), and the
			// builder threads podman's message so the UI can explain the
			// state and offer force.
			Refusal: func(cause string) error {
				return appErrors.ErrContainerRunning.WithMessage(cause)
			},
			IsRefusal: podmanerr.IsRunningContainerRefusal,
		}, "failed to remove container")
	})
}

// Prune removes every stopped container on the host in one call and
// returns podman's per-container reports. Running containers are
// untouched — podman's own semantics, which is the blast-radius
// mitigation for this endpoint.
func (s *ContainerService) Prune(ctx context.Context, userID, hostID string) ([]*reports.PruneReport, error) {
	var prunes []*reports.PruneReport
	err := s.engines.WithEngine(ctx, userID, hostID, "containers.Prune", func(e ContainerEngine) error {
		p, err := e.Prune()
		if err != nil {
			return fmt.Errorf("failed to prune containers: %w", err)
		}
		prunes = p
		return nil
	})
	return prunes, err
}

// Stats answers a one-shot stats snapshot: every running container on
// the host when no target IDs are given (all set), or the target
// containers' snapshot otherwise. A not-found answer is the domain's
// existing 404 sentinel with podman's own message riding along as the
// disambiguator: podman's stats handler maps any first-report error to
// 404, so an unknown container and a known-but-not-running one cannot
// be honestly distinguished here.
func (s *ContainerService) Stats(ctx context.Context, userID, hostID string, ids []string, all bool) ([]libpoddefine.ContainerStats, error) {
	var stats []libpoddefine.ContainerStats
	err := s.engines.WithEngine(ctx, userID, hostID, "containers.Stats", func(e ContainerEngine) error {
		snapshot, err := e.Stats(ids, all)
		if err != nil {
			if podmanerr.IsNotFound(err) {
				return appErrors.ErrContainerNotFound.WithMessage(podmanerr.Message(err))
			}
			return fmt.Errorf("failed to get container stats: %w", err)
		}
		stats = snapshot
		return nil
	})
	return stats, err
}

// GetLogs retrieves logs for a specific container on the specified
// host. The buffered ritual (options build, writer pair, combine,
// not-found and cancellation mapping) belongs to the kit's FetchLogs;
// the service keeps validation, engine acquisition, and dispatch.
func (s *ContainerService) GetLogs(ctx context.Context, userID, hostID, containerID string, since time.Time, tail int64) (*ContainerLogsResponse, error) {
	var response *ContainerLogsResponse
	err := s.engines.WithEngine(ctx, userID, hostID, "containers.GetLogs", func(e ContainerEngine) error {
		logs, err := streaming.FetchLogs(ctx, streaming.FetchLogsOp{
			Target:   containerID,
			Since:    since,
			Tail:     tail,
			NotFound: appErrors.ErrContainerNotFound,
			Logs:     e.Logs,
		})
		if err != nil {
			return err
		}

		response = &ContainerLogsResponse{
			ID:   containerID,
			Logs: logs,
		}
		return nil
	})
	return response, err
}

// GetLogsWithStream retrieves logs for a specific container with
// streaming callback. The follow ritual (options build, writer pair,
// not-found and cancellation mapping) belongs to the kit's FollowLogs;
// the service keeps validation, engine acquisition, and target dispatch.
func (s *ContainerService) GetLogsWithStream(ctx context.Context, userID, hostID, containerID string, since time.Time, tail int64, logCallback func(event streaming.LogEvent)) error {
	return s.engines.WithEngine(ctx, userID, hostID, "containers.GetLogsWithStream", func(e ContainerEngine) error {
		return streaming.FollowLogs(ctx, streaming.FollowLogsOp{
			Target:   containerID,
			Since:    since,
			Tail:     tail,
			NotFound: appErrors.ErrContainerNotFound,
			Logs:     e.Logs,
			OnEvent:  logCallback,
		})
	})
}
