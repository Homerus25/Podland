//go:build integration

package integration

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/stretchr/testify/require"
)

// Test_Integration_Quadlets_AutoUpdate closes out the quadlet auto-update
// epic (190-04): end-to-end coverage of
// POST /api/machines/{id}/quadlets/auto-update against the real podman
// machine, in --dry-run mode only. Real update runs hit live registries
// and mutate containers — explicitly out of scope for the harness.
//
// podman auto-update only considers units that declare an update policy,
// so the fixture is a quadlet whose .container file carries
// `AutoUpdate=registry`; a policy-less quadlet would make the endpoint a
// silent no-op and every stream assertion vacuous. The fixture is started
// so its systemd unit is active — inactive units are skipped by
// podman auto-update too.
//
// Verified contract:
//
//	unauthenticated request         → 401
//	read-level access               → 403 ACCESS_DENIED (write gate)
//	write access + dryRun=true      → 200 NDJSON stream:
//	                                  zero or more log events, then
//	                                  exactly one terminal complete
//	                                  event with a parseable report
//	dryRun leaves the fixture alone → container still running, image untouched
func Test_Integration_Quadlets_AutoUpdate(t *testing.T) {
	suite.SkipIfUnavailable(t)
	registerQuadletsSweep()

	machineID := suite.MachineID

	// The endpoint shells out to `podman auto-update` over SSH as the
	// machine's SSH user; same transport preconditions as the lifecycle
	// slice (user systemd must be usable for the fixture unit to run).
	requireUserSystemd(t)

	ensureBusyboxImage(t)

	// Run-scoped fixture: file on the remote machine becomes
	// `<name>.container`, the unit `<name>.service`, and the spawned
	// container `systemd-<name>` (podman quadlet naming).
	quadletName := "it-" + suite.RunID + "-autoupd"
	quadletID := quadletName + ".container"

	autoUpdatePath := func(query string) string {
		u := suite.ServerURL + "/api/machines/" + machineID + "/quadlets/auto-update"
		if query != "" {
			u += "?" + query
		}
		return u
	}

	// Per-test cleanup, registered before create so it runs even if a
	// later phase fails. Best-effort; the sweep is the abort backstop.
	t.Cleanup(func() {
		forceDeleteQuadlet(t, machineID, quadletID)
		removeSpawnedContainer(t, quadletName)
	})

	// --- Setup: create + start the auto-update quadlet ---------------
	//
	// AutoUpdate=registry is what makes this unit visible to
	// `podman auto-update`; without it the endpoint is a no-op.
	createQuadlet(t, machineID, quadletCreateBody{
		Name:     quadletName,
		Resource: "container",
		// A policy-less short name ("busybox") is refused at start:
		// auto updates require a fully-qualified image reference.
		Content: `[Container]
Image=docker.io/library/busybox:latest
AutoUpdate=registry
Exec=sh -c "sleep 3600"
`,
	})
	startQuadlet(t, machineID, quadletID)
	waitForQuadletRunning(t, machineID, quadletID,
		"auto-update slice setup: fixture quadlet must be running to be visible to podman auto-update")

	// --- AC: unauthenticated request is rejected ----------------------
	t.Run("UnauthenticatedRejected", func(t *testing.T) {
		resp, err := http.Post(autoUpdatePath("dryRun=true"), "", nil)
		require.NoError(t, err, "unauthenticated POST must reach the in-process server")
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		require.Equalf(t, http.StatusUnauthorized, resp.StatusCode,
			"unauthenticated auto-update must be rejected with 401, got %d. body: %s",
			resp.StatusCode, string(raw))
	})

	// --- AC: read-only access rejected at write level -----------------
	t.Run("ReadOnlyAccessRejected", func(t *testing.T) {
		client := levelUserClient(t, models.PermissionRead)
		resp, err := client.Post(autoUpdatePath("dryRun=true"), "", nil)
		require.NoError(t, err, "read-level POST must reach the in-process server")
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		require.Equalf(t, http.StatusForbidden, resp.StatusCode,
			"read-level auto-update must be rejected with 403, got %d. body: %s",
			resp.StatusCode, string(raw))

		var errResp quadletErrorEnvelope
		require.NoErrorf(t, json.Unmarshal(raw, &errResp),
			"read-level auto-update: error response must decode. body: %s", string(raw))
		require.Equal(t, "ACCESS_DENIED", errResp.Error.Code,
			"read-level auto-update: error code must be ACCESS_DENIED")
	})

	// --- AC: write access streams log events + one terminal complete --
	t.Run("WriteAccessStreamsNDJSONCompleteReport", func(t *testing.T) {
		resp, err := suite.AuthenticatedClient().Post(autoUpdatePath("dryRun=true"), "", nil)
		require.NoError(t, err, "write-level dry-run POST must reach the in-process server")
		defer resp.Body.Close()

		require.Equalf(t, http.StatusOK, resp.StatusCode,
			"dry-run auto-update must answer 200")
		require.Contains(t, resp.Header.Get("Content-Type"), "application/json",
			"the NDJSON stream must be served as JSON (shared SetNDJSONHeaders contract)")

		scanner := bufio.NewScanner(resp.Body)
		var completeEvents, logEvents, errorEvents int
		var lastEventType string
		var report struct {
			Type      string `json:"type"`
			Operation string `json:"operationId"`
			Report    *struct {
				Containers []struct {
					ContainerName string `json:"containerName"`
					ImageName     string `json:"imageName"`
					Updated       string `json:"updated"`
				} `json:"containers"`
				Overall  string `json:"overall"`
				ExitCode int    `json:"exitCode"`
			} `json:"report"`
		}
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			require.NoErrorf(t, json.Unmarshal([]byte(line), &report),
				"every NDJSON line must decode as an event. line: %s", line)
			require.NotEmpty(t, report.Operation, "every event must carry an operationId")
			switch report.Type {
			case "log":
				logEvents++
			case "complete":
				completeEvents++
				require.NotNilf(t, report.Report,
					"the complete event must carry the report. line: %s", line)
			case "error":
				errorEvents++
			default:
				t.Fatalf("unknown event type %q. line: %s", report.Type, line)
			}
			lastEventType = report.Type
		}
		require.NoErrorf(t, scanner.Err(), "scanning the NDJSON stream must not fail")

		// podman auto-update always emits at least some human-readable
		// output before the JSON report, so log events are expected —
		// but the hard contract is the single terminal complete event.
		require.Equalf(t, 0, errorEvents,
			"a dry-run must not terminate with an error event")
		require.Equalf(t, 1, completeEvents,
			"the stream must end with exactly one complete event (log events: %d)", logEvents)
		require.Equal(t, "complete", lastEventType,
			"the complete event must be the last event in the stream")
		require.NotNil(t, report.Report, "the complete event must carry the report")
		require.NotEmpty(t, report.Report.Overall, "the report must carry an overall status")
		// The fixture's container declares AutoUpdate=registry and is
		// running, so it must be visible to podman auto-update's report —
		// the exact name podman derives from the quadlet (systemd-<name>).
		fixtureContainer := "systemd-" + quadletName
		var fixtureListed bool
		for _, c := range report.Report.Containers {
			if c.ContainerName == fixtureContainer {
				fixtureListed = true
			}
		}
		require.Truef(t, fixtureListed,
			"the report must list the fixture container %q — the AutoUpdate=registry policy makes it visible. overall: %q. containers: %+v",
			fixtureContainer, report.Report.Overall, report.Report.Containers)
	})

	// --- AC: dry-run performs no mutation ------------------------------
	t.Run("DryRunLeavesContainerUnmutated", func(t *testing.T) {
		got := getQuadlet(t, machineID, quadletID)
		require.Equal(t, "running", got.State,
			"the dry-run must leave the fixture quadlet running")
		require.Contains(t, got.Content, "Image=docker.io/library/busybox:latest",
			"the dry-run must leave the fixture's content untouched")
	})
}
