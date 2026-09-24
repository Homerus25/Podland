package quadlets

import (
	"context"
	"errors"
	"testing"

	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// TestServiceImport_ImportsAndCollectsFailures pins the per-entry contract:
// every entry is installed independently, successful installs are collected
// with their reported path, and name collisions land in Failed with the
// already-exists message instead of failing the whole request.
func TestServiceImport_ImportsAndCollectsFailures(t *testing.T) {
	t.Parallel()

	engine := &fakeQuadletEngine{
		installFn: func(fileName string, content []byte, replace bool) (*entities.QuadletInstallReport, error) {
			if fileName == "db.container" {
				return nil, &errorhandling.ErrorModel{
					ResponseCode: 500,
					Message:      "db.container: conflict",
					Because:      "name in use",
				}
			}
			return &entities.QuadletInstallReport{
				InstalledQuadlets: map[string]string{"/tmp/" + fileName: "/etc/containers/systemd/" + fileName},
			}, nil
		},
	}
	// A successful per-entry install is followed by the user-manager
	// daemon-reload, so the service needs a (succeeding) SSH exec wired.
	service := NewQuadletService(fakeEngineProvider{engine: engine}, testutils.NewMockSSHExec())

	req := QuadletImportRequest{
		Quadlets: []QuadletImportEntry{
			{Name: "web", Resource: "container", Content: "[Container]\nImage=nginx\n"},
			{Name: "db", Resource: "container", Content: "[Container]\nImage=postgres\n"},
		},
	}

	response, err := service.Import(context.Background(), "user-1", "machine-1", req)

	require.NoError(t, err, "per-entry failures are collected, not returned")
	require.NotNil(t, response)
	require.Len(t, response.Imported, 1)
	assert.Equal(t, "web", response.Imported[0].Name)
	assert.Equal(t, "/etc/containers/systemd/web.container", response.Imported[0].Path)
	require.Len(t, response.Failed, 1)
	assert.Equal(t, "db", response.Failed[0].Name)
	assert.Contains(t, response.Failed[0].Error, "quadlet already exists",
		"a name-in-use refusal must be reported as an already-exists failure")
}

// TestServiceImport_InstallFailureIsCollected pins that a non-collision
// install failure also stays per-entry (wrapped install error, not a
// top-level failure).
func TestServiceImport_InstallFailureIsCollected(t *testing.T) {
	t.Parallel()

	engine := &fakeQuadletEngine{
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return nil, errors.New("systemd reload failed")
		},
	}
	// The batch completes with collected per-entry failures, so the
	// post-batch daemon-reload still runs — a (succeeding) SSH exec is
	// wired for it.
	service := NewQuadletService(fakeEngineProvider{engine: engine}, testutils.NewMockSSHExec())

	response, err := service.Import(context.Background(), "user-1", "machine-1", QuadletImportRequest{
		Quadlets: []QuadletImportEntry{
			{Name: "web", Resource: "container", Content: "[Container]\nImage=nginx\n"},
		},
	})

	require.NoError(t, err)
	require.Len(t, response.Failed, 1)
	assert.Contains(t, response.Failed[0].Error, "failed to install quadlet")
	assert.Len(t, response.Imported, 0)
}

func TestServiceImport_EmptyRequest(t *testing.T) {
	t.Parallel()

	// The engine stays unarmed: an empty request must not touch podman.
	service := NewQuadletService(fakeEngineProvider{engine: &fakeQuadletEngine{}}, nil)

	ctx := context.Background()
	req := QuadletImportRequest{
		Quadlets: []QuadletImportEntry{},
	}

	response, err := service.Import(ctx, "user-1", "machine-1", req)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Len(t, response.Imported, 0)
	assert.Len(t, response.Failed, 0)
}

func TestQuadletImportRequest_Validation(t *testing.T) {
	t.Run("empty quadlets list should fail validation", func(t *testing.T) {
		req := QuadletImportRequest{
			Quadlets: []QuadletImportEntry{},
		}

		validate := getValidator()
		err := validate.Struct(req)
		assert.Error(t, err)
	})

	t.Run("valid request should pass validation", func(t *testing.T) {
		req := QuadletImportRequest{
			Quadlets: []QuadletImportEntry{
				{
					Name:     "web",
					Resource: "container",
					Content:  "[Container]\nImage=nginx\n",
				},
			},
		}

		validate := getValidator()
		err := validate.Struct(req)
		assert.NoError(t, err)
	})

	t.Run("entry without name should fail validation", func(t *testing.T) {
		req := QuadletImportRequest{
			Quadlets: []QuadletImportEntry{
				{
					Name:     "",
					Resource: "container",
					Content:  "[Container]\nImage=nginx\n",
				},
			},
		}

		validate := getValidator()
		err := validate.Struct(req)
		assert.Error(t, err)
	})

	t.Run("entry without resource should fail validation", func(t *testing.T) {
		req := QuadletImportRequest{
			Quadlets: []QuadletImportEntry{
				{
					Name:     "web",
					Resource: "",
					Content:  "[Container]\nImage=nginx\n",
				},
			},
		}

		validate := getValidator()
		err := validate.Struct(req)
		assert.Error(t, err)
	})

	t.Run("entry without content should fail validation", func(t *testing.T) {
		req := QuadletImportRequest{
			Quadlets: []QuadletImportEntry{
				{
					Name:     "web",
					Resource: "container",
					Content:  "",
				},
			},
		}

		validate := getValidator()
		err := validate.Struct(req)
		assert.Error(t, err)
	})

	t.Run("entry with invalid resource should fail validation", func(t *testing.T) {
		req := QuadletImportRequest{
			Quadlets: []QuadletImportEntry{
				{
					Name:     "web",
					Resource: "invalid",
					Content:  "[Container]\nImage=nginx\n",
				},
			},
		}

		validate := getValidator()
		err := validate.Struct(req)
		assert.Error(t, err)
	})
}

func TestQuadletImportResponse_Structure(t *testing.T) {
	t.Run("empty response should have empty arrays", func(t *testing.T) {
		response := &QuadletImportResponse{
			Imported: make([]QuadletImportResult, 0),
			Failed:   make([]QuadletImportError, 0),
		}

		assert.Len(t, response.Imported, 0)
		assert.Len(t, response.Failed, 0)
	})

	t.Run("response with mixed results", func(t *testing.T) {
		response := &QuadletImportResponse{
			Imported: []QuadletImportResult{
				{
					ID:    "web",
					Name:  "web",
					Path:  "/etc/containers/systemd/web.container",
					State: "unknown",
				},
			},
			Failed: []QuadletImportError{
				{
					Name:  "db",
					Error: "quadlet already exists",
				},
			},
		}

		assert.Len(t, response.Imported, 1)
		assert.Len(t, response.Failed, 1)
		assert.Equal(t, "web", response.Imported[0].Name)
		assert.Equal(t, "db", response.Failed[0].Name)
	})
}

func getValidator() interface {
	Struct(any) error
} {
	return validator.New()
}
