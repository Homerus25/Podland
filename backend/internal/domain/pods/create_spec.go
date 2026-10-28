package pods

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
// builder settlement, backend-conventions): request in, populated pod
// specgen out, plus the memory-parse error — the builder's only error
// branch (podman's units parser; an unparseable value answers the
// documented 400 VALIDATION_FAILED). Every other wire-documented guard
// is decode-level, owned by web.Decode and the request's struct tags.
// The service's WithEngine closure stays a thin lease: the engine call
// plus the podman-side 4xx classification.
func buildCreateSpec(request PodCreateRequest) (*specgen.PodSpecGenerator, error) {
	spec := specgen.NewPodSpecGenerator()
	spec.Name = request.Name

	if len(request.Labels) > 0 {
		spec.Labels = make(map[string]string, len(request.Labels))
		maps.Copy(spec.Labels, request.Labels)
	}

	// Networks map to the spec's network map, one empty-options
	// entry per requested network, mirroring containers.
	if len(request.Networks) > 0 {
		spec.Networks = make(map[string]netTypes.PerNetworkOptions, len(request.Networks))
		for _, network := range request.Networks {
			spec.Networks[network] = netTypes.PerNetworkOptions{}
		}
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

	// Bind mounts map to the spec's mounts: rbind propagation, the
	// requested access mode (rw when omitted), then the SELinux
	// relabel (z for shared, Z for private) and chown (U) flags the
	// request carries, mirroring the containers create mapping.
	for _, mount := range request.Mounts {
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
		spec.Mounts = append(spec.Mounts, runtimespec.Mount{
			Type:        "bind",
			Source:      mount.Source,
			Destination: mount.Destination,
			Options:     options,
		})
	}

	// Port mappings publish on the pod's infra container; the protocol
	// defaults to tcp when the request omits it, mirroring containers.
	if len(request.PortMappings) > 0 {
		spec.PortMappings = make([]netTypes.PortMapping, 0, len(request.PortMappings))
		for _, mapping := range request.PortMappings {
			protocol := mapping.Protocol
			if protocol == "" {
				protocol = "tcp"
			}
			spec.PortMappings = append(spec.PortMappings, netTypes.PortMapping{
				HostPort:      uint16(mapping.HostPort),
				ContainerPort: uint16(mapping.ContainerPort),
				Protocol:      protocol,
			})
		}
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

// defaultMode returns the requested mount access mode, defaulting to rw
// when the request omits it.
func defaultMode(mode string) string {
	if mode == "" {
		return "rw"
	}
	return mode
}
