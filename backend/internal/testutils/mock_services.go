package testutils

import (
	"context"
	"fmt"
	"strings"

	"github.com/Homerus25/podland/backend/internal/connections"
	"github.com/Homerus25/podland/backend/internal/sshexec"
	"go.podman.io/podman/v6/pkg/bindings"
)

// MockEncryptionService implements a mock encryption service for testing.
// Mirrors the machines domain's sshKeyEncryptor seam: SSH-key encryption
// only, since that is the only capability any test exercises through it.
type MockEncryptionService struct {
	encryptSSHError error
	encryptSSHFunc  func(string) ([]byte, string, error)
}

func NewMockEncryptionService() *MockEncryptionService {
	return &MockEncryptionService{}
}

func NewMockEncryptionServiceWithErrors(encryptSSHKeyErr error) *MockEncryptionService {
	return &MockEncryptionService{
		encryptSSHError: encryptSSHKeyErr,
	}
}

// NewMockEncryptionServiceWithCustomFunctions allows custom SSH-key encryption logic
func NewMockEncryptionServiceWithCustomFunctions(
	encryptSSHFunc func(string) ([]byte, string, error),
) *MockEncryptionService {
	return &MockEncryptionService{
		encryptSSHFunc: encryptSSHFunc,
	}
}

func (m *MockEncryptionService) EncryptSSHKey(privateKey string) ([]byte, string, error) {
	if m.encryptSSHError != nil {
		return nil, "", m.encryptSSHError
	}
	if m.encryptSSHFunc != nil {
		return m.encryptSSHFunc(privateKey)
	}
	// Simple mock SSH encryption
	encrypted := fmt.Appendf(nil, "ssh-encrypted:%s", privateKey)
	fingerprint := fmt.Sprintf("sha256:%x", []byte(privateKey))
	return encrypted, fingerprint, nil
}

// MockTestConnectionCall records a single TestConnection invocation so
// tests can assert the probe received the expected (defaulted) raw
// credentials. Defined locally (rather than reusing a domain type) so
// testutils does not import the machines package and create a test
// import cycle with the white-box machines tests.
type MockTestConnectionCall struct {
	Hostname         string
	Port             int
	SSHUsername      string
	SSHPrivateKey    string
	PodmanSocketPath string
}

// MockConnectionManager implements the connections.Manager seam for
// testing.
type MockConnectionManager struct {
	acquireErr          error
	testConnectionErr   error
	testConnectionCalls []MockTestConnectionCall
	probeErr            error
	probeCalls          []MockProbeCall
	withConnectionCalls []MockWithConnectionCall
	removeMachineCalls  []string
}

// MockWithConnectionCall records a single WithConnection invocation.
type MockWithConnectionCall struct {
	Op        string
	MachineID string
	UserID    string
}

// The mock satisfies the full connections.Manager seam so services can
// depend on the interface in production wiring and tests.
var _ connections.Manager = (*MockConnectionManager)(nil)

// MockProbeCall records a single ProbeMachine invocation.
type MockProbeCall struct {
	MachineID string
	UserID    string
}

func NewMockConnectionManager() *MockConnectionManager {
	return &MockConnectionManager{}
}

// NewMockConnectionManagerWithProbeError returns a MockConnectionManager
// whose ProbeMachine always fails with the given error. Use this to
// exercise the live status probe's failure branch (probe error and probe
// timeout both map to "offline").
func NewMockConnectionManagerWithProbeError(probeErr error) *MockConnectionManager {
	return &MockConnectionManager{
		probeErr: probeErr,
	}
}

func NewMockConnectionManagerWithAcquireError(acquireErr error) *MockConnectionManager {
	return &MockConnectionManager{
		acquireErr: acquireErr,
	}
}

// mockConnectionContext is the stand-in connection context both mock
// surfaces hand out. It carries no real podman client, so tunnel engine
// calls through it fail — the shape unit tests rely on.
func mockConnectionContext() context.Context {
	return context.WithValue(context.Background(), "mock-connection", &bindings.Connection{})
}

