//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test_Integration_Quadlets_Generate is the quadlet generate vertical slice
// for the real-podman integration suite (PRD 080-11). Unlike the other
// quadlet slices, generate never touches the remote machine's quadlet
// store: the service inspects an existing container (or pod) over the
// podman API and converts it to `.container`/`.pod` content **locally in
// the backend** (generateContainerQuadlet / generatePodQuadlet — not
// podman's own converter). The slice proves that inspect + local-render
// path end-to-end:
//
//	POST /api/machines/{id}/quadlets/{quadletId}/generate
//	     body {resourceId, resourceType} → 200 {…, state: "unknown", content}
//
// QUIRK (documented in 070-08): the {quadletId} path param is effectively
// ignored by the service — the body's resourceId is the source of truth.
// This slice pins that quirk by passing a placeholder path param and
// asserting the generate still resolves the body's resource.
//
// Isolation/cleanup model (matching the containers/quadlet slices): the
// source container is created with a run-scoped name (it-<runID>-gen) via
// the containers slice's createContainer helper; t.Cleanup force-removes
// it via the podman connection. The pod path (if practical) creates a
// throwaway pod over the CLI — the backend exposes no pod API — with the
// same t.Cleanup + sweep backstop. Nothing is installed on the remote
// machine by generate itself, so there is no quadlet to clean up.
func Test_Integration_Quadlets_Generate(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerContainersSweep()
	registerPodsSweep()

	machineID := suite.MachineID

	// `podman create` does not auto-pull; the source container needs the
	// image present. Reuse the containers slice's idempotent helper.
	ensureBusyboxImage(t)

	// Run-scoped names so the suite sweeps can find anything this run
	// leaves behind.
	containerName := "it-" + suite.RunID + "-gen"

	// Per-test cleanup: force-remove the source container. Registered
	// before create so it runs even if a later phase fails. Best-effort:
	// the containers sweep is the abort backstop.
	var containerID string
	t.Cleanup(func() {
		if containerID == "" {
			return
		}
		if out, err := podmanConnRun("rm", "-f", containerID); err != nil {
			t.Logf("cleanup: rm -f %q best-effort failed: %v (%s)", containerID, err, strings.TrimSpace(out))
		}
	})

	// --- Setup: create the source container (created state is enough;
	// inspect works on a never-started container — see PRD tech notes).
	// Parent-level setup like the lifecycle slice: createContainer's
	// asserts are fatal, so no t.Failed guard is needed after it.
	containerID = createContainer(t, machineID, containerCreateBody{
		Name:    containerName,
		Image:   "busybox:latest",
		Command: []string{"sh", "-c", "sleep 600"},
	})

	// --- AC #1/#2: generate from the real container; nothing installed --
	t.Run("GenerateFromContainer", func(t *testing.T) {
		before := listQuadletNames(t, machineID)

		generated := generateQuadlet(t, machineID, "ignored-path-param", quadletGenerateBody{
			ResourceID:   containerName,
			ResourceType: "container",
		})

		require.Equal(t, "unknown", generated.State,
			"a generated (not installed) quadlet must report state=unknown")
		require.Equal(t, "container", generated.Resource,
			"resource must echo the request's resourceType")
		// The quadlet name is derived from the inspect report's Name —
		// podman may report it with a leading slash, so compare trimmed.
		require.Equal(t, containerName, strings.TrimPrefix(generated.Name, "/"),
			"generated quadlet name must come from the source container")

		// Structural assertions per the PRD: assert the shape of the
		// rendered content, not an exact string (the renderer's exact
		// output is free to change; [Container] + the source image are
		// the contract).
		require.NotEmpty(t, generated.Content,
			"generated content must be a non-empty string")
		require.Contains(t, generated.Content, "[Container]",
			"container quadlet content must contain a [Container] section")
		require.Contains(t, generated.Content, "busybox",
			"container quadlet content must reference the source image")

		// No files installed on the remote machine: the quadlet list is
		// unchanged by a generate.
		require.Equal(t, before, listQuadletNames(t, machineID),
			"generate must not install anything on the remote machine")
	})

	// --- AC #3: generate for a missing container → 404 ------------------
	t.Run("MissingContainerReturns404", func(t *testing.T) {
		generateQuadletExpectError(t, machineID, "ignored-path-param", quadletGenerateBody{
			ResourceID:   "it-" + suite.RunID + "-no-such-container",
			ResourceType: "container",
		},
			http.StatusNotFound, "CONTAINER_NOT_FOUND",
			"generate from a missing container")
	})

	// --- AC #4: the resourceType=pod path -------------------------------
	// The backend exposes no pod API, so the throwaway pod is created over
	// the podman CLI (same connection the backend uses) — the PRD's "if
	// practical" ruling. A freshly created pod (never started) is
	// inspectable, which is all generate needs.
	t.Run("GenerateFromPod", func(t *testing.T) {
		podName := "it-" + suite.RunID + "-genpod"
		// Per-test cleanup registered before create so it runs even if a
		// later assertion fails. Best-effort; the pods sweep is the abort
		// backstop.
		t.Cleanup(func() { removePod(t, podName) })
		createPod(t, podName)

		before := listQuadletNames(t, machineID)

		generated := generateQuadlet(t, machineID, "ignored-path-param", quadletGenerateBody{
			ResourceID:   podName,
			ResourceType: "pod",
		})

		require.Equal(t, "unknown", generated.State,
			"a generated (not installed) quadlet must report state=unknown")
		require.Equal(t, "pod", generated.Resource,
			"resource must echo the request's resourceType")
		require.Equal(t, podName, strings.TrimPrefix(generated.Name, "/"),
			"generated quadlet name must come from the source pod")

		require.NotEmpty(t, generated.Content,
			"generated content must be a non-empty string")
		require.Contains(t, generated.Content, "[Pod]",
			"pod quadlet content must contain a [Pod] section")
		require.Contains(t, generated.Content, "PodName="+podName,
			"pod quadlet content must reference the source pod")

		require.Equal(t, before, listQuadletNames(t, machineID),
			"generate must not install anything on the remote machine")
	})
}

