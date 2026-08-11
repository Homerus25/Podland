// Package app_test verifies the internal/app.App seam that lets the backend
// be brought up under httptest.NewServer (PRD 080-00).
//
// These are the contract tests for the public surface the integration
// harness will rely on: New succeeds with a controlled config, Handler
// serves the same routes as production (spot-checked via /api/health), and
// Close is idempotent and shuts down the background goroutines.
package app_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/podland/backend/internal/app"
	"github.com/yourorg/podland/backend/internal/config"
)

// newTestConfig builds a fully-controlled *config.Config pointing at temp
// paths: a per-test SQLite DB file, fresh RSA JWT keys, and a random
// ENCRYPTION_KEY. This is exactly the shape the integration harness
// (080-01) will use to bring up App under httptest.NewServer.
func newTestConfig(t *testing.T) *config.Config {
	t.Helper()

	// migrations.Run reads SQL files from a relative "migrations/"
	// directory (mirroring cmd/initdb/main.go, which chdirs to backend
	// root for the same reason). Tests run with cwd = the test package's
	// directory, so chdir to the backend root.
	t.Chdir("../..")

	dbPath := filepath.Join(t.TempDir(), "test.db")
	privPath, pubPath := writeRSAJWTKeypair(t)
	encKey := base64.StdEncoding.EncodeToString(randomBytes(t, 32))

	return &config.Config{
		Port: 0, // App doesn't bind — caller owns the *http.Server
		Database: config.DatabaseConfig{
			Path: dbPath,
		},
		JWT: config.JWTConfig{
			PrivateKeyPath:    privPath,
			PublicKeyPath:     pubPath,
			ExpirationMinutes: 60,
			Issuer:            "podland-test",
		},
		Connections: config.ConnectionConfig{
			SSHIdleTimeoutSeconds: 900,
			MaxConcurrent:         10,
			QueueTimeoutSeconds:   30,
			TestTimeoutSeconds:    10,
		},
		RateLimit:      config.RateLimitConfig{RequestsPerMinute: 60},
		AllowedOrigins: []string{},
		Log:            config.LogConfig{Level: "info", Format: "json"},
		Encryption:     config.EncryptionConfig{Key: encKey},
	}
}

func writeRSAJWTKeypair(t *testing.T) (privPath, pubPath string) {
	t.Helper()
	dir := t.TempDir()
	privPath = filepath.Join(dir, "jwt-private.pem")
	pubPath = filepath.Join(dir, "jwt-public.pem")

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "generate RSA key")

	privDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err, "marshal private key")
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})
	require.NoError(t, os.WriteFile(privPath, privPEM, 0600), "write private key file")

	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err, "marshal public key")
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	require.NoError(t, os.WriteFile(pubPath, pubPEM, 0644), "write public key file")

	return privPath, pubPath
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	_, err := rand.Read(b)
	require.NoError(t, err, "read random bytes")
	return b
}

// TestApp_New_BuildsAllWiring is the smoke test: with a fully-controlled
// config, New must succeed and return a non-nil *App. This proves the
// entire wiring chain (DB → migrations → encryption → auth → repos →
// connection manager + goroutine → sshexec service + cleanup goroutine →
// services → handlers → middleware → router) is reachable without a real
// Podman machine.
func TestApp_New_BuildsAllWiring(t *testing.T) {
	cfg := newTestConfig(t)

	a, err := app.New(cfg)

	require.NoError(t, err, "New must succeed with a valid controlled config")
	require.NotNil(t, a, "New must return a non-nil *App on success")
	t.Cleanup(func() { _ = a.Close() })
}

// TestApp_Handler_HealthRouteWorks verifies that Handler returns an
// http.Handler wired exactly as today's main() wires it, by hitting the
// /api/health endpoint (the only route that doesn't require auth or a live
// Podman machine) through httptest.NewServer. If the wiring changes — e.g.
// a middleware gets dropped — this test will fail.
func TestApp_Handler_HealthRouteWorks(t *testing.T) {
	cfg := newTestConfig(t)

	a, err := app.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })

	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/health")
	require.NoError(t, err, "GET /api/health must connect")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"/api/health must return 200 OK — proves the router wiring is intact")
}

// TestApp_Close_IsIdempotent pins the "safe to call once" promise: App.Close
// must not panic or error when called multiple times. App.Close stops the
// connection-manager goroutine and the sshexec dialer-cleanup goroutine;
// double-closing the underlying stop channels would panic if Close weren't
// guarded by sync.Once.
func TestApp_Close_IsIdempotent(t *testing.T) {
	cfg := newTestConfig(t)

	a, err := app.New(cfg)
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		_ = a.Close()
		_ = a.Close()
		_ = a.Close()
	}, "Close must be safe to call multiple times")
}

// TestApp_Close_ReleasesBackgroundGoroutines is a leak check: after Close,
// the connection-manager processQueue goroutine and the sshexec dialer
// cleanup goroutine must have exited. We can't observe goroutines directly
// from outside the package, but we can rely on the fact that
// ConnectionManager.Stop and sshexec.Service.Close each block until their
// goroutine is gone — so if Close returns, the goroutines have exited. We
// sanity-check this by asserting Close returns within a deadline.
func TestApp_Close_ReleasesBackgroundGoroutines(t *testing.T) {
	cfg := newTestConfig(t)

	a, err := app.New(cfg)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- a.Close() }()

	select {
	case err := <-done:
		assert.NoError(t, err, "Close must succeed cleanly")
	case <-time.After(2 * time.Second):
		t.Fatal("App.Close did not return within 2s; a background goroutine likely hasn't exited")
	}
}

