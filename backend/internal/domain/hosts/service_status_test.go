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

// seedStatusHost persists a host owned by ownerID and grants the
// caller access, so status checks reach the probe rather than stopping at
// the not-found / access-denied gates.
func seedStatusHost(t *testing.T, hostRepo *testutils.MockHostRepository, accessRepo *testutils.MockHostAccessRepository, hostID, ownerID, callerID string) {
	t.Helper()
	hostRepo.AddHost(&models.Host{ID: hostID, Name: "m", OwnerID: ownerID})
	accessRepo.AddAccess(hostID, callerID, models.PermissionRead, time.Now())
}

func TestHostService_Status_ProbeSuccessIsOnline(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	connManager := testutils.NewMockConnectionManager()
	encryption := testutils.NewMockEncryptionService()

	seedStatusHost(t, hostRepo, accessRepo, "host-1", "owner-1", "owner-1")

	service := NewHostService(hostRepo, accessRepo, connManager, encryption, newFakeSSHInvalidator())

	status, checkedAt, err := service.GetHostStatus(context.Background(), "owner-1", "host-1")

	require.NoError(t, err)
	assert.Equal(t, "online", status)
	assert.False(t, checkedAt.IsZero(), "checkedAt must be set")
}

func TestHostService_Status_ProbeErrorIsOffline(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	connManager := testutils.NewMockConnectionManagerWithProbeError(errors.New("dial tcp: connection refused"))
	encryption := testutils.NewMockEncryptionService()

	seedStatusHost(t, hostRepo, accessRepo, "host-1", "owner-1", "owner-1")

	service := NewHostService(hostRepo, accessRepo, connManager, encryption, newFakeSSHInvalidator())

	status, checkedAt, err := service.GetHostStatus(context.Background(), "owner-1", "host-1")

	require.NoError(t, err, "offline is a valid answer, not an error")
	assert.Equal(t, "offline", status)
	assert.False(t, checkedAt.IsZero())
}

func TestHostService_Status_ProbeTimeoutIsOffline(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	connManager := testutils.NewMockConnectionManagerWithProbeError(context.DeadlineExceeded)
	encryption := testutils.NewMockEncryptionService()

	seedStatusHost(t, hostRepo, accessRepo, "host-1", "owner-1", "owner-1")

	service := NewHostService(hostRepo, accessRepo, connManager, encryption, newFakeSSHInvalidator())

	status, _, err := service.GetHostStatus(context.Background(), "owner-1", "host-1")

	require.NoError(t, err)
	assert.Equal(t, "offline", status)
}

func TestHostService_Status_UnknownHostIsNotFound(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	connManager := testutils.NewMockConnectionManagerWithProbeError(errors.New("probe must not run"))
	encryption := testutils.NewMockEncryptionService()

	service := NewHostService(hostRepo, accessRepo, connManager, encryption, newFakeSSHInvalidator())

	_, _, err := service.GetHostStatus(context.Background(), "owner-1", "missing")

	require.Error(t, err)
	assert.Equal(t, appErrors.ErrHostNotFound, err)
}

func TestHostService_Status_MissingAccessIsDenied(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	connManager := testutils.NewMockConnectionManagerWithProbeError(errors.New("probe must not run"))
	encryption := testutils.NewMockEncryptionService()

	hostRepo.AddHost(&models.Host{ID: "host-1", Name: "m", OwnerID: "owner-1"})

	service := NewHostService(hostRepo, accessRepo, connManager, encryption, newFakeSSHInvalidator())

	_, _, err := service.GetHostStatus(context.Background(), "stranger-1", "host-1")

	require.Error(t, err)
	assert.Equal(t, appErrors.ErrAccessDenied, err)
}

func TestHostService_Status_ProbeReceivesHostAndUserIDs(t *testing.T) {
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	connManager := testutils.NewMockConnectionManager()
	encryption := testutils.NewMockEncryptionService()

	seedStatusHost(t, hostRepo, accessRepo, "host-1", "owner-1", "owner-1")

	service := NewHostService(hostRepo, accessRepo, connManager, encryption, newFakeSSHInvalidator())

	_, _, err := service.GetHostStatus(context.Background(), "owner-1", "host-1")
	require.NoError(t, err)

	assert.Equal(t, []testutils.MockProbeCall{{HostID: "host-1", UserID: "owner-1"}}, connManager.ProbeCalls())
}