// ---------------------------------------------------------------------------
// HTTP helpers — each hits the in-process backend via the authenticated client
// ---------------------------------------------------------------------------

// quadletGenerateBody mirrors the JSON-accepted fields of
// quadlets.QuadletGenerateRequest. Kept as a local type so the
// integration package doesn't import the quadlets domain package
// (matching the other quadlet slices).
type quadletGenerateBody struct {
	ResourceID   string `json:"resourceId"`
	ResourceType string `json:"resourceType"`
}

// quadletGenerateResponse mirrors the fields of
// quadlets.QuadletGenerateResponse that this slice consumes.
type quadletGenerateResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Resource string `json:"resource"`
	State    string `json:"state"`
	Content  string `json:"content"`
}

// generateQuadlet POSTs /quadlets/{quadletId}/generate and asserts 200.
//
// quadletID is the (service-ignored) path param — see the quirk note on
// the test. Passing anything but "" exercises the route.
func generateQuadlet(t *testing.T, machineID, quadletID string, body quadletGenerateBody) quadletGenerateResponse {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)

	url := suite.ServerURL + "/api/machines/" + machineID + "/quadlets/" + quadletID + "/generate"
	resp, err := suite.AuthenticatedClient().Post(url, "application/json", bytes.NewReader(payload))
	require.NoError(t, err, "POST /quadlets/{id}/generate must connect to the in-process server")
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"generate quadlet: expected 200, got %d. body: %s", resp.StatusCode, string(raw))

	var generated quadletGenerateResponse
	require.NoErrorf(t, json.Unmarshal(raw, &generated),
		"generate quadlet: response must decode as QuadletGenerateResponse. body: %s", string(raw))
	return generated
}

