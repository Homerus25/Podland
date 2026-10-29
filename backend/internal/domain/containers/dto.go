package containers

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	netTypes "go.podman.io/common/libnetwork/types"
	libpoddefine "go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
	define "go.podman.io/podman/v6/pkg/ps/define"
)

// ContainerGetResponse represents detailed information about a specific container
// This is a simplified DTO that only includes basic container information
type ContainerGetResponse struct {
	// Container ID
	ID string `json:"id"`
	// Container name
	Name string `json:"name"`
	// Container state
	State string `json:"state"`
	// Container status
	Status string `json:"status"`
	// Container image
	Image string `json:"image"`
	// Container image ID
	ImageID string `json:"imageId,omitempty"`
	// Time when container was created
	CreatedAt string `json:"createdAt"`
	// Container command
	Command []string `json:"command,omitempty"`
	// Container arguments
	Args []string `json:"args,omitempty"`
	// Container entrypoint
	Entrypoint []string `json:"entrypoint,omitempty"`
	// Secrets mounted in the container
	Secrets []ContainerSecretResponse `json:"secrets,omitempty"`
	// Named volume mounts
	Volumes []ContainerVolumeResponse `json:"volumes,omitempty"`
	// Ports the container exposes
	ExposedPorts []ExposedPortResponse `json:"exposedPorts,omitempty"`
	// Container working directory
	WorkingDir string `json:"workingDir,omitempty"`
	// Container environment variables
	Environment map[string]string `json:"environment,omitempty"`
	// Container labels
	Labels map[string]string `json:"labels,omitempty"`
	// Container ports
	Ports []PortMappingResponse `json:"ports,omitempty"`
	// Container mounts
	Mounts []string `json:"mounts,omitempty"`
	// Container networks
	Networks []string `json:"networks,omitempty"`
	// Container restart count
	Restarts uint `json:"restarts"`
	// If container has exited/stopped
	Exited bool `json:"exited"`
	// Exit code if container has exited
	ExitCode *int32 `json:"exitCode,omitempty"`
	// Container size information
	Size *ContainerSizeResponse `json:"size,omitempty"`
	// Current resource limits (cpus, memory), mapped from the inspect
	// report; nil when the container carries no limits
	Limits *ContainerLimitsResponse `json:"limits,omitempty"`
	// Current healthcheck configuration, mapped from the inspect
	// report; nil when the container carries no healthcheck
	Healthcheck *ContainerHealthcheckResponse `json:"healthcheck,omitempty"`
}

// ContainerLimitsResponse reports the container's current resource
// limits — the prefill for the edit form. CPUs derives from the inspect
// report's CPU quota/period; memory renders back into podman units
// syntax, the same vocabulary the update request accepts.
type ContainerLimitsResponse struct {
	// CPU limit (quota/period from the inspect report; 0 when unset)
	CPUs float64 `json:"cpus"`
	// Memory limit in podman units syntax (e.g. "512m"; empty when
	// unset)
	Memory string `json:"memory"`
}

// ContainerHealthcheckResponse reports the container's current
// healthcheck configuration — the prefill for the edit form. The
// command derives from the inspect report's test array (CMD arguments
// or a CMD-SHELL command joined into one string); durations render as
// Go duration strings, the same vocabulary the update request accepts.
type ContainerHealthcheckResponse struct {
	// Healthcheck command (empty when the healthcheck is disabled)
	Command string `json:"command"`
	// Probe interval as a Go duration string
	Interval string `json:"interval,omitempty"`
	// Probe timeout as a Go duration string
	Timeout string `json:"timeout,omitempty"`
	// Start period as a Go duration string
	StartPeriod string `json:"startPeriod,omitempty"`
	// Retries before the container is marked unhealthy
	Retries uint `json:"retries,omitempty"`
	// Whether the healthcheck is disabled (a NONE test)
	Disabled bool `json:"disabled"`
}

// PortMappingResponse represents a port mapping in the API response
type PortMappingResponse struct {
	ContainerPort int    `json:"containerPort"`
	HostPort      int    `json:"hostPort"`
	Protocol      string `json:"protocol"`
	HostIP        string `json:"hostIp,omitempty"`
}

// ContainerSizeResponse represents container size information
type ContainerSizeResponse struct {
	RootFsSize int64 `json:"rootFsSize"`
	RwSize     int64 `json:"rwSize"`
}

// ContainerSecretResponse represents a secret mounted in the container
type ContainerSecretResponse struct {
	Name string `json:"name"`
	UID  uint32 `json:"uid"`
	GID  uint32 `json:"gid"`
	Mode uint32 `json:"mode"`
}

// ContainerVolumeResponse represents a named volume mount of the container
type ContainerVolumeResponse struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	RW          bool   `json:"rw"`
}

