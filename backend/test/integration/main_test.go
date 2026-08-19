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
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yourorg/podland/backend/internal/app"
	"github.com/yourorg/podland/backend/internal/config"
	"github.com/yourorg/podland/backend/internal/testutils"
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

// TestMain owns the entire real-machine pipeline:
//
//  1. Ensure the podman machine is running (idempotent; skips the suite
//     if the binary or the machine is unavailable).
//  2. Discover the SSH/socket/identity coordinates from
//     `podman system connection list`, with per-field env overrides.
//  3. Bring the backend up in-process: app.New(testCfg) wrapped in
//     httptest.NewServer. Temp SQLite file, temp JWT PEMs, random
//     ENCRYPTION_KEY — fully isolated from the dev DB.
//  4. Seed one user (bcrypt, mirroring cmd/initdb) and exercise the real
//     POST /api/auth/login; capture the token from the JSON body.
//  5. Register the discovered machine once via POST /api/machines;
//     share the returned ID across all tests.
//  6. Readiness: poll `podman machine inspect` until State==running,
//     then retry a real podman-over-SSH probe (`podman --connection …
//     ps`) with exponential backoff. A VM marked "running" is not the
//     same as a socket ready to accept connections.
//  7. Run the tests.
//  8. Teardown: httptest server close → app.Close → temp files →
//     podman machine stop. A SIGINT/SIGTERM handler routes through the
//     same teardown path so Ctrl-C still stops the VM.
//
// If any setup step fails fatally (e.g. backend won't boot), TestMain
// prints the error, sets suite.skipReason, and lets the tests run so
// they t.Skip cleanly rather than reporting a panic in TestMain.
func TestMain(m *testing.M) {
	os.Exit(run(m))
}

