package containers

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/yourorg/podland/backend/internal/connections"
	"github.com/yourorg/podland/backend/internal/domain/machines"
	appErrors "github.com/yourorg/podland/backend/internal/errors"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
	"go.podman.io/podman/v6/pkg/specgen"
)

// concreteConnectionManager wraps a *connections.ConnectionManager to implement ConnectionManagerInterface
// This is needed for backward compatibility
type concreteConnectionManager struct {
	connManager *connections.ConnectionManager
}

// GetConnection implements ConnectionManagerInterface by calling the underlying ConnectionManager
func (c *concreteConnectionManager) GetConnection(ctx context.Context, machineID, userID string) (any, error) {
	return c.connManager.GetConnection(ctx, machineID, userID)
}

func (c *concreteConnectionManager) Release(machineID string) {
	c.connManager.Release(machineID)
}

// ContainerServiceInterface defines the interface for container services
// This interface is used for dependency injection and testing
type ContainerServiceInterface interface {
	List(ctx context.Context, userID, machineID string, opts entities.ContainerListOptions) ([]entities.ListContainer, error)
	Create(ctx context.Context, userID, machineID string, request ContainerCreateRequest) (*ContainerCreateResponse, error)
	Get(ctx context.Context, userID, machineID, containerID string) (*entities.ContainerInspectReport, error)
	Start(ctx context.Context, userID, machineID, containerID string) (*ContainerStartResponse, error)
	Stop(ctx context.Context, userID, machineID, containerID string) (*ContainerStopResponse, error)
	Delete(ctx context.Context, userID, machineID, containerID string, force bool) error
	GetLogs(ctx context.Context, userID, machineID, containerID string, since time.Time, tail int64) (*ContainerLogsResponse, error)
}

// ContainerService provides business logic for containers
type ContainerService struct {
	connManager machines.ConnectionManagerInterface
}

// NewContainerService creates a new ContainerService
func NewContainerService(connManager machines.ConnectionManagerInterface) *ContainerService {
	return &ContainerService{
		connManager: connManager,
	}
}

// NewContainerServiceWithConcreteDeps creates a new ContainerService with concrete dependencies
// This maintains backward compatibility with existing code that uses *connections.ConnectionManager
func NewContainerServiceWithConcreteDeps(connManager *connections.ConnectionManager) *ContainerService {
	return NewContainerService(&concreteConnectionManager{connManager: connManager})
}

// List returns a list of containers on the specified machine
func (s *ContainerService) List(ctx context.Context, userID, machineID string, opts entities.ContainerListOptions) ([]entities.ListContainer, error) {
	// Get connection context to the machine
	connCtxInterface, err := s.connManager.GetConnection(context.Background(), machineID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection to machine: %w", err)
	}

	defer s.connManager.Release(machineID)

	// Convert interface to context.Context
	connCtx, ok := connCtxInterface.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid connection type: expected context.Context")
	}

	// Create container engine with the connection context
	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	// Call the official podman ContainerList method
	containers, err := containerEngine.ContainerList(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	return containers, nil
}

