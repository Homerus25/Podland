//go:build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The second-host harness slice (issue 208). Host B is the cross-
// host fixture: a second testhost container with podman state
// independent of host A's, registered through the same public API as
// any host and torn down by the suite cleanup stack. These two tests
// pin the two guarantees downstream slices (quadlet move/copy) build on:
//
//   - Registration works and the host is usable end to end
//     (Test_Integration_SecondHost_Registration) — no systemd needed,
//     so this runs everywhere the suite runs.
//   - B's quadlet state is genuinely separate from A's
//     (Test_Integration_SecondHost_IndependentQuadletState) — the
//     issue's AC #1. Quadlet install/list dial the user systemd over
//     DBus, so like every quadlet slice this gates on the systemd probe
//     and skips on the systemd-free container test host.
//
// Isolation model: host B itself is suite-owned (never deleted by a
// test); anything a test installs on it uses run-scoped names and its
// own t.Cleanup, exactly like host A. Whatever a failed test leaves
// on B dies with B's container at teardown.

// Test_Integration_SecondHost_Registration pins the harness
// mechanics: the lazy boot produces a host row the authenticated user
// can see and write through the full HTTP → JWT → SSH tunnel path. Runs
// on the container test host (systemd-free or not) and under the
// PODLAND_TEST_SSH_* escape hatch alike — in the latter it exercises the
// shared-host fallback, which is fine for registration mechanics.
func Test_Integration_SecondHost_Registration(t *testing.T) {
	suite.SkipIfUnavailable(t)

	b := suite.RegisterSecondHost(t)
	require.NotEmpty(t, b.HostID, "host B must have a registered row ID")
	require.NotEmpty(t, b.ConnectionName, "host B must expose a podman connection name for CLI helpers")
	require.NotEmpty(t, b.Target.SSHKey, "host B's target must carry the SSH key material for direct probes")

	// The row is visible in the owner's host list with the
	// caller-enriched permission fields. An owner's own row reports
	// "admin" (the highest level, implying write) — assert at-least-write
	// rather than a specific level so a future owner-level rename fails
	// loudly here instead of silently downgrading the fixture.
	entry, listed := findHostInList(t, b.HostID)
	require.True(t, listed, "host B must appear in the authenticated user's host list")
	require.Contains(t, []string{"admin", "write"}, entry.Permission,
		"host B must be writable by the authenticated user")
	require.True(t, entry.IsOwner, "the suite user must own host B")

	// The row serves requests against its own host through the full
	// tunnel — proving B is a working host, not just a DB row.
	names := listContainerNames(t, b.HostID)
	t.Logf("host B containers: %v", names)
}

// Test_Integration_SecondHost_IndependentQuadletState is the issue's
// AC #1: a quadlet installed through host A is not listed on host
// B, and vice versa — the independence guarantee that makes physical
// transfers observable for the quadlet move/copy slices.
//
// Gating, in order (each skip is the harness doing its job, not a gap):
//
//  1. requireUserSystemd — host A's quadlet surface needs user
//     systemd, the same gate every quadlet slice uses. On the
//     systemd-free container test host this test skips before host B
//     is even booted.
//  2. !b.Independent — under the PODLAND_TEST_SSH_* escape hatch the
//     helper falls back to a second host ROW on the same external
//     host (recorded in secondhost_test.go), where A and B share podman
//     state and an independence assertion is meaningless.
//  3. userSystemdAvailableOn(b.Target) — host B needs its own user
//     systemd manager for the same reason host A does.
func Test_Integration_SecondHost_IndependentQuadletState(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerQuadletsSweep()

	requireUserSystemd(t)

	b := suite.RegisterSecondHost(t)
	if !b.Independent {
		t.Skip("host B shares host A's host (PODLAND_TEST_SSH_* fallback) — quadlet state cannot be independent there")
	}
	if !userSystemdAvailableOn(b.Target) {
		t.Skip("host B user systemd unusable (is-system-running did not answer running/degraded)")
	}

	// Run-scoped names so the sweep and cleanups can find exactly what
	// this test created. Cleanup is registered before the creates and
	// is 404-tolerant; the cross-host entries are cleaned up too in
	// case a future regression actually leaks a quadlet across hosts.
	nameA := "it-" + suite.RunID + "-xmach-a"
	nameB := "it-" + suite.RunID + "-xmach-b"
	t.Cleanup(func() { deleteQuadlet(t, suite.HostID, nameA+".container") })
	t.Cleanup(func() { deleteQuadlet(t, suite.HostID, nameB+".container") })
	t.Cleanup(func() { deleteQuadlet(t, b.HostID, nameA+".container") })
	t.Cleanup(func() { deleteQuadlet(t, b.HostID, nameB+".container") })

	recipe := quadletCreateBody{
		Name:     nameA,
		Resource: "container",
		Content:  runnableContainerQuadletContent,
	}

	// --- AC: a quadlet installed through host A is not listed on B -
	createQuadlet(t, suite.HostID, recipe)
	// Presence proof on A first, so an always-empty list on B couldn't
	// pass the absence assertion vacuously.
	requireQuadletListed(t, suite.HostID, nameA+".container",
		"host A must list the quadlet it installed")
	requireQuadletNotListed(t, b.HostID, nameA+".container",
		"a quadlet installed through host A must not be listed on host B (independent state)")

	// --- and the reverse direction, so B is proven a separate host
	// rather than a stale mirror of A ---------------------------------
	recipeB := recipe
	recipeB.Name = nameB
	createQuadlet(t, b.HostID, recipeB)
	requireQuadletListed(t, b.HostID, nameB+".container",
		"host B must list the quadlet it installed")
	requireQuadletNotListed(t, suite.HostID, nameB+".container",
		"a quadlet installed through host B must not be listed on host A (independent state)")
}