// run is the body of TestMain, extracted so a deferred teardown always
// runs — even on early-abort setup paths. (os.Exit does not run
// defers, so calling it from inside the body would skip teardown; the
// only os.Exit is the one in TestMain itself.)
//
// On any setup failure, run records a skipReason on the suite and
// returns m.Run() so every test t.Skips cleanly. Teardown — registered
// incrementally as setup steps succeed — still fires via the defer.
func run(m *testing.M) int {
	// Build a Suite with a run-scoped identity up front so even an
	// early-aborting run has a valid RunID for any diagnostic output.
	suite = &Suite{
		RunID: newRunID(),
	}

	// Teardown is collected incrementally as setup succeeds; this defer
	// guarantees it runs even if a later setup step fails. The
	// signal-handler path runs teardown through teardownOnce too, so a
	// Ctrl-C mid-setup still stops the VM if it had been started.
	var teardownOnce sync.Once
	teardown := func() {
		teardownOnce.Do(func() { suite.teardown() })
	}
	defer teardown()

	// bail records a skip reason, logs it, and returns the tests' exit
	// code so every Test_* t.Skips cleanly. Using it keeps the setup
	// steps linear instead of repeating the same 3-line block nine
	// times. (It returns the m.Run() code rather than calling os.Exit
	// so the deferred teardown still fires.)
	bail := func(step string, err error) int {
		suite.skipReason = fmt.Sprintf("%s: %v", step, err)
		logf("integration: %s", suite.skipReason)
		return m.Run()
	}

	// migrations.Run reads from "migrations/" relative to cwd; tests
	// run with cwd = test/integration, so chdir to the backend root.
	// This mirrors what internal/app/app_test.go does with t.Chdir.
	backendRoot, err := resolveBackendRoot()
	if err != nil {
		return bail("could not locate backend root", err)
	}
	if err := os.Chdir(backendRoot); err != nil {
		return bail("could not chdir to backend root", err)
	}

	// --- Step 1: ensure machine -------------------------------------
	machineName := envOr("PODLAND_TEST_MACHINE_NAME", "podman-machine-default")
	suite.MachineName = machineName
	started, startErr := ensureMachine(machineName)
	if startErr != nil {
		// Missing podman binary is a clean skip; anything else is a
		// real environment problem we surface loudly.
		if startErr == errPodmanNotFound {
			return bail("ensureMachine", errPodmanNotFound)
		}
		return bail(fmt.Sprintf("ensureMachine(%q)", machineName), startErr)
	}
	// If we started (or observed already-running) the named machine, we
	// own stopping it in teardown. If the named machine doesn't exist
	// (e.g. targeting the remote `pi` connection), started=false and we
	// skip the stop — the user manages lifecycle out-of-band.
	if started {
		suite.RegisterCleanup("podman machine stop", func() error {
			return stopMachine(machineName)
		})
	}

	// Signal handler — installed AFTER we know we're touching a real VM
	// so an unrelated Ctrl-C during a no-op run doesn't fire teardown
	// prematurely. The handler runs teardown (once) and exits 1, which
	// `go test` reports as a failed run.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logf("integration: received %s, tearing down", sig)
		teardown()
		os.Exit(1)
	}()

	// --- Step 2: discover target ------------------------------------
	disc, err := discoverTarget(machineName)
	if err != nil {
		return bail("discoverTarget", err)
	}
	logf("integration: target discovered: host=%s port=%d user=%s socket=%s identity=<redacted>",
		disc.Host, disc.Port, disc.SSHUser, disc.SocketPath)

	// --- Step 3: backend in-process ---------------------------------
	tempCfg, tempDirs, err := buildTestConfig()
	if err != nil {
		return bail("buildTestConfig", err)
	}
	application, err := app.New(tempCfg)
	if err != nil {
		return bail("app.New", err)
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
	//	httptest server close → app.Close → delete temp files → podman machine stop
	//
	// (server close first so in-flight requests can't reach a half-torn
	// down app; app.Close before RemoveAll so SQLite file handles are
	// released before the files vanish; temp files before machine stop
	// because file cleanup is local+fast and machine stop is slow+remote).
	// The machine-stop cleanup is registered earlier in ensureMachine,
	// so it runs last as required.
	suite.RegisterCleanup("remove temp dirs", func() error {
		var firstErr error
		for _, d := range tempDirs {
			if err := os.RemoveAll(d); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	})
	suite.RegisterCleanup("app.Close", func() error {
		return application.Close()
	})
	suite.RegisterCleanup("httptest server close", func() error {
		srv.Close()
		return nil
	})

	// --- Step 4: seed user + real login -----------------------------
	username := "it-" + suite.RunID + "-admin"
	password := "it-" + suite.RunID + "-pw-" + uuid.New().String()[:8]
	ownerUserID, err := application.SeedUser(username, password)
	if err != nil {
		return bail("SeedUser", err)
	}
	suite.OwnerUserID = ownerUserID

	token, err := realLogin(suite.ServerURL, username, password)
	if err != nil {
		return bail("realLogin", err)
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
		return bail("SeedUser (other)", err)
	}
	suite.OtherUserID = otherUserID

	// --- Step 5: register machine once ------------------------------
	keyMaterial, err := os.ReadFile(disc.IdentityPath)
	if err != nil {
		return bail(fmt.Sprintf("read SSH identity %s", disc.IdentityPath), err)
	}

	machineID, err := registerMachine(suite.ServerURL, token, machineRegistration{
		Name:    "it-" + suite.RunID + "-" + machineName,
		Host:    disc.Host,
		Port:    disc.Port,
		SSHUser: disc.SSHUser,
		Socket:  disc.SocketPath,
		SSHKey:  string(keyMaterial),
	})
	if err != nil {
		return bail("registerMachine", err)
	}
	suite.MachineID = machineID
	// Stash the reachable target so downstream slices can register
	// additional throwaway machines against the same host via
	// RegisterReachableMachine. Required since 080-05: a throwaway
	// pointing at an unreachable host can no longer be registered.
	suite.goodMachineRegistration = reachableTarget{
		Host:    disc.Host,
		Port:    disc.Port,
		SSHUser: disc.SSHUser,
		Socket:  disc.SocketPath,
		SSHKey:  string(keyMaterial),
	}
	logf("integration: machine registered: id=%s", machineID)

	// --- Step 6: readiness ------------------------------------------
	if err := waitForReadiness(machineName, disc, started); err != nil {
		return bail("waitForReadiness", err)
	}
	logf("integration: machine is ready")

	// --- Step 7: run the tests --------------------------------------
	// Teardown runs via the deferred teardownOnce above.
	return m.Run()
}

// teardown is the suite-level cleanup. It runs the cleanup stack (LIFO)
// and prints any failures. It is safe to call exactly once per Suite
// (guarded by teardownOnce in TestMain).
func (s *Suite) teardown() {
	s.runCleanups(logf)
}

// ---------------------------------------------------------------------------
// Step 1: ensure machine
// ---------------------------------------------------------------------------

// errPodmanNotFound is the sentinel returned by ensureMachine when the
// podman binary is not on PATH. TestMain maps this to a clean skip
// rather than a fatal failure, so `go test -tags=integration` on a
// machine without podman exits 0.
var errPodmanNotFound = errors.New("podman binary not found on PATH")

// ensureMachine guarantees that the named podman VM is running. Returns
// started=true if TestMain owns stopping it (i.e. the VM exists and is
// now running). Returns started=false without error when the named VM
// does not exist — this is the "escape hatch" for users targeting an
// external host (e.g. `pi`) via the SSH env overrides.
//
// "Already running" is treated as success but started=true so teardown
// stops the VM at the end of the run (matching the AC: "TestMain starts
// and stops the podman machine").
func ensureMachine(machineName string) (started bool, err error) {
	podmanPath, err := exec.LookPath("podman")
	if err != nil {
		return false, errPodmanNotFound
	}

	// Check existence + state in one inspect call. `podman machine
	// inspect <name>` prints `[]` and exits 125 if the VM does not
	// exist; on success it prints a JSON array with a State field.
	state, err := machineState(machineName)
	if err != nil {
		// Distinguish "does not exist" from real failures. We see this
		// when targeting an external connection like `pi` — there's no
		// local VM to start/stop, and that's fine.
		if isMachineNotFound(err) {
			logf("integration: machine %q does not exist; skipping start/stop (targeting external host?)", machineName)
			return false, nil
		}
		return false, fmt.Errorf("check machine state: %w", err)
	}

	if state == "running" {
		// Already up — we still own the stop so the run leaves nothing
		// behind. (This is the typical case on a dev box where the user
		// already started the machine.)
		return true, nil
	}

	logf("integration: starting machine %q (was %q)", machineName, state)
	cmd := exec.Command(podmanPath, "machine", "start", machineName)
	if out, err := cmd.CombinedOutput(); err != nil {
		// "already running" can race in between our inspect and start;
		// treat it as success.
		if isAlreadyRunning(string(out)) {
			return true, nil
		}
		return false, fmt.Errorf("podman machine start: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return true, nil
}

// stopMachine is the teardown counterpart to ensureMachine. Best-effort:
// a failed stop is logged but does not turn the run red.
func stopMachine(machineName string) error {
	podmanPath, err := exec.LookPath("podman")
	if err != nil {
		return nil // nothing we can do
	}
	cmd := exec.Command(podmanPath, "machine", "stop", machineName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("podman machine stop: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// machineState returns the State field of `podman machine inspect
// <name>` (e.g. "running", "stopped", "starting"). When the named VM
// does not exist, podman prints "VM does not exist" on stderr and exits
// 125; we capture stderr so isMachineNotFound can recognise that case
// (used by ensureMachine to implement the escape-hatch escape).
func machineState(machineName string) (string, error) {
	out, err := exec.Command("podman", "machine", "inspect", machineName, "--format", "{{.State}}").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w (%s)", err, strings.TrimSpace(string(out)))
	}
	s := strings.TrimSpace(string(out))
	if s == "" || s == "[]" {
		return "", fmt.Errorf("machine %q does not exist", machineName)
	}
	return s, nil
}

// isMachineNotFound reports whether err came from a `podman machine`
// invocation against a name with no matching VM.
func isMachineNotFound(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "VM does not exist") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "no such machine")
}

// isAlreadyRunning reports whether combinedOutput from `podman machine
// start` indicates the VM was already running. Podman prints this when
// it loses the start race.
func isAlreadyRunning(combined string) bool {
	return strings.Contains(combined, "already running")
}

// ---------------------------------------------------------------------------
// Step 2: discover target
// ---------------------------------------------------------------------------

// discoveredTarget is the resolved SSH/socket/identity coordinate set
// for the machine under test. Each field is the result of "discovered
// value, unless the matching PODLAND_TEST_SSH_* env var overrides it".
type discoveredTarget struct {
	Host         string
	Port         int
	SSHUser      string
	SocketPath   string
	IdentityPath string
}

// discoverTarget parses `podman system connection list --format json`
// for the connection matching machineName and then applies the
// PODLAND_TEST_SSH_* env overrides. The JSON form is much easier to
// parse reliably than the columnar default output.
//
// The connection entry's URI looks like:
//
//	ssh://core@127.0.0.1:44425/run/user/1000/podman/podman.sock
//
// We split that into user/host/port/socket and take Identity verbatim.
//
// Escape hatch: if the connection name is not in `podman system
// connection list` at all (e.g. targeting the `pi` host directly), the
// function still succeeds provided every PODLAND_TEST_SSH_* env var is
// set — there's nothing to discover, so the overrides have to carry the
// full coordinate set.
func discoverTarget(machineName string) (*discoveredTarget, error) {
	connections, err := listConnections()
	if err != nil {
		return nil, fmt.Errorf("list podman connections: %w", err)
	}

	var conn *podmanConnection
	for i := range connections {
		if connections[i].Name == machineName {
			conn = &connections[i]
			break
		}
	}

	// Track origin of each field so the escape-hatch path can require
	// them all to be set when there's no connection entry to default
	// from.
	var (
		host, user, socket, identity string
		port                         int
	)

	if conn != nil {
		u, h, p, s, err := parseSSHURI(conn.URI)
		if err != nil {
			return nil, fmt.Errorf("parse connection URI %q: %w", conn.URI, err)
		}
		user, host, port, socket = u, h, p, s
		identity = conn.Identity
	}

	// Apply env overrides — the escape hatch for targeting `pi` or any
	// other host without changing the connection-list output.
	if v := os.Getenv("PODLAND_TEST_SSH_HOST"); v != "" {
		host = v
	}
	if v := os.Getenv("PODLAND_TEST_SSH_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("PODLAND_TEST_SSH_PORT=%q is not an integer: %w", v, err)
		}
		port = p
	}
	if v := os.Getenv("PODLAND_TEST_SSH_USER"); v != "" {
		user = v
	}
	if v := os.Getenv("PODLAND_TEST_SSH_SOCKET"); v != "" {
		socket = v
	}
	if v := os.Getenv("PODLAND_TEST_SSH_KEY_PATH"); v != "" {
		identity = v
	}

	if conn == nil {
		// No connection entry was the basis for the defaults — every
		// field must therefore have come from an env override. Missing
		// ones mean the user pointed at a name we don't know AND didn't
		// give us coordinates via env.
		missing := []string{}
		if host == "" {
			missing = append(missing, "PODLAND_TEST_SSH_HOST")
		}
		if port == 0 {
			missing = append(missing, "PODLAND_TEST_SSH_PORT")
		}
		if user == "" {
			missing = append(missing, "PODLAND_TEST_SSH_USER")
		}
		if socket == "" {
			missing = append(missing, "PODLAND_TEST_SSH_SOCKET")
		}
		if identity == "" {
			missing = append(missing, "PODLAND_TEST_SSH_KEY_PATH")
		}
		if len(missing) > 0 {
			return nil, fmt.Errorf(
				"no podman system connection named %q (have: %s) and these env overrides are also missing: %s",
				machineName, connectionNames(connections), strings.Join(missing, ", "))
		}
	} else if host == "" || port == 0 || user == "" || socket == "" || identity == "" {
		return nil, fmt.Errorf("incomplete target: host=%q port=%d user=%q socket=%q identity=%q",
			host, port, user, socket, identity)
	}

	return &discoveredTarget{
		Host:         host,
		Port:         port,
		SSHUser:      user,
		SocketPath:   socket,
		IdentityPath: identity,
	}, nil
}

// podmanConnection is the JSON shape emitted by `podman system
// connection list --format json`. Only the fields we need are modelled.
type podmanConnection struct {
	Name     string `json:"Name"`
	URI      string `json:"URI"`
	Identity string `json:"Identity"`
}

// listConnections shells out to podman and decodes the JSON output.
func listConnections() ([]podmanConnection, error) {
	out, err := exec.Command("podman", "system", "connection", "list", "--format", "json").Output()
	if err != nil {
		return nil, err
	}
	var connections []podmanConnection
	if err := json.Unmarshal(out, &connections); err != nil {
		return nil, fmt.Errorf("decode connection list JSON: %w", err)
	}
	return connections, nil
}

func connectionNames(cs []podmanConnection) string {
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = c.Name
	}
	return strings.Join(names, ", ")
}

// parseSSHURI splits an `ssh://user@host:port/path` URI into its parts.
// The stdlib url.Parse handles this directly; we just re-package the
// fields. An empty port in the URI is treated as an error here (podman
// always emits one).
func parseSSHURI(uri string) (user, host string, port int, socket string, err error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", "", 0, "", err
	}
	if u.Scheme != "ssh" {
		return "", "", 0, "", fmt.Errorf("expected ssh:// scheme, got %q", u.Scheme)
	}
	if u.User != nil {
		user = u.User.Username()
	}
	host = u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		return "", "", 0, "", fmt.Errorf("URI %q has no port", uri)
	}
	port, err = strconv.Atoi(portStr)
	if err != nil {
		return "", "", 0, "", fmt.Errorf("URI %q port %q: %w", uri, portStr, err)
	}
	if u.Path == "" {
		return "", "", 0, "", fmt.Errorf("URI %q has no socket path", uri)
	}
	socket = u.Path
	return user, host, port, socket, nil
}

