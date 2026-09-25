package quadlets

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/sshexec"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// newTestHandler builds the handler around the real service running over
// the given fake engine (and a passing SSH exec, which the lifecycle
// endpoints run systemctl through). Tests arm only the operations their
// scenario needs; an unarmed operation panics if the handler reaches it.
func newTestHandler(engine *fakeQuadletEngine) *QuadletHandler {
	return NewQuadletHandler(NewQuadletService(fakeEngineProvider{engine: engine}, testutils.NewMockSSHExec()))
}

// newLifecycleTestHandler builds the handler around the real service whose
// systemctl runs through the given SSH exec double.
func newLifecycleTestHandler(engine *fakeQuadletEngine, sshExec sshexec.SSHExecInterface) *QuadletHandler {
	return NewQuadletHandler(NewQuadletService(fakeEngineProvider{engine: engine}, sshExec))
}

// quadletFixture builds a ListQuadlet like podman's list answer: the name,
// an install path (whose extension drives the resource derivation), and a
// systemd status string.
func quadletFixture(name, path, status string) *entities.ListQuadlet {
	return &entities.ListQuadlet{Name: name, UnitName: unitNameFor(name), Path: path, Status: status}
}

// unitNameFor derives the systemd unit name the production quadlet list
// reports for a fixture.
func unitNameFor(name string) string {
	if idx := strings.LastIndex(name, "."); idx > 0 {
		return name[:idx] + ".service"
	}
	return name + ".service"
}

func listFixture() []*entities.ListQuadlet {
	return []*entities.ListQuadlet{
		quadletFixture("my-container.container", "/etc/containers/systemd/my-container.container", "running"),
		quadletFixture("my-pod.pod", "/etc/containers/systemd/my-pod.pod", "stopped"),
	}
}

// quadletRequest builds a request carrying the identity and chi URL params
// the mounted route would extract.
func quadletRequest(method, target, userID, machineID, quadletID string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", machineID)
	if quadletID != "" {
		rctx.URLParams.Add("quadletId", quadletID)
	}
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, contextutils.UserIDContextKey, userID)
	return req.WithContext(ctx)
}

func TestQuadletHandler_List(t *testing.T) {
	tests := []struct {
		name            string
		queryParams     string
		machineQuadlets []*entities.ListQuadlet
		wantIDs         []string
		expectedStatus  int
	}{
		{
			name:            "successful list",
			queryParams:     "",
			machineQuadlets: listFixture(),
			wantIDs:         []string{"my-container.container", "my-pod.pod"},
			expectedStatus:  http.StatusOK,
		},
		{
			name:            "successful list empty",
			queryParams:     "",
			machineQuadlets: nil,
			wantIDs:         []string{},
			expectedStatus:  http.StatusOK,
		},
		{
			name:            "filter by resource",
			queryParams:     "?resource=container",
			machineQuadlets: listFixture(),
			wantIDs:         []string{"my-container.container"},
			expectedStatus:  http.StatusOK,
		},
		{
			name:            "filter by name",
			queryParams:     "?name=pod",
			machineQuadlets: listFixture(),
			wantIDs:         []string{"my-pod.pod"},
			expectedStatus:  http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &fakeQuadletEngine{
				listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
					return tt.machineQuadlets, nil
				},
			}

			req := quadletRequest("GET", "/api/machines/machine123/quadlets"+tt.queryParams, "user123", "machine123", "", nil)

			rr := httptest.NewRecorder()
			newTestHandler(engine).List(rr, req)

			require.Equal(t, tt.expectedStatus, rr.Code)

			var got []QuadletListResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
			ids := make([]string, 0, len(got))
			for _, q := range got {
				ids = append(ids, q.ID)
			}
			assert.ElementsMatch(t, tt.wantIDs, ids)
			for _, q := range got {
				assert.NotEmpty(t, q.Resource, "resource must be derived from the install path")
				assert.NotEmpty(t, q.State, "state must be parsed from the status")
			}
		})
	}
}

