//go:build integration

package integration

import (
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/Homerus25/podland/backend/internal/app"
)

// Suite is the shared fixture constructed by TestMain and exposed to
// every test in package integration via the package-level `suite`
// variable. Downstream slices (080-02 images, 080-03 containers, …)
// build on top of these helpers rather than re-deriving them.
//
// Concurrency: Suite is intended to be read-only after TestMain returns.
// The cleanup stack is the only mutable field and is guarded by mu.
// Tests run sequentially by default (`go test` without `-parallel`), and
// the helpers here do not try to make concurrent test execution safe —
// callers that opt into `-parallel` must synchronise their own access.
type Suite struct {
	// ServerURL is the base URL of the in-process httptest server
	// (e.g. "http://127.0.0.1:1234"). Tests append API paths to this.
	ServerURL string

	// MachineID is the UUID of the machine registered once during
	// TestMain. It is shared across all tests in the run. Do not delete
	// it from a test — that would break every other test in the suite.
	MachineID string

	// MachineName is the podman system-connection name (resolved from
	// PODLAND_TEST_MACHINE_NAME, default "podman-machine-default") that
	// the backend's registered machine points at. Downstream slices use
	// it for the isolation/cleanup CLI scaffolding (tag, rmi, sweep) so
	// they hit the same host the backend does — instead of re-reading
	// the env var (and risking drift if the var name changes).
	//
	// The readiness probe in TestMain guarantees `podman --connection
	// <MachineName>` is reachable before any test runs, so CLI helpers
	// can rely on it without their own probe.
	MachineName string

	// Token is the JWT bearer token captured from POST /api/auth/login
	// during TestMain. AuthenticatedClient injects it as
	// `Authorization: Bearer <token>`, matching middleware/auth.go.
	Token string

	// OwnerUserID is the DB-level ID (a UUID) of the user the suite
	// registered and logged in as during TestMain. Tests that need to
	// reference the authenticated user by ID (e.g. asserting owner
	// self-revoke is rejected in 080-04) use this rather than guessing
	// or re-deriving it from the JWT.
	OwnerUserID string

	// OtherUserID is the DB-level ID of a second user seeded during
	// TestMain. It exists to satisfy the machine_access(user_id) FK so
	// the machines CRUD/access slice (080-04) can exercise
	// grant/revoke/list against a real grantee without minting a token
	// for them. The second user is never logged in.
	OtherUserID string

	// DBPath is the filesystem path of the suite's temp SQLite database.
	// Tests that must assert on persisted state directly (e.g. that only
	// a refresh token's hash is stored, 100-01) open it read-side via
	// openSuiteDB rather than reaching into the App.
	DBPath string

	// RunID is the run-scoped identifier (a short UUID) embedded in
	// RunTag. Anything the suite creates on the shared machine is tagged
	// with RunTag so per-run isolation is possible and the suite sweep
	// can find its own garbage.
	RunID string

	// goodMachineRegistration holds the coordinates + identity the
	// shared good machine was registered with (the discovered podman
	// target). It is NOT used directly by tests; RegisterReachableMachine
	// consumes it to register additional throwaway machines against the
	// same reachable target. This exists because 080-05 made
	// MachineService.Create probe the target at registration, so a
	// throwaway pointing at an unreachable host (the original 080-04
	// pattern) can no longer be registered — slices that need a
	// mutable throwaway (PUT/access/DELETE) must now point at a host
	// that actually answers.
	goodMachineRegistration reachableTarget

	// application is the in-process backend built by TestMain. It is
	// held so suite methods can expose narrow seams (e.g. SeedUserWithRole)
	// without handing every test the whole App.
	application *app.App

	// skipReason is the reason TestMain decided the suite cannot run
	// (e.g. podman binary missing, machine unavailable). When non-empty,
	// SkipIfUnavailable triggers t.Skip so a clean `go test` run on a
	// machine without podman still exits 0.
	skipReason string

	// cleanup stack — LIFO, mirrors t.Cleanup semantics. Run during
	// suite teardown after m.Run() returns (or on signal).
	cleanupMu sync.Mutex
	cleanups  []cleanupStep
}

// cleanupStep is one entry in the suite teardown stack.
type cleanupStep struct {
	name string
	fn   func() error
}

// SkipIfUnavailable makes a test skip cleanly when TestMain could not
// build the full fixture (no podman binary, no machine, etc.). Call it
// as the first line of every test in the suite:
//
//	func Test_Integration_Foo(t *testing.T) {
//	    suite.SkipIfUnavailable(t)
//	    …
//	}
//
// This keeps `go test -tags=integration` green on machines that lack a
// podman installation, satisfying the "skips cleanly" AC.
func (s *Suite) SkipIfUnavailable(t *testing.T) {
	t.Helper()
	if s == nil {
		t.Skip("integration suite: TestMain did not initialise the fixture")
	}
	if s.skipReason != "" {
		t.Skipf("integration suite: %s", s.skipReason)
	}
}

