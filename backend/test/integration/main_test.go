//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/app"
	"github.com/Homerus25/podland/backend/internal/config"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/google/uuid"
)

// suite is the shared fixture built by TestMain and consumed by every
// Test_* function in this package. Tests reach it via the package-level
// `suite` variable; SkipIfUnavailable gates entry when TestMain couldn't
// build the full pipeline.
var suite *Suite

// logf is the suite's printer — used for setup/teardown diagnostics so
// they appear in `go test -v` output interleaved with test progress.
// Tests themselves use t.Logf.
var logf = log.Printf

// TestMain owns the entire test-host pipeline:
//
//  1. Resolve the test host. By default that is the container test host
//     (see testhost.go): ensure the image exists (fail fast naming the
//     make target), start the privileged test-host container under the
//     serialized-runs contract (fixed name + fixed loopback port), and
//     inject a freshly generated ed25519 key as core's authorized key.
//     When any PODLAND_TEST_SSH_* override is set, the container
//     lifecycle is skipped entirely and the suite targets that
//     externally managed host (the escape hatch).
//  2. Register a real `podman system connection` under
//     PODLAND_TEST_CONNECTION_NAME pointing at the target, so the
//     `podman --connection <name>` test helpers work unchanged.
//  3. Readiness: wait for the published sshd port (container mode),
//     then retry a real podman-over-SSH probe with exponential backoff.
//  4. Bring the backend up in-process: app.New(testCfg) wrapped in
//     httptest.NewServer. Temp SQLite file, temp JWT PEMs, random
//     ENCRYPTION_KEY — fully isolated from the dev DB.
//  5. Seed one user via the seeding seam (bcrypt) and exercise the real
//     POST /api/auth/login; capture the token from the JSON body.
//  6. Register the test host once via POST /api/machines — the same
//     public path any machine takes; the backend never learns the
//     container exists.
//  7. Run the tests.
//  8. Teardown: httptest server close → app.Close → remove temp dirs →
//     podman system connection rm → podman rm -f the container. The
//     persistent volumes are deliberately retained (image cache and
//     stable SSH host keys). A SIGINT/SIGTERM handler routes through
//     the same teardown path so Ctrl-C still cleans up.
//
// Setup failure semantics: a missing podman binary is the one clean
// skip (tests t.Skip, exit 0). Every other setup failure — missing
// image, serialized-runs collision, cold socket, backend that won't
// boot — is a hard failure: TestMain prints the error, tears down
// whatever it already owns, and exits 1 without running the tests. A
// broken environment must not masquerade as a green skipped run.
func TestMain(m *testing.M) {
	os.Exit(run(m))
}

