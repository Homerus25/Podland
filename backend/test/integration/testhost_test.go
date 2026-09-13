//go:build integration

package integration

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// Unit tests for the container test-host helpers that are pure Go and
// need no podman. They deliberately do NOT call suite.SkipIfUnavailable:
// they pin helper behaviour, not the host pipeline, and must pass no
// matter where TestMain got to. (TestMain still runs for every test in
// this package — that is the price of living behind the integration
// tag — but these tests never touch the suite fixture.)

// TestHost_GenerateRunKeypair pins the per-run identity contract: the
// private key is an unencrypted OpenSSH PEM that x/crypto parses, the
// public line is exactly that key's authorized_keys line, and the
// private file is 0600 (podman's SSH layer refuses world-readable
// identities).
func TestHost_GenerateRunKeypair(t *testing.T) {
	dir := t.TempDir()

	privPath, pubLine, err := generateRunKeypair(dir)
	require.NoError(t, err, "generating the run keypair must succeed")

	raw, err := os.ReadFile(privPath)
	require.NoError(t, err, "private key file must be readable")

	info, err := os.Stat(privPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(),
		"private key must be 0600, got %v", info.Mode().Perm())

	signer, err := ssh.ParsePrivateKey(raw)
	require.NoError(t, err, "private key must parse as an unencrypted OpenSSH key")
	assert.Equal(t, "ssh-ed25519", signer.PublicKey().Type(),
		"key type must be ed25519")

	// The public line must round-trip to the private key's public half —
	// an independent source of truth (the signer), not a re-derivation
	// of what the helper produced.
	assert.Equal(t,
		string(ssh.MarshalAuthorizedKey(signer.PublicKey())),
		pubLine+"\n",
		"public line must be the authorized_keys form of the private key's public half")
}

// TestHost_SSHOverrides pins the escape-hatch contract: setting ANY of
// the PODLAND_TEST_SSH_* vars opts out of the container lifecycle, and
// the opt-out is all-or-nothing — a partial set is a configuration
// error naming every missing variable.
func TestHost_SSHOverrides(t *testing.T) {
	clearAll := func(t *testing.T) {
		t.Helper()
		for _, k := range sshOverrideEnvs {
			t.Setenv(k, "")
		}
	}

	t.Run("none set is container mode", func(t *testing.T) {
		clearAll(t)
		assert.False(t, sshOverridesSet(), "no overrides set must mean container lifecycle")
	})

	t.Run("all set resolves the external target", func(t *testing.T) {
		clearAll(t)
		t.Setenv("PODLAND_TEST_SSH_HOST", "pi.lan")
		t.Setenv("PODLAND_TEST_SSH_PORT", "2222")
		t.Setenv("PODLAND_TEST_SSH_USER", "podman")
		t.Setenv("PODLAND_TEST_SSH_SOCKET", "/run/user/1000/podman/podman.sock")
		t.Setenv("PODLAND_TEST_SSH_KEY_PATH", "/keys/id_external")

		require.True(t, sshOverridesSet(), "any override set must mean escape hatch")

		target, identityPath, err := externalTargetFromEnv()
		require.NoError(t, err)
		assert.Equal(t, reachableTarget{
			Host:    "pi.lan",
			Port:    2222,
			SSHUser: "podman",
			Socket:  "/run/user/1000/podman/podman.sock",
			SSHKey:  "",
		}, target)
		assert.Equal(t, "/keys/id_external", identityPath)
	})

	t.Run("partial set names every missing override", func(t *testing.T) {
		clearAll(t)
		t.Setenv("PODLAND_TEST_SSH_HOST", "pi.lan")

		_, _, err := externalTargetFromEnv()
		require.Error(t, err)
		for _, missing := range []string{
			"PODLAND_TEST_SSH_PORT",
			"PODLAND_TEST_SSH_USER",
			"PODLAND_TEST_SSH_SOCKET",
			"PODLAND_TEST_SSH_KEY_PATH",
		} {
			assert.Contains(t, err.Error(), missing, "error must name %s", missing)
		}
	})

	t.Run("non-integer port is rejected", func(t *testing.T) {
		clearAll(t)
		t.Setenv("PODLAND_TEST_SSH_HOST", "pi.lan")
		t.Setenv("PODLAND_TEST_SSH_PORT", "ssh")
		t.Setenv("PODLAND_TEST_SSH_USER", "podman")
		t.Setenv("PODLAND_TEST_SSH_SOCKET", "/sock")
		t.Setenv("PODLAND_TEST_SSH_KEY_PATH", "/keys/id_external")

		_, _, err := externalTargetFromEnv()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "PODLAND_TEST_SSH_PORT")
	})
}

