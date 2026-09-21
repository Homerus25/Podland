//go:build integration

package integration

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// The container test host: a privileged, systemd-booted Fedora container
// that stands in for a podman machine (image built by `make
// testhost-build` from test/integration/testhost/Dockerfile). Everything
// the harness knows about that host lives in this file: the run
// contract, the per-run identity, and the connection registration the
// CLI test helpers dial through.
//
// The backend product code is deliberately unaware of the container —
// the harness registers it through the same public API as any machine.
const (
	// testhostImage must match TESTHOST_IMAGE in the backend Makefile.
	testhostImage = "podland-testhost"

	// testhostBuildTarget is the make target that builds testhostImage.
	// Named verbatim in the harness's fail-fast error when the image is
	// missing so the fix is one copy-paste away.
	testhostBuildTarget = "testhost-build"

	// Fixed container and volume names + a fixed loopback port implement
	// the serialized-runs model: a second run on the same host collides
	// loudly instead of racing over the shared storage volumes.
	testhostContainer   = "podland-it-testhost"
	testhostSSHVolume   = "podland-it-testhost-ssh"   // /etc/ssh — stable host keys
	testhostStoreVolume = "podland-it-testhost-store" // rootless podman image cache

	// The published sshd endpoint and the rootless socket behind it.
	testhostHost    = "127.0.0.1"
	testhostPort    = 24422
	testhostSSHUser = "core"
	testhostSocket  = "/run/user/1000/podman/podman.sock"

	// testhostConnectionDefault is the `podman system connection` name
	// registered per run (overridable via PODLAND_TEST_CONNECTION_NAME).
	// Suite.MachineName carries it to every `podman --connection` helper.
	testhostConnectionDefault = "podland-it"
)

// ensureTesthostImage fails fast when the test-host image is absent —
// with the make target that builds it, so a fresh clone produces one
// actionable error instead of a cryptic `podman run` failure.
func ensureTesthostImage() error {
	if out, err := exec.Command("podman", "image", "inspect", testhostImage).CombinedOutput(); err != nil {
		return fmt.Errorf(
			"integration test host image %q is missing — run `make %s` in backend/ to build it (%s)",
			testhostImage, testhostBuildTarget, strings.TrimSpace(string(out)))
	}
	return nil
}

