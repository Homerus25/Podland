package containers

import (
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	libpoddefine "go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// stringPtr is the test-local address-of helper for the update request's
// pointer fields.
func stringPtr(s string) *string {
	return &s
}

// uintPtr is the test-local address-of helper for the update request's
// retries field.
func uintPtr(u uint) *uint {
	return &u
}

// TestBuildUpdateOptions_CpusForwardsPeriodAndQuota pins the cpus
// mapping: a present cpus value converts through podman's own
// cores conversion — identical to the create builder — onto the update
// options' CPU limit.
func TestBuildUpdateOptions_CpusForwardsPeriodAndQuota(t *testing.T) {
	opts, err := buildUpdateOptions(ContainerUpdateRequest{CPUs: 1.5})

	require.NoError(t, err)
	require.NotNil(t, opts.Resources, "a cpus value must set the update's resource limits")
	require.NotNil(t, opts.Resources.CPU, "a cpus value must set the update's CPU limit")
	require.NotNil(t, opts.Resources.CPU.Period)
	require.NotNil(t, opts.Resources.CPU.Quota)
	assert.Equal(t, uint64(100000), *opts.Resources.CPU.Period, "the period must match the create builder's conversion")
	assert.Equal(t, int64(150000), *opts.Resources.CPU.Quota, "the quota must be cpus x period, like the create builder")
	assert.Nil(t, opts.Resources.Memory, "a cpus-only request must leave the memory limit alone (patch semantics)")
	assert.Nil(t, opts.ChangedHealthCheckConfiguration, "a cpus-only request must not touch the healthcheck")
}

// TestBuildUpdateOptions_MemoryForwardsByteLimit pins the memory
// mapping: a present memory value parses through podman's units parser —
// exactly like create — onto the update options' memory limit.
func TestBuildUpdateOptions_MemoryForwardsByteLimit(t *testing.T) {
	opts, err := buildUpdateOptions(ContainerUpdateRequest{Memory: "512m"})

	require.NoError(t, err)
	require.NotNil(t, opts.Resources, "a memory value must set the update's resource limits")
	require.NotNil(t, opts.Resources.Memory, "a memory value must set the update's memory limit")
	require.NotNil(t, opts.Resources.Memory.Limit)
	assert.Equal(t, int64(512*1024*1024), *opts.Resources.Memory.Limit, "the memory must parse through podman's units parser like create")
	assert.Nil(t, opts.Resources.CPU, "a memory-only request must leave the CPU limit alone (patch semantics)")
}

// TestBuildUpdateOptions_HealthcheckFieldsForward pins the healthcheck
// mapping: every present field forwards onto podman's
// changed-healthcheck configuration, and absent fields stay nil
// (unchanged). Durations forward as the Go duration strings podman's own
// CLI syntax expects.
func TestBuildUpdateOptions_HealthcheckFieldsForward(t *testing.T) {
	opts, err := buildUpdateOptions(ContainerUpdateRequest{
		Healthcheck: &ContainerHealthcheckUpdateRequest{
			Cmd:         stringPtr("curl -f http://localhost/health"),
			Interval:    stringPtr("30s"),
			Timeout:     stringPtr("5s"),
			StartPeriod: stringPtr("1m"),
			Retries:     uintPtr(3),
		},
	})

	require.NoError(t, err)
	require.NotNil(t, opts.ChangedHealthCheckConfiguration)
	changed := opts.ChangedHealthCheckConfiguration
	require.NotNil(t, changed.HealthCmd)
	assert.Equal(t, "curl -f http://localhost/health", *changed.HealthCmd)
	require.NotNil(t, changed.HealthInterval)
	assert.Equal(t, "30s", *changed.HealthInterval)
	require.NotNil(t, changed.HealthTimeout)
	assert.Equal(t, "5s", *changed.HealthTimeout)
	require.NotNil(t, changed.HealthStartPeriod)
	assert.Equal(t, "1m", *changed.HealthStartPeriod)
	require.NotNil(t, changed.HealthRetries)
	assert.Equal(t, uint(3), *changed.HealthRetries)
	assert.Nil(t, changed.NoHealthCheck, "a request without the disable flag must leave it nil (unchanged)")
	assert.Nil(t, opts.Resources, "a healthcheck-only request must not touch the resource limits")
}

// TestBuildUpdateOptions_HealthcheckAbsentFieldsStayNil pins the patch
// semantics inside the healthcheck sub-object: only the fields the
// request carries are forwarded, so a partial update leaves the absent
// pointers nil — podman's own "nil means unchanged" contract.
func TestBuildUpdateOptions_HealthcheckAbsentFieldsStayNil(t *testing.T) {
	opts, err := buildUpdateOptions(ContainerUpdateRequest{
		Healthcheck: &ContainerHealthcheckUpdateRequest{
			Interval: stringPtr("10s"),
		},
	})

	require.NoError(t, err)
	changed := opts.ChangedHealthCheckConfiguration
	require.NotNil(t, changed)
	assert.Nil(t, changed.HealthCmd, "an absent cmd must stay nil (unchanged)")
	require.NotNil(t, changed.HealthInterval)
	assert.Equal(t, "10s", *changed.HealthInterval)
	assert.Nil(t, changed.HealthTimeout, "an absent timeout must stay nil (unchanged)")
	assert.Nil(t, changed.HealthStartPeriod, "an absent startPeriod must stay nil (unchanged)")
	assert.Nil(t, changed.HealthRetries, "an absent retries must stay nil (unchanged)")
}

// TestBuildUpdateOptions_DisableFlagForwardsNoHealthCheck pins the
// disable mapping: the request's disable flag forwards as podman's
// no-healthcheck flag.
func TestBuildUpdateOptions_DisableFlagForwardsNoHealthCheck(t *testing.T) {
	opts, err := buildUpdateOptions(ContainerUpdateRequest{
		Healthcheck: &ContainerHealthcheckUpdateRequest{
			Disabled: boolPtr(true),
		},
	})

	require.NoError(t, err)
	changed := opts.ChangedHealthCheckConfiguration
	require.NotNil(t, changed)
	require.NotNil(t, changed.NoHealthCheck)
	assert.True(t, *changed.NoHealthCheck, "the disable flag must forward as podman's no-healthcheck flag")
	assert.Nil(t, changed.HealthCmd, "a disable-only request must carry no other healthcheck field")
}

// TestBuildUpdateOptions_DisableFlagExclusiveWithOtherFields pins the
// documented exclusivity: the disable flag combined with any other
// healthcheck field answers 400 VALIDATION_FAILED — podman refuses the
// combination ("cannot specify both --no-healthcheck and other
// HealthCheck flags"), so the backend answers before any round trip.
func TestBuildUpdateOptions_DisableFlagExclusiveWithOtherFields(t *testing.T) {
	cases := []struct {
		name        string
		healthcheck ContainerHealthcheckUpdateRequest
	}{
		{"with cmd", ContainerHealthcheckUpdateRequest{Cmd: stringPtr("true")}},
		{"with interval", ContainerHealthcheckUpdateRequest{Interval: stringPtr("30s")}},
		{"with timeout", ContainerHealthcheckUpdateRequest{Timeout: stringPtr("5s")}},
		{"with startPeriod", ContainerHealthcheckUpdateRequest{StartPeriod: stringPtr("1m")}},
		{"with retries", ContainerHealthcheckUpdateRequest{Retries: uintPtr(3)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.healthcheck.Disabled = boolPtr(true)

			opts, err := buildUpdateOptions(ContainerUpdateRequest{Healthcheck: &tc.healthcheck})

			assert.Nil(t, opts, "an exclusive violation must not produce update options")
			var appErr *appErrors.AppError
			require.ErrorAs(t, err, &appErr)
			assert.Equal(t, "VALIDATION_FAILED", appErr.Code)
			assert.Contains(t, appErr.Message, "healthcheck", "the message must name the field group")
		})
	}
}

// TestBuildUpdateOptions_UnparseableMemoryAnswersValidationFailed pins
// the memory parse: podman's units parser exactly like create, with the
// documented 400 VALIDATION_FAILED naming the field.
func TestBuildUpdateOptions_UnparseableMemoryAnswersValidationFailed(t *testing.T) {
	opts, err := buildUpdateOptions(ContainerUpdateRequest{Memory: "not-a-size"})

	assert.Nil(t, opts, "an unparseable memory value must not produce update options")
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "VALIDATION_FAILED", appErr.Code)
	assert.Contains(t, appErr.Message, "memory", "the message must name the field")
	assert.Contains(t, appErr.Message, "512m", "the message must name the expected syntax")
}

