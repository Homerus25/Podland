package volumes

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// Service behaviour tests run the real VolumeService against the fake
// engine, so the mapping/composition rules below are pinned as behaviour,
// not invocation shape. Connection acquisition (and its 502 mapping) is the
// provider seam: the connections package owns that contract and pins it in
// its own tests.

// notFoundErr builds the 404 ErrorModel shape podman's tunnel layer
// produces for a missing resource.
func notFoundErr() error {
	return &errorhandling.ErrorModel{
		ResponseCode: http.StatusNotFound,
		Message:      "no such volume",
	}
}

func inspectFixture(name string) *entities.VolumeInspectReport {
	fixture := namedVolumeFixture("2026-08-30T10:00:00Z")
	fixture.Name = name
	return &entities.VolumeInspectReport{VolumeConfigResponse: &fixture}
}

func TestVolumeService_List_ReturnsEngineVolumes(t *testing.T) {
	volumes := []*entities.VolumeListReport{
		{VolumeConfigResponse: namedVolumeFixture("2026-08-30T10:00:00Z")},
		{VolumeConfigResponse: anonymousVolumeFixture()},
	}
	engine := &fakeVolumeEngine{
		listFn: func(entities.VolumeListOptions) ([]*entities.VolumeListReport, error) {
			return volumes, nil
		},
	}
	service := NewVolumeService(fakeEngineProvider{engine: engine})

	got, err := service.List(context.Background(), "user", "machine", entities.VolumeListOptions{})

	require.NoError(t, err)
	assert.Equal(t, volumes, got)
}

func TestVolumeService_List_WrapsEngineFailure(t *testing.T) {
	engine := &fakeVolumeEngine{
		listFn: func(entities.VolumeListOptions) ([]*entities.VolumeListReport, error) {
			return nil, errors.New("boom")
		},
	}
	service := NewVolumeService(fakeEngineProvider{engine: engine})

	got, err := service.List(context.Background(), "user", "machine", entities.VolumeListOptions{})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to list volumes", "engine failures must be wrapped consistently")
}

func TestVolumeService_Get_MapsMissingVolumeTo404(t *testing.T) {
	engine := &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) {
			return nil, notFoundErr()
		},
	}
	service := NewVolumeService(fakeEngineProvider{engine: engine})

	got, err := service.Get(context.Background(), "user", "machine", "my-volume")

	require.ErrorIs(t, err, appErrors.ErrVolumeNotFound)
	assert.Nil(t, got)
}

func TestVolumeService_Get_MapsEmptyInspectTo404(t *testing.T) {
	engine := &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) {
			return nil, nil
		},
	}
	service := NewVolumeService(fakeEngineProvider{engine: engine})

	got, err := service.Get(context.Background(), "user", "machine", "my-volume")

	require.ErrorIs(t, err, appErrors.ErrVolumeNotFound)
	assert.Nil(t, got)
}

func TestVolumeService_Get_WrapsEngineFailure(t *testing.T) {
	engine := &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) {
			return nil, errors.New("boom")
		},
	}
	service := NewVolumeService(fakeEngineProvider{engine: engine})

	got, err := service.Get(context.Background(), "user", "machine", "my-volume")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to inspect volume", "engine failures must be wrapped consistently")
}

func TestVolumeService_Create_InspectsFreshlyCreatedVolume(t *testing.T) {
	var inspectedName string
	engine := &fakeVolumeEngine{
		createFn: func(entities.VolumeCreateOptions) (*entities.IDOrNameResponse, error) {
			return &entities.IDOrNameResponse{IDOrName: "generated-name"}, nil
		},
		inspectFn: func(name string) (*entities.VolumeInspectReport, error) {
			inspectedName = name
			return inspectFixture(name), nil
		},
	}
	service := NewVolumeService(fakeEngineProvider{engine: engine})

	got, err := service.Create(context.Background(), "user", "machine", VolumeCreateRequest{})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "generated-name", inspectedName, "the created volume's (possibly generated) name must be inspected")
	assert.Equal(t, "generated-name", got.Name)
}

func TestVolumeService_Create_MapsClientErrorsToInvalidRequest(t *testing.T) {
	engine := &fakeVolumeEngine{
		createFn: func(entities.VolumeCreateOptions) (*entities.IDOrNameResponse, error) {
			return nil, &errorhandling.ErrorModel{
				ResponseCode: http.StatusBadRequest,
				Message:      "volume name invalid",
			}
		},
	}
	service := NewVolumeService(fakeEngineProvider{engine: engine})

	got, err := service.Create(context.Background(), "user", "machine", VolumeCreateRequest{Name: "bad"})

	require.Error(t, err)
	assert.Nil(t, got)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadRequest, appErr.Status)
	assert.Equal(t, "INVALID_REQUEST", appErr.Code)
}

func TestVolumeService_Delete_MapsMissingVolumeTo404(t *testing.T) {
	engine := &fakeVolumeEngine{
		removeFn: func(string, bool) ([]*entities.VolumeRmReport, error) {
			return []*entities.VolumeRmReport{{Err: notFoundErr()}}, nil
		},
	}
	service := NewVolumeService(fakeEngineProvider{engine: engine})

	err := service.Delete(context.Background(), "user", "machine", "my-volume", false)

	require.ErrorIs(t, err, appErrors.ErrVolumeNotFound)
}

func TestVolumeService_Delete_MapsInUseTo409(t *testing.T) {
	engine := &fakeVolumeEngine{
		removeFn: func(string, bool) ([]*entities.VolumeRmReport, error) {
			return []*entities.VolumeRmReport{{
				Err: &errorhandling.ErrorModel{
					ResponseCode: http.StatusConflict,
					Message:      "volume is in use by a container",
				},
			}}, nil
		},
	}
	service := NewVolumeService(fakeEngineProvider{engine: engine})

	err := service.Delete(context.Background(), "user", "machine", "my-volume", false)

	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusConflict, appErr.Status)
	assert.Equal(t, "VOLUME_IN_USE", appErr.Code)
	assert.Contains(t, appErr.Message, "volume is in use by a container", "podman's refusal text must reach the frontend")
}

