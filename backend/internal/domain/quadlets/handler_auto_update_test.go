package quadlets

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/sshexec"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// parseNDJSON decodes the streamed response body into events.
func parseNDJSON(t *testing.T, body string) []AutoUpdateEvent {
	t.Helper()
	var events []AutoUpdateEvent
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var event AutoUpdateEvent
		require.NoError(t, json.Unmarshal(line, &event), "each NDJSON line must be a valid event: %s", line)
		events = append(events, event)
	}
	require.NoError(t, scanner.Err())
	return events
}

func newAutoUpdateTestServer(t *testing.T, sshExec sshexec.SSHExecInterface) (*chi.Mux, *httptest.ResponseRecorder) {
	t.Helper()
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
	}
	provider := fakeEngineProvider{engine: engine}
	service := NewQuadletService(provider, sshExec, lookupForUser("core"))
	handler := NewQuadletHandler(service, NewQuadletTransferService(provider, nil, nil, 0, service))
	r := chi.NewRouter()
	r.Post("/api/machines/{id}/quadlets/auto-update", handler.AutoUpdate)
	rec := httptest.NewRecorder()
	return r, rec
}

func autoUpdateRequest(t *testing.T, url string) *http.Request {
	t.Helper()
	ctx := context.Background()
	ctx = contextutils.SetUserID(ctx, "test-user")
	ctx = contextutils.SetRequestID(ctx, "test-request-id")
	req := httptest.NewRequest("POST", url, nil)
	return req.WithContext(ctx)
}

// reportJSON is a podman auto-update --format json report naming one
// updated container.
const reportJSON = `[{"Unit":"web.service","ContainerName":"web","ImageName":"nginx:latest","Updated":"true"}]`

func TestQuadletHandler_AutoUpdate_SuccessStream(t *testing.T) {
	sshExec := testutils.NewMockSSHExecWithStreamFunc(func(_ context.Context, _, _, _ string, _ sshexec.ExecOptions, onLine func(sshexec.OutputLine)) (sshexec.ExecResult, error) {
		onLine(sshexec.OutputLine{Stream: sshexec.StdoutStream, Text: "Pulling nginx:latest"})
		onLine(sshexec.OutputLine{Stream: sshexec.StdoutStream, Text: reportJSON})
		return sshexec.ExecResult{ExitCode: 0, Stdout: "Pulling nginx:latest\n" + reportJSON}, nil
	})

	router, rec := newAutoUpdateTestServer(t, sshExec)
	router.ServeHTTP(rec, autoUpdateRequest(t, "/api/machines/machine-1/quadlets/auto-update"))

	assert.Equal(t, http.StatusOK, rec.Code)

	events := parseNDJSON(t, rec.Body.String())
	require.Len(t, events, 4, "the plan event, two streamed log lines, and the terminal complete event")

	assert.Equal(t, AutoUpdateEventTypePlan, events[0].Type, "the plan event opens the stream")
	assert.Empty(t, events[0].Plan, "the empty machine answers an empty plan")
	assert.Equal(t, "machine-1", events[0].MachineID)
	assert.NotEmpty(t, events[0].OperationID)
	assert.NotEmpty(t, events[0].Timestamp)

	assert.Equal(t, AutoUpdateEventTypeLog, events[1].Type)
	assert.Equal(t, "Pulling nginx:latest", events[1].Line)
	assert.Equal(t, "stdout", events[1].Stream)
	assert.Equal(t, "machine-1", events[1].MachineID)
	assert.NotEmpty(t, events[1].OperationID)
	assert.NotEmpty(t, events[1].Timestamp)

	assert.Equal(t, AutoUpdateEventTypeLog, events[2].Type, "the machine's report line arrives as a log event too")

	assert.Equal(t, AutoUpdateEventTypeDone, events[3].Type)
	require.NotNil(t, events[3].Report)
	assert.Equal(t, AutoUpdateCompleted, events[3].Report.Overall)
	require.Len(t, events[3].Report.Containers, 1)
	assert.Equal(t, "web", events[3].Report.Containers[0].ContainerName)
	assert.Nil(t, events[3].Error)

	// All events share one operation ID and the terminal event is last.
	for _, event := range events {
		assert.Equal(t, events[0].OperationID, event.OperationID)
	}
}

