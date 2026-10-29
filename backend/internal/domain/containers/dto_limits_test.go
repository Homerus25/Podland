package containers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// limitsInspectReport builds an inspect report carrying the resource
// limits and healthcheck the detail DTO maps: a 1.5-cpus CPU limit
// (quota/period), a 512MiB memory limit, and a CMD-SHELL healthcheck
// with all timing fields set.
func limitsInspectReport() *entities.ContainerInspectReport {
	return &entities.ContainerInspectReport{InspectContainerData: &define.InspectContainerData{
		ID:   "container-123",
		Name: "/web",
		HostConfig: &define.InspectContainerHostConfig{
			Memory:    512 * 1024 * 1024,
			CpuPeriod: 100000,
			CpuQuota:  150000,
		},
		Config: &define.InspectContainerConfig{
			Healthcheck: &manifest.Schema2HealthConfig{
				Test:        []string{"CMD-SHELL", "curl -f http://localhost/health || exit 1"},
				Interval:    30 * time.Second,
				Timeout:     5 * time.Second,
				StartPeriod: 1 * time.Minute,
				Retries:     3,
			},
		},
	}}
}

// TestFromPodmanInspectContainer_MapsLimitsAndHealthcheck pins the
// detail DTO's limits and healthcheck mapping: cpus derives from the
// inspect report's CPU quota/period, memory renders back into podman
// units syntax, and the healthcheck's command derives from the test
// array with the timing fields as Go duration strings — the prefill
// contract for the edit form.
func TestFromPodmanInspectContainer_MapsLimitsAndHealthcheck(t *testing.T) {
	got := FromPodmanInspectContainer(limitsInspectReport())

	require.NotNil(t, got.Limits, "a report with limits must map them")
	assert.Equal(t, 1.5, got.Limits.CPUs, "cpus derives from the report's quota/period")
	assert.Equal(t, "512m", got.Limits.Memory, "memory renders back into podman units syntax")

	require.NotNil(t, got.Healthcheck, "a report with a healthcheck must map it")
	assert.Equal(t, "curl -f http://localhost/health || exit 1", got.Healthcheck.Command,
		"the command derives from the test array's CMD-SHELL entry")
	assert.Equal(t, "30s", got.Healthcheck.Interval)
	assert.Equal(t, "5s", got.Healthcheck.Timeout)
	assert.Equal(t, "1m0s", got.Healthcheck.StartPeriod)
	assert.Equal(t, uint(3), got.Healthcheck.Retries)
	assert.False(t, got.Healthcheck.Disabled, "a CMD-SHELL test is an enabled healthcheck")
}

// TestFromPodmanInspectContainer_MapsCmdTestCommand pins the CMD test
// shape: the command derives from the test array's arguments joined
// into one string.
func TestFromPodmanInspectContainer_MapsCmdTestCommand(t *testing.T) {
	report := limitsInspectReport()
	report.Config.Healthcheck = &manifest.Schema2HealthConfig{
		Test: []string{"CMD", "pg_isready", "-U", "postgres"},
	}

	got := FromPodmanInspectContainer(report)

	require.NotNil(t, got.Healthcheck)
	assert.Equal(t, "pg_isready -U postgres", got.Healthcheck.Command,
		"a CMD test's arguments join into the command string")
}

// TestFromPodmanInspectContainer_MapsDisabledHealthcheck pins the
// disabled derivation: a NONE test marks the healthcheck disabled, the
// podman update disable flag's own storage shape.
func TestFromPodmanInspectContainer_MapsDisabledHealthcheck(t *testing.T) {
	report := limitsInspectReport()
	report.Config.Healthcheck = &manifest.Schema2HealthConfig{
		Test: []string{"NONE"},
	}

	got := FromPodmanInspectContainer(report)

	require.NotNil(t, got.Healthcheck)
	assert.True(t, got.Healthcheck.Disabled, "a NONE test marks the healthcheck disabled")
	assert.Empty(t, got.Healthcheck.Command)
	assert.Empty(t, got.Healthcheck.Interval, "a zero duration (inherit default) maps to an empty string, never \"0s\"")
}

