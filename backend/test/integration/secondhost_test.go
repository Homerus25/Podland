//go:build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Harness decision (issue 208) — recorded here per that ticket's AC.
//
// The suite boots a SECOND testhost container for host B (the
// ticket's preferred shape): same image and boot contract as the
// primary host, but its own container name and published port
// (127.0.0.1:24423) — and since the hosts share no volumes, its own
// container filesystem, hence its own podman store and SSH host keys.
// Host B's podman state is therefore genuinely independent of
// host A's — a quadlet installed through host A is not listed on
// host B, and the physical file transfer of a cross-host
// operation is observable on the target host. The shared-host fallback (two host rows on one
// testhost) was rejected as the default because a cross-host
// transfer between two rows on the SAME store is not just unobservable
// but semantically wrong: installing "on B" would collide with the very
// file being moved.
//
// The second container boots LAZILY: the first test that calls
// RegisterSecondHost pays for the boot, and runs that never exercise
// a cross-host feature never boot it at all — existing single-host
// slices run unchanged and at the old cost. Cleanup is suite-registered
// at boot time (host row → connection → container), so even a test
// that fails after registering host B leaves nothing behind.
//
// Under the PODLAND_TEST_SSH_* escape hatch the host lifecycle belongs
// to the operator, not the suite, so no second container is booted:
// RegisterSecondHost then registers a second host ROW against the
// same external host. That row shares host A's podman state —
// assertions against it are contract-level only and a physical transfer
// is not observable. SecondHost.Independent reports which shape the
// helper produced, so cross-host tests can skip (or downgrade to
// contract assertions) when independence is unavailable.
//
// Quadlet-level coverage: the testhost image boots under systemd, and
// the systemd probe checks the manager that answers for the registered
// SSH login, so in container mode the quadlet-independence assertions
// in second_host_test.go execute against both hosts. Under the
// PODLAND_TEST_SSH_* escape hatch they still gate out on the shared
// host, where independence is not observable.

// SecondHost describes host B as registered by
// RegisterSecondHost.
type SecondHost struct {
	// HostID is host B's row ID (UUID), usable in
	// /api/hosts/{id}/... paths via the suite's authenticated
	// client. The suite user owns it, so it is writable.
	HostID string

	// ConnectionName is the `podman system connection` name CLI helpers
	// dial host B's host through (suite.HostName + "-second" in
	// container mode; suite.HostName itself under the shared-host
	// fallback, where B has no host of its own). Its reachability is
	// probe-guaranteed before the helper returns.
	ConnectionName string

	// Target carries host B's SSH coordinates, including the private
	// key material, so tests can dial B directly (e.g. the user-systemd
	// probe). Under the fallback these are host A's coordinates.
	Target reachableTarget

	// Independent reports whether B's podman state is physically
	// separate from host A's (second container booted) or shared with
	// it (PODLAND_TEST_SSH_* fallback). Cross-host tests that assert
	// physical state must skip when this is false.
	Independent bool
}

// secondHostOnce lazily boots and registers host B at most once
// per run; every later caller gets the same host.
var (
	secondHostOnce   sync.Once
	secondHostResult SecondHost
	secondHostErr    error
)

// RegisterSecondHost registers a second host the authenticated
// suite user can write to, backed by its own testhost container
// (container mode) or by the shared external host (escape-hatch mode —
// see the decision comment above). The heavy work runs at most once per
// run; the host row, its podman connection and (container mode) the
// container are cleaned up through the suite cleanup stack, LIFO:
// host row delete → connection rm → container rm.
//
// A failed boot is remembered for the rest of the run: every later
// caller fails fast with the same error. Deliberate — a broken
// environment must not be re-paid per test — so the first caller's
// failure is the only diagnosis output you get.
//
// The returned host is a shared fixture: do not delete it from a
// test — later tests may still be using it. Mutate only resources you
// created yourself (run-scoped names + your own t.Cleanup, like on
// host A).
func (s *Suite) RegisterSecondHost(t *testing.T) SecondHost {
	t.Helper()
	secondHostOnce.Do(func() {
		if sshOverridesSet() {
			secondHostResult, secondHostErr = s.registerFallbackHostRow()
			return
		}
		secondHostResult, secondHostErr = s.bootAndRegisterSecondHost()
	})
	if secondHostErr != nil {
		t.Fatalf("register second host: %v", secondHostErr)
	}
	return secondHostResult
}

