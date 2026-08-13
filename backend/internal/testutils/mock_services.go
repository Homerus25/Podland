package testutils

import (
	"context"
	"fmt"

	"github.com/yourorg/podland/backend/internal/sshexec"
	"go.podman.io/podman/v6/pkg/bindings"
)

// MockEncryptionService implements a mock encryption service for testing
type MockEncryptionService struct {
	encryptError   error
	decryptError   error
	encryptFunc    func(string) ([]byte, error)
	decryptFunc    func([]byte) (string, error)
	encryptSSHFunc func(string) ([]byte, string, error)
	decryptSSHFunc func([]byte) (string, error)
}

func NewMockEncryptionService() *MockEncryptionService {
	return &MockEncryptionService{}
}

func NewMockEncryptionServiceWithErrors(encryptErr, decryptErr error) *MockEncryptionService {
	return &MockEncryptionService{
		encryptError: encryptErr,
		decryptError: decryptErr,
	}
}

// NewMockEncryptionServiceWithCustomFunctions allows custom encryption/decryption logic
func NewMockEncryptionServiceWithCustomFunctions(
	encryptFunc func(string) ([]byte, error),
	decryptFunc func([]byte) (string, error),
	encryptSSHFunc func(string) ([]byte, string, error),
	decryptSSHFunc func([]byte) (string, error),
) *MockEncryptionService {
	return &MockEncryptionService{
		encryptFunc:    encryptFunc,
		decryptFunc:    decryptFunc,
		encryptSSHFunc: encryptSSHFunc,
		decryptSSHFunc: decryptSSHFunc,
	}
}

func (m *MockEncryptionService) Encrypt(data string) ([]byte, error) {
	if m.encryptError != nil {
		return nil, m.encryptError
	}
	if m.encryptFunc != nil {
		return m.encryptFunc(data)
	}
	// Simple mock encryption - just convert to bytes with prefix
	return fmt.Appendf(nil, "encrypted:%s", data), nil
}

func (m *MockEncryptionService) Decrypt(data []byte) (string, error) {
	if m.decryptError != nil {
		return "", m.decryptError
	}
	if m.decryptFunc != nil {
		return m.decryptFunc(data)
	}
	// Simple mock decryption - remove the prefix
	dataStr := string(data)
	if len(dataStr) > len("encrypted:") && dataStr[:len("encrypted:")] == "encrypted:" {
		return dataStr[len("encrypted:"):], nil
	}
	return dataStr, nil
}

func (m *MockEncryptionService) EncryptSSHKey(privateKey string) ([]byte, string, error) {
	if m.encryptError != nil {
		return nil, "", m.encryptError
	}
	if m.encryptSSHFunc != nil {
		return m.encryptSSHFunc(privateKey)
	}
	// Simple mock SSH encryption
	encrypted := fmt.Appendf(nil, "ssh-encrypted:%s", privateKey)
	fingerprint := fmt.Sprintf("sha256:%x", []byte(privateKey))
	return encrypted, fingerprint, nil
}

func (m *MockEncryptionService) DecryptSSHKey(encryptedKey []byte) (string, error) {
	if m.decryptError != nil {
		return "", m.decryptError
	}
	if m.decryptSSHFunc != nil {
		return m.decryptSSHFunc(encryptedKey)
	}
	// Simple mock SSH decryption
	dataStr := string(encryptedKey)
	if len(dataStr) > len("ssh-encrypted:") && dataStr[:len("ssh-encrypted:")] == "ssh-encrypted:" {
		return dataStr[len("ssh-encrypted:"):], nil
	}
	return dataStr, nil
}

// MockPodmanConnectionService implements a mock Podman connection service
type MockPodmanConnectionService struct {
	connectError error
	closeError   error
}

func NewMockPodmanConnectionService() *MockPodmanConnectionService {
	return &MockPodmanConnectionService{}
}

func NewMockPodmanConnectionServiceWithErrors(connectErr, closeErr error) *MockPodmanConnectionService {
	return &MockPodmanConnectionService{
		connectError: connectErr,
		closeError:   closeErr,
	}
}

func (m *MockPodmanConnectionService) Connect(ctx context.Context, machineID string, sshConfig any) (any, error) {
	if m.connectError != nil {
		return nil, m.connectError
	}
	// Return a mock connection
	return &MockPodmanConnection{MachineID: machineID}, nil
}

func (m *MockPodmanConnectionService) Close(ctx context.Context, connection any) error {
	if m.closeError != nil {
		return m.closeError
	}
	return nil
}

// MockPodmanConnection represents a mock Podman connection
type MockPodmanConnection struct {
	MachineID string
}

func (m *MockPodmanConnection) GetMachineID() string {
	return m.MachineID
}

func (m *MockPodmanConnection) Execute(command string, args ...any) (any, error) {
	// Simple mock execution - just return the command
	return fmt.Sprintf("executed: %s %v", command, args), nil
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

// MockConnectionManager implements a mock connection manager for testing
type MockConnectionManager struct {
	getConnectionErr    error
	testConnectionErr   error
	testConnectionCalls []MockTestConnectionCall
}

func NewMockConnectionManager() *MockConnectionManager {
	return &MockConnectionManager{}
}

func NewMockConnectionManagerWithError(getConnectionErr error) *MockConnectionManager {
	return &MockConnectionManager{
		getConnectionErr: getConnectionErr,
	}
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

func (m *MockConnectionManager) GetConnection(machineID, userID string) (context.Context, error) {
	if m.getConnectionErr != nil {
		return nil, m.getConnectionErr
	}
	// Return a mock context with a mock connection
	mockConn := &bindings.Connection{}
	mockCtx := context.WithValue(context.Background(), "mock-connection", mockConn)
	return mockCtx, nil
}

func (m *MockConnectionManager) Release(machineID string) {
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
}

// MockSSHExecCall records a single Exec invocation, for assertion in tests.
type MockSSHExecCall struct {
	MachineID string
	UserID    string
	Command   string
	Opts      sshexec.ExecOptions
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
