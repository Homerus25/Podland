package containers

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"github.com/Homerus25/podland/backend/internal/streaming"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/specgen"
)

// ContainerService provides business logic for containers. Every
// machine-scoped podman call crosses the ContainerEngineProvider (the
// connection seam); the service itself owns only domain mapping,
// composition, and the log-line event conversion.
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

// Create creates a new container on the specified machine
func (s *ContainerService) Create(ctx context.Context, userID, machineID string, request ContainerCreateRequest) (*ContainerCreateResponse, error) {
	var response *ContainerCreateResponse
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
		createResponse, err := e.Create(spec)
		if err != nil {
			return fmt.Errorf("failed to create container: %w", err)
		}

		// Convert the response to our DTO format
		response = &ContainerCreateResponse{
			ID:        createResponse.Id,
			Name:      request.Name,
			State:     "created",
			Image:     request.Image,
			ImageID:   "", // ImageID is not available in ContainerCreateReport
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}

		return nil
	})
	return response, err
}

// Get returns detailed information about a specific container
func (s *ContainerService) Get(ctx context.Context, userID, machineID, containerID string) (*entities.ContainerInspectReport, error) {
	var report *entities.ContainerInspectReport
	err := s.engines.WithEngine(ctx, userID, machineID, "containers.Get", func(e ContainerEngine) error {
		// Call the official podman ContainerInspect method
		containerReports, _, err := e.Inspect(containerID)
		if err != nil {
			return fmt.Errorf("failed to inspect container: %w", err)
		}

		if len(containerReports) == 0 {
			return fmt.Errorf("no container found with ID: %s", containerID)
		}

		report = containerReports[0]
		return nil
	})
	return report, err
}

// validateContainerOperationParams validates common parameters for container operations
func validateContainerOperationParams(userID, machineID, containerID string) error {
	if userID == "" {
		return fmt.Errorf("userID is required")
	}
	if machineID == "" {
		return fmt.Errorf("machineID is required")
	}
	if containerID == "" {
		return fmt.Errorf("containerID is required")
	}
	return nil
}