// TestBuildUpdateOptions_MalformedDurationAnswersValidationFailed pins
// the duration parse: interval, timeout, and startPeriod parse as Go
// durations — podman's own parser — and a malformed value answers the
// documented 400 VALIDATION_FAILED naming the field.
func TestBuildUpdateOptions_MalformedDurationAnswersValidationFailed(t *testing.T) {
	cases := []struct {
		name        string
		healthcheck ContainerHealthcheckUpdateRequest
		field       string
	}{
		{"interval", ContainerHealthcheckUpdateRequest{Interval: stringPtr("not-a-duration")}, "interval"},
		{"timeout", ContainerHealthcheckUpdateRequest{Timeout: stringPtr("later")}, "timeout"},
		{"startPeriod", ContainerHealthcheckUpdateRequest{StartPeriod: stringPtr("soon")}, "startPeriod"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := buildUpdateOptions(ContainerUpdateRequest{Healthcheck: &tc.healthcheck})

			assert.Nil(t, opts, "a malformed duration must not produce update options")
			var appErr *appErrors.AppError
			require.ErrorAs(t, err, &appErr)
			assert.Equal(t, "VALIDATION_FAILED", appErr.Code)
			assert.Contains(t, appErr.Message, tc.field, "the message must name the field")
		})
	}
}