// generateQuadletExpectError POSTs /quadlets/{quadletId}/generate and
// asserts the expected HTTP status plus the machine-readable error code
// from the {"error":{code,message}} envelope.
func generateQuadletExpectError(t *testing.T, machineID, quadletID string, body quadletGenerateBody, wantStatus int, wantCode, context string) {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)

	url := suite.ServerURL + "/api/machines/" + machineID + "/quadlets/" + quadletID + "/generate"
	resp, err := suite.AuthenticatedClient().Post(url, "application/json", bytes.NewReader(payload))
	require.NoError(t, err, "%s: POST /quadlets/{id}/generate must connect", context)
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, wantStatus, resp.StatusCode,
		"%s: expected %d, got %d. body: %s", context, wantStatus, resp.StatusCode, string(raw))

	var errResp quadletErrorEnvelope
	require.NoErrorf(t, json.Unmarshal(raw, &errResp),
		"%s: error response must decode. body: %s", context, string(raw))
	require.Equal(t, wantCode, errResp.Error.Code,
		"%s: error code must be %s", context, wantCode)
}

// ---------------------------------------------------------------------------
// Throwaway pod (the backend has no pod API — CLI scaffolding)
// ---------------------------------------------------------------------------

// createPod creates a throwaway pod over the podman CLI. The pod is never
// started: generate only inspects it, and a freshly created pod (with its
// infra container in Created state) is fully inspectable.
func createPod(t *testing.T, name string) {
	t.Helper()
	if out, err := podmanConnRun("pod", "create", "--name", name); err != nil {
		t.Fatalf("create pod %s (prerequisite): %v (%s)", name, err, strings.TrimSpace(out))
	}
}

// removePod force-removes a pod over the podman CLI (best-effort).
func removePod(t *testing.T, name string) {
	t.Helper()
	if out, err := podmanConnRun("pod", "rm", "-f", name); err != nil {
		t.Logf("cleanup pod rm -f %q best-effort failed: %v (%s)", name, err, strings.TrimSpace(out))
	}
}

// ---------------------------------------------------------------------------
// Suite sweep (covers aborts)
// ---------------------------------------------------------------------------

// registerPodsSweepOnce ensures the pods suite-sweep cleanup is registered
// at most once per process. The sweep force-removes any pod whose name
// mentions this run's RunID — i.e. garbage a test left behind when its own
// t.Cleanup didn't fire (panic, SIGINT before the per-test cleanup
// registered, …). Registered at the top of the generate test, so it is in
// place before any garbage is created. Pods need their own sweep: a pod
// survives its infra container, so the containers sweep's `rm -f` cannot
// remove run-scoped pod garbage. Best-effort: failures are logged by
// runCleanups, never fatal.
var registerPodsSweepOnce sync.Once

func registerPodsSweep() {
	registerPodsSweepOnce.Do(func() {
		runID := suite.RunID
		suite.RegisterCleanup("sweep run-scoped pods", func() error {
			sweepRunScopedPods(runID)
			return nil
		})
	})
}

// sweepRunScopedPods lists every pod on the machine and force-removes any
// whose name mentions the runID. See registerPodsSweep for the dedicated
// sweep rationale.
func sweepRunScopedPods(runID string) {
	out, err := podmanConnRun("pod", "ps", "--format", "{{.Name}}")
	if err != nil {
		// Best-effort: a failed list (e.g. machine already stopped) is
		// not a fatal teardown problem.
		return
	}
	for _, name := range strings.Split(out, "\n") {
		name = strings.TrimSpace(name)
		if name == "" || !strings.Contains(name, runID) {
			continue
		}
		// pod rm -f the run-scoped pod (also removes its infra container).
		// Ignore errors: the per-test cleanup is the primary path; this is
		// the backstop.
		_, _ = podmanConnRun("pod", "rm", "-f", name)
	}
}