// Start starts a container by ID on the specified machine
func (s *ContainerService) Start(ctx context.Context, userID, machineID, containerID string) (*ContainerStartResponse, error) {
	if err := validateContainerOperationParams(userID, machineID, containerID); err != nil {
		return nil, err
	}

	var response *ContainerStartResponse
	err := s.engines.WithEngine(ctx, userID, machineID, "containers.Start", func(e ContainerEngine) error {
		startReports, err := e.Start(containerID)
		if err != nil {
			return fmt.Errorf("failed to start container: %w", err)
		}

		if len(startReports) == 0 {
			return fmt.Errorf("no start report returned for container: %s", containerID)
		}

		containerReports, _, err := e.Inspect(containerID)
		containerName := containerID
		if err == nil && len(containerReports) > 0 {
			if len(containerReports[0].Name) > 0 {
				containerName = strings.TrimPrefix(containerReports[0].Name, "/")
			}
		}

		response = &ContainerStartResponse{
			ID:        containerID,
			Name:      containerName,
			State:     "running",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
		return nil
	})
	return response, err
}

// Stop stops a container by ID on the specified machine
func (s *ContainerService) Stop(ctx context.Context, userID, machineID, containerID string) (*ContainerStopResponse, error) {
	if err := validateContainerOperationParams(userID, machineID, containerID); err != nil {
		return nil, err
	}

	var response *ContainerStopResponse
	err := s.engines.WithEngine(ctx, userID, machineID, "containers.Stop", func(e ContainerEngine) error {
		stopReports, err := e.Stop(containerID)
		if err != nil {
			return fmt.Errorf("failed to stop container: %w", err)
		}

		if len(stopReports) == 0 {
			return fmt.Errorf("no stop report returned for container: %s", containerID)
		}

		containerReports, _, err := e.Inspect(containerID)
		containerName := containerID
		if err == nil && len(containerReports) > 0 {
			if len(containerReports[0].Name) > 0 {
				containerName = strings.TrimPrefix(containerReports[0].Name, "/")
			}
		}

		response = &ContainerStopResponse{
			ID:        containerID,
			Name:      containerName,
			State:     "stopped",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
		return nil
	})
	return response, err
}

// Delete removes a container by ID on the specified machine.
// The force option allows removing running containers.
func (s *ContainerService) Delete(ctx context.Context, userID, machineID, containerID string, force bool) error {
	if err := validateContainerOperationParams(userID, machineID, containerID); err != nil {
		return err
	}

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

// GetLogs retrieves logs for a specific container on the specified machine
func (s *ContainerService) GetLogs(ctx context.Context, userID, machineID, containerID string, since time.Time, tail int64) (*ContainerLogsResponse, error) {
	if err := validateContainerOperationParams(userID, machineID, containerID); err != nil {
		return nil, err
	}

	var response *ContainerLogsResponse
	err := s.engines.WithEngine(ctx, userID, machineID, "containers.GetLogs", func(e ContainerEngine) error {
		// Prepare buffers for stdout and stderr
		var stdoutBuffer, stderrBuffer bytes.Buffer

		// Set up container logs options
		logsOpts := entities.ContainerLogsOptions{
			Follow:     false,
			Since:      since,
			Tail:       tail,
			Timestamps: false,
		}

		// Set writers to capture output
		logsOpts.StdoutWriter = &stdoutBuffer
		logsOpts.StderrWriter = &stderrBuffer

		// Fetch logs from container
		err := e.Logs(containerID, logsOpts)
		if err != nil {
			if podmanerr.IsNotFound(err) {
				return appErrors.ErrContainerNotFound
			}
			return fmt.Errorf("failed to get container logs: %w", err)
		}

		// Combine stdout and stderr logs
		combinedLogs := strings.TrimSuffix(stdoutBuffer.String(), "\n")
		if stderrBuffer.Len() > 0 {
			stderrContent := strings.TrimSuffix(stderrBuffer.String(), "\n")
			if combinedLogs != "" {
				combinedLogs += "\n" + stderrContent
			} else {
				combinedLogs = stderrContent
			}
		}

		response = &ContainerLogsResponse{
			ID:   containerID,
			Logs: combinedLogs,
		}
		return nil
	})
	return response, err
}

// GetLogsWithStream retrieves logs for a specific container with streaming callback
// When follow=true, this method streams log lines one at a time via the logCallback function.
// The kit's LogEventWriter converts each podman log line to a LogEvent:
// stdout at INFO, stderr at ERROR.
func (s *ContainerService) GetLogsWithStream(ctx context.Context, userID, machineID, containerID string, since time.Time, tail int64, logCallback func(event streaming.LogEvent)) error {
	if err := validateContainerOperationParams(userID, machineID, containerID); err != nil {
		return err
	}

	return s.engines.WithEngine(ctx, userID, machineID, "containers.GetLogsWithStream", func(e ContainerEngine) error {
		// Set up container logs options for streaming
		logsOpts := entities.ContainerLogsOptions{
			Follow:     true,
			Since:      since,
			Tail:       tail,
			Timestamps: true,
		}

		// Set writers to capture streaming output
		logsOpts.StdoutWriter = streaming.NewLogEventWriter("INFO", logCallback)
		logsOpts.StderrWriter = streaming.NewLogEventWriter("ERROR", logCallback)

		// Fetch logs from container (this will block until follow ends or context is cancelled)
		err := e.Logs(containerID, logsOpts)
		if err != nil {
			if podmanerr.IsNotFound(err) {
				return appErrors.ErrContainerNotFound
			}
			if ctx.Err() == context.Canceled {
				return appErrors.ErrOperationCancelled
			}
			return fmt.Errorf("failed to get container logs: %w", err)
		}

		return nil
	})
}