// ExposedPortResponse represents a port the container exposes
type ExposedPortResponse struct {
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

// ContainerListResponse represents a container in the API response
// This DTO is used to serialize container information for the API
type ContainerListResponse struct {
	// Unique identifier for the container
	ID string `json:"id"`
	// Names assigned to the container
	Names []string `json:"names"`
	// Container image
	Image string `json:"image"`
	// Container image ID
	ImageID string `json:"imageId,omitempty"`
	// Container command
	Command []string `json:"command,omitempty"`
	// Time when container was created
	CreatedAt string `json:"createdAt"`
	// Current state of the container
	State string `json:"state"`
	// Human-readable status of the container
	Status string `json:"status"`
	// Number of times the container has been restarted
	Restarts uint `json:"restarts"`
	// If container has exited/stopped
	Exited bool `json:"exited"`
	// Exit code if container has exited
	ExitCode *int32 `json:"exitCode,omitempty"`
	// Container labels
	Labels map[string]string `json:"labels,omitempty"`
	// Port mappings
	Ports []PortMappingResponse `json:"ports,omitempty"`
	// Size information (if requested)
	Size *ContainerSizeResponse `json:"size,omitempty"`
	// Network names assigned to the container
	Networks []string `json:"networks,omitempty"`
	// Mount points
	Mounts []string `json:"mounts,omitempty"`
}

// ContainerCreateRequest represents a request to create a new container
// This DTO is used to deserialize the API request for container creation
type ContainerCreateRequest struct {
	// Name of the container (optional)
	Name string `json:"name,omitempty" validate:"omitempty,max=255"`
	// Image to use for the container (required)
	Image string `json:"image" validate:"required,min=1"`
	// Command to run in the container (optional)
	Command []string `json:"command,omitempty"`
	// Arguments to the command (optional)
	Args []string `json:"args,omitempty"`
	// Working directory inside the container (optional)
	WorkingDir string `json:"workingDir,omitempty"`
	// Environment variables (optional)
	Environment map[string]string `json:"environment,omitempty"`
	// Labels for the container (optional)
	Labels map[string]string `json:"labels,omitempty"`
	// Port bindings (optional)
	PortBindings []PortBindingRequest `json:"portBindings,omitempty" validate:"omitempty,dive"`
	// Bind mounts (optional)
	Mounts []MountRequest `json:"mounts,omitempty" validate:"omitempty,dive"`
	// Named volumes to mount (optional)
	Volumes []VolumeRequest `json:"volumes,omitempty" validate:"omitempty,dive"`
	// Networks to attach the container to (optional)
	Networks []string `json:"networks,omitempty"`
	// Pod to join (optional, name or ID); when set, the container is
	// created inside that pod and the pod owns its networking
	Pod string `json:"pod,omitempty"`
	// Secrets to mount into the container (optional)
	Secrets []SecretRequest `json:"secrets,omitempty" validate:"omitempty,dive"`
	// Host devices to pass through to the container (optional)
	Devices []DeviceRequest `json:"devices,omitempty" validate:"omitempty,dive"`
	// Run container in privileged mode (optional)
	Privileged bool `json:"privileged,omitempty"`
	// Linux capabilities to add to the container (optional); entries pass
	// through to podman verbatim, so an unknown name fails at podman
	CapAdd []string `json:"capAdd,omitempty"`
	// Restart policy (optional)
	RestartPolicy string `json:"restartPolicy,omitempty" validate:"omitempty,oneof=no always unless-stopped on-failure"`
	// Remove the container when it exits (optional)
	Remove *bool `json:"remove,omitempty"`
	// User to run the container as, UID or username (optional)
	User string `json:"user,omitempty"`
	// Timezone inside the container, IANA name (optional)
	Timezone string `json:"timezone,omitempty"`
	// Memory limit (optional)
	Memory string `json:"memory,omitempty"`
	// CPU limit (optional)
	CPUs float64 `json:"cpus,omitempty"`
}

// ContainerRenameRequest is the body of POST
// /api/machines/{id}/containers/{containerId}/rename. The struct tags own
// the wire-documented guards (an empty or over-long name answers 400
// VALIDATION_FAILED through web.Decode); podman's own name regex is not
// duplicated here — the engine is the validator of the name's shape, and
// its rejection maps to 400 INVALID_REQUEST.
type ContainerRenameRequest struct {
	// New name for the container (required)
	Name string `json:"name" validate:"required,min=1,max=255"`
}

// ContainerUpdateRequest is the body of POST
// /api/machines/{id}/containers/{containerId}/update. The contract is
// patch semantics: omitted fields mean "unchanged", and only present
// fields are forwarded to podman's container update. The wire guards
// that can be struct tags live there (the positive-cpus guard, owned by
// web.Decode per the create-builder settlement); the ones that cannot
// (the memory units parse, the Go duration parses, the disable-flag
// exclusivity) live in the pure update builder (update_spec.go),
// mirroring the create builder's memory-parse exception.
type ContainerUpdateRequest struct {
	// CPU limit (optional, positive; omitted or zero means unchanged).
	// The struct tag owns the guard: zero is the absent marker (patch
	// semantics), a negative value fails decode with the documented 400
	// VALIDATION_FAILED.
	CPUs float64 `json:"cpus,omitempty" validate:"omitempty,gt=0"`
	// Memory limit in podman units syntax (optional, e.g. "512m";
	// omitted or empty means unchanged)
	Memory string `json:"memory,omitempty"`
	// Healthcheck changes (optional; absent means the healthcheck is
	// unchanged)
	Healthcheck *ContainerHealthcheckUpdateRequest `json:"healthcheck,omitempty"`
}

// ContainerHealthcheckUpdateRequest carries the healthcheck fields of an
// update request. Every field is a pointer so presence is detectable —
// nil means unchanged, mirroring podman's own changed-healthcheck
// struct. The disable flag is mutually exclusive with every other field
// (enforced in the update builder, because go-playground/validator has
// no cross-field tag).
type ContainerHealthcheckUpdateRequest struct {
	// Healthcheck command in podman CLI syntax — a bare command or a
	// CMD/CMD-SHELL-prefixed one (optional)
	Cmd *string `json:"cmd,omitempty"`
	// Probe interval as a Go duration string (optional, e.g. "30s")
	Interval *string `json:"interval,omitempty"`
	// Probe timeout as a Go duration string (optional, e.g. "5s")
	Timeout *string `json:"timeout,omitempty"`
	// Start period as a Go duration string (optional, e.g. "0m30s")
	StartPeriod *string `json:"startPeriod,omitempty"`
	// Retries before the container is marked unhealthy (optional,
	// non-negative)
	Retries *uint `json:"retries,omitempty"`
	// Disable the container's healthcheck (optional; mutually exclusive
	// with every other field)
	Disabled *bool `json:"disabled,omitempty"`
}

// PortBindingRequest represents a port binding request
type PortBindingRequest struct {
	HostPort      int    `json:"hostPort" validate:"required,min=1,max=65535"`
	ContainerPort int    `json:"containerPort" validate:"required,min=1,max=65535"`
	Protocol      string `json:"protocol,omitempty" validate:"omitempty,oneof=tcp udp"`
}

// MountRequest represents a mount/volume request
type MountRequest struct {
	Source      string `json:"source" validate:"required,min=1"`
	Destination string `json:"destination" validate:"required,min=1"`
	Mode        string `json:"mode,omitempty" validate:"omitempty,oneof=ro rw"`
	// SELinux shared relabel (z): the mounted content is shared between
	// containers (optional). Mutually exclusive with SelinuxPrivate —
	// enforced by an explicit check in the handler next to validate.Struct,
	// because go-playground/validator has no cross-field tag.
	SelinuxShared bool `json:"selinuxShared,omitempty"`
	// SELinux private relabel (Z): only this container may access the
	// mounted content (optional). Mutually exclusive with SelinuxShared.
	SelinuxPrivate bool `json:"selinuxPrivate,omitempty"`
	// Chown the mounted content to the container's user (U) (optional)
	ChownUser bool `json:"chownUser,omitempty"`
}

// VolumeRequest represents a named volume to mount into the container
type VolumeRequest struct {
	// Name of the named volume (required)
	Name string `json:"name" validate:"required,min=1"`
	// Destination path inside the container (required)
	Destination string `json:"destination" validate:"required,min=1"`
	// Mount mode (optional, defaults to rw)
	Mode string `json:"mode,omitempty" validate:"omitempty,oneof=ro rw"`
}

// SecretRequest represents a secret to mount into the container
type SecretRequest struct {
	// Name of the secret on the machine (required)
	Source string `json:"source" validate:"required,min=1"`
	// Path inside the container to mount the secret at (optional,
	// defaults to /run/secrets/<source>)
	Target string `json:"target,omitempty"`
}

// DeviceRequest represents a host device to pass through to the container
type DeviceRequest struct {
	// Path of the device on the host (required)
	HostPath string `json:"hostPath" validate:"required,min=1"`
	// Path of the device inside the container (required)
	ContainerPath string `json:"containerPath" validate:"required,min=1"`
}

// ContainerCreateResponse represents the response after creating a container
type ContainerCreateResponse struct {
	// Container ID
	ID string `json:"id"`
	// Container name
	Name string `json:"name"`
	// Container state
	State string `json:"state"`
	// Container image
	Image string `json:"image"`
	// Container image ID
	ImageID string `json:"imageId,omitempty"`
	// Creation timestamp
	CreatedAt string `json:"createdAt"`
}

// FromContainerCreateReport maps podman's create report and the create
// request to the create response DTO. The report carries only the
// container ID; name and image come from the request, and the state and
// timestamp are stamped at mapping time.
func FromContainerCreateReport(report *entities.ContainerCreateReport, request ContainerCreateRequest) ContainerCreateResponse {
	return ContainerCreateResponse{
		ID:        report.Id,
		Name:      request.Name,
		State:     "created",
		Image:     request.Image,
		ImageID:   "", // ImageID is not available in ContainerCreateReport
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// ContainerListResult wraps the list of containers with metadata
// This provides a consistent response structure for the API
type ContainerListResult struct {
	// List of containers
	Containers []ContainerListResponse `json:"containers"`
	// Total count of containers
	Total int `json:"total"`
	// Request timestamp
	Timestamp string `json:"timestamp"`
}

// FromPodmanInspectContainer converts a podman ContainerInspectReport entity to ContainerGetResponse
// This is a simplified mapping function that only includes basic container information
func FromPodmanInspectContainer(containerReport *entities.ContainerInspectReport) ContainerGetResponse {
	if containerReport == nil {
		return ContainerGetResponse{}
	}

	// Create a simple response with basic container information
	// Use safe accessors that handle nil checks
	response := ContainerGetResponse{
		ID:        getContainerID(containerReport),
		Name:      getContainerName(containerReport),
		State:     getContainerState(containerReport),
		Status:    getContainerStatus(containerReport),
		Image:     getContainerImage(containerReport),
		ImageID:   getContainerImageID(containerReport),
		CreatedAt: formatCreatedTime(containerReport),
		Restarts:  uint(getContainerRestartCount(containerReport)),
		Exited:    isContainerExited(containerReport),
	}

	// Add optional fields if available
	if containerReport.Config != nil {
		response.Command = containerReport.Config.Cmd
		// Args might not be available in the Config, so we'll skip it
		response.Entrypoint = containerReport.Config.Entrypoint
		response.Secrets = mapContainerSecrets(containerReport.Config.Secrets)
		response.ExposedPorts = mapExposedPorts(containerReport.Config.ExposedPorts)
		response.WorkingDir = containerReport.Config.WorkingDir
		response.Labels = containerReport.Config.Labels

		// Parse environment variables
		if len(containerReport.Config.Env) > 0 {
			response.Environment = make(map[string]string)
			for _, envVar := range containerReport.Config.Env {
				// Simple parsing of KEY=VALUE format
				if parts := strings.SplitN(envVar, "=", 2); len(parts) == 2 {
					response.Environment[parts[0]] = parts[1]
				}
			}
		}
	}

	// Named volume mounts, filtered from the report's mounts
	response.Volumes = mapContainerVolumes(containerReport.Mounts)

	// The detail page's mounts row lists every mount the inspect report
	// carries — named volumes and binds alike — as --mount-style strings
	// matching the frontend fixture format.
	response.Mounts = mapContainerMounts(containerReport.Mounts)

	// The current resource limits and healthcheck — the edit form's
	// prefill, owned by the backend mapping (the frontend never parses
	// the raw inspect report for form defaults).
	response.Limits = mapContainerLimits(containerReport.HostConfig)
	response.Healthcheck = mapContainerHealthcheck(containerReport.Config)

	// The attached networks, sorted by name for a stable response
	response.Networks = mapContainerNetworks(containerReport.NetworkSettings)

	// Skip size information for now since the field types are unclear

	// Add exit code if container has exited
	if containerReport.State != nil && containerReport.State.ExitCode != 0 {
		exitCode := containerReport.State.ExitCode
		response.ExitCode = &exitCode
	}

	return response
}

// FromPodmanListContainer converts a podman ListContainer entity to
// ContainerListResponse. Typed at both ends, mirroring the volumes and
// images list mappers (backend-conventions, candidate C22).
func FromPodmanListContainer(listContainer entities.ListContainer) ContainerListResponse {
	return ContainerListResponse{
		ID:        listContainer.ID,
		Names:     listContainer.Names,
		Image:     listContainer.Image,
		ImageID:   listContainer.ImageID,
		Command:   listContainer.Command,
		CreatedAt: listContainer.CreatedAt,
		State:     listContainer.State,
		Status:    listContainer.Status,
		Restarts:  listContainer.Restarts,
		Exited:    listContainer.Exited,
		ExitCode: func() *int32 {
			if listContainer.ExitCode != 0 {
				v := listContainer.ExitCode
				return &v
			}
			return nil
		}(),
		Labels:   listContainer.Labels,
		Ports:    mapPortMappings(listContainer.Ports),
		Size:     mapContainerSize(listContainer.Size),
		Networks: listContainer.Networks,
		Mounts:   listContainer.Mounts,
	}
}

// ToContainerListResult converts a list of podman containers to ContainerListResult
// This function handles the complete transformation from service layer to API response
func ToContainerListResult(containers []entities.ListContainer) ContainerListResult {
	result := ContainerListResult{
		Containers: make([]ContainerListResponse, len(containers)),
		Total:      len(containers),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	for i, container := range containers {
		result.Containers[i] = FromPodmanListContainer(container)
	}

	return result
}

// Helper function to map port mappings
func mapPortMappings(ports []netTypes.PortMapping) []PortMappingResponse {
	if len(ports) == 0 {
		return nil
	}
	result := make([]PortMappingResponse, len(ports))
	for i, port := range ports {
		result[i] = PortMappingResponse{
			ContainerPort: int(port.ContainerPort),
			HostPort:      int(port.HostPort),
			Protocol:      port.Protocol,
			HostIP:        port.HostIP,
		}
	}
	return result
}

// Helper function to map container size
func mapContainerSize(size *define.ContainerSize) *ContainerSizeResponse {
	if size == nil {
		return nil
	}
	return &ContainerSizeResponse{
		RootFsSize: size.RootFsSize,
		RwSize:     size.RwSize,
	}
}

// Helper function to map the secrets mounted in the container
func mapContainerSecrets(secrets []*libpoddefine.InspectSecret) []ContainerSecretResponse {
	if len(secrets) == 0 {
		return nil
	}
	result := make([]ContainerSecretResponse, 0, len(secrets))
	for _, secret := range secrets {
		if secret == nil {
			continue
		}
		result = append(result, ContainerSecretResponse{
			Name: secret.Name,
			UID:  secret.UID,
			GID:  secret.GID,
			Mode: secret.Mode,
		})
	}
	return result
}

// Helper function to map the report's mounts down to named volume mounts
func mapContainerVolumes(mounts []libpoddefine.InspectMount) []ContainerVolumeResponse {
	var result []ContainerVolumeResponse
	for _, mount := range mounts {
		if mount.Type != "volume" {
			continue
		}
		result = append(result, ContainerVolumeResponse{
			Name:        mount.Name,
			Source:      mount.Source,
			Destination: mount.Destination,
			RW:          mount.RW,
		})
	}
	return result
}

// Helper function to map the report's mounts to --mount-style strings
// ("type=…,source=…,destination=…"), the format the frontend's detail page
// renders. Every mount is included — named volumes and binds alike — in the
// report's order.
func mapContainerMounts(mounts []libpoddefine.InspectMount) []string {
	if len(mounts) == 0 {
		return nil
	}
	result := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		result = append(result, fmt.Sprintf("type=%s,source=%s,destination=%s", mount.Type, mount.Source, mount.Destination))
	}
	return result
}

// mapContainerLimits maps the inspect report's host config down to the
// current resource limits: cpus derives from the CPU quota/period when
// both are set, and memory renders back into podman units syntax
// (renderMemoryLimit, the same vocabulary RAMInBytes parses — the
// create page's and the podman CLI's "512m", not go-units' "512MiB").
// A host config without limits maps nil, so the JSON response omits the
// field.
func mapContainerLimits(hostConfig *libpoddefine.InspectContainerHostConfig) *ContainerLimitsResponse {
	if hostConfig == nil {
		return nil
	}
	limits := &ContainerLimitsResponse{}
	if hostConfig.CpuPeriod > 0 && hostConfig.CpuQuota > 0 {
		limits.CPUs = float64(hostConfig.CpuQuota) / float64(hostConfig.CpuPeriod)
	}
	if hostConfig.Memory > 0 {
		limits.Memory = renderMemoryLimit(hostConfig.Memory)
	}
	if limits.CPUs == 0 && limits.Memory == "" {
		return nil
	}
	return limits
}

// renderMemoryLimit renders a byte limit back into podman units syntax —
// the inverse of go-units' RAMInBytes, which parses "512m" as 512 MiB:
// the exact binary multiples render with their one-letter suffix (b, k,
// m, g, t; k = 1024), the largest exact multiple wins, and a limit that
// is no clean multiple falls back to plain bytes with the "b" suffix.
// The create page's memory field and the podman CLI accept exactly this
// vocabulary, so a prefilled value survives an in-place edit.
func renderMemoryLimit(limit int64) string {
	steps := []struct {
		size   int64
		suffix string
	}{
		{1 << 40, "t"},
		{1 << 30, "g"},
		{1 << 20, "m"},
		{1 << 10, "k"},
	}
	for _, step := range steps {
		if limit >= step.size && limit%step.size == 0 {
			return strconv.FormatInt(limit/step.size, 10) + step.suffix
		}
	}
	return strconv.FormatInt(limit, 10) + "b"
}

// mapContainerHealthcheck maps the inspect report's healthcheck config
// down to the current healthcheck: the command derives from the test
// array (CMD arguments or a CMD-SHELL command joined into one string; a
// NONE test marks the healthcheck disabled), and the durations render
// as Go duration strings — the same vocabulary the update request
// accepts. A zero duration means "inherit podman's default", so it maps
// to an empty string: prefilled into a form and resubmitted, a "0s"
// would disable the probe timer instead of leaving it unchanged. A
// config without a healthcheck maps nil, so the JSON response omits
// the field.
func mapContainerHealthcheck(config *libpoddefine.InspectContainerConfig) *ContainerHealthcheckResponse {
	if config == nil || config.Healthcheck == nil {
		return nil
	}
	healthcheck := &ContainerHealthcheckResponse{
		Retries: uint(config.Healthcheck.Retries),
	}
	if config.Healthcheck.Interval > 0 {
		healthcheck.Interval = config.Healthcheck.Interval.String()
	}
	if config.Healthcheck.Timeout > 0 {
		healthcheck.Timeout = config.Healthcheck.Timeout.String()
	}
	if config.Healthcheck.StartPeriod > 0 {
		healthcheck.StartPeriod = config.Healthcheck.StartPeriod.String()
	}
	switch {
	case len(config.Healthcheck.Test) == 0:
	case config.Healthcheck.Test[0] == "NONE":
		healthcheck.Disabled = true
	case config.Healthcheck.Test[0] == "CMD":
		healthcheck.Command = strings.Join(config.Healthcheck.Test[1:], " ")
	case config.Healthcheck.Test[0] == "CMD-SHELL":
		healthcheck.Command = strings.Join(config.Healthcheck.Test[1:], " ")
	default:
		healthcheck.Command = strings.Join(config.Healthcheck.Test, " ")
	}
	return healthcheck
}

// Helper function to map the report's attached networks to their names,
// sorted for a stable response. Nil-safe: a report without network
// settings maps to nil.
func mapContainerNetworks(networkSettings *libpoddefine.InspectNetworkSettings) []string {
	if networkSettings == nil || len(networkSettings.Networks) == 0 {
		return nil
	}
	result := make([]string, 0, len(networkSettings.Networks))
	for name := range networkSettings.Networks {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// Helper function to map the config's exposed-port set ("8080/tcp" keys)
// to a slice sorted by container port, so the response is stable
func mapExposedPorts(exposedPorts map[string]struct{}) []ExposedPortResponse {
	if len(exposedPorts) == 0 {
		return nil
	}
	result := make([]ExposedPortResponse, 0, len(exposedPorts))
	for key := range exposedPorts {
		ports, protocol, ok := parseExposedPortKey(key)
		if !ok {
			continue
		}
		for _, port := range ports {
			result = append(result, ExposedPortResponse{
				ContainerPort: port,
				Protocol:      protocol,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ContainerPort != result[j].ContainerPort {
			return result[i].ContainerPort < result[j].ContainerPort
		}
		return result[i].Protocol < result[j].Protocol
	})
	return result
}

// parseExposedPortKey splits an exposed-port key ("8080/tcp" or
// "2000-2002/udp") into its ports and protocol; a key without a
// protocol defaults to tcp. A range key expands to each port in the
// range, so no exposed port is silently dropped.
func parseExposedPortKey(key string) ([]int, string, bool) {
	portPart, protocol, _ := strings.Cut(key, "/")
	if protocol == "" {
		protocol = "tcp"
	}
	// A range key ("2000-2002") expands to each port in the range.
	if start, end, ok := strings.Cut(portPart, "-"); ok {
		startPort, err := strconv.Atoi(start)
		if err != nil {
			return nil, "", false
		}
		endPort, err := strconv.Atoi(end)
		if err != nil {
			return nil, "", false
		}
		if startPort > endPort || endPort-startPort > 1000 {
			return nil, "", false
		}
		ports := make([]int, 0, endPort-startPort+1)
		for p := startPort; p <= endPort; p++ {
			ports = append(ports, p)
		}
		return ports, protocol, true
	}
	port, err := strconv.Atoi(portPart)
	if err != nil {
		return nil, "", false
	}
	return []int{port}, protocol, true
}

// Helper function to get container ID from report
func getContainerID(containerReport *entities.ContainerInspectReport) string {
	if containerReport != nil {
		// Access ID safely - it might be from the embedded struct
		return containerReport.ID
	}
	return ""
}

// Helper function to get container name from report
func getContainerName(containerReport *entities.ContainerInspectReport) string {
	if containerReport != nil && len(containerReport.Name) > 0 {
		// Remove leading "/" if present
		return strings.TrimPrefix(containerReport.Name, "/")
	}
	if containerReport != nil {
		return containerReport.ID
	}
	return ""
}

// Helper function to get container restart count from report
func getContainerRestartCount(containerReport *entities.ContainerInspectReport) int32 {
	if containerReport != nil {
		return containerReport.RestartCount
	}
	return 0
}

// Helper function to get container image from report
func getContainerImage(containerReport *entities.ContainerInspectReport) string {
	if containerReport != nil && containerReport.Config != nil {
		return containerReport.Config.Image
	}
	return ""
}

// Helper function to get the container's image ID from the report: the
// inspect report's top-level Image field carries the image ID (sha256:…),
// distinct from Config.Image, which carries the image name the container
// was created from. The detail page's image cross-link targets the ID.
func getContainerImageID(containerReport *entities.ContainerInspectReport) string {
	if containerReport != nil {
		return containerReport.Image
	}
	return ""
}

// Helper function to get container state from report
func getContainerState(containerReport *entities.ContainerInspectReport) string {
	if containerReport != nil && containerReport.State != nil {
		if containerReport.State.Running {
			return "running"
		} else if containerReport.State.ExitCode != 0 {
			return "exited"
		} else if containerReport.State.Paused {
			return "paused"
		} else if containerReport.State.Restarting {
			return "restarting"
		} else if containerReport.State.Dead {
			return "dead"
		}
	}
	return "created"
}

// Helper function to get container status from report
func getContainerStatus(containerReport *entities.ContainerInspectReport) string {
	if containerReport != nil && containerReport.State != nil {
		if containerReport.State.Running {
			return "Up"
		} else if containerReport.State.ExitCode != 0 {
			return "Exited"
		}
	}
	return "Created"
}

// Helper function to format created time
func formatCreatedTime(containerReport *entities.ContainerInspectReport) string {
	if containerReport != nil && !containerReport.Created.IsZero() {
		return containerReport.Created.Format(time.RFC3339)
	}
	return ""
}

// ContainerStartResponse represents the response after starting a container
type ContainerStartResponse struct {
	// Container ID
	ID string `json:"id"`
	// Container name
	Name string `json:"name"`
	// Container state after start
	State string `json:"state"`
	// Operation timestamp
	Timestamp string `json:"timestamp"`
}

// ContainerStopResponse represents the response after stopping a container
type ContainerStopResponse struct {
	// Container ID
	ID string `json:"id"`
	// Container name
	Name string `json:"name"`
	// Container state after stop
	State string `json:"state"`
	// Operation timestamp
	Timestamp string `json:"timestamp"`
}

// ContainerRestartResponse represents the response after restarting a
// container. It mirrors the start/stop response shape — the containers
// domain's own precedent, not the pods action shape.
type ContainerRestartResponse struct {
	// Container ID
	ID string `json:"id"`
	// Container name
	Name string `json:"name"`
	// Container state after restart
	State string `json:"state"`
	// Operation timestamp
	Timestamp string `json:"timestamp"`
}

// ContainerStartResult is the service-layer outcome of a start: the
// container's identity only. The response DTO's state and timestamp are
// stamped by the handler's mapping, not by the service.
type ContainerStartResult struct {
	// Container ID
	ID string
	// Container name (inspect-resolved, leading slash trimmed)
	Name string
}

// ContainerStopResult is the service-layer outcome of a stop: the
// container's identity only. The response DTO's state and timestamp are
// stamped by the handler's mapping, not by the service.
type ContainerStopResult struct {
	// Container ID
	ID string
	// Container name (inspect-resolved, leading slash trimmed)
	Name string
}

// ContainerRestartResult is the service-layer outcome of a restart: the
// container's identity only, like the stop result. The response DTO's
// state and timestamp are stamped by the handler's mapping, not by the
// service.
type ContainerRestartResult struct {
	// Container ID
	ID string
	// Container name (inspect-resolved, leading slash trimmed)
	Name string
}

// FromContainerStartResult maps the service's start result to the
// response DTO, stamping the post-start state and the operation
// timestamp at mapping time.
func FromContainerStartResult(result ContainerStartResult) ContainerStartResponse {
	return ContainerStartResponse{
		ID:        result.ID,
		Name:      result.Name,
		State:     "running",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// FromContainerStopResult maps the service's stop result to the
// response DTO, stamping the post-stop state and the operation
// timestamp at mapping time.
func FromContainerStopResult(result ContainerStopResult) ContainerStopResponse {
	return ContainerStopResponse{
		ID:        result.ID,
		Name:      result.Name,
		State:     "stopped",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// FromContainerRestartResult maps the service's restart result to the
// response DTO, stamping the post-restart state and the operation
// timestamp at mapping time.
func FromContainerRestartResult(result ContainerRestartResult) ContainerRestartResponse {
	return ContainerRestartResponse{
		ID:        result.ID,
		Name:      result.Name,
		State:     "running",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// ContainerLogsResponse represents the response for container logs
type ContainerLogsResponse struct {
	// Container ID
	ID string `json:"id"`
	// Container logs content (multiline string)
	Logs string `json:"logs"`
}

// ContainerPruneResponse is one entry in the prune API response: the
// removed container's ID and the reclaimed size (bytes), plus any
// per-container error podman reported while removing it.
type ContainerPruneResponse struct {
	// ID of the removed container
	ID string `json:"id"`
	// Reclaimed size in bytes, when podman reports it
	Size uint64 `json:"size"`
	// Per-container error podman reported, if the removal failed
	Error string `json:"error,omitempty"`
}

// FromPodmanContainerPruneReports converts podman's prune reports to the
// API array. An empty prune maps to an empty array, never nil.
func FromPodmanContainerPruneReports(reports []*reports.PruneReport) []ContainerPruneResponse {
	out := make([]ContainerPruneResponse, 0, len(reports))
	for _, report := range reports {
		if report == nil {
			continue
		}
		entry := ContainerPruneResponse{
			ID:   report.Id,
			Size: report.Size,
		}
		if report.Err != nil {
			entry.Error = report.Err.Error()
		}
		out = append(out, entry)
	}
	return out
}

// ContainerStatsResponse is one container's row in a stats snapshot: raw
// numeric values, no formatting server-side. The network byte counters
// are summed over the container's interfaces at mapping time.
type ContainerStatsResponse struct {
	// Container ID
	ID string `json:"id"`
	// Container name
	Name string `json:"name"`
	// CPU usage in percent
	CPUPercent float64 `json:"cpuPercent"`
	// Memory usage in bytes
	MemoryUsage uint64 `json:"memoryUsage"`
	// Memory limit in bytes
	MemoryLimit uint64 `json:"memoryLimit"`
	// Memory usage in percent of the limit
	MemoryPercent float64 `json:"memoryPercent"`
	// Network bytes received, summed over interfaces
	NetworkBytesIn uint64 `json:"networkBytesIn"`
	// Network bytes sent, summed over interfaces
	NetworkBytesOut uint64 `json:"networkBytesOut"`
	// Block device bytes read
	BlockBytesIn uint64 `json:"blockBytesIn"`
	// Block device bytes written
	BlockBytesOut uint64 `json:"blockBytesOut"`
	// Number of processes in the container
	PIDs uint64 `json:"pids"`
}

// ContainerStatsResult wraps the stats snapshot with metadata, the same
// envelope shape as the container list.
type ContainerStatsResult struct {
	// One stats row per container in the snapshot
	Containers []ContainerStatsResponse `json:"containers"`
	// Count of stats rows
	Total int `json:"total"`
	// Snapshot timestamp
	Timestamp string `json:"timestamp"`
}

// FromPodmanContainerStats converts a podman ContainerStats entity to the
// stats DTO. Typed at both ends, mirroring the list mappers
// (backend-conventions, candidate C22); the network byte counters are
// summed over the entity's interfaces.
func FromPodmanContainerStats(stats libpoddefine.ContainerStats) ContainerStatsResponse {
	var networkBytesIn, networkBytesOut uint64
	for _, iface := range stats.Network {
		networkBytesIn += iface.RxBytes
		networkBytesOut += iface.TxBytes
	}
	return ContainerStatsResponse{
		ID:              stats.ContainerID,
		Name:            stats.Name,
		CPUPercent:      stats.CPU,
		MemoryUsage:     stats.MemUsage,
		MemoryLimit:     stats.MemLimit,
		MemoryPercent:   stats.MemPerc,
		NetworkBytesIn:  networkBytesIn,
		NetworkBytesOut: networkBytesOut,
		BlockBytesIn:    stats.BlockInput,
		BlockBytesOut:   stats.BlockOutput,
		PIDs:            stats.PIDs,
	}
}

// ToContainerStatsResult converts a stats snapshot to the list envelope.
// An empty snapshot maps to an empty array, never null.
func ToContainerStatsResult(stats []libpoddefine.ContainerStats) ContainerStatsResult {
	result := ContainerStatsResult{
		Containers: make([]ContainerStatsResponse, len(stats)),
		Total:      len(stats),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}
	for i, container := range stats {
		result.Containers[i] = FromPodmanContainerStats(container)
	}
	return result
}

// Helper function to check if container is exited
func isContainerExited(containerReport *entities.ContainerInspectReport) bool {
	if containerReport != nil && containerReport.State != nil {
		return containerReport.State.ExitCode != 0
	}
	return false
}