// TestHost_CollisionClassification pins the serialized-runs guard: the
// two collision shapes podman emits (name in use, port in use) must be
// recognized from combined output, and ordinary failures must not be.
func TestHost_CollisionClassification(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "container name in use",
			output: "Error: creating container storage: the container name \"podland-it-testhost\" is already in use",
			want:   true,
		},
		{
			name:   "published port in use",
			output: "Error: netavark: bind 127.0.0.1:24422: address already in use",
			want:   true,
		},
		{
			name:   "image missing is not a collision",
			output: "Error: locating image with name \"podland-testhost\": not found",
			want:   false,
		},
		{
			name:   "plain runtime failure is not a collision",
			output: "Error: /dev/fuse: no such file or directory",
			want:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isSerializedRunsCollision(tc.output))
		})
	}
}

// TestHost_KnownHostsMismatchDetection pins the mismatch classifier:
// a changed host key is a deterministic failure (retrying cannot help),
// so the probe must recognise podman's phrasing of it to fail fast with
// the fix hint instead of burning its whole retry budget.
func TestHost_KnownHostsMismatchDetection(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "changed host key",
			output: "ssh: handshake failed: knownhosts: key mismatch from [127.0.0.1]:24422",
			want:   true,
		},
		{
			name:   "ssh host key verification failed",
			output: "Host key verification failed.",
			want:   true,
		},
		{
			name:   "ssh remote identification changed",
			output: "WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED! Offending key in /home/user/.ssh/known_hosts:3",
			want:   true,
		},
		{
			name:   "refused is transient, not a mismatch",
			output: "ssh: dial tcp 127.0.0.1:24422: connect: connection refused",
			want:   false,
		},
		{
			name:   "clean output",
			output: "CONTAINER ID  IMAGE  COMMAND",
			want:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isKnownHostsMismatch(tc.output))
		})
	}
}

// TestHost_WaitForSSHDPort pins the port-readiness helper against a
// real listener: it returns once something accepts, and errors once the
// budget expires with nothing listening.
func TestHost_WaitForSSHDPort(t *testing.T) {
	t.Run("accepting port succeeds immediately", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer ln.Close()

		err = waitForSSHDPort("127.0.0.1", portOf(t, ln), 2*time.Second)
		assert.NoError(t, err, "an accepting listener must satisfy the wait")
	})

	t.Run("nothing listening exhausts the budget", func(t *testing.T) {
		// Grab a port that is guaranteed free by binding and closing.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		port := portOf(t, ln)
		ln.Close()

		start := time.Now()
		err = waitForSSHDPort("127.0.0.1", port, 300*time.Millisecond)
		elapsed := time.Since(start)

		assert.Error(t, err, "a closed port must fail the wait")
		assert.Contains(t, err.Error(), "sshd")
		assert.LessOrEqual(t, elapsed, 3*time.Second,
			"the wait must honour its budget instead of hanging")
	})
}

// TestHost_TCPPortAvailable pins the port pre-check used for the
// serialized-runs guard: busy means false, free means true.
func TestHost_TCPPortAvailable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	assert.False(t, tcpPortAvailable("127.0.0.1", portOf(t, ln)),
		"a bound port must not report available")

	freePort := portOf(t, ln)
	ln.Close()
	assert.True(t, tcpPortAvailable("127.0.0.1", freePort),
		"a released port must report available")
}

// portOf extracts a listener's port, failing the test if it cannot.
func portOf(t *testing.T, ln net.Listener) int {
	t.Helper()
	addr, ok := ln.Addr().(*net.TCPAddr)
	require.True(t, ok, "listener must be a TCP listener")
	return addr.Port
}
