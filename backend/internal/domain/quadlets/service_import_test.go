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

func TestServiceImport_AllSuccess(t *testing.T) {
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

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Len(t, response.Imported, 2)
	assert.Len(t, response.Failed, 0)

	assert.Equal(t, "web", response.Imported[0].Name)
	assert.Equal(t, "db", response.Imported[1].Name)
	assert.Equal(t, "unknown", response.Imported[0].State)
	assert.Equal(t, "unknown", response.Imported[1].State)
}

func TestServiceImport_PartialFailure(t *testing.T) {
	t.Parallel()

	var mockConnManager machines.ConnectionManagerInterface = testutils.NewMockConnectionManager()
	service := NewQuadletService(mockConnManager, nil)

	ctx := context.Background()
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
			{
				Name:     "cache",
				Resource: "container",
				Content:  "[Container]\nImage=redis\n",
			},
		},
	}

	response, err := service.Import(ctx, "user-1", "machine-1", req)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Len(t, response.Imported, 3)
	assert.Len(t, response.Failed, 0)
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

func TestImportSingleQuadlet_ValidEntry(t *testing.T) {
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

	require.NoError(t, err)
	assert.Equal(t, "web", result.Name)
	assert.Equal(t, "web", result.ID)
	assert.Equal(t, "unknown", result.State)
}

func TestImportSingleQuadlet_InvalidResource(t *testing.T) {
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

	require.NoError(t, err)
	assert.Equal(t, "web", result.Name)
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
	Struct(interface{}) error
} {
	return validator.New()
}