package quadlets

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// moveErrorEnvelope mirrors the {"requestId","error":{code,message}}
// shape the endpoint envelope answers failures with — only the fields the
// assertions consume are named.
type moveErrorEnvelope struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

// newMovePreviewTestHandler builds the handler over the real service with
// per-machine fake engines and a passing access store — the wiring the
// move-preview handler exercises (one lease per machine, plus the target
// gate).
func newMovePreviewTestHandler(sourceEngine, targetEngine *fakeQuadletEngine) *QuadletHandler {
	service := newMovePreviewTestService(sourceEngine, targetEngine,
		&fakeAccessRepo{getPermissionFn: allowTargetRead})
	return NewQuadletHandler(service)
}

func movePreviewRequest(target, userID, machineID, quadletID string) *http.Request {
	return quadletRequest("GET", target, userID, machineID, quadletID, nil)
}

func TestQuadletHandler_MovePreview_ReturnsPreview(t *testing.T) {
	sourceEngine := &fakeQuadletEngine{
		printFn: func(string) (string, error) {
			return movePreviewContent, nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "running"},
			}, nil
		},
	}
	targetEngine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", Path: "/y/web.container", Status: "stopped"},
			}, nil
		},
	}

	req := movePreviewRequest("/api/machines/machine-a/quadlets/web.container/move-preview?targetMachineId=machine-b",
		"user123", "machine-a", "web.container")

	rr := httptest.NewRecorder()
	newMovePreviewTestHandler(sourceEngine, targetEngine).MovePreview(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var got QuadletMovePreviewResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Len(t, got.Volumes, 2)
	assert.Equal(t, "pgdata", got.Volumes[0].Source)
	assert.Equal(t, MovePreviewVolumeKindVolume, got.Volumes[0].Kind)
	assert.Equal(t, "/srv/backup", got.Volumes[1].Source)
	assert.Equal(t, MovePreviewVolumeKindHostPath, got.Volumes[1].Kind)
	assert.True(t, got.Collision, "a target holding the same name and resource collides")
	assert.True(t, got.Running, "running reflects the source quadlet's live state")
}

func TestQuadletHandler_MovePreview_EmptyVolumesRendersAsArray(t *testing.T) {
	sourceEngine := &fakeQuadletEngine{
		printFn: func(string) (string, error) { return "[Container]\nImage=busybox\n", nil },
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"},
			}, nil
		},
	}
	targetEngine := &fakeQuadletEngine{listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
		return nil, nil
	}}

	req := movePreviewRequest("/api/machines/machine-a/quadlets/web.container/move-preview?targetMachineId=machine-b",
		"user123", "machine-a", "web.container")

	rr := httptest.NewRecorder()
	newMovePreviewTestHandler(sourceEngine, targetEngine).MovePreview(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.JSONEq(t, `{"volumes":[],"secrets":[],"collision":false,"running":false}`, rr.Body.String(),
		"a quadlet without Volume= or Secret= lines must render empty JSON arrays, not null")
}

func TestQuadletHandler_MovePreview_MissingQuadletIDIs400(t *testing.T) {
	req := movePreviewRequest("/api/machines/machine-a/quadlets//move-preview?targetMachineId=machine-b",
		"user123", "machine-a", "")

	rr := httptest.NewRecorder()
	newMovePreviewTestHandler(&fakeQuadletEngine{}, &fakeQuadletEngine{}).MovePreview(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQuadletHandler_MovePreview_MissingTargetMachineIDIs400(t *testing.T) {
	req := movePreviewRequest("/api/machines/machine-a/quadlets/web.container/move-preview",
		"user123", "machine-a", "web.container")

	rr := httptest.NewRecorder()
	newMovePreviewTestHandler(&fakeQuadletEngine{}, &fakeQuadletEngine{}).MovePreview(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)

	var errResp moveErrorEnvelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "MISSING_TARGET_MACHINE_ID", errResp.Error.Code)
}

func TestQuadletHandler_MovePreview_TargetAccessDeniedIs403(t *testing.T) {
	// The handler drives the real service, so the 403 comes from the
	// service's target gate: the access store answers with no row and the
	// source engine's list is armed to prove the source lease was never
	// crossed (no false preview).
	sourceEngine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			panic("source engine must not be reached when the target gate refuses")
		},
	}
	service := NewQuadletService(fakeEngineProvider{engines: map[string]QuadletEngine{
		"machine-a": sourceEngine,
	}}, nil, nil).WithAccessRepo(&fakeAccessRepo{getPermissionFn: func(string, string) (string, error) {
		return "", nil
	}})
	handler := NewQuadletHandler(service)

	req := movePreviewRequest("/api/machines/machine-a/quadlets/web.container/move-preview?targetMachineId=machine-b",
		"user123", "machine-a", "web.container")

	rr := httptest.NewRecorder()
	handler.MovePreview(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)

	var errResp moveErrorEnvelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "ACCESS_DENIED", errResp.Error.Code)
}