// TestFromPodmanInspectContainer_ZeroDurationsMapEmpty pins the inherit
// case: a zero duration in the report means "inherit podman's default",
// so it maps to an empty string — prefilled and resubmitted, a "0s"
// would disable the probe timer instead of leaving it unchanged.
func TestFromPodmanInspectContainer_ZeroDurationsMapEmpty(t *testing.T) {
	report := limitsInspectReport()
	report.Config.Healthcheck = &manifest.Schema2HealthConfig{
		Test: []string{"CMD-SHELL", "true"},
	}

	got := FromPodmanInspectContainer(report)

	require.NotNil(t, got.Healthcheck)
	assert.Empty(t, got.Healthcheck.Interval)
	assert.Empty(t, got.Healthcheck.Timeout)
	assert.Empty(t, got.Healthcheck.StartPeriod)
}

// TestFromPodmanInspectContainer_PartialLimits pin the partial cases: a
// memory-only limit maps with a zero cpus, and a cpus-only limit maps
// with an empty memory — the prefill shows only what is configured.
func TestFromPodmanInspectContainer_PartialLimits(t *testing.T) {
	memoryOnly := limitsInspectReport()
	memoryOnly.HostConfig = &define.InspectContainerHostConfig{Memory: 256 * 1024 * 1024}

	got := FromPodmanInspectContainer(memoryOnly)

	require.NotNil(t, got.Limits)
	assert.Equal(t, "256m", got.Limits.Memory)
	assert.Zero(t, got.Limits.CPUs, "a report without a CPU quota maps a zero cpus")

	cpusOnly := limitsInspectReport()
	cpusOnly.HostConfig = &define.InspectContainerHostConfig{CpuPeriod: 100000, CpuQuota: 100000}

	got = FromPodmanInspectContainer(cpusOnly)

	require.NotNil(t, got.Limits)
	assert.Equal(t, 1.0, got.Limits.CPUs)
	assert.Empty(t, got.Limits.Memory, "a report without a memory limit maps an empty memory")
}

// TestRenderMemoryLimit pins the memory renderer: the exact binary
// multiples render with their one-letter podman suffix — the vocabulary
// RAMInBytes parses and the create page's memory field accepts — the
// largest exact multiple wins, and a limit that is no clean multiple
// falls back to plain bytes with the "b" suffix.
func TestRenderMemoryLimit(t *testing.T) {
	cases := []struct {
		name  string
		limit int64
		want  string
	}{
		{"512 MiB", 512 * 1024 * 1024, "512m"},
		{"one kibibyte", 1024, "1k"},
		{"two gibibytes", 2 * 1024 * 1024 * 1024, "2g"},
		{"one tebibyte", 1024 * 1024 * 1024 * 1024, "1t"},
		{"one and a half kibibytes is no clean multiple", 1536, "1536b"},
		{"sub-kilobyte", 512, "512b"},
		{"single byte", 1, "1b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, renderMemoryLimit(tc.limit))
		})
	}
}

// TestFromPodmanInspectContainer_NoLimitsNoHealthcheckStayNil pins the
// absent cases: a report without limits and without a healthcheck maps
// both fields nil, so the JSON response omits them.
func TestFromPodmanInspectContainer_NoLimitsNoHealthcheckStayNil(t *testing.T) {
	report := &entities.ContainerInspectReport{InspectContainerData: &define.InspectContainerData{
		ID:         "container-123",
		Name:       "/web",
		HostConfig: &define.InspectContainerHostConfig{},
		Config:     &define.InspectContainerConfig{},
	}}

	got := FromPodmanInspectContainer(report)

	assert.Nil(t, got.Limits, "a report without limits maps nil limits")
	assert.Nil(t, got.Healthcheck, "a report without a healthcheck maps nil healthcheck")
}
