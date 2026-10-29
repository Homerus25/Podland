package containers

import (
	"fmt"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	units "github.com/docker/go-units"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	libpoddefine "go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
	podmanUtil "go.podman.io/podman/v6/pkg/util"
)

// buildUpdateOptions is the pure update builder — the update twin of the
// create builder settlement (backend-conventions): request in, populated
// podman update options out, plus the request-local validation branches.
// Patch semantics are the contract: only the fields the request carries
// are forwarded, absent fields stay nil, and podman merges resource
// fields — so a request without limits leaves Resources nil and a
// request without healthcheck fields leaves the changed-healthcheck
// configuration nil. The wire guards that can be struct tags live on the
// request DTO (the positive-cpus guard, owned by web.Decode); the
// validation branches that cannot be struct tags live here, mirroring
// the create builder's memory-parse exception: the memory parse
// (podman's units parser), the Go duration parses (podman's own
// healthcheck parser is time.ParseDuration), and the disable-flag
// exclusivity (podman refuses the combination, so the backend answers
// the documented 400 VALIDATION_FAILED without a round trip). The
// service builds the options before the lease is acquired, so a
// request-local validation failure never costs an engine lease.
func buildUpdateOptions(request ContainerUpdateRequest) (*entities.ContainerUpdateOptions, error) {
	opts := &entities.ContainerUpdateOptions{}

	// Resource limits: cpus maps onto the update's CPU limit through
	// podman's own cores conversion — identical to the create builder —
	// and memory parses through podman's units parser into a byte
	// limit. Each limit lands on its own sub-struct so podman merges
	// only what the request carried: a cpus-only update leaves the
	// memory limit alone, and vice versa. A non-positive cpus never
	// reaches the builder — the request's struct tag answers the
	// documented 400 at decode time.
	if request.CPUs != 0 {
		period, quota := podmanUtil.CoresToPeriodAndQuota(request.CPUs)
		ensureUpdateResources(opts).CPU = &runtimespec.LinuxCPU{Period: &period, Quota: &quota}
	}
	if request.Memory != "" {
		memoryBytes, err := units.RAMInBytes(request.Memory)
		if err != nil {
			return nil, appErrors.ErrValidationFailed.WithMessage(
				fmt.Sprintf("memory: %q is not a valid size (e.g. \"512m\", \"1g\")", request.Memory))
		}
		ensureUpdateResources(opts).Memory = &runtimespec.LinuxMemory{Limit: &memoryBytes}
	}

	// Healthcheck: every present field forwards onto podman's
	// changed-healthcheck configuration — a struct of optional pointers
	// whose nil means unchanged, which is exactly the patch contract.
	// Durations forward as the Go duration strings podman's own parser
	// (time.ParseDuration) expects, so the backend's parse doubles as
	// the wire guard.
	if request.Healthcheck != nil {
		healthcheck := request.Healthcheck

		// The disable flag is mutually exclusive with every other
		// healthcheck field: podman refuses the combination, so the
		// backend answers the documented 400 without a round trip.
		if healthcheck.Disabled != nil && (healthcheck.Cmd != nil ||
			healthcheck.Interval != nil ||
			healthcheck.Timeout != nil ||
			healthcheck.StartPeriod != nil ||
			healthcheck.Retries != nil) {
			return nil, appErrors.ErrValidationFailed.WithMessage(
				"healthcheck: disabled is mutually exclusive with every other healthcheck field")
		}

		changed := &libpoddefine.UpdateHealthCheckConfig{}
		if healthcheck.Disabled != nil {
			changed.NoHealthCheck = healthcheck.Disabled
		}
		if healthcheck.Cmd != nil {
			changed.HealthCmd = healthcheck.Cmd
		}
		if healthcheck.Interval != nil {
			if err := parseUpdateDuration("interval", *healthcheck.Interval); err != nil {
				return nil, err
			}
			changed.HealthInterval = healthcheck.Interval
		}
		if healthcheck.Timeout != nil {
			if err := parseUpdateDuration("timeout", *healthcheck.Timeout); err != nil {
				return nil, err
			}
			changed.HealthTimeout = healthcheck.Timeout
		}
		if healthcheck.StartPeriod != nil {
			if err := parseUpdateDuration("startPeriod", *healthcheck.StartPeriod); err != nil {
				return nil, err
			}
			changed.HealthStartPeriod = healthcheck.StartPeriod
		}
		if healthcheck.Retries != nil {
			changed.HealthRetries = healthcheck.Retries
		}
		opts.ChangedHealthCheckConfiguration = changed
	}

	return opts, nil
}

// parseUpdateDuration guards one healthcheck duration field: the value
// must parse as a Go duration — podman's own healthcheck parser is
// time.ParseDuration — or the update answers the documented 400
// VALIDATION_FAILED naming the field.
func parseUpdateDuration(field, value string) error {
	if _, err := time.ParseDuration(value); err != nil {
		return appErrors.ErrValidationFailed.WithMessage(
			fmt.Sprintf("healthcheck.%s: %q is not a valid duration (e.g. \"30s\")", field, value))
	}
	return nil
}

// ensureUpdateResources lazily creates the update options' resource
// limits, so a request with only one limit kind never allocates the
// other's sub-struct.
func ensureUpdateResources(opts *entities.ContainerUpdateOptions) *runtimespec.LinuxResources {
	if opts.Resources == nil {
		opts.Resources = &runtimespec.LinuxResources{}
	}
	return opts.Resources
}
