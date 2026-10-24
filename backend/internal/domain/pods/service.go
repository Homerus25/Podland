package pods

import (
	"context"
	"fmt"
	"maps"
	"strings"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/inspect"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"github.com/Homerus25/podland/backend/internal/removal"
	units "github.com/docker/go-units"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	netTypes "go.podman.io/common/libnetwork/types"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/specgen"
	podmanUtil "go.podman.io/podman/v6/pkg/util"
)

// PodService provides business logic for pods. Pods live only in the
// remote machine's podman; this service proxies podman directly and
// persists nothing locally, matching VolumeService/NetworkService. Every
// machine-scoped podman call crosses the PodEngineProvider (the
// connection seam); the service itself owns only domain mapping.
type PodService struct {
	engines PodEngineProvider
}

// NewPodService creates a new PodService
func NewPodService(engines PodEngineProvider) *PodService {
	return &PodService{
		engines: engines,
	}
}

// List returns every pod on the specified machine — everything podman
// reports; no filtering.
func (s *PodService) List(ctx context.Context, userID, machineID string, opts entities.PodPSOptions) ([]*entities.ListPodsReport, error) {
	var pods []*entities.ListPodsReport
	err := s.engines.WithEngine(ctx, userID, machineID, "pods.List", func(e PodEngine) error {
		p, err := e.List(opts)
		if err != nil {
			return fmt.Errorf("failed to list pods: %w", err)
		}
		pods = p
		return nil
	})
	return pods, err
}

// Create creates a new pod on the specified machine. The request's
// networks map onto the pod spec's network map with empty per-network
// options (mirroring containers), volumes and mounts map onto the pod
// spec's storage config, and the infra container stays at podman's
// default. cpus maps onto the spec's CPU limit (quota = cpus x 100000,
// period 100000); memory parses through podman's units parser into a
// byte limit — an unparseable value is a 400 VALIDATION_FAILED. The
// service returns podman's create report; the handler maps the response
// DTO.
func (s *PodService) Create(ctx context.Context, userID, machineID string, request PodCreateRequest) (*entities.PodCreateReport, error) {
	var report *entities.PodCreateReport
	err := s.engines.WithEngine(ctx, userID, machineID, "pods.Create", func(e PodEngine) error {
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

		// Bind mounts map to the spec's mounts: rbind propagation and
		// the requested access mode (rw when omitted), mirroring the
		// containers create mapping.
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

		// Resource limits: cpus maps onto the spec's CPU limit (quota =
		// cpus x 100000, period 100000, podman's own cores conversion);
		// memory parses through podman's units parser into a byte limit.
		// An unparseable value is the documented 400 VALIDATION_FAILED.
		if request.CPUs != 0 || request.Memory != "" {
			limits := &runtimespec.LinuxResources{}
			if request.CPUs != 0 {
				period, quota := podmanUtil.CoresToPeriodAndQuota(request.CPUs)
				limits.CPU = &runtimespec.LinuxCPU{Period: &period, Quota: &quota}
			}
			if request.Memory != "" {
				memoryBytes, err := units.RAMInBytes(request.Memory)
				if err != nil {
					return appErrors.ErrValidationFailed.WithMessage(fmt.Sprintf("memory: %q is not a valid size (e.g. \"512m\", \"1g\")", request.Memory))
				}
				limits.Memory = &runtimespec.LinuxMemory{Limit: &memoryBytes}
			}
			spec.ResourceLimits = limits
		}

		// Create the pod. A podman-side 4xx rejection (duplicate name,
		// invalid name format) means the request itself was bad: it
		// surfaces as 400 INVALID_REQUEST carrying podman's message,
		// mirroring the volumes create mapping.
		created, err := e.Create(entities.PodSpec{PodSpecGen: *spec})
		if err != nil {
			if podmanerr.IsClientError(err) || podmanerr.IsConflict(err) || podmanerr.IsNotFound(err) {
				return appErrors.ErrInvalidRequest.WithMessage(podmanerr.Message(err))
			}
			return fmt.Errorf("failed to create pod: %w", err)
		}

		report = created
		return nil
	})
	return report, err
}

// defaultMode returns the requested mount access mode, defaulting to rw
// when the request omits it.
func defaultMode(mode string) string {
	if mode == "" {
		return "rw"
	}
	return mode
}

// Get returns detailed information about a specific pod. The raw inspect
// report crosses to the handler, which maps the detail DTO.
func (s *PodService) Get(ctx context.Context, userID, machineID, podID string) (*entities.PodInspectReport, error) {
	var report *entities.PodInspectReport
	err := s.engines.WithEngine(ctx, userID, machineID, "pods.Get", func(e PodEngine) error {
		inspectReport, err := inspectPod(e, podID)
		if err != nil {
			return err
		}

		report = inspectReport
		return nil
	})
	return report, err
}

