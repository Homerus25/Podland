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
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	netTypes "go.podman.io/common/libnetwork/types"
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

		// Port bindings map to the spec's port mappings; the protocol
		// defaults to tcp when the request omits it.
		if len(request.PortBindings) > 0 {
			spec.PortMappings = make([]netTypes.PortMapping, 0, len(request.PortBindings))
			for _, binding := range request.PortBindings {
				protocol := binding.Protocol
				if protocol == "" {
					protocol = "tcp"
				}
				spec.PortMappings = append(spec.PortMappings, netTypes.PortMapping{
					HostPort:      uint16(binding.HostPort),
					ContainerPort: uint16(binding.ContainerPort),
					Protocol:      protocol,
				})
			}
		}

		// Bind mounts map to the spec's mounts: rbind propagation, the
		// requested access mode (rw when omitted), and the SELinux
		// relabel (z/Z) and chown (U) flags the request carries.
		for _, mount := range request.Mounts {
			spec.Mounts = append(spec.Mounts, bindMount(mount.Source, mount.Destination, mountOptions(mount)...))
		}

		// Restart policy, remove-on-exit, user, and timezone map
		// straight through.
		if request.RestartPolicy != "" {
			spec.RestartPolicy = request.RestartPolicy
		}
		if request.Remove != nil {
			spec.Remove = request.Remove
		}
		if request.User != "" {
			spec.User = request.User
		}
		if request.Timezone != "" {
			spec.Timezone = request.Timezone
		}

		// Networks map to the spec's network map, one empty-options
		// entry per requested network.
		if len(request.Networks) > 0 {
			spec.Networks = make(map[string]netTypes.PerNetworkOptions, len(request.Networks))
			for _, network := range request.Networks {
				spec.Networks[network] = netTypes.PerNetworkOptions{}
			}
		}

		// The pod field maps straight through: podman accepts a pod
		// name or ID, so the free string reaches the spec's Pod field
		// as-is and podman resolves it at create time.
		if request.Pod != "" {
			spec.Pod = request.Pod
		}

		// Named volumes map to the spec's volumes; the access mode
		// rides along as the mount option (rw when omitted).
		for _, volume := range request.Volumes {
			spec.Volumes = append(spec.Volumes, &specgen.NamedVolume{
				Name:    volume.Name,
				Dest:    volume.Destination,
				Options: []string{defaultMode(volume.Mode)},
			})
		}

		// Secrets map to the spec's secrets; the mount target defaults
		// to /run/secrets/<source> when the request omits it.
		for _, secret := range request.Secrets {
			target := secret.Target
			if target == "" {
				target = "/run/secrets/" + secret.Source
			}
			spec.Secrets = append(spec.Secrets, specgen.Secret{
				Source: secret.Source,
				Target: target,
			})
		}

		// Devices map to the spec's device list by host path; a device
		// whose container path differs also gets a bind mount at the
		// container path.
		for _, device := range request.Devices {
			spec.Devices = append(spec.Devices, runtimespec.LinuxDevice{Path: device.HostPath})
			if device.ContainerPath != device.HostPath {
				spec.Mounts = append(spec.Mounts, bindMount(device.HostPath, device.ContainerPath, "rbind"))
			}
		}

		// Privileged maps through a pointer, mirroring Remove: only a
		// request that asks for it sets the spec's flag, so the default
		// stays an unprivileged container.
		if request.Privileged {
			spec.Privileged = &request.Privileged
		}

		// CapAdd maps verbatim: capability names are podman's
		// vocabulary, so an unknown name is podman's refusal to answer,
		// not a silent drop here.
		if len(request.CapAdd) > 0 {
			spec.CapAdd = request.CapAdd
		}

		// Memory and cpus stay unmapped (out of scope).

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

// bindMount builds one bind mount for the spec's Mounts: the rbind
// propagation option rides along with any further options (the access
// mode for request mounts, nothing extra for device container paths).
func bindMount(source, destination string, options ...string) runtimespec.Mount {
	return runtimespec.Mount{
		Type:        "bind",
		Source:      source,
		Destination: destination,
		Options:     options,
	}
}

// mountOptions assembles one request mount's spec options: rbind
// propagation, the access mode (rw when omitted), then the SELinux
// relabel (z for shared, Z for private) and chown (U) flags the request
// carries, in that order.
func mountOptions(mount MountRequest) []string {
	options := []string{"rbind", defaultMode(mount.Mode)}
	if mount.SelinuxShared {
		options = append(options, "z")
	}
	if mount.SelinuxPrivate {
		options = append(options, "Z")
	}
	if mount.ChownUser {
		options = append(options, "U")
	}
	return options
}

// defaultMode returns the requested mount access mode, defaulting to rw
// when the request omits it.
func defaultMode(mode string) string {
	if mode == "" {
		return "rw"
	}
	return mode
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
