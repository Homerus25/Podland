package quadlets

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
	engine := &fakeQuadletEngine{}
	handler := NewQuadletHandler(NewQuadletService(fakeEngineProvider{engine: engine}, sshExec, lookupForUser("core")))
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
	require.Len(t, events, 3, "two streamed log lines plus the terminal complete event")

	assert.Equal(t, AutoUpdateEventTypeLog, events[0].Type)
	assert.Equal(t, "Pulling nginx:latest", events[0].Line)
	assert.Equal(t, "stdout", events[0].Stream)
	assert.Equal(t, "machine-1", events[0].MachineID)
	assert.NotEmpty(t, events[0].OperationID)
	assert.NotEmpty(t, events[0].Timestamp)

	assert.Equal(t, AutoUpdateEventTypeLog, events[1].Type, "the machine's report line arrives as a log event too")

	assert.Equal(t, AutoUpdateEventTypeDone, events[2].Type)
	require.NotNil(t, events[2].Report)
	assert.Equal(t, AutoUpdateCompleted, events[2].Report.Overall)
	require.Len(t, events[2].Report.Containers, 1)
	assert.Equal(t, "web", events[2].Report.Containers[0].ContainerName)
	assert.Nil(t, events[2].Error)

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
	require.Len(t, events, 2, "the streamed report line plus the terminal complete event")
	assert.Equal(t, AutoUpdateEventTypeLog, events[0].Type)

	assert.Equal(t, AutoUpdateEventTypeDone, events[1].Type, "partial failure is data in the complete event, not an error")
	require.NotNil(t, events[1].Report)
	assert.Equal(t, AutoUpdateWithFailures, events[1].Report.Overall)
	assert.Equal(t, AutoUpdateItemFailed, events[1].Report.Containers[0].Updated)
	assert.Equal(t, "database is locked", events[1].Report.Containers[0].Error)
}

func TestQuadletHandler_AutoUpdate_UnparsableReportCompletes(t *testing.T) {
	sshExec := testutils.NewMockSSHExecWithStreamFunc(func(_ context.Context, _, _, _ string, _ sshexec.ExecOptions, onLine func(sshexec.OutputLine)) (sshexec.ExecResult, error) {
		onLine(sshexec.OutputLine{Stream: sshexec.StdoutStream, Text: "not json"})
		return sshexec.ExecResult{ExitCode: 0, Stdout: "not json"}, nil
	})

	router, rec := newAutoUpdateTestServer(t, sshExec)
	router.ServeHTTP(rec, autoUpdateRequest(t, "/api/machines/machine-1/quadlets/auto-update"))

	events := parseNDJSON(t, rec.Body.String())
	require.Len(t, events, 2)
	assert.Equal(t, AutoUpdateEventTypeLog, events[0].Type, "raw output arrives via log events")
	assert.Equal(t, "not json", events[0].Line)
	assert.Equal(t, AutoUpdateEventTypeDone, events[1].Type)
	require.NotNil(t, events[1].Report)
	assert.Equal(t, AutoUpdateUnparsable, events[1].Report.Overall)
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
	require.Len(t, events, 1)
	require.NotNil(t, events[0].Report)
	assert.Equal(t, AutoUpdateDryRun, events[0].Report.Overall)
}

// --- Route wiring: the quadlet subtree gates auto-update at write level ---
//
// These tests mount the auto-update route on a chi subtree exactly the way
// internal/app/app.go does (RequireMachineAccess at /{id},
// RequireMachinePermission(write) on the POST) so the 403/404 access
// semantics the ACs promise are exercised through the real middleware.

func autoUpdateAccessRouter(machineRepo *testutils.MockMachineRepository, accessRepo *testutils.MockMachineAccessRepository, service *QuadletService) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/machines", func(r chi.Router) {
		r.With(appmiddleware.RequireMachineAccess(machineRepo, accessRepo)).Route("/{id}", func(r chi.Router) {
			requireWrite := appmiddleware.RequireMachinePermission(machineRepo, accessRepo, models.PermissionWrite)
			r.Route("/quadlets", func(r chi.Router) {
				r.With(requireWrite).Post("/auto-update", NewQuadletHandler(service).AutoUpdate)
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
			engine := &fakeQuadletEngine{}
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
	engine := &fakeQuadletEngine{}
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
	engine := &fakeQuadletEngine{}
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
