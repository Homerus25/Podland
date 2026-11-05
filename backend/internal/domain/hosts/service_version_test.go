package hosts

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/testutils"
)

// newVersionTestService builds a service whose remote version fetch is the
// given fake, seeded with one accessible host, so version checks reach
// the connection seam rather than stopping at the not-found / access
// gates.
func newVersionTestService(t *testing.T, connManager *testutils.MockConnectionManager, fetch versionFetcher) (*HostService, *testutils.MockHostRepository, *testutils.MockHostAccessRepository) {
	t.Helper()
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	service := NewHostService(hostRepo, accessRepo, connManager, testutils.NewMockEncryptionService(), newFakeSSHInvalidator())
	service.fetchVersion = fetch
	hostRepo.AddHost(&models.Host{ID: "host-1", Name: "m", OwnerID: "owner-1"})
	accessRepo.AddAccess("host-1", "owner-1", models.PermissionRead, time.Now())
	return service, hostRepo, accessRepo
}

func TestHostService_Version_FetchSuccessReturnsVersions(t *testing.T) {
	connManager := testutils.NewMockConnectionManager()
	service, _, _ := newVersionTestService(t, connManager, func(ctx context.Context) (versionReport, error) {
		return versionReport{PodmanVersion: "5.4.1", APIVersion: "5.4.1"}, nil
	})

	podmanVersion, apiVersion, err := service.GetHostVersion(context.Background(), "owner-1", "host-1")

	require.NoError(t, err)
	require.NotNil(t, podmanVersion)
	require.NotNil(t, apiVersion)
	assert.Equal(t, "5.4.1", *podmanVersion)
	assert.Equal(t, "5.4.1", *apiVersion)
}

func TestHostService_Version_FetchErrorIsNulls(t *testing.T) {
	connManager := testutils.NewMockConnectionManager()
	service, _, _ := newVersionTestService(t, connManager, func(ctx context.Context) (versionReport, error) {
		return versionReport{}, errors.New("version call failed")
	})

	podmanVersion, apiVersion, err := service.GetHostVersion(context.Background(), "owner-1", "host-1")

	require.NoError(t, err, "unavailable is a valid answer, not an error")
	assert.Nil(t, podmanVersion)
	assert.Nil(t, apiVersion)
}

func TestHostService_Version_ConnectionFailureIsNulls(t *testing.T) {
	connManager := testutils.NewMockConnectionManagerWithAcquireError(appErrors.ErrConnectionFailed)
	service, _, _ := newVersionTestService(t, connManager, func(ctx context.Context) (versionReport, error) {
		return versionReport{}, errors.New("fetch must not run")
	})

	podmanVersion, apiVersion, err := service.GetHostVersion(context.Background(), "owner-1", "host-1")

	require.NoError(t, err, "unavailable is a valid answer, not an error")
	assert.Nil(t, podmanVersion)
	assert.Nil(t, apiVersion)
}

func TestHostService_Version_UnknownHostIsNotFound(t *testing.T) {
	connManager := testutils.NewMockConnectionManager()
	service, _, _ := newVersionTestService(t, connManager, func(ctx context.Context) (versionReport, error) {
		return versionReport{}, errors.New("fetch must not run")
	})

	_, _, err := service.GetHostVersion(context.Background(), "owner-1", "missing")

	require.Error(t, err)
	assert.Equal(t, appErrors.ErrHostNotFound, err)
}

func TestHostService_Version_MissingAccessIsDenied(t *testing.T) {
	connManager := testutils.NewMockConnectionManager()
	service, hostRepo, _ := newVersionTestService(t, connManager, func(ctx context.Context) (versionReport, error) {
		return versionReport{}, errors.New("fetch must not run")
	})
	hostRepo.AddHost(&models.Host{ID: "host-2", Name: "m2", OwnerID: "owner-1"})

	_, _, err := service.GetHostVersion(context.Background(), "stranger-1", "host-2")

	require.Error(t, err)
	assert.Equal(t, appErrors.ErrAccessDenied, err)
}

func TestHostService_Version_ConnectionReceivesHostAndUserIDs(t *testing.T) {
	connManager := testutils.NewMockConnectionManager()
	service, _, _ := newVersionTestService(t, connManager, func(ctx context.Context) (versionReport, error) {
		return versionReport{PodmanVersion: "5.4.1", APIVersion: "5.4.1"}, nil
	})

	_, _, err := service.GetHostVersion(context.Background(), "owner-1", "host-1")
	require.NoError(t, err)

	require.Len(t, connManager.WithConnectionCalls(), 1)
	call := connManager.WithConnectionCalls()[0]
	assert.Equal(t, "host-1", call.HostID)
	assert.Equal(t, "owner-1", call.UserID)
	assert.Equal(t, "hosts.GetVersion", call.Op)
}
