package quadlets

import (
	"context"
	"errors"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourorg/podland/backend/internal/domain/machines"
	appErrors "github.com/yourorg/podland/backend/internal/errors"
	"github.com/yourorg/podland/backend/internal/testutils"
)

// TestServiceImport_InvocationShape covers the connection-stage behaviour and
// the invocation shape, matching the convention used by the other service
// seams (List/Get/Create/Update/Delete): the podman engine boundary is built
// inline and is not mocked, so the bindings-level Install call fails because
// there is no real podman backend behind the mock connection context.
//
// Unlike Create, Import collects per-entry failures instead of surfacing a
// top-level error, so the invocation shape here is: the connection succeeds
// (no top-level error), every entry's install fails, and each failure is
// captured in response.Failed with an install error (not a connection error).
func TestServiceImport_InvocationShape(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewQuadletService(mockConnManager, nil)

	req := QuadletImportRequest{
		Quadlets: []QuadletImportEntry{
			{
				Name:     "web",
				Resource: "container",
				Content:  "[Container]\nImage=nginx\n",
			},
			{
				Name:     "db",
				Resource: "container",
				Content:  "[Container]\nImage=postgres\n",
			},
		},
	}

	response, err := service.Import(ctx, "user-1", "machine-1", req)

	require.NoError(t, err, "Connection should succeed; per-entry failures are collected, not returned")
	require.NotNil(t, response)
	assert.Len(t, response.Imported, 0, "No entry can be imported without a real podman backend")
	assert.Len(t, response.Failed, 2, "Every entry should be reported as failed")

	for _, f := range response.Failed {
		assert.NotContains(t, f.Error, "CONNECTION_FAILED",
			"Per-entry error should be an install failure, not a connection failure")
	}
}

func TestServiceImport_ConnectionFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	connErr := errors.New("SSH connection failed")
	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManagerWithError(connErr)
	service := NewQuadletService(mockConnManager, nil)

	req := QuadletImportRequest{
		Quadlets: []QuadletImportEntry{
			{
				Name:     "web",
				Resource: "container",
				Content:  "[Container]\nImage=nginx\n",
			},
		},
	}

	response, err := service.Import(ctx, "user-1", "machine-1", req)

	require.Error(t, err)
	assert.Nil(t, response)
	assert.ErrorIs(t, err, appErrors.ErrConnectionFailed)
}

func TestServiceImport_EmptyRequest(t *testing.T) {
	t.Parallel()

	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewQuadletService(mockConnManager, nil)

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

// TestImportSingleQuadlet_InvocationShape covers the invocation shape of the
// per-entry helper: the mock connection context carries no real podman client,
// so the bindings-level Install call fails. The success/error mapping and
// already-exists handling are pure branches tested via the DTO and helper
// functions; exercising the happy path requires a real podman backend.
func TestImportSingleQuadlet_InvocationShape(t *testing.T) {
	t.Parallel()

	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewQuadletService(mockConnManager, nil)

	connCtx, _ := mockConnManager.GetConnection("machine-1", "user-1")

	entry := QuadletImportEntry{
		Name:     "web",
		Resource: "container",
		Content:  "[Container]\nImage=nginx\n",
	}

	result, err := service.importSingleQuadlet(connCtx, entry)

	assert.Error(t, err, "Should return error from install call with mock connection")
	assert.Equal(t, QuadletImportResult{}, result, "Should return zero result when install call fails")
	assert.NotContains(t, err.Error(), "CONNECTION_FAILED",
		"Error should not be a connection error — connection should have succeeded")
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
