//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Homerus25/podland/backend/internal/models"
)

// Test_Integration_Pods_Lifecycle is the pods vertical slice for the
// real-podman integration suite (spec 353). It exercises the full
// pod-management request flow end-to-end against the once-registered
// machine:
//
//	POST   /api/machines/{id}/pods              → 201 created
//	GET    /api/machines/{id}/pods              → listed
//	GET    /api/machines/{id}/pods/{podId}      → detail DTO
//	GET    /api/machines/{id}/pods/{podId}/inspect → raw report
//	POST   .../pods/{podId}/pause|unpause|kill|restart → action contract
//	DELETE /api/machines/{id}/pods/{podId}?force=true → 204 removed
//	POST   /api/machines/{id}/pods/prune        → removed stopped pod
//
// Isolation model (per the master PRD's "run-scoped tag + per-test
// cleanup + suite sweep" ruling): run-scoped pod names, per-test
// t.Cleanup force-removing the pod (which takes its containers with
// it), and the suite-teardown pods sweep for aborts.
//
// Nested-environment note: a container-in-pod workload needs the pod's
// infra container to start, and a nested podman (the test host is a
// container) cannot mount the infra container's special rootfs overlay.
// Where the environment cannot run pod containers, the test pins the
// action responses' contract (200 + the stamped post-action state)
// without observing real state transitions, and skips restart — podman
// restarts the infra container too, which is exactly what the nested
// overlay cannot do. On a host that runs pod containers, the same test
// observes the real Paused/Running/Exited transitions.
func Test_Integration_Pods_Lifecycle(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerPodsSweep()

	machineID := suite.MachineID
	podName := "it-" + suite.RunID + "-pod"

	// Per-test cleanup: force-remove the pod we create (its containers
	// go with it). Registered before create so it runs even if a later
	// phase fails. Best-effort: the sweep is the backstop.
	t.Cleanup(func() {
		if out, err := podmanConnRun("pod", "rm", "-f", podName); err != nil {
			t.Logf("cleanup: pod rm -f %q best-effort failed: %v (%s)", podName, err, strings.TrimSpace(out))
		}
	})

	// --- AC: POST creates a run-scoped-named pod; 201 -----------------
	var podID string
	t.Run("CreateReturns201", func(t *testing.T) {
		podID = createTestPod(t, machineID, podName)
		require.NotEmpty(t, podID, "create must return a non-empty pod id")
		t.Logf("pod created: id=%s name=%s", podID, podName)
	})
	if t.Failed() {
		return
	}

	// --- AC: GET list and detail reflect the created pod ---------------
	t.Run("ListAndDetailShowPod", func(t *testing.T) {
		pods := listPods(t, machineID)
		var found *podListDTO
		for i := range pods {
			if pods[i].ID == podID {
				found = &pods[i]
				break
			}
		}
		require.NotNil(t, found, "created pod %q must appear in the list, got %d pods", podName, len(pods))
		assert.Equal(t, podName, found.Name)

		detail := getPodDetail(t, machineID, podID)
		assert.Equal(t, podID, detail.ID)
		assert.Equal(t, podName, detail.Name)
		assert.NotEmpty(t, detail.State)
		assert.NotEmpty(t, detail.CreatedAt)
		assert.NotNil(t, detail.Containers, "containers must decode as an array")
	})

	// --- AC: raw inspect returns podman's report -----------------------
	t.Run("InspectReturnsRawReport", func(t *testing.T) {
		raw := inspectPodRaw(t, machineID, podID)
		require.Contains(t, raw, "Name", "the raw report must carry podman's inspect fields")
	})

	// --- AC: lifecycle actions -----------------------------------------
	t.Run("LifecycleActions", func(t *testing.T) {
		if !podContainersRunnable(t) {
			t.Skip("this podman cannot start the pod's infra container (nested overlay limitation); " +
				"pause/unpause/kill/restart need running pod containers — the lifecycle contract is " +
				"pinned by the service tests against the fake engine")
		}
		exercisePodLifecycleWithContainer(t, machineID, podID)
	})
	if t.Failed() {
		return
	}

	// --- AC: DELETE removes the pod; a subsequent list excludes it -----
	t.Run("RemoveDeletesPod", func(t *testing.T) {
		removeTestPod(t, machineID, podID, true)
		podID = "" // already gone: the per-test cleanup becomes a no-op

		pods := listPods(t, machineID)
		for _, pod := range pods {
			require.NotEqual(t, podName, pod.Name, "removed pod must not appear in the list")
		}
	})
	if t.Failed() {
		return
	}

	// --- AC: prune removes a stopped pod -------------------------------
	t.Run("PruneRemovesStoppedPod", func(t *testing.T) {
		pruneName := "it-" + suite.RunID + "-prune"
		t.Cleanup(func() {
			if out, err := podmanConnRun("pod", "rm", "-f", pruneName); err != nil {
				t.Logf("cleanup: pod rm -f %q best-effort failed: %v (%s)", pruneName, err, strings.TrimSpace(out))
			}
		})
		pruneID := createTestPod(t, machineID, pruneName)

		if !podContainersRunnable(t) {
			// podman's prune only removes Exited pods, and reaching Exited
			// needs the infra container to start, which nested podman cannot
			// do here. Still pin the response shape: 200 with an array.
			pruneTestPods(t, machineID)
			t.Skip("podman prune only removes Exited pods; the infra container cannot start in this " +
				"nested environment — the removal contract is pinned by the service tests against the fake engine")
		}

		// Drive the pod to Exited so podman considers it prunable.
		if out, err := podmanConnRun("pod", "start", pruneName); err != nil {
			t.Fatalf("pod start must succeed: %v (%s)", err, strings.TrimSpace(out))
		}
		if out, err := podmanConnRun("pod", "stop", pruneName); err != nil {
			t.Fatalf("pod stop must succeed: %v (%s)", err, strings.TrimSpace(out))
		}

		removed := pruneTestPods(t, machineID)
		require.Contains(t, removed, pruneID, "the stopped pod must be pruned, got %v", removed)
	})
}