func TestQuadletHandler_MovePreview_MissingSourceQuadletIs404(t *testing.T) {
	emptyEngine := &fakeQuadletEngine{
		printFn: func(string) (string, error) { return "", nil },
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
	}
	targetEngine := &fakeQuadletEngine{listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
		return nil, nil
	}}

	req := movePreviewRequest("/api/machines/machine-a/quadlets/gone.container/move-preview?targetMachineId=machine-b",
		"user123", "machine-a", "gone.container")

	rr := httptest.NewRecorder()
	newMovePreviewTestHandler(emptyEngine, targetEngine).MovePreview(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)

	var errResp moveErrorEnvelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "QUADLET_NOT_FOUND", errResp.Error.Code)
}

// --- The transfer handler (copy mode, issue 210) ---

// newMoveTestHandler builds the handler over the real service with
// per-machine fake engines, a write-allowing access store, and a succeeding
// SSH exec — the wiring the transfer handler exercises (the target gate,
// both engine leases, and the post-install reload on the target).
func newMoveTestHandler(sourceEngine, targetEngine *fakeQuadletEngine) *QuadletHandler {
	service := newMoveTransferTestService(sourceEngine, targetEngine,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite}, testutils.NewMockSSHExec())
	return NewQuadletHandler(service)
}

// moveTransferSourceFixture is the source lease's answer: one stopped
// container quadlet with printable content.
func moveTransferSourceFixture() *fakeQuadletEngine {
	return moveSourceFixture("web.container", "/x/web.container", "stopped", moveTransferContent)
}

// moveRequest builds a POST .../move request with the given JSON body.
func moveRequest(target string, body string) *http.Request {
	return quadletRequest("POST", target, "user123", "machine-a", "web.container", strings.NewReader(body))
}

const moveEndpoint = "/api/machines/machine-a/quadlets/web.container/move"

func TestQuadletHandler_Move_ReturnsCopiedEnvelope(t *testing.T) {
	targetEngine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return &entities.QuadletInstallReport{
				InstalledQuadlets: map[string]string{"/tmp/web.container": "/etc/containers/systemd/web.container"},
			}, nil
		},
	}

	req := moveRequest(moveEndpoint, `{"targetMachineId":"machine-b","mode":"copy"}`)

	rr := httptest.NewRecorder()
	newMoveTestHandler(moveTransferSourceFixture(), targetEngine).Move(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var got QuadletMoveResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, "web.container", got.ID)
	assert.Equal(t, "container", got.Resource)
	assert.Equal(t, MoveStatusCopied, got.Status)
	assert.Equal(t, "machine-a", got.SourceMachineID)
	assert.Equal(t, "machine-b", got.TargetMachineID)
	assert.Equal(t, "/etc/containers/systemd/web.container", got.Path)
}

func TestQuadletHandler_Move_MissingQuadletIDIs400(t *testing.T) {
	req := quadletRequest("POST", "/api/machines/machine-a/quadlets//move", "user123", "machine-a", "",
		strings.NewReader(`{"targetMachineId":"machine-b","mode":"copy"}`))

	rr := httptest.NewRecorder()
	newMoveTestHandler(&fakeQuadletEngine{}, &fakeQuadletEngine{}).Move(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQuadletHandler_Move_BodyValidationIs400(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "malformed JSON", body: `{`, want: "INVALID_REQUEST"},
		{name: "missing targetMachineId", body: `{"mode":"copy"}`, want: "VALIDATION_FAILED"},
		{name: "mode other than copy/move", body: `{"targetMachineId":"machine-b","mode":"teleport"}`, want: "VALIDATION_FAILED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := moveRequest(moveEndpoint, tt.body)

			rr := httptest.NewRecorder()
			newMoveTestHandler(&fakeQuadletEngine{}, &fakeQuadletEngine{}).Move(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code)

			var errResp moveErrorEnvelope
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
			assert.Equal(t, tt.want, errResp.Error.Code)
		})
	}
}