// run is the body of TestMain, extracted so a deferred teardown always
// runs — even on early-abort setup paths. (os.Exit does not run
// defers, so calling it from inside the body would skip teardown; the
// only os.Exit is the one in TestMain itself, plus the signal-handler
// path which routes through the same teardownOnce.)
//
// Teardown is collected incrementally as setup succeeds; the deferred
// teardownOnce guarantees it runs even if a later step fails. The
// suite's tempRoot has its own safety net inside Suite.teardown for
// paths that abort before the "remove temp dirs" cleanup is registered.
func run(m *testing.M) int {
	// Build a Suite with a run-scoped identity up front so even an
	// early-aborting run has a valid RunID for any diagnostic output.
	suite = &Suite{
		RunID: newRunID(),
	}

	var teardownOnce sync.Once
	teardown := func() {
		teardownOnce.Do(func() { suite.teardown() })
	}
	defer teardown()

	// bail is the clean-skip path: it records a skip reason, logs it,
	// and returns the tests' exit code so every Test_* t.Skips cleanly
	// and `go test` exits 0. It exists for exactly one case now — a
	// host without the podman binary.
	bail := func(step string, err error) int {
		suite.skipReason = fmt.Sprintf("%s: %v", step, err)
		logf("integration: %s", suite.skipReason)
		return m.Run()
	}

	// failFast is the hard-failure path: print, tear down whatever was
	// already acquired, and exit 1 WITHOUT running the tests. Anything
	// the user must fix (image, collisions, unreachable host, broken
	// backend) fails here rather than hiding behind a green skip.
	failFast := func(step string, err error) int {
		logf("integration: FATAL: %s: %v", step, err)
		return 1
	}

	// migrations.Run reads from "migrations/" relative to cwd; tests
	// run with cwd = test/integration, so chdir to the backend root.
	// This mirrors what internal/app/app_test.go does with t.Chdir.
	backendRoot, err := resolveBackendRoot()
	if err != nil {
		return failFast("could not locate backend root", err)
	}
	if err := os.Chdir(backendRoot); err != nil {
		return failFast("could not chdir to backend root", err)
	}

	// --- Step 1: podman binary (the clean-skip contract) ------------
	if _, err := exec.LookPath("podman"); err != nil {
		return bail("podman binary", errPodmanNotFound)
	}

	// One run-scoped temp dir backs everything the run creates on
	// disk: JWT PEMs, the SQLite DB, and (container mode) the per-run
	// SSH identity. It vanishes at teardown.
	tempRoot, err := os.MkdirTemp("", "podland-it-*")
	if err != nil {
		return failFast("create run temp dir", err)
	}
	suite.tempRoot = tempRoot

	// Signal handler — installed BEFORE any host state is acquired, so
	// a Ctrl-C at any point after this (including mid container-boot)
	// tears down whatever has been registered so far instead of leaking
	// it. Runs before the podman-binary bail above are unreachable, so
	// an unrelated Ctrl-C on a no-op host never fires teardown. The
	// handler runs teardown (once) and exits 1, which `go test` reports
	// as a failed run.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logf("integration: received %s, tearing down", sig)
		teardown()
		os.Exit(1)
	}()

	// --- Step 2: resolve the test host ------------------------------
	var target reachableTarget
	var identityPath string
	containerMode := !sshOverridesSet()

	if containerMode {
		if err := ensureTesthostImage(); err != nil {
			return failFast("ensure test host image", err)
		}
		if err := startTesthostContainer(primaryTesthost); err != nil {
			return failFast("start test host container", err)
		}
		// Registered FIRST so the LIFO teardown stack removes the
		// container LAST, after the connection pointing into it is
		// gone. Volumes are retained by removeTesthostContainer.
		suite.RegisterCleanup("podman rm -f test host", func() error {
			return removeTesthostContainer(primaryTesthost)
		})

		// Per-run identity: fresh ed25519 keypair, public half
		// injected into the container, private half living only in
		// tempRoot — nothing private persists beyond this run.
		keyPath, pubLine, err := generateRunKeypair(tempRoot)
		if err != nil {
			return failFast("generate run keypair", err)
		}
		identityPath = keyPath
		if err := injectAuthorizedKey(primaryTesthost.container, pubLine); err != nil {
			return failFast("inject authorized key", err)
		}
		target = reachableTarget{
			Host:    testhostHost,
			Port:    primaryTesthost.port,
			SSHUser: testhostSSHUser,
			Socket:  testhostSocket,
		}
		logf("integration: test host container %q up; identity injected", primaryTesthost.container)
	} else {
		target, identityPath, err = externalTargetFromEnv()
		if err != nil {
			return failFast("resolve PODLAND_TEST_SSH_* overrides", err)
		}
		logf("integration: PODLAND_TEST_SSH_* overrides set — targeting external host %s:%d, container lifecycle skipped",
			target.Host, target.Port)
	}

	// --- Step 3: register the podman connection ---------------------
	// The connection is what the CLI test helpers and the readiness
	// probe dial through; the backend itself only ever sees the
	// machine registered via the public API (step 6).
	connectionName := envOr("PODLAND_TEST_CONNECTION_NAME", testhostConnectionDefault)
	suite.MachineName = connectionName
	if err := registerPodmanConnection(connectionName, identityPath, target); err != nil {
		return failFast("register podman system connection", err)
	}
	suite.RegisterCleanup("podman system connection rm", func() error {
		return removePodmanConnection(connectionName)
	})

	// --- Step 4: readiness ------------------------------------------
	if containerMode {
		if err := waitForSSHDPort(target.Host, target.Port, 30*time.Second); err != nil {
			return failFast("wait for sshd port", err)
		}
	}
	if err := probePodmanConnection(primaryTesthost, connectionName, 60*time.Second); err != nil {
		return failFast("probe podman connection", err)
	}
	suite.RegisterCleanup("ssh-keygen -R [127.0.0.1]:24422",
		func() error {
			_, err := exec.CommandContext(context.Background(), "ssh-keygen", "-R", "[127.0.0.1]:24422").CombinedOutput()
			return err
		})
	logf("integration: target ready: host=%s port=%d user=%s socket=%s connection=%s",
		target.Host, target.Port, target.SSHUser, target.Socket, connectionName)

	// --- Step 5: backend in-process ---------------------------------
	tempCfg, err := buildTestConfig(tempRoot)
	if err != nil {
		return failFast("buildTestConfig", err)
	}
	application, err := app.New(tempCfg)
	if err != nil {
		return failFast("app.New", err)
	}
	suite.application = application
	suite.DBPath = tempCfg.Database.Path
	srv := httptest.NewServer(application.Handler())
	suite.ServerURL = srv.URL
	logf("integration: backend up at %s", suite.ServerURL)

	// Register the backend cleanups in REVERSE of the order they should
	// run, because the cleanup stack is LIFO (mirrors t.Cleanup). The
	// spec-required teardown order is:
	//
	//	httptest server close → app.Close → remove temp dirs
	//	→ podman system connection rm → podman rm -f test host
	//
	// (server close first so in-flight requests can't reach a
	// half-torn-down app; app.Close before RemoveAll so SQLite file
	// handles are released before the files vanish; local temp cleanup
	// before the slower remote teardowns). The container and connection
	// cleanups were registered earlier in setup, so they run last as
	// required.
	suite.RegisterCleanup("remove temp dirs", func() error {
		return os.RemoveAll(tempRoot)
	})
	suite.RegisterCleanup("app.Close", func() error {
		return application.Close()
	})
	suite.RegisterCleanup("httptest server close", func() error {
		srv.Close()
		return nil
	})

	// --- Step 6: seed user + real login -----------------------------
	username := "it-" + suite.RunID + "-admin"
	password := "it-" + suite.RunID + "-pw-" + uuid.New().String()[:8]
	ownerUserID, err := application.SeedUser(username, password)
	if err != nil {
		return failFast("SeedUser", err)
	}
	suite.OwnerUserID = ownerUserID

	token, err := realLogin(suite.ServerURL, username, password)
	if err != nil {
		return failFast("realLogin", err)
	}
	suite.Token = token
	logf("integration: logged in as %q (id=%s)", username, ownerUserID)

	// Seed a second user — never logged in, but its ID is needed by
	// downstream slices (080-04 machines CRUD/access) as the grantee
	// target for POST /api/machines/{id}/access. The user exists only
	// to satisfy the machine_access(user_id) FK; no token is captured
	// because the ACs exercise grant/revoke as the owner, not as the
	// grantee.
	otherUsername := "it-" + suite.RunID + "-other"
	otherUserID, err := application.SeedUser(otherUsername, password)
	if err != nil {
		return failFast("SeedUser (other)", err)
	}
	suite.OtherUserID = otherUserID

	// --- Step 7: register the test host as a machine -----------------
	keyMaterial, err := os.ReadFile(identityPath)
	if err != nil {
		return failFast(fmt.Sprintf("read SSH identity %s", identityPath), err)
	}

	machineID, err := registerMachine(suite.ServerURL, token, machineRegistration{
		Name:    "it-" + suite.RunID + "-" + connectionName,
		Host:    target.Host,
		Port:    target.Port,
		SSHUser: target.SSHUser,
		Socket:  target.Socket,
		SSHKey:  string(keyMaterial),
	})
	if err != nil {
		return failFast("registerMachine", err)
	}
	suite.MachineID = machineID
	// Stash the reachable target so downstream slices can register
	// additional throwaway machines against the same host via
	// RegisterReachableMachine. Required since 080-05: a throwaway
	// pointing at an unreachable host can no longer be registered.
	goodTarget := target
	goodTarget.SSHKey = string(keyMaterial)
	suite.goodMachineRegistration = goodTarget
	logf("integration: machine registered: id=%s", machineID)

	// --- Step 8: run the tests ---------------------------------------
	// Teardown runs via the deferred teardownOnce above.
	return m.Run()
}

