package containers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/inspect"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"github.com/Homerus25/podland/backend/internal/streaming"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
)

// ContainerService provides business logic for containers. Every
// machine-scoped podman call crosses the ContainerEngineProvider (the
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

// List returns a list of containers on the specified machine
func (s *ContainerService) List(ctx context.Context, userID, machineID string, opts entities.ContainerListOptions) ([]entities.ListContainer, error) {
	var containers []entities.ListContainer
	err := s.engines.WithEngine(ctx, userID, machineID, "containers.List", func(e ContainerEngine) error {
		list, err := e.List(opts)
		if err != nil {
			return fmt.Errorf("failed to list containers: %w", err)
		}
		containers = list
		return nil
	})
	return containers, err
}

// Create creates a new container on the specified machine. The
// request → specgen mapping lives in the pure builder (buildCreateSpec,
// create_spec.go — the per-domain create builder settlement,
// backend-conventions); the spec is built before the lease is acquired,
// so a request-local validation failure (the unparseable memory) never
// costs an engine lease, and this closure is a thin lease: the engine
// call plus the not-found classification. The service returns podman's
// create report; the handler maps the response DTO.
func (s *ContainerService) Create(ctx context.Context, userID, machineID string, request ContainerCreateRequest) (*entities.ContainerCreateReport, error) {
	spec, err := buildCreateSpec(request)
	if err != nil {
		return nil, err
	}

	var report *entities.ContainerCreateReport
	err = s.engines.WithEngine(ctx, userID, machineID, "containers.Create", func(e ContainerEngine) error {
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
func (s *ContainerService) Get(ctx context.Context, userID, machineID, containerID string) (*entities.ContainerInspectReport, error) {
	var report *entities.ContainerInspectReport
	err := s.engines.WithEngine(ctx, userID, machineID, "containers.Get", func(e ContainerEngine) error {
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
func (s *ContainerService) Inspect(ctx context.Context, userID, machineID, containerID string) (*entities.ContainerInspectReport, error) {
	var report *entities.ContainerInspectReport
	err := s.engines.WithEngine(ctx, userID, machineID, "containers.Inspect", func(e ContainerEngine) error {
		inspectReport, err := inspectContainer(e, containerID)
		if err != nil {
			return err
		}

		report = inspectReport
		return nil
	})
	return report, err
}

// Start starts a container by ID on the specified machine. The service
// inspects the container first — one inspect serves existence, state,
// and name — and answers the documented wrong-state terminal before
// any start call: podman's already-running refusal produces no error
// shape through the tunnel (verified live, issue #270), so the
// precondition is state-based, mirroring the quadlets
// checkPrecondition. The engine folds podman's report slice into one
// error; a missing container answers ErrContainerNotFound, anything
// else is wrapped, never answered as a running success; the handler
// maps the result to the response DTO.
func (s *ContainerService) Start(ctx context.Context, userID, machineID, containerID string) (*ContainerStartResult, error) {
	var result *ContainerStartResult
	err := s.engines.WithEngine(ctx, userID, machineID, "containers.Start", func(e ContainerEngine) error {
		report, err := inspectContainer(e, containerID)
		if err != nil {
			return err
		}

		if containerRunning(report) {
			return appErrors.ErrContainerAlreadyRunning
		}

		if err := e.Start(containerID); err != nil {
			if podmanerr.IsNotFound(err) {
				return appErrors.ErrContainerNotFound
			}
			return fmt.Errorf("failed to start container: %w", err)
		}

		result = &ContainerStartResult{
			ID:   containerID,
			Name: containerName(report, containerID),
		}
		return nil
	})
	return result, err
}

// Stop stops a container by ID on the specified machine. The service
// inspects the container first — one inspect serves existence, state,
// and name — and answers the documented wrong-state terminal before
// any stop call: podman's not-running refusal produces no error shape
// through the tunnel (verified live, issue #270), so the precondition
// is state-based, mirroring the quadlets checkPrecondition. The
// engine folds podman's report slice into one error; a missing
// container answers ErrContainerNotFound, anything else is wrapped,
// never answered as a stopped success; the handler maps the result to
// the response DTO.
func (s *ContainerService) Stop(ctx context.Context, userID, machineID, containerID string) (*ContainerStopResult, error) {
	var result *ContainerStopResult
	err := s.engines.WithEngine(ctx, userID, machineID, "containers.Stop", func(e ContainerEngine) error {
		report, err := inspectContainer(e, containerID)
		if err != nil {
			return err
		}

		if !containerRunning(report) {
			return appErrors.ErrContainerNotRunning
		}

		if err := e.Stop(containerID); err != nil {
			if podmanerr.IsNotFound(err) {
				return appErrors.ErrContainerNotFound
			}
			return fmt.Errorf("failed to stop container: %w", err)
		}

		result = &ContainerStopResult{
			ID:   containerID,
			Name: containerName(report, containerID),
		}
		return nil
	})
	return result, err
}

// Restart restarts a running container by ID on the specified machine.
// Like Stop it inspects first — one inspect serves existence, state, and
// name — and answers the documented wrong-state terminal before any
// restart call: a stopped container is Start's caller, not this one, so
// the precondition is state-based like Stop's. No timeout or force
// parameters travel: podman's default timeout applies (the quadlets
// restart's no-force precedent, issue 205). The engine folds podman's
// report slice into one error; a missing container answers
// ErrContainerNotFound, anything else is wrapped, never answered as a
// restarted success; the handler maps the result to the response DTO.
func (s *ContainerService) Restart(ctx context.Context, userID, machineID, containerID string) (*ContainerRestartResult, error) {
	var result *ContainerRestartResult
	err := s.engines.WithEngine(ctx, userID, machineID, "containers.Restart", func(e ContainerEngine) error {
		report, err := inspectContainer(e, containerID)
		if err != nil {
			return err
		}

		if !containerRunning(report) {
			return appErrors.ErrContainerNotRunning
		}

		if err := e.Restart(containerID); err != nil {
			if podmanerr.IsNotFound(err) {
				return appErrors.ErrContainerNotFound
			}
			return fmt.Errorf("failed to restart container: %w", err)
		}

		result = &ContainerRestartResult{
			ID:   containerID,
			Name: containerName(report, containerID),
		}
		return nil
	})
	return result, err
}

// inspectContainer resolves one container's inspect report through the
// shared single-report ritual, classifying podman's not-found shapes
// into ErrContainerNotFound. It is the pre-action read for Start, Stop,
// and Restart: one inspect serves existence, state, and name.
func inspectContainer(e ContainerEngine, containerID string) (*entities.ContainerInspectReport, error) {
	reports, inspectErrs, err := e.Inspect(containerID)
	return inspect.Resolve(reports, inspectErrs, err, appErrors.ErrContainerNotFound, "container", containerID)
}

// Rename renames a container on the specified machine and answers the
// container's post-rename inspect report, so the handler maps the same
// detail DTO the detail GET does. Podman renames running and stopped
// containers alike — no state restriction, and no quadlet-owned
// restriction either (podman's CLI imposes neither) — so unlike
// Start/Stop/Restart there is no pre-action state gate. The name's
// shape is podman's to validate: an invalid name is a client error and
// answers 400 INVALID_REQUEST carrying podman's message, while a name
// already in use answers the domain's 409 collision sentinel.
func (s *ContainerService) Rename(ctx context.Context, userID, machineID, containerID, newName string) (*entities.ContainerInspectReport, error) {
	var report *entities.ContainerInspectReport
	err := s.engines.WithEngine(ctx, userID, machineID, "containers.Rename", func(e ContainerEngine) error {
		if err := e.Rename(containerID, newName); err != nil {
			if podmanerr.IsAlreadyExists(err) {
				return appErrors.ErrContainerAlreadyExists
			}
			if podmanerr.IsNotFound(err) {
				return appErrors.ErrContainerNotFound
			}
			if podmanerr.IsClientError(err) {
				return appErrors.ErrInvalidRequest.WithMessage(podmanerr.Message(err))
			}
			// Podman answers a name collision as a 500 sqlite UNIQUE
			// constraint failure rather than a typed conflict
			// (RenameContainer → RewriteContainerConfig in podman's
			// source), so the collision is resolved by asking whether
			// the name is taken instead of by classifying the failure
			// text.
			taken, takenErr := containerNameTaken(e, newName)
			if takenErr != nil {
				return takenErr
			}
			if taken {
				return appErrors.ErrContainerAlreadyExists
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

// containerNameTaken reports whether a container already carries name.
// A not-found answer means the name is free; any other inspect failure
// is reported as-is, so a broken connection never masquerades as a name
// collision.
func containerNameTaken(e ContainerEngine, name string) (bool, error) {
	reports, inspectErrs, err := e.Inspect(name)
	_, resolveErr := inspect.Resolve(reports, inspectErrs, err, appErrors.ErrContainerNotFound, "container", name)
	if resolveErr == nil {
		return true, nil
	}
	if errors.Is(resolveErr, appErrors.ErrContainerNotFound) {
		return false, nil
	}
	return false, resolveErr
}

// containerRunning reports the container's running state, nil-safe
// through the embedded inspect data. A paused container counts as
// running: podman's State.Running is true for it.
func containerRunning(report *entities.ContainerInspectReport) bool {
	return report != nil && report.InspectContainerData != nil && report.State != nil && report.State.Running
}

// containerName resolves the container's name from the inspect report,
// trimming podman's leading slash and falling back to the container ID.
func containerName(report *entities.ContainerInspectReport, containerID string) string {
	if report != nil && report.InspectContainerData != nil && len(report.Name) > 0 {
		return strings.TrimPrefix(report.Name, "/")
	}
	return containerID
}

// Delete removes a container by ID on the specified machine.
// The force option allows removing running containers. The engine
// folds podman's report slice into one error; a missing container
// answers ErrContainerNotFound, anything else is wrapped.
func (s *ContainerService) Delete(ctx context.Context, userID, machineID, containerID string, force bool) error {
	return s.engines.WithEngine(ctx, userID, machineID, "containers.Delete", func(e ContainerEngine) error {
		if err := e.Remove(containerID, force); err != nil {
			if podmanerr.IsNotFound(err) {
				return appErrors.ErrContainerNotFound
			}
			return fmt.Errorf("failed to remove container: %w", err)
		}

		return nil
	})
}

// Prune removes every stopped container on the machine in one call and
// returns podman's per-container reports. Running containers are
// untouched — podman's own semantics, which is the blast-radius
// mitigation for this endpoint.
func (s *ContainerService) Prune(ctx context.Context, userID, machineID string) ([]*reports.PruneReport, error) {
	var prunes []*reports.PruneReport
	err := s.engines.WithEngine(ctx, userID, machineID, "containers.Prune", func(e ContainerEngine) error {
		p, err := e.Prune()
		if err != nil {
			return fmt.Errorf("failed to prune containers: %w", err)
		}
		prunes = p
		return nil
	})
	return prunes, err
}

// GetLogs retrieves logs for a specific container on the specified
// machine. The buffered ritual (options build, writer pair, combine,
// not-found and cancellation mapping) belongs to the kit's FetchLogs;
// the service keeps validation, engine acquisition, and dispatch.
func (s *ContainerService) GetLogs(ctx context.Context, userID, machineID, containerID string, since time.Time, tail int64) (*ContainerLogsResponse, error) {
	var response *ContainerLogsResponse
	err := s.engines.WithEngine(ctx, userID, machineID, "containers.GetLogs", func(e ContainerEngine) error {
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
func (s *ContainerService) GetLogsWithStream(ctx context.Context, userID, machineID, containerID string, since time.Time, tail int64, logCallback func(event streaming.LogEvent)) error {
	return s.engines.WithEngine(ctx, userID, machineID, "containers.GetLogsWithStream", func(e ContainerEngine) error {
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