// Inspect returns the pod's raw podman inspect report. Unlike Get, the
// report crosses the wire unmapped: this endpoint is the raw inspect JSON
// window, so no DTO mapping happens here or in the handler.
func (s *PodService) Inspect(ctx context.Context, userID, machineID, podID string) (*entities.PodInspectReport, error) {
	var report *entities.PodInspectReport
	err := s.engines.WithEngine(ctx, userID, machineID, "pods.Inspect", func(e PodEngine) error {
		inspectReport, err := inspectPod(e, podID)
		if err != nil {
			return err
		}

		report = inspectReport
		return nil
	})
	return report, err
}

// inspectPod resolves one pod's inspect report through the shared
// single-report ritual, classifying podman's not-found shapes into
// ErrPodNotFound.
func inspectPod(e PodEngine, podID string) (*entities.PodInspectReport, error) {
	reports, inspectErrs, err := e.Inspect(podID)
	return inspect.Resolve(reports, inspectErrs, err, appErrors.ErrPodNotFound, "pod", podID)
}

// Pause pauses every running container in the pod.
func (s *PodService) Pause(ctx context.Context, userID, machineID, podID string) error {
	return s.action(ctx, userID, machineID, "pods.Pause", "pause", podID, PodEngine.Pause)
}

// Unpause unpauses every paused container in the pod.
func (s *PodService) Unpause(ctx context.Context, userID, machineID, podID string) error {
	return s.action(ctx, userID, machineID, "pods.Unpause", "unpause", podID, PodEngine.Unpause)
}

// Kill sends podman's default signal to the pod's running containers.
func (s *PodService) Kill(ctx context.Context, userID, machineID, podID string) error {
	return s.action(ctx, userID, machineID, "pods.Kill", "kill", podID, PodEngine.Kill)
}

// Restart restarts the pod's containers.
func (s *PodService) Restart(ctx context.Context, userID, machineID, podID string) error {
	return s.action(ctx, userID, machineID, "pods.Restart", "restart", podID, PodEngine.Restart)
}

// action runs one lifecycle action against the pod: podman's answer is
// classified into the domain's error vocabulary (mapActionError), and an
// unclassified failure is wrapped.
func (s *PodService) action(ctx context.Context, userID, machineID, op, verb, podID string, act func(PodEngine, string) error) error {
	return s.engines.WithEngine(ctx, userID, machineID, op, func(e PodEngine) error {
		if err := act(e, podID); err != nil {
			if appErr := mapActionError(err); appErr != nil {
				return appErr
			}
			return fmt.Errorf("failed to %s pod: %w", verb, err)
		}
		return nil
	})
}

// Remove removes the pod; force also removes its running containers.
// The remove-and-classify ritual owns the outcome: a missing pod is the
// 404 terminal, a refusal (podman's 409 conflict or the running-refusal
// text) is the 409 POD_STATE_CONFLICT carrying podman's message.
func (s *PodService) Remove(ctx context.Context, userID, machineID, podID string, force bool) error {
	return s.engines.WithEngine(ctx, userID, machineID, "pods.Remove", func(e PodEngine) error {
		return removal.Run(func() error {
			return e.Remove(podID, force)
		}, removal.Terminals{
			Missing: appErrors.ErrPodNotFound,
			Refusal: func(cause string) error {
				return appErrors.ErrPodStateConflict.WithMessage(cause)
			},
			IsRefusal: isPodRefusal,
		}, "failed to remove pod")
	})
}

// Prune removes every stopped pod (podman's own selection) and answers
// podman's per-pod reports; the handler maps the prune DTO.
func (s *PodService) Prune(ctx context.Context, userID, machineID string) ([]*entities.PodPruneReport, error) {
	var prunes []*entities.PodPruneReport
	err := s.engines.WithEngine(ctx, userID, machineID, "pods.Prune", func(e PodEngine) error {
		p, err := e.Prune()
		if err != nil {
			return fmt.Errorf("failed to prune pods: %w", err)
		}
		prunes = p
		return nil
	})
	return prunes, err
}

// mapActionError classifies podman's lifecycle answers into the pods
// domain's error vocabulary: a missing pod is the 404 terminal, a
// refusal (podman's 409 conflict or the remove-style running-refusal
// text) is the 409 POD_STATE_CONFLICT carrying podman's message.
// Anything else is not classified — the caller wraps it.
func mapActionError(err error) *appErrors.AppError {
	if podmanerr.IsNotFound(err) {
		return appErrors.ErrPodNotFound
	}
	if isPodRefusal(err) {
		return appErrors.ErrPodStateConflict.WithMessage(podmanerr.Message(err))
	}
	return nil
}

// isPodRefusal reports whether err is podman refusing an action for the
// pod's current state: either a typed 409 or the compat layer's
// untyped running-refusal text.
func isPodRefusal(err error) bool {
	return podmanerr.IsConflict(err) || isStateRefusal(err)
}

// isStateRefusal reports whether err is podman's running-refusal text,
// which the compat layer reports as an untyped 500 rather than a 409.
func isStateRefusal(err error) bool {
	return strings.Contains(strings.ToLower(podmanerr.Message(err)), "cannot be removed without force")
}