func TestQuadletHandler_AutoUpdate_PartialFailureCompletes(t *testing.T) {
	failureJSON := `[{"Unit":"db.service","ContainerName":"db","ImageName":"postgres:16","Updated":"false","Error":"database is locked"}]`
	sshExec := testutils.NewMockSSHExecWithResult(sshexec.ExecResult{ExitCode: 3, Stdout: failureJSON})

	router, rec := newAutoUpdateTestServer(t, sshExec)
	router.ServeHTTP(rec, autoUpdateRequest(t, "/api/machines/machine-1/quadlets/auto-update"))

	assert.Equal(t, http.StatusOK, rec.Code)

	events := parseNDJSON(t, rec.Body.String())
	require.Len(t, events, 3, "the plan event, the streamed report line, and the terminal complete event")
	assert.Equal(t, AutoUpdateEventTypePlan, events[0].Type)
	assert.Equal(t, AutoUpdateEventTypeLog, events[1].Type)

	assert.Equal(t, AutoUpdateEventTypeDone, events[2].Type, "partial failure is data in the complete event, not an error")
	require.NotNil(t, events[2].Report)
	assert.Equal(t, AutoUpdateWithFailures, events[2].Report.Overall)
	assert.Equal(t, AutoUpdateItemFailed, events[2].Report.Containers[0].Updated)
	assert.Equal(t, "database is locked", events[2].Report.Containers[0].Error)
}

func TestQuadletHandler_AutoUpdate_UnparsableReportCompletes(t *testing.T) {
	sshExec := testutils.NewMockSSHExecWithStreamFunc(func(_ context.Context, _, _, _ string, _ sshexec.ExecOptions, onLine func(sshexec.OutputLine)) (sshexec.ExecResult, error) {
		onLine(sshexec.OutputLine{Stream: sshexec.StdoutStream, Text: "not json"})
		return sshexec.ExecResult{ExitCode: 0, Stdout: "not json"}, nil
	})

	router, rec := newAutoUpdateTestServer(t, sshExec)
	router.ServeHTTP(rec, autoUpdateRequest(t, "/api/machines/machine-1/quadlets/auto-update"))

	events := parseNDJSON(t, rec.Body.String())
	require.Len(t, events, 3)
	assert.Equal(t, AutoUpdateEventTypePlan, events[0].Type)
	assert.Equal(t, AutoUpdateEventTypeLog, events[1].Type, "raw output arrives via log events")
	assert.Equal(t, "not json", events[1].Line)
	assert.Equal(t, AutoUpdateEventTypeDone, events[2].Type)
	require.NotNil(t, events[2].Report)
	assert.Equal(t, AutoUpdateUnparsable, events[2].Report.Overall)
}

// TestQuadletHandler_AutoUpdate_DryRunForwarded pins the dryRun query
// decoding: the flag must reach the machine as the CLI's --dry-run argument.
func TestQuadletHandler_AutoUpdate_DryRunForwarded(t *testing.T) {
	var gotOpts sshexec.ExecOptions
	sshExec := testutils.NewMockSSHExecWithStreamFunc(func(_ context.Context, _, _, _ string, opts sshexec.ExecOptions, _ func(sshexec.OutputLine)) (sshexec.ExecResult, error) {
		gotOpts = opts
		return sshexec.ExecResult{ExitCode: 0, Stdout: "[]"}, nil
	})

	router, rec := newAutoUpdateTestServer(t, sshExec)
	router.ServeHTTP(rec, autoUpdateRequest(t, "/api/machines/machine-1/quadlets/auto-update?dryRun=true"))

	assert.Contains(t, gotOpts.Args, "--dry-run", "dryRun=true must reach the machine as the CLI flag")

	events := parseNDJSON(t, rec.Body.String())
	require.Len(t, events, 2, "the plan event plus the terminal complete event")
	assert.Equal(t, AutoUpdateEventTypePlan, events[0].Type)
	require.NotNil(t, events[1].Report)
	assert.Equal(t, AutoUpdateDryRun, events[1].Report.Overall)
}