func TestQuadletHandler_Move_TargetAccessDeniedIs403(t *testing.T) {
	// The handler drives the real service, so the 403 comes from the
	// service's target gate: the access store answers read on the target
	// (below the write the transfer needs) and the source engine's list is
	// armed to panic, proving no podman work preceded the refusal.
	sourceEngine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			panic("source engine must not be reached when the target gate refuses")
		},
	}
	service := NewQuadletService(fakeEngineProvider{engines: map[string]QuadletEngine{
		"machine-a": sourceEngine,
	}}, testutils.NewMockSSHExec(), lookupForUser("core")).WithAccessRepo(&fakeAccessRepo{
		getPermissionFn: func(machineID, _ string) (string, error) {
			if machineID == "machine-b" {
				return models.PermissionRead, nil
			}
			return models.PermissionAdmin, nil
		},
	})
	handler := NewQuadletHandler(service)

	req := moveRequest(moveEndpoint, `{"targetMachineId":"machine-b","mode":"copy"}`)

	rr := httptest.NewRecorder()
	handler.Move(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)

	var errResp moveErrorEnvelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "ACCESS_DENIED", errResp.Error.Code)
}

func TestQuadletHandler_Move_CollisionIs409(t *testing.T) {
	targetEngine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", Path: "/y/web.container", Status: "stopped"},
			}, nil
		},
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			panic("a colliding copy must not install")
		},
	}

	req := moveRequest(moveEndpoint, `{"targetMachineId":"machine-b","mode":"copy"}`)

	rr := httptest.NewRecorder()
	newMoveTestHandler(moveTransferSourceFixture(), targetEngine).Move(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code)

	var errResp moveErrorEnvelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "QUADLET_ALREADY_EXISTS", errResp.Error.Code)
}

func TestQuadletHandler_Move_MissingSourceQuadletIs404(t *testing.T) {
	emptyEngine := &fakeQuadletEngine{
		printFn: func(string) (string, error) { return "", nil },
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
	}

	req := quadletRequest("POST", "/api/machines/machine-a/quadlets/gone.container/move", "user123", "machine-a", "gone.container",
		strings.NewReader(`{"targetMachineId":"machine-b","mode":"copy"}`))

	rr := httptest.NewRecorder()
	newMoveTestHandler(emptyEngine, &fakeQuadletEngine{}).Move(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)

	var errResp moveErrorEnvelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "QUADLET_NOT_FOUND", errResp.Error.Code)
}

// --- The transfer handler (move mode, issue 211) ---

// movableSourceFixture is the source lease's answer for move-mode requests:
// one stopped container quadlet with printable content and an armed remove
// that succeeds.
func movableSourceFixture() *fakeQuadletEngine {
	source := moveTransferSourceFixture()
	source.removeFn = func(string, bool) (*entities.QuadletRemoveReport, error) {
		return &entities.QuadletRemoveReport{}, nil
	}
	return source
}

// installingTargetFixture is a target lease that holds nothing and installs
// whatever it is asked to.
func installingTargetFixture() *fakeQuadletEngine {
	return &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return &entities.QuadletInstallReport{}, nil
		},
	}
}

func TestQuadletHandler_Move_MoveModeReturnsMovedEnvelope(t *testing.T) {
	req := moveRequest(moveEndpoint, `{"targetMachineId":"machine-b","mode":"move"}`)

	rr := httptest.NewRecorder()
	newMoveTestHandler(movableSourceFixture(), installingTargetFixture()).Move(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var got QuadletMoveResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, "web.container", got.ID)
	assert.Equal(t, "container", got.Resource)
	assert.Equal(t, MoveStatusMoved, got.Status)
	assert.Empty(t, got.Error, "a completed move carries no error field value")
}

// TestQuadletHandler_Move_RunningSourceWithoutStopOrForceIs400 pins the
// precondition refusal through the envelope: a running source quadlet and a
// move request without stop/force answers 400 QUADLET_RESOURCE_RUNNING, and
// the target install is never reached.
func TestQuadletHandler_Move_RunningSourceWithoutStopOrForceIs400(t *testing.T) {
	source := moveSourceFixture("web.container", "/x/web.container", "running", moveTransferContent)
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			panic("a refused move must not install on the target")
		},
	}

	req := moveRequest(moveEndpoint, `{"targetMachineId":"machine-b","mode":"move"}`)

	rr := httptest.NewRecorder()
	newMoveTestHandler(source, target).Move(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)

	var errResp moveErrorEnvelope
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "QUADLET_RESOURCE_RUNNING", errResp.Error.Code)
}

