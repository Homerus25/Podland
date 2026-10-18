package containers

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/inspect"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"github.com/Homerus25/podland/backend/internal/streaming"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/specgen"
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

// Create creates a new container on the specified machine. The service
// returns podman's create report; the handler maps the response DTO.
func (s *ContainerService) Create(ctx context.Context, userID, machineID string, request ContainerCreateRequest) (*entities.ContainerCreateReport, error) {
	var report *entities.ContainerCreateReport
	err := s.engines.WithEngine(ctx, userID, machineID, "containers.Create", func(e ContainerEngine) error {
		// Create a basic spec generator with the image
		spec := specgen.NewSpecGenerator(request.Image, false)

		// Set container name if provided
		if request.Name != "" {
			spec.Name = request.Name
		}

		// Set command if provided
		if len(request.Command) > 0 {
			spec.Command = request.Command
		}

		// Set working directory if provided
		if request.WorkingDir != "" {
			spec.WorkDir = request.WorkingDir
		}

		// Set environment variables if provided
		if len(request.Environment) > 0 {
			if spec.Env == nil {
				spec.Env = make(map[string]string)
			}
			maps.Copy(spec.Env, request.Environment)
		}

		// Set labels if provided
		if len(request.Labels) > 0 {
			if spec.Labels == nil {
				spec.Labels = make(map[string]string)
			}
			maps.Copy(spec.Labels, request.Labels)
		}

		// Note: For now, we're skipping port bindings, mounts, privileged mode,
		// restart policy, and resource limits due to API complexity.
		// These can be added in a future enhancement.

		// Create the container using Podman API
		createReport, err := e.Create(spec)
		if err != nil {
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
// checkPrecondition. A missing container answers ErrContainerNotFound,
// a per-item report error is classified and wrapped, never answered as
// a running success; the handler maps the result to the response DTO.
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

		startReports, err := e.Start(containerID)
		if err != nil {
			if podmanerr.IsNotFound(err) {
				return appErrors.ErrContainerNotFound
			}
			return fmt.Errorf("failed to start container: %w", err)
		}

		if len(startReports) == 0 {
			return fmt.Errorf("no start report returned for container: %s", containerID)
		}

		if startReports[0].Err != nil {
			if podmanerr.IsNotFound(startReports[0].Err) {
				return appErrors.ErrContainerNotFound
			}
			return fmt.Errorf("failed to start container: %w", startReports[0].Err)
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
// is state-based, mirroring the quadlets checkPrecondition. A missing
// container answers ErrContainerNotFound, a per-item report error is
// classified and wrapped, never answered as a stopped success; the
// handler maps the result to the response DTO.
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

		stopReports, err := e.Stop(containerID)
		if err != nil {
			if podmanerr.IsNotFound(err) {
				return appErrors.ErrContainerNotFound
			}
			return fmt.Errorf("failed to stop container: %w", err)
		}

		if len(stopReports) == 0 {
			return fmt.Errorf("no stop report returned for container: %s", containerID)
		}

		if stopReports[0].Err != nil {
			if podmanerr.IsNotFound(stopReports[0].Err) {
				return appErrors.ErrContainerNotFound
			}
			return fmt.Errorf("failed to stop container: %w", stopReports[0].Err)
		}

		result = &ContainerStopResult{
			ID:   containerID,
			Name: containerName(report, containerID),
		}
		return nil
	})
	return result, err
}

// inspectContainer resolves one container's inspect report through the
// shared single-report ritual, classifying podman's not-found shapes
// into ErrContainerNotFound. It is the pre-action read for Start and
// Stop: one inspect serves existence, state, and name.
func inspectContainer(e ContainerEngine, containerID string) (*entities.ContainerInspectReport, error) {
	reports, inspectErrs, err := e.Inspect(containerID)
	return inspect.Resolve(reports, inspectErrs, err, appErrors.ErrContainerNotFound, "container", containerID)
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
// The force option allows removing running containers.
func (s *ContainerService) Delete(ctx context.Context, userID, machineID, containerID string, force bool) error {
	return s.engines.WithEngine(ctx, userID, machineID, "containers.Delete", func(e ContainerEngine) error {
		rmReports, err := e.Remove(containerID, force)
		if err != nil {
			if podmanerr.IsNotFound(err) {
				return appErrors.ErrContainerNotFound
			}
			return fmt.Errorf("failed to remove container: %w", err)
		}

		if len(rmReports) == 0 {
			return fmt.Errorf("no remove report returned for container: %s", containerID)
		}

		if rmReports[0].Err != nil {
			if podmanerr.IsNotFound(rmReports[0].Err) {
				return appErrors.ErrContainerNotFound
			}
			return fmt.Errorf("failed to remove container: %w", rmReports[0].Err)
		}

		return nil
	})
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