// bootAndRegisterSecondHost is the container-mode path: boot the second
// testhost container, inject its own per-run identity, register a
// podman connection + the host row, and wire the cleanup chain.
func (s *Suite) bootAndRegisterSecondHost() (SecondHost, error) {
	// The second host gets its own run identity, in its own subdir so
	// the primary identity file (which the primary `podman system
	// connection` still points at by path) is never touched.
	keyDir := filepath.Join(s.tempRoot, "secondhost")
	if err := os.MkdirAll(keyDir, 0o700); err != nil {
		return SecondHost{}, fmt.Errorf("create second-host key dir: %w", err)
	}
	keyPath, pubLine, err := generateRunKeypair(keyDir)
	if err != nil {
		return SecondHost{}, fmt.Errorf("generate second-host keypair: %w", err)
	}

	// From here on, partial boot failures roll back what the container
	// mode acquired so a lazy boot that dies mid-way leaks nothing. The
	// rollback closure grows as resources are acquired; once the host is
	// proven ready the suite cleanup stack takes over ownership (see
	// below) and the rollback is retired.
	if err := startTesthostContainer(secondTesthost); err != nil {
		return SecondHost{}, err
	}
	rollback := func() {
		_ = removeTesthostContainer(secondTesthost)
	}
	if err := injectAuthorizedKey(secondTesthost.container, pubLine); err != nil {
		rollback()
		return SecondHost{}, err
	}

	target := reachableTarget{
		Host:    testhostHost,
		Port:    secondTesthost.port,
		SSHUser: testhostSSHUser,
		Socket:  testhostSocket,
	}
	connectionName := s.HostName + "-second"
	if err := registerPodmanConnection(connectionName, keyPath, target); err != nil {
		rollback()
		return SecondHost{}, err
	}
	rollback = func() {
		_ = removePodmanConnection(connectionName)
		_ = removeTesthostContainer(secondTesthost)
	}

	// Readiness: same ritual as the primary host in TestMain.
	if err := waitForSSHDPort(target.Host, target.Port, 30*time.Second); err != nil {
		rollback()
		return SecondHost{}, err
	}
	if err := probePodmanConnection(secondTesthost, connectionName, 60*time.Second); err != nil {
		rollback()
		return SecondHost{}, err
	}
	// The image regenerates SSH host keys every boot, so the
	// known_hosts entry this run appended would poison the next run's
	// probe — sweep it at teardown, mirroring host A's cleanup in
	// TestMain.
	knownHostsEntry := fmt.Sprintf("[%s]:%d", testhostHost, secondTesthost.port)
	s.RegisterCleanup("ssh-keygen -R "+knownHostsEntry, func() error {
		_, err := exec.CommandContext(context.Background(), "ssh-keygen", "-R", knownHostsEntry).CombinedOutput()
		return err
	})
	logf("integration: second test host container %q up; identity injected", secondTesthost.container)

	// The host is proven ready — from here the suite cleanup stack owns
	// teardown ("like every other suite resource"): registered in REVERSE
	// of the intended execution order (LIFO), so the container rm runs
	// last and the host row delete first, while the backend and its
	// DB are still up. A failure in the remaining steps therefore relies
	// on the stack alone — no rollback double-run.
	s.RegisterCleanup("podman rm -f second test host", func() error {
		return removeTesthostContainer(secondTesthost)
	})
	s.RegisterCleanup("podman system connection rm (second host)", func() error {
		return removePodmanConnection(connectionName)
	})

	// Register the host row through the same public API as host A
	// (and as any production host): encryption-at-rest, owner-access
	// grant, connection probe — the lot.
	keyMaterial, err := os.ReadFile(keyPath)
	if err != nil {
		return SecondHost{}, fmt.Errorf("read second-host SSH identity: %w", err)
	}
	hostID, err := registerHost(s.ServerURL, s.Token, hostRegistration{
		Name:    "it-" + s.RunID + "-" + connectionName,
		Host:    target.Host,
		Port:    target.Port,
		SSHUser: target.SSHUser,
		Socket:  target.Socket,
		SSHKey:  string(keyMaterial),
	})
	if err != nil {
		return SecondHost{}, err
	}
	s.RegisterCleanup("delete second host row", func() error {
		return deleteHostRow(s.ServerURL, s.Token, hostID)
	})
	target.SSHKey = string(keyMaterial)
	logf("integration: second host registered: id=%s connection=%s", hostID, connectionName)

	return SecondHost{
		HostID:         hostID,
		ConnectionName: connectionName,
		Target:         target,
		Independent:    true,
	}, nil
}

// registerFallbackHostRow is the escape-hatch path: the operator owns
// the host lifecycle, so no second container is booted and the new
// host row points at the same external target host A uses. The
// result is contract-level only — see the decision comment above.
func (s *Suite) registerFallbackHostRow() (SecondHost, error) {
	target := s.goodHostRegistration
	hostID, err := registerHost(s.ServerURL, s.Token, hostRegistration{
		Name:    "it-" + s.RunID + "-second-shared",
		Host:    target.Host,
		Port:    target.Port,
		SSHUser: target.SSHUser,
		Socket:  target.Socket,
		SSHKey:  target.SSHKey,
	})
	if err != nil {
		return SecondHost{}, err
	}
	s.RegisterCleanup("delete second host row", func() error {
		return deleteHostRow(s.ServerURL, s.Token, hostID)
	})
	logf("integration: second host registered against the shared external host: id=%s", hostID)
	return SecondHost{
		HostID:         hostID,
		ConnectionName: s.HostName,
		Target:         target,
		Independent:    false,
	}, nil
}

// deleteHostRow DELETEs /api/hosts/{id} as the suite user —
// best-effort teardown (the hosts sweep keyed on the RunID is the
// backstop), so errors are returned for runCleanups to log, and an
// already-gone row (404) is success.
func deleteHostRow(baseURL, token, hostID string) error {
	if baseURL == "" || token == "" {
		return nil
	}
	req, err := http.NewRequest(http.MethodDelete, baseURL+"/api/hosts/"+hostID+"/", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE /api/hosts/%s: %w", hostID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	raw, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("DELETE /api/hosts/%s returned %d: %s", hostID, resp.StatusCode, string(raw))
}
