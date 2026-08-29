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

// seedStatusMachine persists a machine owned by ownerID and grants the
// caller access, so status checks reach the probe rather than stopping at
// the not-found / access-denied gates.
func seedStatusMachine(t *testing.T, machineRepo *testutils.MockMachineRepository, accessRepo *testutils.MockMachineAccessRepository, machineID, ownerID, callerID string) {
	t.Helper()
	machineRepo.AddMachine(&models.Machine{ID: machineID, Name: "m", OwnerID: ownerID})
	accessRepo.AddAccess(machineID, callerID, models.PermissionRead, time.Now())
}

func TestMachineService_Status_ProbeSuccessIsOnline(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()
	connManager := testutils.NewMockConnectionManager()
	encryption := testutils.NewMockEncryptionService()

	seedStatusMachine(t, machineRepo, accessRepo, "machine-1", "owner-1", "owner-1")

	service := NewMachineService(machineRepo, accessRepo, connManager, encryption)

	status, checkedAt, err := service.GetMachineStatus(context.Background(), "owner-1", "machine-1")

	require.NoError(t, err)
	assert.Equal(t, "online", status)
	assert.False(t, checkedAt.IsZero(), "checkedAt must be set")
}

func TestMachineService_Status_ProbeErrorIsOffline(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()
	connManager := testutils.NewMockConnectionManagerWithProbeError(errors.New("dial tcp: connection refused"))
	encryption := testutils.NewMockEncryptionService()

	seedStatusMachine(t, machineRepo, accessRepo, "machine-1", "owner-1", "owner-1")

	service := NewMachineService(machineRepo, accessRepo, connManager, encryption)

	status, checkedAt, err := service.GetMachineStatus(context.Background(), "owner-1", "machine-1")

	require.NoError(t, err, "offline is a valid answer, not an error")
	assert.Equal(t, "offline", status)
	assert.False(t, checkedAt.IsZero())
}

func TestMachineService_Status_ProbeTimeoutIsOffline(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()
	connManager := testutils.NewMockConnectionManagerWithProbeError(context.DeadlineExceeded)
	encryption := testutils.NewMockEncryptionService()

	seedStatusMachine(t, machineRepo, accessRepo, "machine-1", "owner-1", "owner-1")

	service := NewMachineService(machineRepo, accessRepo, connManager, encryption)

	status, _, err := service.GetMachineStatus(context.Background(), "owner-1", "machine-1")

	require.NoError(t, err)
	assert.Equal(t, "offline", status)
}

func TestMachineService_Status_UnknownMachineIsNotFound(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()
	connManager := testutils.NewMockConnectionManagerWithProbeError(errors.New("probe must not run"))
	encryption := testutils.NewMockEncryptionService()

	service := NewMachineService(machineRepo, accessRepo, connManager, encryption)

	_, _, err := service.GetMachineStatus(context.Background(), "owner-1", "missing")

	require.Error(t, err)
	assert.Equal(t, appErrors.ErrMachineNotFound, err)
}

func TestMachineService_Status_MissingAccessIsDenied(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()
	connManager := testutils.NewMockConnectionManagerWithProbeError(errors.New("probe must not run"))
	encryption := testutils.NewMockEncryptionService()

	machineRepo.AddMachine(&models.Machine{ID: "machine-1", Name: "m", OwnerID: "owner-1"})

	service := NewMachineService(machineRepo, accessRepo, connManager, encryption)

	_, _, err := service.GetMachineStatus(context.Background(), "stranger-1", "machine-1")

	require.Error(t, err)
	assert.Equal(t, appErrors.ErrAccessDenied, err)
}

func TestMachineService_Status_ProbeReceivesMachineAndUserIDs(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()
	connManager := testutils.NewMockConnectionManager()
	encryption := testutils.NewMockEncryptionService()

	seedStatusMachine(t, machineRepo, accessRepo, "machine-1", "owner-1", "owner-1")

	service := NewMachineService(machineRepo, accessRepo, connManager, encryption)

	_, _, err := service.GetMachineStatus(context.Background(), "owner-1", "machine-1")
	require.NoError(t, err)

	assert.Equal(t, []testutils.MockProbeCall{{MachineID: "machine-1", UserID: "owner-1"}}, connManager.ProbeCalls())
}