func TestVolumeService_Delete_PropagatesEngineFailure(t *testing.T) {
	engine := &fakeVolumeEngine{
		removeFn: func(string, bool) ([]*entities.VolumeRmReport, error) {
			return nil, errors.New("boom")
		},
	}
	service := NewVolumeService(fakeEngineProvider{engine: engine})

	err := service.Delete(context.Background(), "user", "machine", "my-volume", false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove volume")
}

func TestVolumeService_Prune_ReturnsEngineReports(t *testing.T) {
	prunes := []*reports.PruneReport{{Id: "stale", Size: 4096}}
	engine := &fakeVolumeEngine{
		pruneFn: func() ([]*reports.PruneReport, error) {
			return prunes, nil
		},
	}
	service := NewVolumeService(fakeEngineProvider{engine: engine})

	got, err := service.Prune(context.Background(), "user", "machine")

	require.NoError(t, err)
	assert.Equal(t, prunes, got)
}

func TestVolumeService_Export_GatesOnExistence(t *testing.T) {
	engine := &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) {
			return nil, notFoundErr()
		},
		// exportFn stays unarmed: it must never be reached for a
		// missing volume.
	}
	service := NewVolumeService(fakeEngineProvider{engine: engine})
	var sink bytes.Buffer

	err := service.Export(context.Background(), "user", "machine", "my-volume", &sink)

	require.ErrorIs(t, err, appErrors.ErrVolumeNotFound)
	assert.Zero(t, sink.Len(), "no byte may be written for a missing volume")
}

func TestVolumeService_Export_StreamsEngineBytesUnchanged(t *testing.T) {
	engine := &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) {
			return inspectFixture("my-volume"), nil
		},
		exportFn: func(_ string, w io.Writer) error {
			_, err := w.Write([]byte("tar-bytes"))
			return err
		},
	}
	service := NewVolumeService(fakeEngineProvider{engine: engine})
	var sink bytes.Buffer

	err := service.Export(context.Background(), "user", "machine", "my-volume", &sink)

	require.NoError(t, err)
	assert.Equal(t, "tar-bytes", sink.String())
}

func TestVolumeService_Import_GatesOnExistence(t *testing.T) {
	engine := &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) {
			return nil, notFoundErr()
		},
		// importFn stays unarmed: the archive must not be consumed for
		// a missing volume.
	}
	service := NewVolumeService(fakeEngineProvider{engine: engine})

	err := service.Import(context.Background(), "user", "machine", "my-volume", strings.NewReader("tar"))

	require.ErrorIs(t, err, appErrors.ErrVolumeNotFound)
}

func TestVolumeService_Import_FeedsArchiveToEngine(t *testing.T) {
	var consumed string
	engine := &fakeVolumeEngine{
		inspectFn: func(string) (*entities.VolumeInspectReport, error) {
			return inspectFixture("my-volume"), nil
		},
		importFn: func(_ string, r io.Reader) error {
			data, err := io.ReadAll(r)
			consumed = string(data)
			return err
		},
	}
	service := NewVolumeService(fakeEngineProvider{engine: engine})

	err := service.Import(context.Background(), "user", "machine", "my-volume", strings.NewReader("tar"))

	require.NoError(t, err)
	assert.Equal(t, "tar", consumed)
}

// TestVolumeService_AcquireFailureIsThe502Sentinel pins the service-side
// half of the connection-failure contract: the provider's sentinel crosses
// untouched — the service never wraps, maps, or swallows it (the 502
// mapping itself is pinned in the connections package).
func TestVolumeService_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewVolumeService(fakeEngineProvider{err: appErrors.ErrConnectionFailed})

	_, err := service.List(context.Background(), "user", "machine", entities.VolumeListOptions{})

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	var appErr *appErrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, http.StatusBadGateway, appErr.Status)
}

// TestMapCreateError pins the client-error mapping for create: podman 4xx
// ErrorModels become INVALID_REQUEST carrying podman's message, while 5xx
// and unrelated errors stay unmapped (wrapped 5xx at the handler).
func TestMapCreateError(t *testing.T) {
	appErr := mapCreateError(&errorhandling.ErrorModel{ResponseCode: http.StatusBadRequest, Message: "volume name invalid"})
	require.NotNil(t, appErr, "podman 400 must map to a client error")
	assert.Equal(t, http.StatusBadRequest, appErr.Status)
	assert.Equal(t, "INVALID_REQUEST", appErr.Code)
	assert.Equal(t, "volume name invalid", appErr.Message)

	// The adapter excludes the 404/409 answers from its client-error
	// predicate; volumes create folds them back in, so the duplicate-name
	// 409 stays the 400 INVALID_REQUEST the create contract documents.
	appErr = mapCreateError(&errorhandling.ErrorModel{ResponseCode: http.StatusConflict, Message: `volume with name "x" already exists`})
	require.NotNil(t, appErr, "podman 409 duplicate name must still map to a client error")
	assert.Equal(t, http.StatusBadRequest, appErr.Status)
	assert.Equal(t, "INVALID_REQUEST", appErr.Code)

	assert.Nil(t, mapCreateError(&errorhandling.ErrorModel{ResponseCode: http.StatusInternalServerError}), "podman 5xx must not map to a client error")
	assert.Nil(t, mapCreateError(assert.AnError), "unrelated errors must not map to a client error")
}
