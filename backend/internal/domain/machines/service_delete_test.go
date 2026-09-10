package machines

import (
	"context"
	"fmt"
	"testing"

	"github.com/Homerus25/podland/backend/internal/connections"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSSHInvalidator is the machines-local stand-in for the real
// sshexec.Service at the sshInvalidator seam: it records every
// RemoveMachine call in order. Every NewMachineService call in the
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

func (f *fakeSSHInvalidator) RemoveMachine(machineID string) {
	f.calls = append(f.calls, machineID)
	if f.events != nil {
		*f.events = append(*f.events, "ssh:"+machineID)
	}
}

// evictingConnManager wraps the recording MockConnectionManager and logs
// each RemoveMachine into a shared event list, so a test can pin the
// cache eviction's position relative to the SSH-pool invalidation.
type evictingConnManager struct {
	connections.Manager
	events *[]string
}

func (m *evictingConnManager) RemoveMachine(machineID string) {
	*m.events = append(*m.events, "cache:"+machineID)
	m.Manager.RemoveMachine(machineID)
}

// TestMachineService_Delete_TeardownOrdering pins the delete path's full
// teardown: DB row removal first, then the connection cache's
// RemoveMachine, then the SSH-exec pool invalidation.
func TestMachineService_Delete_TeardownOrdering(t *testing.T) {
	var events []string
	machineRepo := testutils.NewMockMachineRepository()
	accessRepo := testutils.NewMockMachineAccessRepository()
	connMock := testutils.NewMockConnectionManager()
	connManager := &evictingConnManager{Manager: connMock, events: &events}
	sshPool := newFakeSSHInvalidator()
	sshPool.events = &events
	encryption := testutils.NewMockEncryptionService()

	ownerID := "owner-1"
	machine := testutils.GenerateTestMachineWithOwner(ownerID)
	machineRepo.AddMachine(machine)

	service := NewMachineService(machineRepo, accessRepo, connManager, encryption, sshPool)

	require.NoError(t, service.Delete(context.Background(), ownerID, machine.ID))

	gone, err := machineRepo.GetByID(machine.ID)
	require.NoError(t, err)
	assert.Nil(t, gone, "the DB row must be removed before teardown runs")
	assert.Equal(t, []string{machine.ID}, connMock.RemoveMachineCalls(),
		"the connection cache must be told to remove the machine")
	assert.Equal(t, []string{"cache:" + machine.ID, "ssh:" + machine.ID}, events,
		"teardown must evict the connection cache, then invalidate the SSH-exec pool")
	assert.Equal(t, []string{machine.ID}, sshPool.calls,
		"the SSH-exec pool must be invalidated for the deleted machine")
}

// TestMachineService_Delete_FailedDeleteEvictsNothing pins that teardown
// runs only after a successful DB delete: a failing delete must not evict
// a live machine's cached connection or pooled SSH client.
func TestMachineService_Delete_FailedDeleteEvictsNothing(t *testing.T) {
	machineRepo := testutils.NewMockMachineRepositoryWithErrors(nil, nil, nil, nil, fmt.Errorf("database down"))
	accessRepo := testutils.NewMockMachineAccessRepository()
	connMock := testutils.NewMockConnectionManager()
	sshPool := newFakeSSHInvalidator()
	encryption := testutils.NewMockEncryptionService()

	ownerID := "owner-1"
	machine := testutils.GenerateTestMachineWithOwner(ownerID)
	machineRepo.AddMachine(machine)

	service := NewMachineService(machineRepo, accessRepo, connMock, encryption, sshPool)

	err := service.Delete(context.Background(), ownerID, machine.ID)

	require.Error(t, err)
	assert.Empty(t, connMock.RemoveMachineCalls(),
		"a failed delete must not evict the machine's cached connection")
	assert.Empty(t, sshPool.calls,
		"a failed delete must not invalidate the machine's pooled SSH client")
}