// TestQuadletHandler_Move_RemoveFailureReturnsPartialEnvelope pins that a
// removal failure after a successful install is NOT an error response: the
// envelope answers 200 with status "partial" and the failure's detail.
func TestQuadletHandler_Move_RemoveFailureReturnsPartialEnvelope(t *testing.T) {
	source := moveSourceFixture("web.container", "/x/web.container", "stopped", moveTransferContent)
	source.removeFn = func(string, bool) (*entities.QuadletRemoveReport, error) {
		return nil, errors.New("source read-only")
	}

	req := moveRequest(moveEndpoint, `{"targetMachineId":"machine-b","mode":"move"}`)

	rr := httptest.NewRecorder()
	newMoveTestHandler(source, installingTargetFixture()).Move(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "a partial transfer is a success-shaped envelope")

	var got QuadletMoveResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, MoveStatusPartial, got.Status)
	assert.NotEmpty(t, got.Error, "partial must carry the removal failure's detail")
}

// --- Companion .volume quadlets (issue 217) ---

// TestQuadletHandler_MovePreview_CompanionFieldsOnTheWire pins the preview
// contract through the envelope: a Volume= source naming a .volume quadlet
// file on the source renders quadletFile and targetState (here the target
// holds nothing — "absent"), while a dangling suffix reference renders the
// ordinary shape with no companion fields.
func TestQuadletHandler_MovePreview_CompanionFieldsOnTheWire(t *testing.T) {
	content := `[Container]
Image=busybox
Volume=app-data.volume:/var/lib/app
Volume=missing.volume:/var/lib/gone
`
	sourceEngine := &fakeQuadletEngine{
		printFn: func(name string) (string, error) {
			if name == "app-data.volume" {
				return appDataVolumeContent, nil
			}
			return content, nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"},
				{Name: "app-data.volume", Path: "/x/app-data.volume", Status: "stopped"},
			}, nil
		},
	}
	targetEngine := &fakeQuadletEngine{
		printFn: func(string) (string, error) {
			return "", errors.New("no such quadlet")
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
	}

	req := movePreviewRequest("/api/machines/machine-a/quadlets/web.container/move-preview?targetMachineId=machine-b",
		"user123", "machine-a", "web.container")

	rr := httptest.NewRecorder()
	newMovePreviewTestHandler(sourceEngine, targetEngine).MovePreview(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var got QuadletMovePreviewResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Len(t, got.Volumes, 2)
	assert.Equal(t, "app-data.volume", got.Volumes[0].QuadletFile)
	assert.Equal(t, MovePreviewCompanionAbsent, got.Volumes[0].TargetState)
	assert.Empty(t, got.Volumes[1].QuadletFile, "a dangling reference carries no companion fields")
	assert.Empty(t, got.Volumes[1].TargetState)

	// The issue-214 enrichment rides the same entries: each resolves its
	// podman volume (the companion sets no VolumeName=, so the
	// systemd-<name> default; the dangling reference still resolves its
	// default) and, with no volume behind it on the fake, lands dangling.
	assert.Equal(t, "systemd-app-data", got.Volumes[0].PodmanVolume)
	assert.Equal(t, "systemd-missing", got.Volumes[1].PodmanVolume)
	assert.False(t, got.Volumes[0].Migratable)
	assert.Equal(t, MovePreviewReasonDangling, got.Volumes[0].Reason)
	assert.False(t, got.Volumes[0].ExistsOnTarget)

	// The companion fields are omitempty: an ordinary entry renders the
	// parse shape plus the always-on migration facts exactly (the two
	// plain booleans stay visible even when false).
	assert.JSONEq(t, `{"source":"missing.volume","kind":"volume","podmanVolume":"systemd-missing","existsOnTarget":false,"migratable":false,"reason":"source volume does not exist"}`,
		jsonOf(t, got.Volumes[1]))
}

// jsonOf marshals one value for a JSONEq assertion.
func jsonOf(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return string(raw)
}

// TestQuadletHandler_Move_TransfersCompanionsAndHonorsTheFlag pins the
// transfer contract through the envelope: a copy carries the companion
// along (installed on the target before the main quadlet — asserted by the
// install order the recording target observes), and an explicit
// transferVolumeQuadlets=false moves the main quadlet alone.
func TestQuadletHandler_Move_TransfersCompanionsAndHonorsTheFlag(t *testing.T) {
	t.Run("DefaultCarriesCompanions", func(t *testing.T) {
		installs := []string{}
		target := &fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return nil, nil
			},
			installFn: func(fileName string, _ []byte, _ bool) (*entities.QuadletInstallReport, error) {
				installs = append(installs, fileName)
				return &entities.QuadletInstallReport{}, nil
			},
		}

		req := moveRequest(moveEndpoint, `{"targetMachineId":"machine-b","mode":"copy"}`)
		rr := httptest.NewRecorder()
		newMoveTestHandler(companionSourceFixture(companionContent), target).Move(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		var got QuadletMoveResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
		assert.Equal(t, MoveStatusCopied, got.Status)
		assert.Equal(t, []string{"app-data.volume", "web.container"}, installs,
			"the companion installs before the main quadlet, both by default")
	})

	t.Run("FlagFalseMovesMainOnly", func(t *testing.T) {
		installs := []string{}
		target := &fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return nil, nil
			},
			installFn: func(fileName string, _ []byte, _ bool) (*entities.QuadletInstallReport, error) {
				installs = append(installs, fileName)
				return &entities.QuadletInstallReport{}, nil
			},
		}

		req := moveRequest(moveEndpoint, `{"targetMachineId":"machine-b","mode":"copy","transferVolumeQuadlets":false}`)
		rr := httptest.NewRecorder()
		newMoveTestHandler(companionSourceFixture(companionContent), target).Move(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, []string{"web.container"}, installs,
			"transferVolumeQuadlets=false installs the main quadlet only")
	})
}