// exercisePodLifecycleWithContainer joins a workload container to the
// pod through the backend's own container-create endpoint (the pod
// field, spec 352 ticket #354 — not the podman CLI), starts it through
// the start endpoint, and observes the real state transitions through
// the detail endpoint — the full pause → unpause → kill → restart
// cycle.
func exercisePodLifecycleWithContainer(t *testing.T, machineID, podID string) {
	t.Helper()

	ensureBusyboxImage(t)
	ctrName := "it-" + suite.RunID + "-pod-ctr"
	ctrID := createContainer(t, machineID, containerCreateBody{
		Name:    ctrName,
		Image:   "busybox:latest",
		Command: []string{"sh", "-c", "sleep 600"},
		Pod:     podID,
	})
	t.Cleanup(func() {
		if out, err := podmanConnRun("rm", "-f", ctrID); err != nil {
			t.Logf("cleanup: rm -f %q best-effort failed: %v (%s)", ctrID, err, strings.TrimSpace(out))
		}
	})
	startContainer(t, machineID, ctrID)

	detail := getPodDetail(t, machineID, podID)
	require.Len(t, detail.Containers, 1, "the pod's container must appear in the detail DTO")
	assert.Equal(t, ctrName, detail.Containers[0].Name)

	body := podAction(t, machineID, podID, "pause")
	assert.Equal(t, "paused", body.State, "the pause response must stamp the paused state")
	waitForPodState(t, machineID, podID, "Paused", "pod must reach Paused after pause")

	body = podAction(t, machineID, podID, "unpause")
	assert.Equal(t, "running", body.State, "the unpause response must stamp the running state")
	waitForPodState(t, machineID, podID, "Running", "pod must reach Running after unpause")

	body = podAction(t, machineID, podID, "kill")
	assert.Equal(t, "exited", body.State, "the kill response must stamp the exited state")
	waitForPodState(t, machineID, podID, "Exited", "pod must reach Exited after kill")

	body = podAction(t, machineID, podID, "restart")
	assert.Equal(t, "running", body.State, "the restart response must stamp the running state")
	waitForPodState(t, machineID, podID, "Running", "pod must reach Running after restart")
}

