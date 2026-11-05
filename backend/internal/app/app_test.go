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
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/app"
	"github.com/Homerus25/podland/backend/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestConfig builds a fully-controlled *config.Config pointing at temp
// paths: a per-test SQLite DB file, fresh RSA JWT keys, and a random
// ENCRYPTION_KEY. This is exactly the shape the integration harness
// (080-01) will use to bring up App under httptest.NewServer.
func newTestConfig(t *testing.T) *config.Config {
	t.Helper()

	// migrations.Run reads SQL files from a "migrations/" directory
	// relative to cwd. Tests run with cwd = the test package's
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
		Session: config.SessionConfig{TTLDays: 30},
		Connections: config.ConnectionConfig{
			SSHIdleTimeoutSeconds: 900,
			MaxConcurrent:         10,
			TestTimeoutSeconds:    10,
		},
		// The auth budgets are raised well above their production
		// defaults because every app test's boot probe (GET
		// /auth/status) draws from the shared per-IP auth group, and
		// failed-login assertions would otherwise spend the shared
		// credential-failure budget — dozens of requests in one suite
		// would otherwise 429 on the production budgets. The
		// rate-limit behavior itself is pinned in setup_test.go, which
		// overrides these fields per test.
		RateLimit:      config.RateLimitConfig{RequestsPerMinute: 60, AuthRequestsPerMinute: 1000, FailureBudget: 1000, FailureWindowMinutes: 15},
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
// Podman host.
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
// Podman host) through httptest.NewServer. If the wiring changes — e.g.
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

// TestApp_Handler_UsersRouteIsProtected pins the 090-01 wiring: POST
// /api/users must exist behind the JWT middleware, so an anonymous
// caller gets 401 (not 404). If the route is dropped or registered
// outside the protected group, this fails.
func TestApp_Handler_UsersRouteIsProtected(t *testing.T) {
	cfg := newTestConfig(t)

	a, err := app.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })

	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/api/users", "application/json",
		strings.NewReader(`{"username":"someone","password":"secret"}`))
	require.NoError(t, err, "POST /api/users must connect")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"POST /api/users must sit behind the JWT middleware (401, not 404)")
}

// TestApp_Handler_SPARidesUnderAPI pins the mounting contract of the
// embedded frontend (issue 154): the catch-all SPA handler is mounted
// after /api, so real API routes keep precedence, unknown /api paths
// still answer the API's 404 (never the SPA shell), and every other
// path — client-side routes included — falls back to index.html with
// 200, mirroring the Caddy try_files contract it replaces.
func TestApp_Handler_SPARidesUnderAPI(t *testing.T) {
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
		"real API routes must keep precedence over the SPA catch-all")

	apiMiss, err := http.Get(srv.URL + "/api/nonexistent")
	require.NoError(t, err, "GET /api/nonexistent must connect")
	defer apiMiss.Body.Close()
	assert.Equal(t, http.StatusNotFound, apiMiss.StatusCode,
		"unknown /api paths must stay API-shaped (404), never fall back to the SPA")

	spa, err := http.Get(srv.URL + "/hosts/xyz")
	require.NoError(t, err, "GET /hosts/xyz must connect")
	defer spa.Body.Close()
	assert.Equal(t, http.StatusOK, spa.StatusCode,
		"non-API client-side routes fall back to index.html with 200")
	assert.Contains(t, spa.Header.Get("Content-Type"), "text/html")
}

// TestApp_Handler_ServeMuxCanonicalization pins the URL-shape promises of
// the ServeMux router (issue #451): the slashless /api/hosts serves (it
// reaches the JWT middleware: 401, not a route miss), while the slashed
// /api/hosts/ — the one form that would otherwise reach
// RequireHostAccess with an empty {id} — answers the API's JSON 404
// envelope (code NOT_FOUND), as does every other unknown /api path; none
// of them ever falls back to the SPA shell.
func TestApp_Handler_ServeMuxCanonicalization(t *testing.T) {
	cfg := newTestConfig(t)

	a, err := app.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })

	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)

	slashless, err := http.Get(srv.URL + "/api/hosts")
	require.NoError(t, err, "GET /api/hosts must connect")
	defer slashless.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, slashless.StatusCode,
		"the slashless form serves: it must reach the JWT middleware (401), not miss the route (404)")

	for _, target := range []string{"/api/hosts/", "/api/nonexistent"} {
		resp, err := http.Get(srv.URL + target)
		require.NoError(t, err, "GET %s must connect", target)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode,
			"GET %s: the slashed/unknown form must answer 404", target)

		var body struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body),
			"GET %s: the 404 must be the JSON error envelope, never the SPA shell", target)
		assert.Equal(t, "NOT_FOUND", body.Error.Code)
		assert.Equal(t, "Route not found", body.Error.Message)
	}
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

// TestApp_Handler_SecretsRouteIsProtected pins the 160-01 wiring: GET
// /api/hosts/{id}/secrets must exist behind the JWT middleware, so an
// anonymous caller gets 401 (not 404). If the route is dropped or
// registered outside the protected group, this fails. The deeper gates
// (404 unknown host / 403 no-access via RequireHostAccess, which
// runs only for authenticated callers) are covered by the domain-level
// wiring tests in internal/domain/secrets.
func TestApp_Handler_SecretsRouteIsProtected(t *testing.T) {
	cfg := newTestConfig(t)

	a, err := app.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })

	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/hosts/some-host/secrets")
	require.NoError(t, err, "GET /api/hosts/{id}/secrets must connect")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"GET /api/hosts/{id}/secrets must sit behind the JWT middleware (401, not 404)")
}

func TestApp_Handler_NetworksRouteIsProtected(t *testing.T) {
	cfg := newTestConfig(t)

	a, err := app.New(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })

	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/hosts/some-host/networks")
	require.NoError(t, err, "GET /api/hosts/{id}/networks must connect")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"GET /api/hosts/{id}/networks must sit behind the JWT middleware (401, not 404)")
}