// Create creates a new container on the specified machine
func (s *ContainerService) Create(ctx context.Context, userID, machineID string, request ContainerCreateRequest) (*ContainerCreateResponse, error) {
	// Get connection context to the machine
	connCtxInterface, err := s.connManager.GetConnection(context.Background(), machineID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection to machine: %w", err)
	}
	defer s.connManager.Release(machineID)

	// Convert interface to context.Context
	connCtx, ok := connCtxInterface.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid connection type: expected context.Context")
	}

	// Create container engine with the connection context
	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

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
	createResponse, err := containerEngine.ContainerCreate(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Convert the response to our DTO format
	response := &ContainerCreateResponse{
		ID:        createResponse.Id,
		Name:      request.Name,
		State:     "created",
		Image:     request.Image,
		ImageID:   "", // ImageID is not available in ContainerCreateReport
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	return response, nil
}

// Get returns detailed information about a specific container
func (s *ContainerService) Get(ctx context.Context, userID, machineID, containerID string) (*entities.ContainerInspectReport, error) {
	// Get connection context to the machine
	connCtxInterface, err := s.connManager.GetConnection(context.Background(), machineID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection to machine: %w", err)
	}
	defer s.connManager.Release(machineID)

	// Convert interface to context.Context
	connCtx, ok := connCtxInterface.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid connection type: expected context.Context")
	}

	// Create container engine with the connection context
	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	// Call the official podman ContainerInspect method
	containerReports, _, err := containerEngine.ContainerInspect(ctx, []string{containerID}, entities.InspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	if len(containerReports) == 0 {
		return nil, fmt.Errorf("no container found with ID: %s", containerID)
	}

	return containerReports[0], nil
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

	connCtxInterface, err := s.connManager.GetConnection(context.Background(), machineID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection to machine: %w", err)
	}
	defer s.connManager.Release(machineID)

	connCtx, ok := connCtxInterface.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid connection type: expected context.Context")
	}

	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	startReports, err := containerEngine.ContainerStart(ctx, []string{containerID}, entities.ContainerStartOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	if len(startReports) == 0 {
		return nil, fmt.Errorf("no start report returned for container: %s", containerID)
	}

	containerReports, _, err := containerEngine.ContainerInspect(ctx, []string{containerID}, entities.InspectOptions{})
	containerName := containerID
	if err == nil && len(containerReports) > 0 {
		if len(containerReports[0].Name) > 0 {
			containerName = strings.TrimPrefix(containerReports[0].Name, "/")
		}
	}

	return &ContainerStartResponse{
		ID:        containerID,
		Name:      containerName,
		State:     "running",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// Stop stops a container by ID on the specified machine
func (s *ContainerService) Stop(ctx context.Context, userID, machineID, containerID string) (*ContainerStopResponse, error) {
	if err := validateContainerOperationParams(userID, machineID, containerID); err != nil {
		return nil, err
	}

	connCtxInterface, err := s.connManager.GetConnection(context.Background(), machineID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection to machine: %w", err)
	}
	defer s.connManager.Release(machineID)

	connCtx, ok := connCtxInterface.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid connection type: expected context.Context")
	}

	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	stopReports, err := containerEngine.ContainerStop(ctx, []string{containerID}, entities.StopOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to stop container: %w", err)
	}

	if len(stopReports) == 0 {
		return nil, fmt.Errorf("no stop report returned for container: %s", containerID)
	}

	containerReports, _, err := containerEngine.ContainerInspect(ctx, []string{containerID}, entities.InspectOptions{})
	containerName := containerID
	if err == nil && len(containerReports) > 0 {
		if len(containerReports[0].Name) > 0 {
			containerName = strings.TrimPrefix(containerReports[0].Name, "/")
		}
	}

	return &ContainerStopResponse{
		ID:        containerID,
		Name:      containerName,
		State:     "stopped",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// Delete removes a container by ID on the specified machine.
// The force option allows removing running containers.
func (s *ContainerService) Delete(ctx context.Context, userID, machineID, containerID string, force bool) error {
	if err := validateContainerOperationParams(userID, machineID, containerID); err != nil {
		return err
	}

	connCtxInterface, err := s.connManager.GetConnection(context.Background(), machineID, userID)
	if err != nil {
		return fmt.Errorf("failed to get connection to machine: %w", err)
	}
	defer s.connManager.Release(machineID)

	connCtx, ok := connCtxInterface.(context.Context)
	if !ok {
		return fmt.Errorf("invalid connection type: expected context.Context")
	}

	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	rmReports, err := containerEngine.ContainerRm(ctx, []string{containerID}, entities.RmOptions{
		Force: force,
	})
	if err != nil {
		if strings.Contains(err.Error(), "no such container") {
			return appErrors.ErrContainerNotFound
		}
		return fmt.Errorf("failed to remove container: %w", err)
	}

	if len(rmReports) == 0 {
		return fmt.Errorf("no remove report returned for container: %s", containerID)
	}

	if rmReports[0].Err != nil {
		if strings.Contains(rmReports[0].Err.Error(), "no such container") {
			return appErrors.ErrContainerNotFound
		}
		return fmt.Errorf("failed to remove container: %w", rmReports[0].Err)
	}

	return nil
}

// GetLogs retrieves logs for a specific container on the specified machine
func (s *ContainerService) GetLogs(ctx context.Context, userID, machineID, containerID string, since time.Time, tail int64) (*ContainerLogsResponse, error) {
	if err := validateContainerOperationParams(userID, machineID, containerID); err != nil {
		return nil, err
	}

	connCtxInterface, err := s.connManager.GetConnection(context.Background(), machineID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection to machine: %w", err)
	}
	defer s.connManager.Release(machineID)

	connCtx, ok := connCtxInterface.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid connection type: expected context.Context")
	}

	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

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
	err = containerEngine.ContainerLogs(ctx, []string{containerID}, logsOpts)
	if err != nil {
		if strings.Contains(err.Error(), "no such container") || strings.Contains(err.Error(), "container not found") {
			return nil, appErrors.ErrContainerNotFound
		}
		return nil, fmt.Errorf("failed to get container logs: %w", err)
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

	return &ContainerLogsResponse{
		ID:   containerID,
		Logs: combinedLogs,
	}, nil
}