// ---------------------------------------------------------------------------
// Step 3: build test config (temp SQLite, temp JWT PEMs, random ENC key)
// ---------------------------------------------------------------------------

// buildTestConfig produces a fully-controlled *config.Config that uses
// only temp files. The returned tempDirs are removed at teardown. JWT
// keys are generated via internal/testutils.CreateTestJWTKeys (the
// PRD-mandated reuse — single source of truth for test keygen).
func buildTestConfig() (*config.Config, []string, error) {
	root, err := os.MkdirTemp("", "podland-it-*")
	if err != nil {
		return nil, nil, err
	}
	tempDirs := []string{root}

	dbPath := filepath.Join(root, "test.db")
	privPath := filepath.Join(root, "jwt-private.pem")
	pubPath := filepath.Join(root, "jwt-public.pem")
	if err := writeJWTKeypair(privPath, pubPath); err != nil {
		_ = os.RemoveAll(root)
		return nil, nil, fmt.Errorf("write JWT keypair: %w", err)
	}

	encKey, err := randomBase64(32)
	if err != nil {
		_ = os.RemoveAll(root)
		return nil, nil, fmt.Errorf("generate encryption key: %w", err)
	}

	cfg := &config.Config{
		Port: 0, // App doesn't bind; httptest.NewServer does.
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
			QueueTimeoutSeconds:   30,
			TestTimeoutSeconds:    10,
		},
		// The rate limiters are production abuse guards, not the system
		// under test for most slices: the per-user limiter sees the
		// suite authenticate as ONE user and legitimately issue
		// hundreds of requests per minute (state-transition polls alone
		// fire ~2/s for up to 30s each), and the per-IP auth limiter
		// sees every login/refresh in the suite arrive from 127.0.0.1.
		// Both 60/min-class production defaults would cause spurious
		// 429s that poison every test after the first poll-heavy slice,
		// so both ceilings are raised out of the way. The auth limiter
		// gets its own dedicated coverage from a separate low-limit
		// app instance in ratelimit_auth_test.go.
		RateLimit:      config.RateLimitConfig{RequestsPerMinute: 100000, AuthRequestsPerMinute: 100000},
		AllowedOrigins: []string{},
		Log:            config.LogConfig{Level: "info", Format: "json"},
		Encryption:     config.EncryptionConfig{Key: encKey},
	}
	return cfg, tempDirs, nil
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
// Step 4: seed user + real login
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
// Step 5: register machine
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
// exercised exactly once per run.
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
		t.Fatalf("register reachable throwaway %q against the suite's discovered target: %v", name, err)
	}
	if id == "" {
		t.Fatalf("register reachable throwaway %q returned an empty id", name)
	}
	return id
}