// AuthenticatedClient returns an *http.Client that injects
// `Authorization: Bearer <Token>` on every outgoing request, matching
// the scheme the production middleware reads (middleware/auth.go). The
// returned client reuses a single transport; do not mutate it.
//
// The token itself is captured from the real POST /api/auth/login
// response during TestMain — exercising the real auth path, not a
// shortcut around it.
func (s *Suite) AuthenticatedClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &bearerTransport{
			token: s.Token,
			base:  http.DefaultTransport,
		},
	}
}

// bearerTransport wraps an http.RoundTripper to add the Authorization
// header. It exists so tests can use a plain http.Client without every
// call site having to remember to set the header.
type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request before mutating per the RoundTripper contract.
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}

// CookieJarClient returns an *http.Client that persists Set-Cookie
// headers in a standard-library cookie jar across requests — the shape a
// browser session has. The refresh-cookie suite (100-02) is its first
// consumer: log in through one jar client and the refresh cookie rides
// along to every subsequent /api/auth request without tests hand-parsing
// Set-Cookie headers.
//
// Each call returns a fresh jar around the existing transport, so two
// clients behave like two devices: cookies earned by one are invisible
// to the other. The jar is a plain cookiejar (no public-suffix list);
// the suite talks to a loopback httptest origin where that suffices.
func (s *Suite) CookieJarClient() *http.Client {
	// cookiejar.New(nil) cannot fail for a nil-options call; it returns
	// the error solely to allow future option validation.
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: http.DefaultTransport,
		Jar:       jar,
	}
}

// SeedUserWithRole seeds a user with an explicit role through the
// in-process app's seeding seam and returns the new user's ID. It lets
// slices mint, e.g., an admin-role user (mirroring the cmd/initdb
// bootstrap) without bypassing the real login path — the caller still
// has to POST /api/auth/login to act as that user.
func (s *Suite) SeedUserWithRole(username, password, role string) (string, error) {
	return s.application.SeedUserWithRole(username, password, role)
}

// RunTag returns the run-scoped tag (e.g. "podland-test-run=a1b2c3d4")
// used to label anything this suite creates on the shared machine. Tests
// that list resources filter by this tag so a pre-dirty machine doesn't
// cause false failures; the suite sweep uses it to find leftover garbage.
//
// The tag is stable for the lifetime of one `go test` process; the RunID
// half is a fresh UUID per run.
//
// Resource-name prefixing (e.g. "it-<runID>-busybox") is intentionally
// left to downstream slices — the naming convention is per-resource-type
// and pulling it up here would force a one-size-fits-all shape.
func (s *Suite) RunTag() string {
	return "podland-test-run=" + s.RunID
}

// RegisterCleanup registers a function to run during suite teardown.
// Functions run in LIFO order (last registered runs first), mirroring
// t.Cleanup semantics. Use this for suite-scoped resources created in
// TestMain (e.g. the registered machine) or for sweep functions that
// clean up run-tagged resources across domains.
//
// Errors from a cleanup step are logged but do not halt the sweep —
// best-effort cleanup is the goal, so a failing container-delete should
// not block image-delete or machine stop.
//
// Registration order matters: callers must register in REVERSE of the
// intended execution order (e.g. for an ordered teardown A → B → C,
// register C, then B, then A). This matches t.Cleanup's LIFO contract.
func (s *Suite) RegisterCleanup(name string, fn func() error) {
	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()
	s.cleanups = append(s.cleanups, cleanupStep{name: name, fn: fn})
}

// runCleanups executes the cleanup stack in LIFO order. Safe to call
// exactly once (from TestMain teardown, possibly via the signal handler).
// Errors are surfaced via the supplied logger so TestMain can print them
// before exit; they do not stop the sweep.
func (s *Suite) runCleanups(log func(format string, args ...any)) {
	s.cleanupMu.Lock()
	steps := s.cleanups
	s.cleanups = nil
	s.cleanupMu.Unlock()

	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		if err := step.fn(); err != nil {
			log("integration cleanup %q failed: %v", step.name, err)
		}
	}
}

// newRunID returns a short, run-scoped identifier suitable for embedding
// in tags and resource names. It is the first 8 chars of a UUID — enough
// to make collisions across concurrent runs effectively impossible
// without bloating label values.
func newRunID() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")[:8]
}

// reachableTarget is the SSH/podman coordinate set for a host the suite
// has already proven reachable (the discovered podman machine). The
// suite stores the good machine's target here so
// RegisterReachableMachine can register additional throwaway machines
// against the same host — needed because 080-05 made registration probe
// the target, so a throwaway must point at a host that answers.
//
// Defined here (not as machineRegistration, which lives in a _test.go
// file) so the non-test harness.go can reference it without breaking
// `go build -tags=integration`.
type reachableTarget struct {
	Host    string
	Port    int
	SSHUser string
	Socket  string
	SSHKey  string
}