// startTesthostContainer boots the test host with the full run contract:
// privileged (nested rootless podman needs the userns/mount latitude),
// label=disable (no SELinux labeling of the inner overlay), /dev/fuse
// (fuse-overlayfs), sshd published on a fixed loopback port, and the two
// persistent volumes — image cache (fast re-runs) and SSH host keys
// (stable host key, so the bindings' known_hosts auto-append is a
// one-time event rather than a per-run mismatch). The image's own
// ENTRYPOINT is the boot command: a plain-process init (sshd in the
// foreground + `podman system service` for core's socket) that needs no
// systemd — the environments this suite runs in are often containers
// without cgroup delegation, where a nested systemd manager cannot
// boot.
//
// Pre-checked collision surfaces (container name, published port) yield
// the serialized-runs error; `podman run` itself is the backstop.
func startTesthostContainer() error {
	if err := exec.Command("podman", "container", "exists", testhostContainer).Run(); err == nil {
		return fmt.Errorf(
			"container %q already exists — another integration run is active on this host (or a previous run crashed); run `podman rm -f %s` and retry",
			testhostContainer, testhostContainer)
	}
	if !tcpPortAvailable(testhostHost, testhostPort) {
		return fmt.Errorf(
			"port %s:%d is already in use — another integration run is active on this host",
			testhostHost, testhostPort)
	}

	args := []string{
		"run", "--detach",
		"--name", testhostContainer,
		"--privileged",
		"--security-opt", "label=disable",
		"--device", "/dev/fuse",
		"-p", fmt.Sprintf("%s:%d:22", testhostHost, testhostPort),
		"-v", testhostStoreVolume + ":/home/core/.local/share/containers",
		"-v", testhostSSHVolume + ":/etc/ssh",
		testhostImage,
	}
	if out, err := exec.Command("podman", args...).CombinedOutput(); err != nil {
		if isSerializedRunsCollision(string(out)) {
			return fmt.Errorf(
				"podman refused to start the test host — another integration run is active on this host (%s)",
				strings.TrimSpace(string(out)))
		}
		return fmt.Errorf("podman run %s: %w (%s)", testhostContainer, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// removeTesthostContainer tears the test host down. The volumes are
// deliberately retained: the store makes re-runs skip re-pulling images
// and the SSH volume keeps the host key stable.
func removeTesthostContainer() error {
	if out, err := exec.Command("podman", "rm", "-f", testhostContainer).CombinedOutput(); err != nil {
		return fmt.Errorf("podman rm -f %s: %w (%s)", testhostContainer, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// generateRunKeypair creates the per-run SSH identity in dir: a fresh,
// passphrase-less ed25519 keypair whose private half never leaves the
// run's temp dir (same hygiene as the suite's per-run JWT keys) and
// whose public half is injected into the container as core's sole
// authorized key. Returns the private key path and the authorized_keys
// line.
func generateRunKeypair(dir string) (privPath, pubLine string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate ed25519 key: %w", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return "", "", fmt.Errorf("marshal public key: %w", err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "podland-integration-run")
	if err != nil {
		return "", "", fmt.Errorf("marshal private key: %w", err)
	}

	privPath = filepath.Join(dir, "run-ed25519")
	if err := os.WriteFile(privPath, pem.EncodeToMemory(block), 0600); err != nil {
		return "", "", fmt.Errorf("write private key: %w", err)
	}
	pubLine = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
	if err := os.WriteFile(privPath+".pub", []byte(pubLine+"\n"), 0644); err != nil {
		return "", "", fmt.Errorf("write public key: %w", err)
	}
	return privPath, pubLine, nil
}

// injectAuthorizedKey installs pubLine as core's authorized_keys inside
// the running test host, via podman exec as root: write, chown to core,
// and the permissions sshd's StrictModes requires (0700 dir, 0600 file).
// Overwrite (not append) is deliberate: runs are serialized and the
// image ships no committed keys, so the file ends up holding exactly
// this run's key.
func injectAuthorizedKey(pubLine string) error {
	const script = `set -eu
mkdir -p /home/core/.ssh
printf '%s\n' "$PODLAND_IT_PUBKEY" > /home/core/.ssh/authorized_keys
chown core:core /home/core/.ssh /home/core/.ssh/authorized_keys
chmod 700 /home/core/.ssh
chmod 600 /home/core/.ssh/authorized_keys
`
	cmd := exec.Command("podman", "exec", "--user", "root",
		"-e", "PODLAND_IT_PUBKEY="+pubLine,
		testhostContainer, "sh", "-c", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("inject authorized_keys into %s: %w (%s)",
			testhostContainer, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// sshOverrideEnvs are the PODLAND_TEST_SSH_* escape-hatch variables.
// Setting ANY of them targets an externally managed host and skips the
// container lifecycle entirely; the set is all-or-nothing.
var sshOverrideEnvs = []string{
	"PODLAND_TEST_SSH_HOST",
	"PODLAND_TEST_SSH_PORT",
	"PODLAND_TEST_SSH_USER",
	"PODLAND_TEST_SSH_SOCKET",
	"PODLAND_TEST_SSH_KEY_PATH",
}

// sshOverridesSet reports whether the escape hatch is engaged.
func sshOverridesSet() bool {
	for _, key := range sshOverrideEnvs {
		if os.Getenv(key) != "" {
			return true
		}
	}
	return false
}

// externalTargetFromEnv resolves the escape-hatch coordinates. With no
// connection discovery anymore, the overrides must carry the complete
// coordinate set, so a partial set is an error naming every missing
// variable. Returns the target (SSHKey left empty — the key material is
// read from IdentityPath by TestMain) and the identity path.
func externalTargetFromEnv() (reachableTarget, string, error) {
	var missing []string
	get := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}
	host := get("PODLAND_TEST_SSH_HOST")
	portStr := get("PODLAND_TEST_SSH_PORT")
	user := get("PODLAND_TEST_SSH_USER")
	socket := get("PODLAND_TEST_SSH_SOCKET")
	identityPath := get("PODLAND_TEST_SSH_KEY_PATH")
	if len(missing) > 0 {
		return reachableTarget{}, "", fmt.Errorf(
			"PODLAND_TEST_SSH_* escape hatch is partial — set all of %s or none; missing: %s",
			strings.Join(sshOverrideEnvs, ", "), strings.Join(missing, ", "))
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return reachableTarget{}, "", fmt.Errorf("PODLAND_TEST_SSH_PORT=%q is not an integer: %w", portStr, err)
	}
	return reachableTarget{
		Host:    host,
		Port:    port,
		SSHUser: user,
		Socket:  socket,
	}, identityPath, nil
}

// registerPodmanConnection registers the `podman system connection` the
// test helpers (`podman --connection <name>`) and the readiness probe
// dial through. A leftover connection with the same name from a crashed
// run is removed first — the fixed name is suite-owned.
func registerPodmanConnection(name, identityPath string, target reachableTarget) error {
	// Best-effort sweep of a stale leftover; "does not exist" is fine.
	_ = exec.Command("podman", "system", "connection", "rm", name).Run()

	uri := fmt.Sprintf("ssh://%s@%s:%d%s", target.SSHUser, target.Host, target.Port, target.Socket)
	args := []string{"system", "connection", "add", name, "--identity", identityPath, uri}
	if out, err := exec.Command("podman", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("podman system connection add %s (%s): %w (%s)",
			name, uri, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// removePodmanConnection is the teardown counterpart of
// registerPodmanConnection.
func removePodmanConnection(name string) error {
	if out, err := exec.Command("podman", "system", "connection", "rm", name).CombinedOutput(); err != nil {
		return fmt.Errorf("podman system connection rm %s: %w (%s)",
			name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// waitForSSHDPort blocks until the published sshd port accepts a TCP
// connection or the timeout elapses. The port accepting means the
// container's network namespace is up; sshd itself may still be warming,
// which is the socket probe's job to wait out.
func waitForSSHDPort(host string, port int, timeout time.Duration) error {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("sshd port %s not accepting within %s", addr, timeout)
}

// probePodmanConnection retries a real podman-over-SSH probe
// (`podman --connection <name> ps`) with exponential backoff until the
// budget elapses. It exercises the exact path the backend will use —
// including the first-contact known_hosts auto-append — so a green
// probe means the tracer can rely on a warm socket. A host-key
// mismatch is deterministic and fails fast with the fix hint instead of
// burning the budget.
func probePodmanConnection(name string, budget time.Duration) error {
	deadline := time.Now().Add(budget)
	backoff := 1 * time.Second
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		out, err := exec.CommandContext(ctx, "podman", "--connection", name, "ps").CombinedOutput()
		cancel()
		if err == nil {
			return nil
		}
		lastErr = fmt.Errorf("podman --connection %s ps: %w (%s)", name, err, strings.TrimSpace(string(out)))
		if isKnownHostsMismatch(string(out)) {
			return fmt.Errorf(`%w
  hint: the test host's SSH host key changed (the %q volume was wiped or recreated?).
  Remove the stale entry with: ssh-keygen -R "[%s]:%d"`, lastErr, testhostSSHVolume, testhostHost, testhostPort)
		}
		time.Sleep(backoff)
		backoff = min(backoff*2, 8*time.Second)
	}
	return fmt.Errorf("podman socket probe never succeeded within %s: %w", budget, lastErr)
}

// isSerializedRunsCollision reports whether combined output from a
// `podman run` failure is the name/port-in-use shape — the signature of
// a second integration run fighting the first over the fixed container
// name or published port.
func isSerializedRunsCollision(combined string) bool {
	return strings.Contains(strings.ToLower(combined), "already in use")
}

// isKnownHostsMismatch reports whether combined output carries a
// changed-host-key failure. Podman's golang SSH client phrases it as a
// knownhosts key mismatch; OpenSSH tooling as a host-key-verification
// failure / REMOTE HOST IDENTIFICATION HAS CHANGED. Deliberately no
// bare "knownhosts" match: transient errors can name the file without
// the failure being the deterministic, un-retryable mismatch.
func isKnownHostsMismatch(combined string) bool {
	lower := strings.ToLower(combined)
	return strings.Contains(lower, "key mismatch") ||
		strings.Contains(lower, "host key verification failed") ||
		strings.Contains(lower, "remote host identification has changed")
}

// tcpPortAvailable reports whether host:port can be bound right now —
// the pre-flight check behind the serialized-runs port guard.
func tcpPortAvailable(host string, port int) bool {
	ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