// ---------------------------------------------------------------------------
// Step 6: readiness
// ---------------------------------------------------------------------------

// waitForReadiness polls `podman machine inspect` until State==running
// and then retries a real podman-over-SSH probe with exponential
// backoff. The two-phase check exists because a VM marked "running" can
// still have a cold SSH/socket path for several seconds.
//
// When we didn't start a VM (started=false in the escape-hatch path —
// the user is targeting an external host via env overrides), the
// state-poll phase is skipped because there is no machine to inspect.
// The probe phase still runs over the discovered connection name so a
// genuinely-unreachable external host fails fast at startup rather than
// mid-test.
//
// Total budget ~60s (state phase 30s, probe phase 30s). On exhaustion
// the function returns the last error so the caller can surface it.
func waitForReadiness(machineName string, disc *discoveredTarget, startedVM bool) error {
	var lastErr error

	// Phase 1: VM state — only meaningful when we have a local VM.
	if startedVM {
		stateDeadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(stateDeadline) {
			state, err := machineState(machineName)
			if err == nil {
				if state == "running" {
					lastErr = nil
					break
				}
				lastErr = fmt.Errorf("machine %q state=%q, want running", machineName, state)
			} else {
				lastErr = err
			}
			time.Sleep(2 * time.Second)
		}
		if lastErr != nil {
			return fmt.Errorf("state never reached running within 30s: %w", lastErr)
		}
	}

	// Phase 2: real SSH+socket probe. `podman --connection <name> ps`
	// exercises the exact same path the backend will use; if it
	// succeeds, the tracer test can rely on the socket being warm.
	// In the escape-hatch path the connection name is whatever the
	// user pointed at via PODLAND_TEST_MACHINE_NAME; that name has to
	// resolve in `podman system connection list` for the probe to work.
	probeDeadline := time.Now().Add(30 * time.Second)
	backoff := 1 * time.Second
	for time.Now().Before(probeDeadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		out, err := exec.CommandContext(ctx, "podman", "--connection", machineName, "ps").CombinedOutput()
		cancel()
		if err == nil {
			return nil
		}
		lastErr = fmt.Errorf("podman --connection %s ps: %w (%s)", machineName, err, strings.TrimSpace(string(out)))
		time.Sleep(backoff)
		backoff *= 2
		if backoff > 8*time.Second {
			backoff = 8 * time.Second
		}
	}
	return fmt.Errorf("socket probe never succeeded within 30s: %w", lastErr)
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
