package quadlets

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Homerus25/podland/backend/internal/sshexec"
	"github.com/Homerus25/podland/backend/internal/testutils"
)

func newAutoUpdateService(mockSSH *testutils.MockSSHExec) *QuadletService {
	return NewQuadletService(testutils.NewMockConnectionManager(), mockSSH)
}

func TestQuadletService_AutoUpdate_CommandAssembly(t *testing.T) {
	tests := []struct {
		name     string
		dryRun   bool
		wantArgs []string
	}{
		{name: "without dry-run", dryRun: false, wantArgs: []string{"auto-update", "--format", "json"}},
		{name: "with dry-run", dryRun: true, wantArgs: []string{"auto-update", "--format", "json", "--dry-run"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSSH := testutils.NewMockSSHExecWithStreamResult(sshexec.ExecResult{Stdout: "[]", ExitCode: 0})
			svc := newAutoUpdateService(mockSSH)

			_, err := svc.AutoUpdate(context.Background(), "user-1", "machine-1", tt.dryRun, func(sshexec.OutputLine) {})

			require.NoError(t, err)
			require.Len(t, mockSSH.StreamCalls(), 1)
			call := mockSSH.StreamCalls()[0]
			assert.Equal(t, "machine-1", call.MachineID)
			assert.Equal(t, "user-1", call.UserID)
			assert.Equal(t, "podman", call.Command)
			assert.Equal(t, tt.wantArgs, call.Opts.Args)
		})
	}
}

func TestQuadletService_AutoUpdate_LinesForwardedInArrivalOrder(t *testing.T) {
	mockSSH := testutils.NewMockSSHExecWithStreamFunc(func(ctx context.Context, machineID, userID, command string, opts sshexec.ExecOptions, onLine func(sshexec.OutputLine)) (sshexec.ExecResult, error) {
		onLine(sshexec.OutputLine{Stream: sshexec.StdoutStream, Text: "line-1"})
		onLine(sshexec.OutputLine{Stream: sshexec.StderrStream, Text: "err-1"})
		onLine(sshexec.OutputLine{Stream: sshexec.StdoutStream, Text: "line-2"})
		return sshexec.ExecResult{Stdout: "[]", ExitCode: 0}, nil
	})
	svc := newAutoUpdateService(mockSSH)

	var got []sshexec.OutputLine
	_, err := svc.AutoUpdate(context.Background(), "user-1", "machine-1", false, func(line sshexec.OutputLine) {
		got = append(got, line)
	})

	require.NoError(t, err)
	assert.Equal(t, []sshexec.OutputLine{
		{Stream: sshexec.StdoutStream, Text: "line-1"},
		{Stream: sshexec.StderrStream, Text: "err-1"},
		{Stream: sshexec.StdoutStream, Text: "line-2"},
	}, got)
}

func TestQuadletService_AutoUpdate_ParsesReportExitZero(t *testing.T) {
	stdout := "[{\"Unit\":\"web.service\",\"ContainerName\":\"web\",\"ImageName\":\"nginx:latest\",\"Updated\":\"true\"}," +
		"{\"Unit\":\"api.service\",\"ContainerName\":\"api\",\"ImageName\":\"api:1.0\",\"Updated\":\"false\"}]"
	mockSSH := testutils.NewMockSSHExecWithStreamResult(sshexec.ExecResult{Stdout: stdout, ExitCode: 0})
	svc := newAutoUpdateService(mockSSH)

	report, err := svc.AutoUpdate(context.Background(), "user-1", "machine-1", false, func(sshexec.OutputLine) {})

	require.NoError(t, err)
	assert.Equal(t, AutoUpdateCompleted, report.Overall)
	assert.Equal(t, 0, report.ExitCode)
	require.Len(t, report.Containers, 2)
	assert.Equal(t, AutoUpdateItem{
		ContainerName: "web", ImageName: "nginx:latest", Updated: AutoUpdateItemUpdated, Unit: "web.service",
	}, report.Containers[0])
	assert.Equal(t, AutoUpdateItem{
		ContainerName: "api", ImageName: "api:1.0", Updated: AutoUpdateItemUpToDate, Unit: "api.service",
	}, report.Containers[1])
}

func TestQuadletService_AutoUpdate_NonZeroExitWithFailuresIsData(t *testing.T) {
	stdout := "[{\"Unit\":\"web.service\",\"ContainerName\":\"web\",\"ImageName\":\"nginx:latest\",\"Updated\":\"true\"}," +
		"{\"Unit\":\"db.service\",\"ContainerName\":\"db\",\"ImageName\":\"postgres:16\",\"Updated\":\"false\",\"Error\":\"database is locked\"}]"
	mockSSH := testutils.NewMockSSHExecWithStreamResult(sshexec.ExecResult{Stdout: stdout, ExitCode: 3})
	svc := newAutoUpdateService(mockSSH)

	report, err := svc.AutoUpdate(context.Background(), "user-1", "machine-1", false, func(sshexec.OutputLine) {})

	require.NoError(t, err)
	assert.Equal(t, AutoUpdateWithFailures, report.Overall)
	assert.Equal(t, 3, report.ExitCode)
	require.Len(t, report.Containers, 2)
	assert.Equal(t, AutoUpdateItemUpdated, report.Containers[0].Updated)
	assert.Equal(t, AutoUpdateItemFailed, report.Containers[1].Updated)
	assert.Equal(t, "database is locked", report.Containers[1].Error)
}

func TestQuadletService_AutoUpdate_DryRunReportsPendingAndDryRunOverall(t *testing.T) {
	stdout := "[{\"Unit\":\"web.service\",\"ContainerName\":\"web\",\"ImageName\":\"nginx:latest\",\"Updated\":\"pending\"}]"
	mockSSH := testutils.NewMockSSHExecWithStreamResult(sshexec.ExecResult{Stdout: stdout, ExitCode: 0})
	svc := newAutoUpdateService(mockSSH)

	report, err := svc.AutoUpdate(context.Background(), "user-1", "machine-1", true, func(sshexec.OutputLine) {})

	require.NoError(t, err)
	assert.Equal(t, AutoUpdateDryRun, report.Overall)
	require.Len(t, report.Containers, 1)
	assert.Equal(t, AutoUpdateItemPending, report.Containers[0].Updated)
}

func TestQuadletService_AutoUpdate_UnparsableOutputCompletesWithoutError(t *testing.T) {
	mockSSH := testutils.NewMockSSHExecWithStreamResult(sshexec.ExecResult{Stdout: "not json at all", ExitCode: 0})
	svc := newAutoUpdateService(mockSSH)

	report, err := svc.AutoUpdate(context.Background(), "user-1", "machine-1", false, func(sshexec.OutputLine) {})

	require.NoError(t, err)
	assert.Equal(t, AutoUpdateUnparsable, report.Overall)
	assert.Nil(t, report.Containers)
}

func TestQuadletService_AutoUpdate_SSHErrorPropagates(t *testing.T) {
	mockSSH := testutils.NewMockSSHExecWithStreamError(errors.New("dial failed"))
	svc := newAutoUpdateService(mockSSH)

	report, err := svc.AutoUpdate(context.Background(), "user-1", "machine-1", false, func(sshexec.OutputLine) {})

	require.Error(t, err)
	assert.Equal(t, AutoUpdateReport{}, report)
}
