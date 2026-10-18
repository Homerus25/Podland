package networks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// TestFromNetworkInspectReport_MapsAllFields pins the detail mapping:
// every DTO field comes from the inspect report — name, id, driver,
// createdAt (RFC3339), subnets as CIDR strings, and the attached
// containers resolved from the report's Containers map (map key → id,
// entry name → name), ordered by container ID.
func TestFromNetworkInspectReport_MapsAllFields(t *testing.T) {
	base := networkFixture("frontend", "bridge", "2026-09-01T12:30:00Z", "10.89.0.0/16", "fd00:10:89::/64")
	base.ID = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	report := &entities.NetworkInspectReport{
		Network: base,
		Containers: map[string]entities.NetworkContainerInfo{
			"ctr-b": {Name: "db"},
			"ctr-a": {Name: "web"},
		},
	}

	got := FromNetworkInspectReport(report)

	assert.Equal(t, "frontend", got.Name)
	assert.Equal(t, "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", got.ID)
	assert.Equal(t, "bridge", got.Driver)
	assert.Equal(t, "2026-09-01T12:30:00Z", got.CreatedAt)
	assert.Equal(t, []string{"10.89.0.0/16", "fd00:10:89::/64"}, got.Subnets,
		"every subnet must be carried as a CIDR string")
	assert.Equal(t, []NetworkContainerRef{
		{ID: "ctr-a", Name: "web"},
		{ID: "ctr-b", Name: "db"},
	}, got.Containers, "containers resolve from the Containers map, ordered by ID")
}

// TestFromNetworkInspectReport_EmptyReport pins the empty case: a
// zero report maps to the zero DTO with both slices present — an empty
// array on the wire, never null — and the zero creation time in
// RFC3339 form (the same rendering FromPodmanNetwork gives a zero
// network).
func TestFromNetworkInspectReport_EmptyReport(t *testing.T) {
	got := FromNetworkInspectReport(&entities.NetworkInspectReport{})

	assert.Equal(t, NetworkDetailResponse{
		CreatedAt:  "0001-01-01T00:00:00Z",
		Subnets:    []string{},
		Containers: []NetworkContainerRef{},
	}, got)
}

// TestFromNetworkInspectReport_NilReport pins the nil case: a nil
// report maps to the zero DTO with both slices present.
func TestFromNetworkInspectReport_NilReport(t *testing.T) {
	got := FromNetworkInspectReport(nil)

	assert.Equal(t, NetworkDetailResponse{
		Subnets:    []string{},
		Containers: []NetworkContainerRef{},
	}, got)
}