// TestQuadletHandler_Move_DifferingCompanionIs409 pins the refusal through
// the envelope: a same-name companion with different content answers 409
// QUADLET_ALREADY_EXISTS, and the message names the file.
func TestQuadletHandler_Move_DifferingCompanionIs409(t *testing.T) {
	target := &fakeQuadletEngine{
		printFn: func(string) (string, error) {
			return "[Volume]\nDriver=local\n", nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "app-data.volume", Path: "/y/app-data.volume", Status: "stopped"},
			}, nil
		},
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			panic("a differing companion must stop the transfer before anything installs")
		},
	}

	req := moveRequest(moveEndpoint, `{"targetMachineId":"machine-b","mode":"copy"}`)

	rr := httptest.NewRecorder()
	newMoveTestHandler(companionSourceFixture(companionContent), target).Move(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code)

	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "QUADLET_ALREADY_EXISTS", errResp.Error.Code)
	assert.Contains(t, errResp.Error.Message, "app-data.volume",
		"the 409 names the companion file")
}

// --- Volume data migration transport (issue 214) ---

// parseMoveNDJSON decodes a streamed move response body into events.
func parseMoveNDJSON(t *testing.T, body string) []QuadletMoveEvent {
	t.Helper()
	var events []QuadletMoveEvent
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var event QuadletMoveEvent
		require.NoError(t, json.Unmarshal(line, &event), "each NDJSON line must be a valid event: %s", line)
		events = append(events, event)
	}
	require.NoError(t, scanner.Err())
	return events
}

// migrateMoveHandler builds the handler over migration-armed fakes: the
// source holds the pgdata volume and exports a small tar for it; the
// target holds nothing, creates, and imports.
func migrateMoveHandler() (*QuadletHandler, *string) {
	payload := buildVolumeTar(&testing.T{}, map[string][]byte{"pgdata/PG_VERSION": []byte("17")})
	gotImported := ""
	source := &fakeQuadletEngine{
		printFn: func(string) (string, error) { return moveVolumeContent, nil },
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"},
			}, nil
		},
		volumeInspectFn: func(name string) (*QuadletVolumeInspect, error) {
			if name == "pgdata" {
				return &QuadletVolumeInspect{Name: "pgdata"}, nil
			}
			return nil, nil
		},
		volumeExportFn: func(_ string, w io.Writer) error {
			_, err := w.Write(payload)
			return err
		},
	}
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return &entities.QuadletInstallReport{}, nil
		},
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) { return nil, nil },
		volumeCreateFn:  func(string) error { return nil },
		volumeImportFn: func(name string, r io.Reader) error {
			data, err := io.ReadAll(r)
			gotImported = string(data)
			return err
		},
	}
	handler := NewQuadletHandler(newMoveTransferTestService(source, target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite}, testutils.NewMockSSHExec()))
	return handler, &gotImported
}