// teardown is the suite-level cleanup. It runs the cleanup stack (LIFO)
// and prints any failures. The tempRoot sweep after the stack is a
// safety net for setup paths that aborted before the "remove temp dirs"
// cleanup was registered; on the normal path the stack already removed
// it and the second RemoveAll is a no-op. Safe to call exactly once per
// Suite (guarded by teardownOnce in TestMain).
func (s *Suite) teardown() {
	s.runCleanups(logf)
	if s.tempRoot != "" {
		if err := os.RemoveAll(s.tempRoot); err != nil {
			logf("integration cleanup \"remove temp dirs\" failed: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Podman binary sentinel
// ---------------------------------------------------------------------------

// errPodmanNotFound is the sentinel used when the podman binary is not
// on PATH. TestMain maps this to a clean skip rather than a fatal
// failure, so `go test -tags=integration` on a machine without podman
// exits 0.
var errPodmanNotFound = errors.New("podman binary not found on PATH")

// ---------------------------------------------------------------------------
// Step 5: build test config (temp SQLite, temp JWT PEMs, random ENC key)
// ---------------------------------------------------------------------------

// buildTestConfig produces a fully-controlled *config.Config inside the
// given run temp root (owned by TestMain and removed at teardown). JWT
// keys are generated via internal/testutils.CreateTestJWTKeys (the
// PRD-mandated reuse — single source of truth for test keygen).
func buildTestConfig(root string) (*config.Config, error) {
	dbPath := filepath.Join(root, "test.db")
	privPath := filepath.Join(root, "jwt-private.pem")
	pubPath := filepath.Join(root, "jwt-public.pem")
	if err := writeJWTKeypair(privPath, pubPath); err != nil {
		return nil, fmt.Errorf("write JWT keypair: %w", err)
	}

	encKey, err := randomBase64(32)
	if err != nil {
		return nil, fmt.Errorf("generate encryption key: %w", err)
	}

	cfg := &config.Config{
		Port: 0, // App doesn't bind; httptest.NewServer does.
		Volumes: config.VolumeConfig{
			MaxUploadBytes: 2 << 30,
		},
		Database: config.DatabaseConfig{
			Path: dbPath,
		},
		JWT: config.JWTConfig{
			PrivateKeyPath:    privPath,
			PublicKeyPath:     pubPath,
			ExpirationMinutes: 60,
			Issuer:            "podland-it",
		},
		Session: config.SessionConfig{TTLDays: 30},
		Connections: config.ConnectionConfig{
			SSHIdleTimeoutSeconds: 900,
			MaxConcurrent:         10,
			TestTimeoutSeconds:    10,
		},
		// The rate limiters are production abuse guards, not the system
		// under test for most slices: the per-user limiter sees the
		// suite authenticate as ONE user and legitimately issue
		// hundreds of requests per minute (state-transition polls alone
		// fire ~2/s for up to 30s each), the per-IP auth limiter sees
		// every login/refresh in the suite arrive from 127.0.0.1, and
		// the credential-failure budget would otherwise lock any
		// username whose negative assertions pile up past five. All
		// three ceilings are raised out of the way — the production
		// defaults would cause spurious 429s that poison every test
		// after the first poll-heavy slice. The auth limiters get
		// their own dedicated coverage from separate low-limit app
		// instances in ratelimit_auth_test.go.
		RateLimit:      config.RateLimitConfig{RequestsPerMinute: 100000, AuthRequestsPerMinute: 100000, FailureBudget: 100000, FailureWindowMinutes: 15},
		AllowedOrigins: []string{},
		Log:            config.LogConfig{Level: "info", Format: "json"},
		Encryption:     config.EncryptionConfig{Key: encKey},
	}
	return cfg, nil
}

// writeJWTKeypair generates a fresh RSA keypair via the shared
// internal/testutils helper and writes the PEMs to the given paths.
// Reuses CreateTestJWTKeys rather than re-implementing keygen so the
// suite stays in lock-step with the rest of the test corpus.
func writeJWTKeypair(privPath, pubPath string) error {
	privPEM, pubPEM, err := testutils.CreateTestJWTKeys()
	if err != nil {
		return err
	}
	if err := os.WriteFile(privPath, []byte(privPEM), 0600); err != nil {
		return err
	}
	return os.WriteFile(pubPath, []byte(pubPEM), 0644)
}

// randomBase64 returns n cryptographically-random bytes encoded as
// base64 (suitable for an ENCRYPTION_KEY value).
func randomBase64(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// ---------------------------------------------------------------------------
// Step 6: seed user + real login
// ---------------------------------------------------------------------------

// (Seeding is delegated to App.SeedUser so the integration package
// doesn't reach into App's *sql.DB. The helper lives in internal/app
// alongside the DB it writes to; see app.go.)

// loginResponse mirrors the JSON body of POST /api/auth/login. Only the
// field we consume is named.
type loginResponse struct {
	Token string `json:"token"`
}

// realLogin exercises POST /api/auth/login against the in-process
// server and returns the bearer token from the JSON body. This is the
// same path the production frontend uses; we deliberately do NOT mint
// a token directly via auth.GenerateToken, because doing so would
// bypass the HTTP layer the suite is meant to exercise.
func realLogin(baseURL, username, password string) (string, error) {
	body, err := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})
	if err != nil {
		return "", err
	}
	resp, err := http.Post(baseURL+"/api/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("POST /api/auth/login: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("login returned %d: %s", resp.StatusCode, string(raw))
	}
	var lr loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return "", fmt.Errorf("decode login response: %w", err)
	}
	if lr.Token == "" {
		return "", fmt.Errorf("login response missing token")
	}
	return lr.Token, nil
}

// ---------------------------------------------------------------------------
// Step 7: register machine
// ---------------------------------------------------------------------------

// machineRegistration is the request body for POST /api/machines,
// flattened for the test's convenience. registerMachine converts it to
// the JSON the handler expects.
type machineRegistration struct {
	Name    string
	Host    string
	Port    int
	SSHUser string
	Socket  string
	SSHKey  string
}

// registerMachine POSTs to /api/machines as the seeded user and returns
// the new machine's UUID from the response body. This is the canonical
// registration path — encryption-at-rest, owner-access grant, the lot —
// exercised exactly once per run for the shared machine, and once per
// throwaway via RegisterReachableMachine.
func registerMachine(baseURL, token string, req machineRegistration) (string, error) {
	body, err := json.Marshal(map[string]any{
		"name":             req.Name,
		"hostname":         req.Host,
		"port":             req.Port,
		"sshUsername":      req.SSHUser,
		"podmanSocketPath": req.Socket,
		"sshPrivateKey":    req.SSHKey,
	})
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequest(http.MethodPost, baseURL+"/api/machines/", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("POST /api/machines: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("register machine returned %d: %s", resp.StatusCode, string(raw))
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", fmt.Errorf("decode register response: %w", err)
	}
	if created.ID == "" {
		return "", fmt.Errorf("register response missing id")
	}
	return created.ID, nil
}

// RegisterReachableMachine registers a SECOND machine pointing at the
// same reachable target as the shared good machine, returning its ID.
// The caller owns cleanup (register a t.Cleanup that DELETEs the ID via
// the API, as the machines slice does).
//
// Why this exists: 080-05 made MachineService.Create probe the target at
// registration, so a throwaway pointing at an unreachable host (the
// original 080-04 bad-host pattern) is now rejected with 400 and never
// persisted. Slices that need a mutable throwaway (PUT / access /
// DELETE) must therefore register against the reachable target. This
// helper reuses the coordinates TestMain proved reachable so the
// throwaway sails through the connection probe.
//
// The name should include suite.RunID so the suite sweep can find any
// leftover if the caller's own t.Cleanup does not fire.
func (s *Suite) RegisterReachableMachine(t *testing.T, name string) string {
	t.Helper()
	id, err := registerMachine(s.ServerURL, s.Token, machineRegistration{
		Name:    name,
		Host:    s.goodMachineRegistration.Host,
		Port:    s.goodMachineRegistration.Port,
		SSHUser: s.goodMachineRegistration.SSHUser,
		Socket:  s.goodMachineRegistration.Socket,
		SSHKey:  s.goodMachineRegistration.SSHKey,
	})
	if err != nil {
		t.Fatalf("register reachable throwaway %q against the suite's test host: %v", name, err)
	}
	if id == "" {
		t.Fatalf("register reachable throwaway %q returned an empty id", name)
	}
	return id
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// resolveBackendRoot walks up from the test source directory to find
// the directory containing go.mod for the backend module. Required
// because migrations.Run reads from a relative "migrations/" path.
func resolveBackendRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no go.mod found walking up from %s", cwd)
}

// envOr returns the value of the env var if set and non-empty, else
// def. Used for the handful of knobs the suite respects.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