// TestQuadletHandler_List_PodResourceFields pins the list DTO rendering:
// unit name and parsed state are carried next to the pod resource.
func TestQuadletHandler_List_PodResourceFields(t *testing.T) {
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return listFixture(), nil
		},
	}

	req := quadletRequest("GET", "/api/machines/machine123/quadlets?name=pod", "user123", "machine123", "", nil)

	rr := httptest.NewRecorder()
	newTestHandler(engine).List(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.JSONEq(t, `[{"id":"my-pod.pod","name":"my-pod.pod","path":"/etc/containers/systemd/my-pod.pod","unit":"my-pod.service","resource":"pod","state":"stopped"}]`, rr.Body.String())
}

// TestQuadletHandler_Get pins the inspect contract through the real
// service: the quadlet's metadata (resolved from the machine's list) is
// answered together with the unit file's content (resolved via print).
func TestQuadletHandler_Get(t *testing.T) {
	tests := []struct {
		name           string
		quadletID      string
		content        string
		expectedStatus int
		wantBody       string
	}{
		{
			name:           "successful get",
			quadletID:      "my-container.container",
			content:        "[Unit]\nDescription=My Container\n\n[Container]\nImage=nginx:latest\n",
			expectedStatus: http.StatusOK,
			wantBody:       `{"id":"my-container.container","name":"my-container.container","path":"/etc/containers/systemd/my-container.container","unit":"my-container.service","resource":"container","state":"running","content":"[Unit]\nDescription=My Container\n\n[Container]\nImage=nginx:latest\n"}`,
		},
		{
			name:           "pod resource type",
			quadletID:      "my-pod.pod",
			content:        "[Unit]\nDescription=My Pod\n\n[Pod]\nPodmanArgs=--name=my-pod\n",
			expectedStatus: http.StatusOK,
			wantBody:       `{"id":"my-pod.pod","name":"my-pod.pod","path":"/etc/containers/systemd/my-pod.pod","unit":"my-pod.service","resource":"pod","state":"stopped","content":"[Unit]\nDescription=My Pod\n\n[Pod]\nPodmanArgs=--name=my-pod\n"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &fakeQuadletEngine{
				printFn: func(name string) (string, error) {
					assert.Equal(t, tt.quadletID, name)
					return tt.content, nil
				},
				listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
					return listFixture(), nil
				},
			}

			req := quadletRequest("GET", "/api/machines/machine123/quadlets/"+tt.quadletID, "user123", "machine123", tt.quadletID, nil)

			rr := httptest.NewRecorder()
			newTestHandler(engine).Get(rr, req)

			require.Equal(t, tt.expectedStatus, rr.Code)
			assert.JSONEq(t, tt.wantBody, rr.Body.String())
		})
	}

	t.Run("missing quadlet ID", func(t *testing.T) {
		engine := &fakeQuadletEngine{}

		req := quadletRequest("GET", "/api/machines/machine123/quadlets/", "user123", "machine123", "", nil)

		rr := httptest.NewRecorder()
		newTestHandler(engine).Get(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

// TestQuadletHandler_Create pins the create contract through the real
// service: the decoded request is installed under the derived filename and
// the response echoes the created quadlet.
func TestQuadletHandler_Create_Cases(t *testing.T) {
	successBody := `{"name":"my-container","resource":"container","content":"[Container]\nImage=nginx:latest\n"}`

	t.Run("successful create", func(t *testing.T) {
		var gotFileName string
		var gotContent []byte
		var gotReplace bool
		engine := &fakeQuadletEngine{
			installFn: func(fileName string, content []byte, replace bool) (*entities.QuadletInstallReport, error) {
				gotFileName, gotContent, gotReplace = fileName, content, replace
				return &entities.QuadletInstallReport{
					InstalledQuadlets: map[string]string{"src": "/etc/containers/systemd/my-container.container"},
				}, nil
			},
		}

		req := quadletRequest("POST", "/api/machines/machine123/quadlets", "user123", "machine123", "", bytes.NewBufferString(successBody))

		rr := httptest.NewRecorder()
		newTestHandler(engine).Create(rr, req)

		require.Equal(t, http.StatusCreated, rr.Code)
		assert.Equal(t, "my-container.container", gotFileName, "the resource extension must complete the install name")
		assert.Equal(t, "[Container]\nImage=nginx:latest\n", string(gotContent), "the content must be installed byte-identical")
		assert.False(t, gotReplace, "a create must not replace an existing installation")
		assert.JSONEq(t, `{"id":"my-container","name":"my-container","path":"/etc/containers/systemd/my-container.container","resource":"container","state":"unknown","content":"[Container]\nImage=nginx:latest\n"}`, rr.Body.String())
	})

	for _, tc := range []struct {
		name        string
		requestBody string
	}{
		{name: "malformed JSON body", requestBody: `{not valid json`},
		{name: "validation failure: missing name", requestBody: `{"resource":"container","content":"y"}`},
		{name: "validation failure: invalid resource", requestBody: `{"name":"x","resource":"kube","content":"y"}`},
		{name: "validation failure: missing content", requestBody: `{"name":"x","resource":"container"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &fakeQuadletEngine{}

			req := quadletRequest("POST", "/api/machines/machine123/quadlets", "user123", "machine123", "", bytes.NewBufferString(tc.requestBody))

			rr := httptest.NewRecorder()
			newTestHandler(engine).Create(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

// TestQuadletHandler_Update pins the update contract through the real
// service: existence is resolved from the machine's list, the old quadlet
// is removed forced, and the new content is installed with replace under
// the preserved filename.
func TestQuadletHandler_Update(t *testing.T) {
	successBody := `{"content":"[Container]\nImage=nginx:1.25\n"}`

	t.Run("successful update returns 200 with the updated quadlet", func(t *testing.T) {
		var removedName string
		var removedForce bool
		var gotFileName string
		var gotContent []byte
		var gotReplace bool
		engine := &fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return listFixture(), nil
			},
			removeFn: func(name string, force bool) (*entities.QuadletRemoveReport, error) {
				removedName, removedForce = name, force
				return &entities.QuadletRemoveReport{}, nil
			},
			installFn: func(fileName string, content []byte, replace bool) (*entities.QuadletInstallReport, error) {
				gotFileName, gotContent, gotReplace = fileName, content, replace
				return &entities.QuadletInstallReport{
					InstalledQuadlets: map[string]string{"src": "/etc/containers/systemd/my-container.container"},
				}, nil
			},
		}

		req := quadletRequest("PUT", "/api/machines/machine123/quadlets/my-container.container", "user123", "machine123", "my-container.container", bytes.NewBufferString(successBody))

		rr := httptest.NewRecorder()
		newTestHandler(engine).Update(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "my-container.container", removedName)
		assert.True(t, removedForce, "an update must remove forced so an active resource does not block the replace")
		assert.Equal(t, "my-container.container", gotFileName, "the rebuilt install must keep the original filename")
		assert.Equal(t, "[Container]\nImage=nginx:1.25\n", string(gotContent), "the new content must reach the machine")
		assert.True(t, gotReplace, "an update must install with replace")
		assert.JSONEq(t, `{"id":"my-container.container","name":"my-container.container","path":"/etc/containers/systemd/my-container.container","unit":"my-container.service","resource":"container","state":"unknown","content":"[Container]\nImage=nginx:1.25\n"}`, rr.Body.String())
	})

	for _, tc := range []struct {
		name        string
		quadletID   string
		requestBody string
	}{
		{name: "validation failure: missing content", quadletID: "my-container.container", requestBody: `{}`},
		{name: "validation failure: empty content", quadletID: "my-container.container", requestBody: `{"content":""}`},
		{name: "missing quadlet ID", quadletID: "", requestBody: `{"content":"x"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &fakeQuadletEngine{}

			req := quadletRequest("PUT", "/api/machines/machine123/quadlets/"+tc.quadletID, "user123", "machine123", tc.quadletID, bytes.NewBufferString(tc.requestBody))

			rr := httptest.NewRecorder()
			newTestHandler(engine).Update(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

// TestQuadletHandler_Delete pins the delete contract through the real
// service: the parsed force/stop flags reach the machine's remove call,
// a successful delete answers 204 with an empty body, and a malformed
// force flag is ignored (deletes as unforced).
func TestQuadletHandler_Delete(t *testing.T) {
	tests := []struct {
		name          string
		quadletID     string
		queryParams   string
		expectedForce bool
	}{
		{
			name:          "successful delete returns 204 with empty body",
			quadletID:     "my-container.container",
			queryParams:   "",
			expectedForce: false,
		},
		{
			name:          "force=true query honoured",
			quadletID:     "my-container.container",
			queryParams:   "?force=true",
			expectedForce: true,
		},
		{
			name:          "stop=false query honoured",
			quadletID:     "my-container.container",
			queryParams:   "?stop=false",
			expectedForce: false,
		},
		{
			name:          "force=true&stop=false combined",
			quadletID:     "my-container.container",
			queryParams:   "?force=true&stop=false",
			expectedForce: true,
		},
		{
			name:          "invalid force boolean is ignored (defaults to false)",
			quadletID:     "my-container.container",
			queryParams:   "?force=notabool",
			expectedForce: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedName string
			var capturedForce bool
			engine := &fakeQuadletEngine{
				listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
					// Status must be a non-running state so the
					// running-guard does not refuse the delete.
					return []*entities.ListQuadlet{
						quadletFixture(tt.quadletID, "/etc/containers/systemd/"+tt.quadletID, "stopped"),
					}, nil
				},
				removeFn: func(name string, force bool) (*entities.QuadletRemoveReport, error) {
					capturedName, capturedForce = name, force
					return &entities.QuadletRemoveReport{}, nil
				},
			}

			req := quadletRequest("DELETE", "/api/machines/machine123/quadlets/"+tt.quadletID+tt.queryParams, "user123", "machine123", tt.quadletID, nil)

			rr := httptest.NewRecorder()
			newTestHandler(engine).Delete(rr, req)

			require.Equal(t, http.StatusNoContent, rr.Code)
			assert.Empty(t, rr.Body.String(), "a 204 must carry no body")
			assert.Equal(t, tt.quadletID, capturedName, "quadlet name should be passed through")
			assert.Equal(t, tt.expectedForce, capturedForce, "force flag should match expected")
		})
	}

	t.Run("running quadlet with stop=false is refused with 400", func(t *testing.T) {
		engine := &fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return runningContainerFixture(), nil
			},
			// removeFn stays unarmed: a refused delete must not remove.
		}

		req := quadletRequest("DELETE", "/api/machines/machine123/quadlets/my-container.container?stop=false", "user123", "machine123", "my-container.container", nil)

		rr := httptest.NewRecorder()
		newTestHandler(engine).Delete(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("running quadlet with the default stop deletes with 204", func(t *testing.T) {
		engine := &fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return runningContainerFixture(), nil
			},
			removeFn: func(string, bool) (*entities.QuadletRemoveReport, error) {
				return &entities.QuadletRemoveReport{}, nil
			},
		}

		req := quadletRequest("DELETE", "/api/machines/machine123/quadlets/my-container.container", "user123", "machine123", "my-container.container", nil)

		rr := httptest.NewRecorder()
		newTestHandler(engine).Delete(rr, req)

		require.Equal(t, http.StatusNoContent, rr.Code)
		assert.Empty(t, rr.Body.String(), "a 204 must carry no body")
	})

	t.Run("missing quadlet ID", func(t *testing.T) {
		engine := &fakeQuadletEngine{}

		req := quadletRequest("DELETE", "/api/machines/machine123/quadlets/", "user123", "machine123", "", nil)

		rr := httptest.NewRecorder()
		newTestHandler(engine).Delete(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

// TestQuadletHandler_Start pins the start HTTP contract through the real
// service: a stopped quadlet starts and the response reports the running
// state (the resolved-unit argv is pinned at the applyUnitAction seam).
func TestQuadletHandler_Start(t *testing.T) {
	t.Run("successful start returns 200 with running state", func(t *testing.T) {
		engine := &fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return []*entities.ListQuadlet{{
					Name:     "my-container.container",
					UnitName: "my-container.service",
					Path:     "/etc/containers/systemd/my-container.container",
					Status:   "stopped",
				}}, nil
			},
		}
		sshExec := testutils.NewMockSSHExecWithFunc(func(_ context.Context, _, _, _ string, _ sshexec.ExecOptions) (sshexec.ExecResult, error) {
			return sshexec.ExecResult{ExitCode: 0}, nil
		})

		req := quadletRequest("POST", "/api/machines/machine123/quadlets/my-container.container/start", "user123", "machine123", "my-container.container", nil)

		rr := httptest.NewRecorder()
		newLifecycleTestHandler(engine, sshExec).Start(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		assert.JSONEq(t, `{"id":"my-container.container","state":"running","unit":"my-container.service"}`, rr.Body.String())
	})

	t.Run("missing quadlet ID", func(t *testing.T) {
		engine := &fakeQuadletEngine{}

		req := quadletRequest("POST", "/api/machines/machine123/quadlets//start", "user123", "machine123", "", nil)

		rr := httptest.NewRecorder()
		newTestHandler(engine).Start(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

// runningContainerFixture is the quadlet whose managed resource is up:
// stop and restart precondition on this state.
func runningContainerFixture() []*entities.ListQuadlet {
	return []*entities.ListQuadlet{{
		Name:     "my-container.container",
		UnitName: "my-container.service",
		Path:     "/etc/containers/systemd/my-container.container",
		Status:   "running",
	}}
}

// TestQuadletHandler_Stop pins the stop HTTP contract through the real
// service: force and timeout are accepted from the query, success answers
// 200 with the stopped state, and a timeout expiry answers 504. The argv
// grammar is pinned once at the applyUnitAction seam (lifecycle_test.go).
func TestQuadletHandler_Stop(t *testing.T) {
	tests := []struct {
		name        string
		queryParams string
		execResult  sshexec.ExecResult
		wantStatus  int
	}{
		{
			name:        "successful stop returns 200 with stopped state",
			queryParams: "",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "force=true query honoured",
			queryParams: "?force=true",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "timeout query reaches the wrapper and stops",
			queryParams: "?timeout=10",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "force and timeout combined",
			queryParams: "?force=true&timeout=5",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "invalid timeout is ignored",
			queryParams: "?timeout=notanint",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "negative timeout is ignored",
			queryParams: "?timeout=-1",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "timeout expiry answers 504 (the wait was given up)",
			queryParams: "?timeout=1",
			execResult:  sshexec.ExecResult{ExitCode: 124},
			wantStatus:  http.StatusGatewayTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &fakeQuadletEngine{
				listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
					return runningContainerFixture(), nil
				},
			}
			sshExec := testutils.NewMockSSHExecWithFunc(func(_ context.Context, _, _, _ string, _ sshexec.ExecOptions) (sshexec.ExecResult, error) {
				return tt.execResult, nil
			})

			req := quadletRequest("POST", "/api/machines/machine123/quadlets/my-container.container/stop"+tt.queryParams, "user123", "machine123", "my-container.container", nil)

			rr := httptest.NewRecorder()
			newLifecycleTestHandler(engine, sshExec).Stop(rr, req)

			require.Equal(t, tt.wantStatus, rr.Code)
			if tt.wantStatus == http.StatusOK {
				assert.JSONEq(t, `{"id":"my-container.container","state":"stopped","unit":"my-container.service"}`, rr.Body.String())
			}
		})
	}

	t.Run("missing quadlet ID", func(t *testing.T) {
		engine := &fakeQuadletEngine{}

		req := quadletRequest("POST", "/api/machines/machine123/quadlets//stop", "user123", "machine123", "", nil)

		rr := httptest.NewRecorder()
		newTestHandler(engine).Stop(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

// TestQuadletHandler_Restart pins the restart HTTP contract through the
// real service: the response carries the running state and a restartedAt
// timestamp, and force/timeout are accepted from the query (the timeout
// bounds the wait for the whole restart; its argv is pinned at the
// applyUnitAction seam).
func TestQuadletHandler_Restart(t *testing.T) {
	t.Run("successful restart returns 200 with running state and restartedAt", func(t *testing.T) {
		engine := &fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return runningContainerFixture(), nil
			},
		}
		sshExec := testutils.NewMockSSHExecWithFunc(func(_ context.Context, _, _, _ string, _ sshexec.ExecOptions) (sshexec.ExecResult, error) {
			return sshexec.ExecResult{ExitCode: 0}, nil
		})

		req := quadletRequest("POST", "/api/machines/machine123/quadlets/my-container.container/restart", "user123", "machine123", "my-container.container", nil)

		rr := httptest.NewRecorder()
		newLifecycleTestHandler(engine, sshExec).Restart(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"state":"running"`)
		assert.Contains(t, rr.Body.String(), "restartedAt", "restart response must include restartedAt")
	})

	t.Run("force and timeout accepted from the query", func(t *testing.T) {
		engine := &fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return runningContainerFixture(), nil
			},
		}
		sshExec := testutils.NewMockSSHExecWithFunc(func(_ context.Context, _, _, _ string, _ sshexec.ExecOptions) (sshexec.ExecResult, error) {
			return sshexec.ExecResult{ExitCode: 0}, nil
		})

		req := quadletRequest("POST", "/api/machines/machine123/quadlets/my-container.container/restart?force=true&timeout=20", "user123", "machine123", "my-container.container", nil)

		rr := httptest.NewRecorder()
		newLifecycleTestHandler(engine, sshExec).Restart(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"state":"running"`)
	})

	t.Run("missing quadlet ID", func(t *testing.T) {
		engine := &fakeQuadletEngine{}

		req := quadletRequest("POST", "/api/machines/machine123/quadlets//restart", "user123", "machine123", "", nil)

		rr := httptest.NewRecorder()
		newTestHandler(engine).Restart(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

// TestQuadletHandler_Generate pins the generate contract through the real
// service: an existing container or pod on the machine is inspected and
// converted into a quadlet unit file.
func TestQuadletHandler_Generate(t *testing.T) {
	t.Run("successful generate from container", func(t *testing.T) {
		var gotResourceID string
		engine := &fakeQuadletEngine{
			inspectContainerFn: func(id string) ([]*entities.ContainerInspectReport, []error, error) {
				gotResourceID = id
				return []*entities.ContainerInspectReport{{
					InspectContainerData: &define.InspectContainerData{
						Name:   "my-container",
						Config: &define.InspectContainerConfig{Image: "nginx:latest"},
					},
				}}, nil, nil
			},
		}

		req := quadletRequest("POST", "/api/machines/machine123/quadlets/generate", "user123", "machine123", "generate",
			bytes.NewBufferString(`{"resourceId":"my-container","resourceType":"container"}`))

		rr := httptest.NewRecorder()
		newTestHandler(engine).Generate(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "my-container", gotResourceID, "the requested resource must be the inspected one")
		assert.Contains(t, rr.Body.String(), `"resource":"container"`)
		assert.Contains(t, rr.Body.String(), "ContainerName=my-container")
		assert.Contains(t, rr.Body.String(), "Image=nginx:latest")
	})

	t.Run("successful generate from pod", func(t *testing.T) {
		engine := &fakeQuadletEngine{
			inspectPodFn: func(string) ([]*entities.PodInspectReport, []error, error) {
				return []*entities.PodInspectReport{{
					InspectPodData: &define.InspectPodData{Name: "my-pod"},
				}}, nil, nil
			},
		}

		req := quadletRequest("POST", "/api/machines/machine123/quadlets/generate", "user123", "machine123", "generate",
			bytes.NewBufferString(`{"resourceId":"my-pod","resourceType":"pod"}`))

		rr := httptest.NewRecorder()
		newTestHandler(engine).Generate(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"resource":"pod"`)
		assert.Contains(t, rr.Body.String(), "PodName=my-pod")
	})

	for _, tc := range []struct {
		name        string
		requestBody string
	}{
		{name: "validation failure: missing resourceId", requestBody: `{"resourceType":"container"}`},
		{name: "validation failure: empty resourceId", requestBody: `{"resourceId":"","resourceType":"container"}`},
		{name: "validation failure: missing resourceType", requestBody: `{"resourceId":"my-container"}`},
		{name: "validation failure: empty resourceType", requestBody: `{"resourceId":"my-container","resourceType":""}`},
		{name: "validation failure: invalid resourceType", requestBody: `{"resourceId":"my-resource","resourceType":"invalid"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &fakeQuadletEngine{}

			req := quadletRequest("POST", "/api/machines/machine123/quadlets/generate", "user123", "machine123", "generate",
				bytes.NewBufferString(tc.requestBody))

			rr := httptest.NewRecorder()
			newTestHandler(engine).Generate(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}