// TestQuadletHandler_AutoUpdate_FilterForwarded pins the filter query
// decoding under the write gate: the unit name must reach the machine as
// podman auto-update's positional argument, composing with --dry-run, and
// an absent or empty filter must behave exactly as without one.
func TestQuadletHandler_AutoUpdate_FilterForwarded(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		wantArgs []string
	}{
		{name: "filter forwarded as positional arg", query: "?filter=web.service", wantArgs: []string{"auto-update", "--format", "json", "web.service"}},
		{name: "filter composes with dry-run", query: "?filter=web.service&dryRun=true", wantArgs: []string{"auto-update", "--format", "json", "--dry-run", "web.service"}},
		{name: "absent filter behaves as today", query: "", wantArgs: []string{"auto-update", "--format", "json"}},
		{name: "empty filter behaves as today", query: "?filter=", wantArgs: []string{"auto-update", "--format", "json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			machineRepo := testutils.NewMockMachineRepository()
			machineRepo.AddMachine(&models.Machine{ID: "machine-1", Name: "machine-one", OwnerID: "owner-1"})
			accessRepo := testutils.NewMockMachineAccessRepository()
			accessRepo.AddAccess("machine-1", "grantee-1", models.PermissionWrite, time.Now())
			var gotOpts sshexec.ExecOptions
			sshExec := testutils.NewMockSSHExecWithStreamFunc(func(_ context.Context, _, _, _ string, opts sshexec.ExecOptions, _ func(sshexec.OutputLine)) (sshexec.ExecResult, error) {
				gotOpts = opts
				return sshexec.ExecResult{ExitCode: 0, Stdout: "[]"}, nil
			})
			engine := &fakeQuadletEngine{
				listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
					return nil, nil
				},
			}
			service := NewQuadletService(fakeEngineProvider{engine: engine}, sshExec, lookupForUser("core"))
			router := autoUpdateAccessRouter(machineRepo, accessRepo, service)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, requestAsUserForAutoUpdate(http.MethodPost, "/api/machines/machine-1/quadlets/auto-update"+tt.query, "grantee-1"))

			assert.Equal(t, http.StatusOK, w.Result().StatusCode, "write-level grantee must pass the gate")
			assert.Equal(t, tt.wantArgs, gotOpts.Args, "query %q must assemble the podman args", tt.query)
		})
	}
}

// --- Route wiring: the quadlet subtree gates auto-update at write level ---
//
// These tests mount the auto-update route on a chi subtree exactly the way
// internal/app/app.go does (RequireMachineAccess at /{id},
// RequireMachinePermission(write) on the POST) so the 403/404 access
// semantics the ACs promise are exercised through the real middleware.

func autoUpdateAccessRouter(machineRepo *testutils.MockMachineRepository, accessRepo *testutils.MockMachineAccessRepository, service *QuadletService) http.Handler {
	// The auto-update route never crosses the transfer module; the handler
	// is still constructed over both services, with an unused engine
	// provider for the transfer half.
	transfer := NewQuadletTransferService(fakeEngineProvider{}, nil, nil, 0, service)
	r := chi.NewRouter()
	r.Route("/api/machines", func(r chi.Router) {
		r.With(appmiddleware.RequireMachineAccess(machineRepo, accessRepo)).Route("/{id}", func(r chi.Router) {
			requireWrite := appmiddleware.RequireMachinePermission(machineRepo, accessRepo, models.PermissionWrite)
			r.Route("/quadlets", func(r chi.Router) {
				r.With(requireWrite).Post("/auto-update", NewQuadletHandler(service, transfer).AutoUpdate)
			})
		})
	})
	return r
}

func TestQuadletHandler_AutoUpdate_WriteGate(t *testing.T) {
	tests := []struct {
		name         string
		level        string
		expectStatus int
		expectCalled bool
	}{
		{name: "read-level grantee is denied", level: models.PermissionRead, expectStatus: http.StatusForbidden, expectCalled: false},
		{name: "write-level grantee passes", level: models.PermissionWrite, expectStatus: http.StatusOK, expectCalled: true},
		{name: "admin-level grantee passes", level: models.PermissionAdmin, expectStatus: http.StatusOK, expectCalled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			machineRepo := testutils.NewMockMachineRepository()
			machineRepo.AddMachine(&models.Machine{ID: "machine-1", Name: "machine-one", OwnerID: "owner-1"})
			accessRepo := testutils.NewMockMachineAccessRepository()
			accessRepo.AddAccess("machine-1", "grantee-1", tt.level, time.Now())
			called := false
			sshExec := testutils.NewMockSSHExecWithStreamFunc(func(_ context.Context, _, _, _ string, _ sshexec.ExecOptions, _ func(sshexec.OutputLine)) (sshexec.ExecResult, error) {
				called = true
				return sshexec.ExecResult{ExitCode: 0, Stdout: "[]"}, nil
			})
			engine := &fakeQuadletEngine{
				listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
					return nil, nil
				},
			}
			service := NewQuadletService(fakeEngineProvider{engine: engine}, sshExec, lookupForUser("core"))
			router := autoUpdateAccessRouter(machineRepo, accessRepo, service)

			req := httptest.NewRequest("POST", "/api/machines/machine-1/quadlets/auto-update", nil)
			req = req.WithContext(contextutils.SetUserID(req.Context(), "grantee-1"))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectStatus, w.Result().StatusCode, "level %s must answer %d", tt.level, tt.expectStatus)
			assert.Equal(t, tt.expectCalled, called, "handler must run=%v for level %s", tt.expectCalled, tt.level)
		})
	}
}