// NewMockConnectionManagerWithTestError returns a MockConnectionManager
// whose TestConnection always fails with the given error. Use this to
// exercise the registration connection-test failure branch in
// MachineService.Create (the machine must not be persisted and Create
// must surface ErrConnectionTestFailed).
func NewMockConnectionManagerWithTestError(testErr error) *MockConnectionManager {
	return &MockConnectionManager{
		testConnectionErr: testErr,
	}
}

// WithConnection runs fn with a mock connection context, mirroring the
// production contract: an injected acquisition error short-circuits fn and
// crosses as-is (the 502 mapping is the production manager's job),
// otherwise fn's error propagates untouched.
func (m *MockConnectionManager) WithConnection(ctx context.Context, userID, machineID, op string, fn func(connCtx context.Context) error) error {
	m.withConnectionCalls = append(m.withConnectionCalls, MockWithConnectionCall{Op: op, MachineID: machineID, UserID: userID})
	if m.acquireErr != nil {
		return m.acquireErr
	}
	return fn(mockConnectionContext())
}

// WithConnectionCalls returns a snapshot of every WithConnection
// invocation, in call order.
func (m *MockConnectionManager) WithConnectionCalls() []MockWithConnectionCall {
	out := make([]MockWithConnectionCall, len(m.withConnectionCalls))
	copy(out, m.withConnectionCalls)
	return out
}

// RemoveMachine records the invalidation. MachineService.Delete must call
// it after a successful DB delete; the recording lets service tests pin
// the teardown ordering and the failed-delete-evicts-nothing rule.
func (m *MockConnectionManager) RemoveMachine(machineID string) {
	m.removeMachineCalls = append(m.removeMachineCalls, machineID)
}

// RemoveMachineCalls returns a snapshot of every RemoveMachine invocation,
// in call order.
func (m *MockConnectionManager) RemoveMachineCalls() []string {
	out := make([]string, len(m.removeMachineCalls))
	copy(out, m.removeMachineCalls)
	return out
}

// Stop mirrors the production seam: the mock owns no background workers,
// so shutdown is a no-op.
func (m *MockConnectionManager) Stop() {
}

// ProbeMachine records the call and returns the injected error. The
// default — no error set — is success (the machine is online).
func (m *MockConnectionManager) ProbeMachine(machineID, userID string) error {
	m.probeCalls = append(m.probeCalls, MockProbeCall{MachineID: machineID, UserID: userID})
	return m.probeErr
}

// ProbeCalls returns a snapshot of every ProbeMachine invocation.
func (m *MockConnectionManager) ProbeCalls() []MockProbeCall {
	out := make([]MockProbeCall, len(m.probeCalls))
	copy(out, m.probeCalls)
	return out
}

// TestConnection records the probe arguments and returns the injected
// error. The default — no error set — is success (nil), so existing
// happy-path machine-service tests keep passing once Create calls the
// probe.
func (m *MockConnectionManager) TestConnection(ctx context.Context, hostname string, port int, sshUsername, sshPrivateKey, podmanSocketPath string) error {
	m.testConnectionCalls = append(m.testConnectionCalls, MockTestConnectionCall{
		Hostname:         hostname,
		Port:             port,
		SSHUsername:      sshUsername,
		SSHPrivateKey:    sshPrivateKey,
		PodmanSocketPath: podmanSocketPath,
	})
	return m.testConnectionErr
}

// TestConnectionCalls returns a snapshot of the arguments passed to every
// TestConnection invocation, for asserting the probe received the
// expected (defaulted) credentials.
func (m *MockConnectionManager) TestConnectionCalls() []MockTestConnectionCall {
	out := make([]MockTestConnectionCall, len(m.testConnectionCalls))
	copy(out, m.testConnectionCalls)
	return out
}

