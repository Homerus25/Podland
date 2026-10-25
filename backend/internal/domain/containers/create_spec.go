package containers

import (
	"fmt"
	"maps"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	units "github.com/docker/go-units"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	netTypes "go.podman.io/common/libnetwork/types"
	"go.podman.io/podman/v6/pkg/specgen"
	podmanUtil "go.podman.io/podman/v6/pkg/util"
)

// buildCreateSpec is the pure create builder (the per-domain create
// builder settlement, backend-conventions): request in, populated
// specgen out, plus the memory-parse error — the builder's only error
// branch (podman's units parser; an unparseable value answers the
// documented 400 VALIDATION_FAILED). Every other wire-documented guard
// is decode-level, owned by web.Decode and the request's struct tags.
// The service's WithEngine closure stays a thin lease: the engine call
// plus the not-found classification.
func buildCreateSpec(request ContainerCreateRequest) (*specgen.SpecGenerator, error) {
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

	// Resource limits: cpus maps onto the spec's CPU limit (quota =
	// cpus x 100000, period 100000, podman's own cores conversion);
	// memory parses through podman's units parser into a byte limit.
	// A request without limits leaves the spec's resource limits unset.
	if request.CPUs != 0 || request.Memory != "" {
		limits := &runtimespec.LinuxResources{}
		if request.CPUs != 0 {
			period, quota := podmanUtil.CoresToPeriodAndQuota(request.CPUs)
			limits.CPU = &runtimespec.LinuxCPU{Period: &period, Quota: &quota}
		}
		if request.Memory != "" {
			memoryBytes, err := units.RAMInBytes(request.Memory)
			if err != nil {
				return nil, appErrors.ErrValidationFailed.WithMessage(fmt.Sprintf("memory: %q is not a valid size (e.g. \"512m\", \"1g\")", request.Memory))
			}
			limits.Memory = &runtimespec.LinuxMemory{Limit: &memoryBytes}
		}
		spec.ResourceLimits = limits
	}

	return spec, nil
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