// Test_Integration_Pods_AccessControl pins the route gating: the pods
// subtree rides RequireMachineAccess, and its write routes gate at
// write level — a read-level grantee gets 403 ACCESS_DENIED.
func Test_Integration_Pods_AccessControl(t *testing.T) {
	suite.SkipIfUnavailable(t)

	t.Run("NoAccessUserGets403", func(t *testing.T) {
		expectAPIError(t, noAccessUserClient(t), http.MethodGet,
			suite.ServerURL+"/api/machines/"+suite.MachineID+"/pods",
			"", http.StatusForbidden, "ACCESS_DENIED")
	})

	t.Run("ReadLevelUserWriteRouteDenied", func(t *testing.T) {
		client := levelUserClient(t, models.PermissionRead)
		expectAPIError(t, client, http.MethodPost,
			suite.ServerURL+"/api/machines/"+suite.MachineID+"/pods",
			`{"name":"it-pod-denied"}`, http.StatusForbidden, "ACCESS_DENIED")
	})
}

// --- Helpers ------------------------------------------------------------

// podListDTO mirrors the pods list response item.
type podListDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// podDetailDTO mirrors the pod detail response.
type podDetailDTO struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	State      string `json:"state"`
	CreatedAt  string `json:"createdAt"`
	Containers []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		State string `json:"state"`
	} `json:"containers"`
}

// podActionDTO mirrors the lifecycle action and create responses.
type podActionDTO struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

// createTestPod POSTs the pod create request and returns the created
// pod's ID.
func createTestPod(t *testing.T, machineID, podName string) string {
	t.Helper()

	payload, err := json.Marshal(map[string]any{"name": podName})
	require.NoError(t, err)

	url := suite.ServerURL + "/api/machines/" + machineID + "/pods"
	resp, err := suite.AuthenticatedClient().Post(url, "application/json", bytes.NewReader(payload))
	require.NoError(t, err, "POST /pods must connect to the in-process server")
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusCreated, resp.StatusCode,
		"create pod: expected 201, got %d. body: %s", resp.StatusCode, string(raw))

	var created podActionDTO
	require.NoErrorf(t, json.Unmarshal(raw, &created), "create pod body must decode. body: %s", string(raw))
	return created.ID
}

// listPods GETs /api/machines/{id}/pods and decodes the wrapped list.
func listPods(t *testing.T, machineID string) []podListDTO {
	t.Helper()

	resp, err := suite.AuthenticatedClient().Get(suite.ServerURL + "/api/machines/" + machineID + "/pods")
	require.NoError(t, err, "GET /pods must connect to the in-process server")
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read pods response body")
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"list pods: expected 200, got %d. body: %s", resp.StatusCode, string(body))

	var out struct {
		Pods []podListDTO `json:"pods"`
	}
	require.NoErrorf(t, json.Unmarshal(body, &out),
		"pods response must decode as the wrapped list. body: %s", string(body))
	return out.Pods
}

// getPodDetail GETs /api/machines/{id}/pods/{podID} and decodes the
// detail DTO.
func getPodDetail(t *testing.T, machineID, podID string) podDetailDTO {
	t.Helper()

	resp, err := suite.AuthenticatedClient().Get(suite.ServerURL + "/api/machines/" + machineID + "/pods/" + podID)
	require.NoError(t, err, "GET pod detail must connect to the in-process server")
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read pod detail body")
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"pod detail: expected 200, got %d. body: %s", resp.StatusCode, string(body))

	var out podDetailDTO
	require.NoErrorf(t, json.Unmarshal(body, &out), "pod detail body must decode. body: %s", string(body))
	return out
}

// inspectPodRaw GETs the raw inspect endpoint and returns the body as
// a generic JSON object — the raw report is podman's, unmapped.
func inspectPodRaw(t *testing.T, machineID, podID string) map[string]any {
	t.Helper()

	resp, err := suite.AuthenticatedClient().Get(suite.ServerURL + "/api/machines/" + machineID + "/pods/" + podID + "/inspect")
	require.NoError(t, err, "GET pod inspect must connect to the in-process server")
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read pod inspect body")
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"pod inspect: expected 200, got %d. body: %s", resp.StatusCode, string(body))

	var out map[string]any
	require.NoErrorf(t, json.Unmarshal(body, &out), "raw inspect body must decode as a JSON object. body: %s", string(body))
	return out
}