// MockSSHExec implements sshexec.SSHExecInterface for tests. It supports
// both error-injection (the simple cases: a single fixed result or a single
// fixed error) and custom-function injection (per-call scripting, used when
// a test needs to vary behaviour across multiple Exec calls).
//
// Mirrors the MockEncryptionService mock pattern (error fields + func fields).
type MockSSHExec struct {
	execResult sshexec.ExecResult
	execError  error
	execFunc   func(ctx context.Context, machineID, userID, command string, opts sshexec.ExecOptions) (sshexec.ExecResult, error)
	execCalls  []MockSSHExecCall

	streamFunc   func(ctx context.Context, machineID, userID, command string, opts sshexec.ExecOptions, onLine func(sshexec.OutputLine)) (sshexec.ExecResult, error)
	streamResult *sshexec.ExecResult
	streamError  error
	streamCalls  []MockSSHExecStreamCall
}

// MockSSHExecCall records a single Exec invocation, for assertion in tests.
type MockSSHExecCall struct {
	MachineID string
	UserID    string
	Command   string
	Opts      sshexec.ExecOptions
}

// MockSSHExecStreamCall records a single ExecStream invocation, for
// assertion in tests. Lines holds every output line the mock emitted, in
// emission order.
type MockSSHExecStreamCall struct {
	MachineID string
	UserID    string
	Command   string
	Opts      sshexec.ExecOptions
	Lines     []sshexec.OutputLine
}

// NewMockSSHExec returns a MockSSHExec whose Exec succeeds with a zero
// exit code and empty output.
func NewMockSSHExec() *MockSSHExec {
	return &MockSSHExec{}
}

// NewMockSSHExecWithResult returns a MockSSHExec whose Exec always returns
// the given result (and no error). Use this for happy-path quadlet-service
// tests where the remote systemctl produced a specific stdout/stderr/exit.
func NewMockSSHExecWithResult(result sshexec.ExecResult) *MockSSHExec {
	return &MockSSHExec{execResult: result}
}

// NewMockSSHExecWithError returns a MockSSHExec whose Exec always fails with
// the given error. Use this to exercise SSH-layer failure handling in
// callers (e.g. CONNECTION_FAILED mapping).
func NewMockSSHExecWithError(err error) *MockSSHExec {
	return &MockSSHExec{execError: err}
}

// NewMockSSHExecWithStreamResult returns a MockSSHExec whose ExecStream
// emits the given result's stdout/stderr as streamed lines (stdout first,
// then stderr) and returns the result with no error. Exec behaviour is
// unaffected.
func NewMockSSHExecWithStreamResult(result sshexec.ExecResult) *MockSSHExec {
	return &MockSSHExec{streamResult: &result}
}

// NewMockSSHExecWithStreamError returns a MockSSHExec whose ExecStream
// fails with the given error, for exercising SSH-layer failure handling in
// streaming callers. Exec behaviour is unaffected.
func NewMockSSHExecWithStreamError(err error) *MockSSHExec {
	return &MockSSHExec{streamError: err}
}

// NewMockSSHExecWithStreamFunc returns a MockSSHExec that delegates each
// ExecStream to the supplied function, so streaming tests can script exact
// line emission and per-call results.
func NewMockSSHExecWithStreamFunc(f func(ctx context.Context, machineID, userID, command string, opts sshexec.ExecOptions, onLine func(sshexec.OutputLine)) (sshexec.ExecResult, error)) *MockSSHExec {
	return &MockSSHExec{streamFunc: f}
}

// NewMockSSHExecWithFunc returns a MockSSHExec that delegates each Exec to
// the supplied function. Use this when a test needs to vary behaviour across
// calls (e.g. first call returns "already running", second returns success).
func NewMockSSHExecWithFunc(f func(ctx context.Context, machineID, userID, command string, opts sshexec.ExecOptions) (sshexec.ExecResult, error)) *MockSSHExec {
	return &MockSSHExec{execFunc: f}
}