func TestQuadletHandler_AutoUpdate_UnknownMachineIsNotFound(t *testing.T) {
	called := false
	sshExec := testutils.NewMockSSHExecWithStreamFunc(func(_ context.Context, _, _, _ string, _ sshexec.ExecOptions, _ func(sshexec.OutputLine)) (sshexec.ExecResult, error) {
		called = true
		return sshexec.ExecResult{ExitCode: 0, Stdout: "[]"}, nil
	})
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
	}
	router := autoUpdateAccessRouter(testutils.NewMockMachineRepository(), testutils.NewMockMachineAccessRepository(),
		NewQuadletService(fakeEngineProvider{engine: engine}, sshExec, lookupForUser("core")))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, requestAsUserForAutoUpdate(http.MethodPost, "/api/machines/no-such-machine/quadlets/auto-update", "owner-1"))

	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode, "unknown machine must be rejected with 404")
	assert.Contains(t, w.Body.String(), "MACHINE_NOT_FOUND", "error body should carry the MACHINE_NOT_FOUND code")
	assert.False(t, called, "handler must not run for an unknown machine")
}

func TestQuadletHandler_AutoUpdate_UserWithoutAccessIsForbidden(t *testing.T) {
	called := false
	sshExec := testutils.NewMockSSHExecWithStreamFunc(func(_ context.Context, _, _, _ string, _ sshexec.ExecOptions, _ func(sshexec.OutputLine)) (sshexec.ExecResult, error) {
		called = true
		return sshexec.ExecResult{ExitCode: 0, Stdout: "[]"}, nil
	})
	machineRepo := testutils.NewMockMachineRepository()
	machineRepo.AddMachine(&models.Machine{ID: "machine-1", Name: "machine-one", OwnerID: "owner-1"})
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
	}
	router := autoUpdateAccessRouter(machineRepo, testutils.NewMockMachineAccessRepository(),
		NewQuadletService(fakeEngineProvider{engine: engine}, sshExec, lookupForUser("core")))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, requestAsUserForAutoUpdate(http.MethodPost, "/api/machines/machine-1/quadlets/auto-update", "stranger-1"))

	assert.Equal(t, http.StatusForbidden, w.Result().StatusCode, "caller without an access row must be rejected with 403")
	assert.Contains(t, w.Body.String(), "ACCESS_DENIED", "the deny must carry the ACCESS_DENIED code")
	assert.False(t, called, "handler must not run without an access row")
}

func requestAsUserForAutoUpdate(method, target, userID string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	return req.WithContext(contextutils.SetUserID(req.Context(), userID))
}

// --- The plan and pulling events through the HTTP surface (issue 448) ---

// autoUpdatePlanTestServer mounts the auto-update route over the plan
// fixture engine: web.container carries AutoUpdate=registry with a short
// image name, broken.container is unreadable (the unit-only degradation).
func autoUpdatePlanTestServer(t *testing.T, sshExec sshexec.SSHExecInterface) (*chi.Mux, *httptest.ResponseRecorder) {
	t.Helper()
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/etc/containers/systemd/web.container"},
				{Name: "broken.container", UnitName: "broken.service", Path: "/etc/containers/systemd/broken.container"},
			}, nil
		},
		printFn: func(name string) (string, error) {
			if name == "web.container" {
				return "[Container]\nImage=nginx:latest\nAutoUpdate=registry\n", nil
			}
			return "", errors.New("unreadable file")
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, sshExec, lookupForUser("core"))
	transfer := NewQuadletTransferService(fakeEngineProvider{}, nil, nil, 0, service)
	r := chi.NewRouter()
	r.Post("/api/machines/{id}/quadlets/auto-update", NewQuadletHandler(service, transfer).AutoUpdate)
	rec := httptest.NewRecorder()
	return r, rec
}

