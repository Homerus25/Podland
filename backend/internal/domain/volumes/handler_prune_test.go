package volumes

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
)

// pruneRequest serves POST /api/hosts/{id}/volumes/prune through
// the handler with an authenticated test identity.
func pruneRequest(prune http.HandlerFunc) *httptest.ResponseRecorder {
	return testutils.Perform(prune, testutils.Request{
		Method:    http.MethodPost,
		Path:      "/api/hosts/test-host/volumes/prune",
		UserID:    "test-user",
		URLParams: map[string]string{"id": "test-host"},
	})
}

// TestVolumeHandler_Prune_Success pins the prune contract: 200 with one
// report entry per removed volume, name and reclaimed size mapped, and
// per-volume errors carried in the entry.
func TestVolumeHandler_Prune_Success(t *testing.T) {
	prunes := []*reports.PruneReport{
		{Id: "unused-a", Size: 4096},
		{Id: "unused-b", Size: 0, Err: fmt.Errorf("device or resource busy")},
	}
	engine := &fakeVolumeEngine{
		pruneFn: func() ([]*reports.PruneReport, error) {
			return prunes, nil
		},
	}

	w := pruneRequest(newTestHandler(engine).Prune)

	assert.Equal(t, 200, w.Code, "prune should return 200")
	var got []VolumePruneResponse
	require.NoError(t, jsonUnmarshalSlice(w.Body.Bytes(), &got), "response must decode as a report array")
	require.Len(t, got, 2, "each removed volume must have a report entry")
	assert.Equal(t, "unused-a", got[0].Name)
	assert.Equal(t, uint64(4096), got[0].Size)
	assert.Empty(t, got[0].Error)
	assert.Equal(t, "unused-b", got[1].Name)
	assert.Equal(t, "device or resource busy", got[1].Error, "per-volume error must be carried")
}

// TestVolumeHandler_Prune_EmptyReport pins that pruning with nothing to
// remove still returns 200 with an empty JSON array (not nil/null).
func TestVolumeHandler_Prune_EmptyReport(t *testing.T) {
	engine := &fakeVolumeEngine{
		pruneFn: func() ([]*reports.PruneReport, error) {
			return []*reports.PruneReport{}, nil
		},
	}

	w := pruneRequest(newTestHandler(engine).Prune)

	assert.Equal(t, 200, w.Code)
	var got []VolumePruneResponse
	require.NoError(t, jsonUnmarshalSlice(w.Body.Bytes(), &got))
	assert.NotNil(t, got, "response must decode as an empty array, not null")
	assert.Empty(t, got)
}

// TestFromPodmanPruneReports_NilEntries pins that nil reports are skipped
// rather than marshalled as empty entries.
func TestFromPodmanPruneReports_NilEntries(t *testing.T) {
	got := FromPodmanPruneReports([]*reports.PruneReport{nil, {Id: "x", Size: 1}})
	require.Len(t, got, 1)
	assert.Equal(t, "x", got[0].Name)
}