// Exec implements sshexec.SSHExecInterface.
func (m *MockSSHExec) Exec(ctx context.Context, machineID, userID, command string, opts sshexec.ExecOptions) (sshexec.ExecResult, error) {
	m.execCalls = append(m.execCalls, MockSSHExecCall{
		MachineID: machineID,
		UserID:    userID,
		Command:   command,
		Opts:      opts,
	})
	if m.execFunc != nil {
		return m.execFunc(ctx, machineID, userID, command, opts)
	}
	if m.execError != nil {
		return sshexec.ExecResult{}, m.execError
	}
	return m.execResult, nil
}

// ExecStream implements sshexec.SSHExecInterface.
//
// When a stream func is configured it is invoked as-is. Otherwise the mock
// uses the configured streaming result/error if any, falling back to the
// configured Exec result/error — emitting the result's captured stdout and
// stderr lines through onLine (stdout first, then stderr) so callers that
// only configured Exec behaviour still work.
func (m *MockSSHExec) ExecStream(ctx context.Context, machineID, userID, command string, opts sshexec.ExecOptions, onLine func(sshexec.OutputLine)) (sshexec.ExecResult, error) {
	call := MockSSHExecStreamCall{
		MachineID: machineID,
		UserID:    userID,
		Command:   command,
		Opts:      opts,
	}
	defer func() { m.streamCalls = append(m.streamCalls, call) }()

	if m.streamFunc != nil {
		result, err := m.streamFunc(ctx, machineID, userID, command, opts, func(line sshexec.OutputLine) {
			call.Lines = append(call.Lines, line)
			onLine(line)
		})
		return result, err
	}

	streamErr := m.streamError
	if streamErr != nil {
		return sshexec.ExecResult{}, streamErr
	}
	result := m.execResult
	if m.streamResult != nil {
		result = *m.streamResult
	}
	for _, line := range splitLines(result.Stdout) {
		call.Lines = append(call.Lines, sshexec.OutputLine{Stream: sshexec.StdoutStream, Text: line})
		onLine(call.Lines[len(call.Lines)-1])
	}
	for _, line := range splitLines(result.Stderr) {
		call.Lines = append(call.Lines, sshexec.OutputLine{Stream: sshexec.StderrStream, Text: line})
		onLine(call.Lines[len(call.Lines)-1])
	}
	return result, nil
}

// splitLines splits captured output into lines, dropping the empty trailing
// element produced by a trailing newline (and returning nothing for empty
// output).
func splitLines(out string) []string {
	out = strings.TrimSuffix(out, "\n")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// StreamCalls returns a snapshot of every ExecStream invocation recorded by
// the mock, including the emitted lines.
func (m *MockSSHExec) StreamCalls() []MockSSHExecStreamCall {
	out := make([]MockSSHExecStreamCall, len(m.streamCalls))
	copy(out, m.streamCalls)
	return out
}

// Calls returns a snapshot of every Exec invocation recorded by the mock.
// Tests use this to assert that callers invoked SSH exec with the expected
// command + args (e.g. systemctl start <unit>).
func (m *MockSSHExec) Calls() []MockSSHExecCall {
	out := make([]MockSSHExecCall, len(m.execCalls))
	copy(out, m.execCalls)
	return out
}

// CallCount returns the number of times Exec has been invoked.
func (m *MockSSHExec) CallCount() int {
	return len(m.execCalls)
}

// Helper function to create a test machine service with all mock dependencies
// Note: This returns individual mock components that can be used to create a MachineService
// The MachineService itself is created by the caller to maintain proper package boundaries
func NewTestMachineServiceComponents() (*MockMachineRepository, *MockMachineAccessRepository, *MockConnectionManager, *MockEncryptionService) {
	return NewMockMachineRepository(), NewMockMachineAccessRepository(), NewMockConnectionManager(), NewMockEncryptionService()
}