// TestBuildUpdateOptions_EmptyRequestForwardsNothing pins the patch
// semantics' base case: a request that omits every field produces update
// options that change nothing — no resource limits, no healthcheck
// changes — so podman merges nothing and every aspect stays unchanged.
func TestBuildUpdateOptions_EmptyRequestForwardsNothing(t *testing.T) {
	opts, err := buildUpdateOptions(ContainerUpdateRequest{})

	require.NoError(t, err)
	assert.Nil(t, opts.Resources, "an empty request must not touch the resource limits")
	assert.Nil(t, opts.ChangedHealthCheckConfiguration, "an empty request must not touch the healthcheck")
}

// TestBuildUpdateOptions_ForwardsOnlyPresentFields pins the full patch
// combination: cpus, memory, and healthcheck fields ride one update
// call together, each limit landing on its own sub-struct so podman
// merges only what the request carried.
func TestBuildUpdateOptions_ForwardsOnlyPresentFields(t *testing.T) {
	opts, err := buildUpdateOptions(ContainerUpdateRequest{
		CPUs:   2,
		Memory: "1g",
		Healthcheck: &ContainerHealthcheckUpdateRequest{
			Cmd:     stringPtr("healthcheck.sh"),
			Retries: uintPtr(5),
		},
	})

	require.NoError(t, err)
	require.NotNil(t, opts.Resources)
	require.NotNil(t, opts.Resources.CPU)
	assert.Equal(t, int64(200000), *opts.Resources.CPU.Quota)
	require.NotNil(t, opts.Resources.Memory)
	assert.Equal(t, int64(1024*1024*1024), *opts.Resources.Memory.Limit)
	changed := opts.ChangedHealthCheckConfiguration
	require.NotNil(t, changed)
	require.NotNil(t, changed.HealthCmd)
	assert.Equal(t, "healthcheck.sh", *changed.HealthCmd)
	require.NotNil(t, changed.HealthRetries)
	assert.Equal(t, uint(5), *changed.HealthRetries)
	assert.Nil(t, changed.HealthInterval, "an absent interval must stay nil (unchanged)")
}

// TestBuildUpdateOptions_TypeShape pins the options' identity wiring:
// the update targets one container and never asks podman for the
// deprecated specgen path or the latest pseudo-target.
func TestBuildUpdateOptions_TypeShape(t *testing.T) {
	opts, err := buildUpdateOptions(ContainerUpdateRequest{Memory: "256m"})

	require.NoError(t, err)
	assert.Nil(t, opts.Specgen, "the deprecated specgen path must stay nil")
	assert.False(t, opts.Latest, "the latest pseudo-target must never be set")
	assert.Empty(t, opts.Env, "environment updates are out of scope")
	assert.Nil(t, opts.RestartPolicy, "restart policy updates are out of scope")
	assert.Nil(t, opts.DevicesLimits, "device limit updates are out of scope")
	assert.Empty(t, opts.Rlimits, "rlimit updates are out of scope")
	var _ *entities.ContainerUpdateOptions = opts
	var _ *libpoddefine.UpdateHealthCheckConfig = opts.ChangedHealthCheckConfiguration
}