// TestQuadletHandler_AutoUpdate_PlanPullingLogCompleteSequence pins the
// emitted event sequence over the fake SSH engine: the plan event opens
// the stream (the unreadable file degraded to a unit-only entry), a
// matching "Trying to pull" line emits a pulling event ahead of its own
// log event, an unmatched image emits nothing but its log event, and the
// complete event terminates the stream — every event stamped with one
// operation ID and the machine ID.
func TestQuadletHandler_AutoUpdate_PlanPullingLogCompleteSequence(t *testing.T) {
	sshExec := testutils.NewMockSSHExecWithStreamFunc(func(_ context.Context, _, _, _ string, _ sshexec.ExecOptions, onLine func(sshexec.OutputLine)) (sshexec.ExecResult, error) {
		onLine(sshexec.OutputLine{Stream: sshexec.StdoutStream, Text: "Trying to pull docker.io/library/nginx:latest..."})
		onLine(sshexec.OutputLine{Stream: sshexec.StdoutStream, Text: "Trying to pull quay.io/unrelated:1..."})
		onLine(sshexec.OutputLine{Stream: sshexec.StdoutStream, Text: reportJSON})
		return sshexec.ExecResult{ExitCode: 0, Stdout: reportJSON}, nil
	})

	router, rec := autoUpdatePlanTestServer(t, sshExec)
	router.ServeHTTP(rec, autoUpdateRequest(t, "/api/machines/machine-1/quadlets/auto-update"))

	assert.Equal(t, http.StatusOK, rec.Code)

	events := parseNDJSON(t, rec.Body.String())
	require.Len(t, events, 6, "plan, pulling, three log events, and the terminal complete event")

	assert.Equal(t, AutoUpdateEventTypePlan, events[0].Type, "the plan event opens the stream")
	assert.Equal(t, []AutoUpdatePlanItem{
		{Unit: "web.service", Image: "nginx:latest"},
		{Unit: "broken.service"},
	}, events[0].Plan, "the unreadable file degrades to a unit-only plan entry")

	assert.Equal(t, AutoUpdateEventTypePulling, events[1].Type, "a matching pull line emits a pulling event")
	assert.Equal(t, "web.service", events[1].Unit)
	assert.Equal(t, "docker.io/library/nginx:latest", events[1].Image)

	assert.Equal(t, AutoUpdateEventTypeLog, events[2].Type, "the pull line itself still arrives as a log event")
	assert.Equal(t, "Trying to pull docker.io/library/nginx:latest...", events[2].Line)

	assert.Equal(t, AutoUpdateEventTypeLog, events[3].Type, "an unmatched image emits no pulling event, only its log event")
	assert.Equal(t, "Trying to pull quay.io/unrelated:1...", events[3].Line)

	assert.Equal(t, AutoUpdateEventTypeLog, events[4].Type)

	assert.Equal(t, AutoUpdateEventTypeDone, events[5].Type, "the complete event terminates the stream")
	require.NotNil(t, events[5].Report)
	assert.Equal(t, AutoUpdateCompleted, events[5].Report.Overall)

	for _, event := range events {
		assert.Equal(t, events[0].OperationID, event.OperationID, "every event shares one operation ID")
		assert.Equal(t, "machine-1", event.MachineID, "every event is stamped with the machine ID")
		assert.NotEmpty(t, event.Timestamp)
	}
}

// TestQuadletHandler_AutoUpdate_FilteredRunNarrowsPlan pins the filtered
// run through the HTTP surface: the plan event lists only the filtered
// unit.
func TestQuadletHandler_AutoUpdate_FilteredRunNarrowsPlan(t *testing.T) {
	sshExec := testutils.NewMockSSHExecWithStreamResult(sshexec.ExecResult{ExitCode: 0, Stdout: reportJSON})

	router, rec := autoUpdatePlanTestServer(t, sshExec)
	router.ServeHTTP(rec, autoUpdateRequest(t, "/api/machines/machine-1/quadlets/auto-update?filter=web.service"))

	assert.Equal(t, http.StatusOK, rec.Code)

	events := parseNDJSON(t, rec.Body.String())
	require.Len(t, events, 3, "the narrowed plan event, the streamed report line, and the complete event")
	assert.Equal(t, AutoUpdateEventTypePlan, events[0].Type)
	assert.Equal(t, []AutoUpdatePlanItem{{Unit: "web.service", Image: "nginx:latest"}}, events[0].Plan,
		"a filtered run's plan lists only the filtered unit")
	assert.Equal(t, AutoUpdateEventTypeDone, events[2].Type)
}
