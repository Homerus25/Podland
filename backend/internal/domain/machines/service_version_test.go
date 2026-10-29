package machines

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
// given fake, seeded with one accessible machine, so version checks reach
// the connection seam rather than stopping at the not-found / access
// gates.
func newVersionTestService(t *testing.T, connManager *testutils.MockConnectionManager, fetch versionFetcher) (*MachineService, *testutils.MockMachineRepository, *testutils.MockMachineAccessRepository) {
	t.Helper()
	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()
	service := NewMachineService(machineRepo, accessRepo, connManager, testutils.NewMockEncryptionService(), newFakeSSHInvalidator())
	service.fetchVersion = fetch
	machineRepo.AddMachine(&models.Machine{ID: "machine-1", Name: "m", OwnerID: "owner-1"})
	accessRepo.AddAccess("machine-1", "owner-1", models.PermissionRead, time.Now())
	return service, machineRepo, accessRepo
}

func TestMachineService_Version_FetchSuccessReturnsVersions(t *testing.T) {
	connManager := testutils.NewMockConnectionManager()
	service, _, _ := newVersionTestService(t, connManager, func(ctx context.Context) (versionReport, error) {
		return versionReport{PodmanVersion: "5.4.1", APIVersion: "5.4.1"}, nil
	})

	podmanVersion, apiVersion, err := service.GetMachineVersion(context.Background(), "owner-1", "machine-1")

	require.NoError(t, err)
	require.NotNil(t, podmanVersion)
	require.NotNil(t, apiVersion)
	assert.Equal(t, "5.4.1", *podmanVersion)
	assert.Equal(t, "5.4.1", *apiVersion)
}

func TestMachineService_Version_FetchErrorIsNulls(t *testing.T) {
	connManager := testutils.NewMockConnectionManager()
	service, _, _ := newVersionTestService(t, connManager, func(ctx context.Context) (versionReport, error) {
		return versionReport{}, errors.New("version call failed")
	})

	podmanVersion, apiVersion, err := service.GetMachineVersion(context.Background(), "owner-1", "machine-1")

	require.NoError(t, err, "unavailable is a valid answer, not an error")
	assert.Nil(t, podmanVersion)
	assert.Nil(t, apiVersion)
}

func TestMachineService_Version_ConnectionFailureIsNulls(t *testing.T) {
	connManager := testutils.NewMockConnectionManagerWithAcquireError(appErrors.ErrConnectionFailed)
	service, _, _ := newVersionTestService(t, connManager, func(ctx context.Context) (versionReport, error) {
		return versionReport{}, errors.New("fetch must not run")
	})

	podmanVersion, apiVersion, err := service.GetMachineVersion(context.Background(), "owner-1", "machine-1")

	require.NoError(t, err, "unavailable is a valid answer, not an error")
	assert.Nil(t, podmanVersion)
	assert.Nil(t, apiVersion)
}

func TestMachineService_Version_UnknownMachineIsNotFound(t *testing.T) {
	connManager := testutils.NewMockConnectionManager()
	service, _, _ := newVersionTestService(t, connManager, func(ctx context.Context) (versionReport, error) {
		return versionReport{}, errors.New("fetch must not run")
	})

	_, _, err := service.GetMachineVersion(context.Background(), "owner-1", "missing")

	require.Error(t, err)
	assert.Equal(t, appErrors.ErrMachineNotFound, err)
}

func TestMachineService_Version_MissingAccessIsDenied(t *testing.T) {
	connManager := testutils.NewMockConnectionManager()
	service, machineRepo, _ := newVersionTestService(t, connManager, func(ctx context.Context) (versionReport, error) {
		return versionReport{}, errors.New("fetch must not run")
	})
	machineRepo.AddMachine(&models.Machine{ID: "machine-2", Name: "m2", OwnerID: "owner-1"})

	_, _, err := service.GetMachineVersion(context.Background(), "stranger-1", "machine-2")

	require.Error(t, err)
	assert.Equal(t, appErrors.ErrAccessDenied, err)
}

func TestMachineService_Version_ConnectionReceivesMachineAndUserIDs(t *testing.T) {
	connManager := testutils.NewMockConnectionManager()
	service, _, _ := newVersionTestService(t, connManager, func(ctx context.Context) (versionReport, error) {
		return versionReport{PodmanVersion: "5.4.1", APIVersion: "5.4.1"}, nil
	})

	_, _, err := service.GetMachineVersion(context.Background(), "owner-1", "machine-1")
	require.NoError(t, err)

	require.Len(t, connManager.WithConnectionCalls(), 1)
	call := connManager.WithConnectionCalls()[0]
	assert.Equal(t, "machine-1", call.MachineID)
	assert.Equal(t, "owner-1", call.UserID)
	assert.Equal(t, "machines.GetVersion", call.Op)
}
