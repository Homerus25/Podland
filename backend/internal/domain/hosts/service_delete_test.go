package hosts

import (
	"context"
	"fmt"
	"testing"

	"github.com/Homerus25/podland/backend/internal/connections"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSSHInvalidator is the hosts-local stand-in for the real
// sshexec.Service at the sshInvalidator seam: it records every
// RemoveHost call in order. Every NewHostService call in the
// package's tests passes one.
type fakeSSHInvalidator struct {
	calls []string
	// events, when set, mirrors each call into a shared ordered event
	// list so a test can pin ordering against other teardown steps.
	events *[]string
}

func newFakeSSHInvalidator() *fakeSSHInvalidator {
	return &fakeSSHInvalidator{}
}

func (f *fakeSSHInvalidator) RemoveHost(hostID string) {
	f.calls = append(f.calls, hostID)
	if f.events != nil {
		*f.events = append(*f.events, "ssh:"+hostID)
	}
}

// evictingConnManager wraps the recording MockConnectionManager and logs
// each RemoveHost into a shared event list, so a test can pin the
// cache eviction's position relative to the SSH-pool invalidation.
type evictingConnManager struct {
	connections.Manager
	events *[]string
}

func (m *evictingConnManager) RemoveHost(hostID string) {
	*m.events = append(*m.events, "cache:"+hostID)
	m.Manager.RemoveHost(hostID)
}

// TestHostService_Delete_TeardownOrdering pins the delete path's full
// teardown: DB row removal first, then the connection cache's
// RemoveHost, then the SSH-exec pool invalidation.
func TestHostService_Delete_TeardownOrdering(t *testing.T) {
	var events []string
	hostRepo := testutils.NewMockHostRepository()
	accessRepo := testutils.NewMockHostAccessRepository()
	connMock := testutils.NewMockConnectionManager()
	connManager := &evictingConnManager{Manager: connMock, events: &events}
	sshPool := newFakeSSHInvalidator()
	sshPool.events = &events
	encryption := testutils.NewMockEncryptionService()

	ownerID := "owner-1"
	host := testutils.GenerateTestHostWithOwner(ownerID)
	hostRepo.AddHost(host)

	service := NewHostService(hostRepo, accessRepo, connManager, encryption, sshPool)

	require.NoError(t, service.Delete(context.Background(), ownerID, host.ID))

	gone, err := hostRepo.GetByID(host.ID)
	require.NoError(t, err)
	assert.Nil(t, gone, "the DB row must be removed before teardown runs")
	assert.Equal(t, []string{host.ID}, connMock.RemoveHostCalls(),
		"the connection cache must be told to remove the host")
	assert.Equal(t, []string{"cache:" + host.ID, "ssh:" + host.ID}, events,
		"teardown must evict the connection cache, then invalidate the SSH-exec pool")
	assert.Equal(t, []string{host.ID}, sshPool.calls,
		"the SSH-exec pool must be invalidated for the deleted host")
}

// TestHostService_Delete_FailedDeleteEvictsNothing pins that teardown
// runs only after a successful DB delete: a failing delete must not evict
// a live host's cached connection or pooled SSH client.
func TestHostService_Delete_FailedDeleteEvictsNothing(t *testing.T) {
	hostRepo := testutils.NewMockHostRepositoryWithErrors(nil, nil, nil, nil, fmt.Errorf("database down"))
	accessRepo := testutils.NewMockHostAccessRepository()
	connMock := testutils.NewMockConnectionManager()
	sshPool := newFakeSSHInvalidator()
	encryption := testutils.NewMockEncryptionService()

	ownerID := "owner-1"
	host := testutils.GenerateTestHostWithOwner(ownerID)
	hostRepo.AddHost(host)

	service := NewHostService(hostRepo, accessRepo, connMock, encryption, sshPool)

	err := service.Delete(context.Background(), ownerID, host.ID)

	require.Error(t, err)
	assert.Empty(t, connMock.RemoveHostCalls(),
		"a failed delete must not evict the host's cached connection")
	assert.Empty(t, sshPool.calls,
		"a failed delete must not invalidate the host's pooled SSH client")
}