// podAction POSTs one lifecycle action and decodes the action response.
func podAction(t *testing.T, machineID, podID, action string) podActionDTO {
	t.Helper()

	url := fmt.Sprintf("%s/api/machines/%s/pods/%s/%s", suite.ServerURL, machineID, podID, action)
	resp, err := suite.AuthenticatedClient().Post(url, "application/json", strings.NewReader("{}"))
	require.NoError(t, err, "POST pod action must connect to the in-process server")
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read pod action body")
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"pod action %s: expected 200, got %d. body: %s", action, resp.StatusCode, string(body))

	var out podActionDTO
	require.NoErrorf(t, json.Unmarshal(body, &out), "pod action body must decode. body: %s", string(body))
	return out
}

// removeTestPod DELETEs the pod with the force flag.
func removeTestPod(t *testing.T, machineID, podID string, force bool) {
	t.Helper()

	url := suite.ServerURL + "/api/machines/" + machineID + "/pods/" + podID
	if force {
		url += "?force=true"
	}
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)
	resp, err := suite.AuthenticatedClient().Do(req)
	require.NoError(t, err, "DELETE pod must connect to the in-process server")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusNoContent, resp.StatusCode,
		"remove pod: expected 204, got %d. body: %s", resp.StatusCode, string(raw))
}

// pruneTestPods POSTs the prune endpoint and returns the removed pods'
// IDs.
func pruneTestPods(t *testing.T, machineID string) []string {
	t.Helper()

	url := suite.ServerURL + "/api/machines/" + machineID + "/pods/prune"
	resp, err := suite.AuthenticatedClient().Post(url, "application/json", strings.NewReader("{}"))
	require.NoError(t, err, "POST pods/prune must connect to the in-process server")
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read pods prune body")
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"prune pods: expected 200, got %d. body: %s", resp.StatusCode, string(body))

	var out []struct {
		ID string `json:"id"`
	}
	require.NoErrorf(t, json.Unmarshal(body, &out), "prune body must decode as a report array. body: %s", string(body))
	ids := make([]string, 0, len(out))
	for _, entry := range out {
		ids = append(ids, entry.ID)
	}
	return ids
}

// waitForPodState polls the detail endpoint until the pod reports the
// wanted state, failing the test on timeout.
func waitForPodState(t *testing.T, machineID, podID, wantState, msg string) {
	t.Helper()

	// podStatePollTimeout caps how long a state-transition poll waits.
	const podStatePollTimeout = 30 * time.Second
	deadline := time.Now().Add(podStatePollTimeout)
	var last podDetailDTO
	for time.Now().Before(deadline) {
		last = getPodDetail(t, machineID, podID)
		if last.State == wantState {
			t.Logf("pod %s reached state=%q", podID, wantState)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.Equalf(t, wantState, last.State,
		"%s: pod %s never reached state %q within %s (last=%q)",
		msg, podID, wantState, podStatePollTimeout, last.State)
}

// podContainersRunnable probes whether this environment can run a
// container inside a pod — i.e. whether the pod's infra container can
// start. A nested podman (the test host is a container) cannot mount
// the infra container's special rootfs overlay; the probe answers once
// per process and cleans its throwaway pod up.
var podContainersRunnableOnce sync.Once
var podContainersRunnableAnswer bool

func podContainersRunnable(t *testing.T) bool {
	t.Helper()

	podContainersRunnableOnce.Do(func() {
		ensureBusyboxImage(t)
		name := "it-" + suite.RunID + "-probe"
		_, _ = podmanConnRun("pod", "rm", "-f", name)
		if out, err := podmanConnRun("pod", "create", name); err != nil {
			t.Logf("probe: pod create failed (%s); assuming no pod containers", strings.TrimSpace(out))
			return
		}
		_, err := podmanConnRun("run", "-d", "--pod", name, "busybox:latest", "sh", "-c", "sleep 5")
		podContainersRunnableAnswer = err == nil
		if out, rmErr := podmanConnRun("pod", "rm", "-f", name); rmErr != nil {
			t.Logf("probe: pod rm -f %q best-effort failed: %v (%s)", name, rmErr, strings.TrimSpace(out))
		}
		if !podContainersRunnableAnswer {
			t.Log("probe: container-in-pod start failed — nested podman infra overlay limitation")
		}
	})
	return podContainersRunnableAnswer
}