// TestQuadletHandler_Move_MigrateStreamsNDJSON pins the conditional
// transport: migrateVolumeData switches the answer to NDJSON — per-volume
// progress events, then exactly one terminal complete event carrying the
// envelope with its volumeMigration block.
func TestQuadletHandler_Move_MigrateStreamsNDJSON(t *testing.T) {
	handler, gotImported := migrateMoveHandler()

	req := moveRequest(moveEndpoint, `{"targetMachineId":"machine-b","mode":"copy","migrateVolumeData":true}`)

	rr := httptest.NewRecorder()
	handler.Move(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	events := parseMoveNDJSON(t, rr.Body.String())
	require.Len(t, events, 3, "volume start, volume done, terminal complete")

	assert.Equal(t, MoveEventTypeVolume, events[0].Type)
	assert.Equal(t, MoveVolumePhaseStart, events[0].Phase)
	assert.Equal(t, "pgdata", events[0].Ref)
	assert.Equal(t, "machine-a", events[0].MachineID)
	assert.NotEmpty(t, events[0].OperationID, "the transport stamps its operation ID")
	assert.NotEmpty(t, events[0].Timestamp)

	assert.Equal(t, MoveEventTypeVolume, events[1].Type)
	assert.Equal(t, MoveVolumePhaseDone, events[1].Phase)

	assert.Equal(t, MoveEventTypeComplete, events[2].Type)
	require.NotNil(t, events[2].Result)
	assert.Equal(t, MoveStatusCopied, events[2].Result.Status)
	require.NotNil(t, events[2].Result.VolumeMigration)
	require.Len(t, events[2].Result.VolumeMigration.Volumes, 1)
	assert.Equal(t, MoveVolumeOutcomeMigrated, events[2].Result.VolumeMigration.Volumes[0].Outcome)
	assert.NotEmpty(t, *gotImported, "the data crossed the pipe")

	for _, event := range events {
		assert.Equal(t, events[0].OperationID, event.OperationID)
		assert.Equal(t, "machine-a", event.MachineID)
	}
}

// TestQuadletHandler_Move_MigrateErrorEventTerminal pins the failure half
// of the streaming contract: a data-phase refusal answers the NDJSON
// transport's terminal error event with the mapped code, not a plain
// envelope.
func TestQuadletHandler_Move_MigrateErrorEventTerminal(t *testing.T) {
	source := &fakeQuadletEngine{
		printFn: func(string) (string, error) { return moveVolumeContent, nil },
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "stopped"},
			}, nil
		},
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) {
			return &QuadletVolumeInspect{Name: "pgdata"}, nil
		},
	}
	target := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) { return nil, nil },
		volumeInspectFn: func(string) (*QuadletVolumeInspect, error) {
			return &QuadletVolumeInspect{Name: "pgdata"}, nil
		},
		volumeExportFn: func(_ string, w io.Writer) error {
			_, err := w.Write(buildVolumeTar(t, map[string][]byte{"existing/file": []byte("keep me")}))
			return err
		},
	}
	handler := NewQuadletHandler(newMoveTransferTestService(source, target,
		&fakeAccessRepo{getPermissionFn: allowTargetWrite}, testutils.NewMockSSHExec()))

	req := moveRequest(moveEndpoint, `{"targetMachineId":"machine-b","mode":"copy","migrateVolumeData":true}`)

	rr := httptest.NewRecorder()
	handler.Move(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "the stream itself succeeds; the failure rides the terminal event")

	events := parseMoveNDJSON(t, rr.Body.String())
	require.Len(t, events, 1, "the refusal fires before any progress event")
	assert.Equal(t, MoveEventTypeError, events[0].Type)
	require.NotNil(t, events[0].Error)
	assert.Equal(t, "VOLUME_MIGRATION_TARGET_NOT_EMPTY", events[0].Error.Code)
	assert.Contains(t, events[0].Error.Message, "pgdata")
}

// TestQuadletHandler_Move_MigratePreStreamErrorStaysPlain pins the
// boundary: errors answered BEFORE the stream begins keep the plain JSON
// envelope, even when the request asked for migration.
func TestQuadletHandler_Move_MigratePreStreamErrorStaysPlain(t *testing.T) {
	req := moveRequest(moveEndpoint, `{"targetMachineId":"machine-b","mode":"teleport","migrateVolumeData":true}`)

	rr := httptest.NewRecorder()
	newMoveTestHandler(&fakeQuadletEngine{}, &fakeQuadletEngine{}).Move(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &envelope))
	assert.Contains(t, envelope, "error", "a pre-stream answer is the endpoint envelope, not an NDJSON event")
}
