package testutils

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/sshexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMockUserRepository tests the mock user repository functionality
func TestMockUserRepository(t *testing.T) {
	t.Run("basic operations", func(t *testing.T) {
		repo := NewMockUserRepository()

		// Test creating a user
		user := GenerateTestUser()
		err := repo.Create(user)
		assert.NoError(t, err)

		// Test getting user by ID
		fetchedUser, err := repo.GetByID(user.ID)
		assert.NoError(t, err)
		assert.NotNil(t, fetchedUser)
		assert.Equal(t, user.ID, fetchedUser.ID)

		// Test getting user by username
		fetchedByUsername, err := repo.GetByUsername(user.Username)
		assert.NoError(t, err)
		assert.NotNil(t, fetchedByUsername)
		assert.Equal(t, user.Username, fetchedByUsername.Username)

		// Test username exists
		exists, err := repo.UsernameExists(user.Username)
		assert.NoError(t, err)
		assert.True(t, exists)

		// Test non-existent user
		nonExistent, err := repo.GetByID("non-existent-id")
		assert.NoError(t, err)
		assert.Nil(t, nonExistent)

		// Test update user
		user.Username = "updated-username"
		err = repo.Update(user)
		assert.NoError(t, err)

		updatedUser, err := repo.GetByID(user.ID)
		assert.NoError(t, err)
		assert.Equal(t, "updated-username", updatedUser.Username)

		// Test delete user
		err = repo.Delete(user.ID)
		assert.NoError(t, err)

		deletedUser, err := repo.GetByID(user.ID)
		assert.NoError(t, err)
		assert.Nil(t, deletedUser)
	})

	t.Run("error injection", func(t *testing.T) {
		expectedErr := errors.New(" repository error")
		repo := NewMockUserRepositoryWithErrors(
			expectedErr, // createErr
			expectedErr, // getByIDErr
			expectedErr, // getByNameErr
			expectedErr, // updateErr
			expectedErr, // deleteErr
			expectedErr, // existsErr
		)

		user := GenerateTestUser()
		err := repo.Create(user)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "repository error")

		_, err = repo.GetByID(user.ID)
		assert.Error(t, err)

		_, err = repo.GetByUsername(user.Username)
		assert.Error(t, err)

		err = repo.Update(user)
		assert.Error(t, err)

		err = repo.Delete(user.ID)
		assert.Error(t, err)

		_, err = repo.UsernameExists(user.Username)
		assert.Error(t, err)
	})

	t.Run("direct user addition", func(t *testing.T) {
		repo := NewMockUserRepository()
		user := GenerateTestUser()

		// Add user directly without going through Create
		repo.AddUser(user)

		fetchedUser, err := repo.GetByID(user.ID)
		assert.NoError(t, err)
		assert.NotNil(t, fetchedUser)
		assert.Equal(t, user.ID, fetchedUser.ID)
	})
}

// TestMockHostRepository tests the mock host repository functionality
func TestMockHostRepository(t *testing.T) {
	t.Run("basic operations", func(t *testing.T) {
		repo := NewMockHostRepository()

		// Test creating a host
		host := GenerateTestHost()
		err := repo.Create(host)
		assert.NoError(t, err)

		// Test getting host by ID
		fetchedHost, err := repo.GetByID(host.ID)
		assert.NoError(t, err)
		assert.NotNil(t, fetchedHost)
		assert.Equal(t, host.ID, fetchedHost.ID)

		// Test updating host
		host.Name = "updated-host-name"
		err = repo.Update(host)
		assert.NoError(t, err)

		updatedHost, err := repo.GetByID(host.ID)
		assert.NoError(t, err)
		assert.Equal(t, "updated-host-name", updatedHost.Name)

		// Test delete host
		err = repo.Delete(host.ID)
		assert.NoError(t, err)

		deletedHost, err := repo.GetByID(host.ID)
		assert.NoError(t, err)
		assert.Nil(t, deletedHost)
	})

	t.Run("ListAccessible", func(t *testing.T) {
		repo := NewMockHostRepository()

		// Create multiple hosts for different owners
		host1 := GenerateTestHost()
		host2 := GenerateTestHost()
		host2.OwnerID = host1.OwnerID // Same owner
		host3 := GenerateTestHost()

		repo.Create(host1)
		repo.Create(host2)
		repo.Create(host3)

		accessible, err := repo.ListAccessible(host1.OwnerID)
		assert.NoError(t, err)
		assert.Len(t, accessible, 2)
	})

	t.Run("NameExistsForUser", func(t *testing.T) {
		repo := NewMockHostRepository()
		host := GenerateTestHost()
		repo.Create(host)

		exists, err := repo.NameExistsForUser(host.OwnerID, host.Name)
		assert.NoError(t, err)
		assert.True(t, exists)

		notExists, err := repo.NameExistsForUser(host.OwnerID, "non-existent-name")
		assert.NoError(t, err)
		assert.False(t, notExists)
	})

	t.Run("error injection", func(t *testing.T) {
		expectedErr := errors.New("host repository error")
		repo := NewMockHostRepositoryWithErrors(
			expectedErr, // listErr
			expectedErr, // getErr
			expectedErr, // createErr
			expectedErr, // updateErr
			expectedErr, // deleteErr
		)

		host := GenerateTestHost()
		err := repo.Create(host)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "host repository error")

		_, err = repo.GetByID(host.ID)
		assert.Error(t, err)

		err = repo.Delete(host.ID)
		assert.Error(t, err)
	})

	t.Run("direct host addition", func(t *testing.T) {
		repo := NewMockHostRepository()
		host := GenerateTestHost()

		// Add host directly without going through Create
		repo.AddHost(host)

		fetchedHost, err := repo.GetByID(host.ID)
		assert.NoError(t, err)
		assert.NotNil(t, fetchedHost)
		assert.Equal(t, host.ID, fetchedHost.ID)
	})
}

// TestMockEncryptionService tests the mock encryption service functionality
func TestMockEncryptionService(t *testing.T) {
	t.Run("SSH key encryption", func(t *testing.T) {
		service := NewMockEncryptionService()

		sshPrivateKey := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ..."
		encrypted, fingerprint, err := service.EncryptSSHKey(sshPrivateKey)
		assert.NoError(t, err)
		assert.NotNil(t, encrypted)
		assert.NotEmpty(t, fingerprint)
	})

	t.Run("error injection", func(t *testing.T) {
		expectedErr := errors.New("encryption error")
		service := NewMockEncryptionServiceWithErrors(expectedErr)

		_, _, err := service.EncryptSSHKey("ssh-key")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "encryption error")
	})

	t.Run("custom functions", func(t *testing.T) {
		customEncryptSSH := func(privateKey string) ([]byte, string, error) {
			return []byte("ssh-custom-" + privateKey), "custom-fingerprint", nil
		}

		service := NewMockEncryptionServiceWithCustomFunctions(customEncryptSSH)

		encrypted, fingerprint, err := service.EncryptSSHKey("test-key")
		assert.NoError(t, err)
		assert.Equal(t, "ssh-custom-test-key", string(encrypted))
		assert.Equal(t, "custom-fingerprint", fingerprint)
	})
}

// TestTestDataGenerators tests the test data generator functions
func TestTestDataGenerators(t *testing.T) {
	t.Run("GenerateTestUser", func(t *testing.T) {
		user := GenerateTestUser()
		assert.NotNil(t, user)
		assert.NotEmpty(t, user.ID)
		assert.NotEmpty(t, user.Username)
		assert.NotEmpty(t, user.PasswordHash)
		assert.False(t, user.CreatedAt.IsZero())
	})

	t.Run("GenerateTestHost", func(t *testing.T) {
		host := GenerateTestHost()
		assert.NotNil(t, host)
		assert.NotEmpty(t, host.ID)
		assert.NotEmpty(t, host.Name)
		assert.NotEmpty(t, host.OwnerID)
		assert.NotEmpty(t, host.Hostname)
		assert.Equal(t, 22, host.Port)            // Default SSH port
		assert.Equal(t, "root", host.SSHUsername) // Default SSH username
		assert.False(t, host.CreatedAt.IsZero())
		assert.False(t, host.UpdatedAt.IsZero())
	})

	t.Run("GenerateValidPEMKey", func(t *testing.T) {
		pemKey := GenerateValidPEMKey()
		assert.NotEmpty(t, pemKey)
		assert.Contains(t, pemKey, "BEGIN RSA PRIVATE KEY")
		assert.Contains(t, pemKey, "END RSA PRIVATE KEY")
	})

	t.Run("GenerateInvalidPEMKey", func(t *testing.T) {
		pemKey := GenerateInvalidPEMKey()
		assert.Equal(t, "invalid-key-data", pemKey)
	})

	t.Run("GenerateValidOpenSSHKey", func(t *testing.T) {
		pemKey := GenerateValidOpenSSHKey()
		assert.NotEmpty(t, pemKey)
		assert.Contains(t, pemKey, "BEGIN OPENSSH PRIVATE KEY")
		assert.Contains(t, pemKey, "END OPENSSH PRIVATE KEY")
	})

	t.Run("GenerateValidECKey", func(t *testing.T) {
		pemKey := GenerateValidECKey()
		assert.NotEmpty(t, pemKey)
		assert.Contains(t, pemKey, "BEGIN EC PRIVATE KEY")
		assert.Contains(t, pemKey, "END EC PRIVATE KEY")
	})

	t.Run("CreateTestJWTKeys", func(t *testing.T) {
		privateKey, publicKey, err := CreateTestJWTKeys()
		assert.NoError(t, err)
		assert.NotEmpty(t, privateKey)
		assert.NotEmpty(t, publicKey)
		assert.Contains(t, privateKey, "BEGIN RSA PRIVATE KEY")
		assert.Contains(t, publicKey, "BEGIN PUBLIC KEY")
	})
}

// TestTestConfiguration tests the test configuration functionality
func TestTestConfiguration(t *testing.T) {
	t.Run("DefaultTestConfig", func(t *testing.T) {
		config := DefaultTestConfig()
		assert.NotNil(t, config)
		assert.Equal(t, 30, int(config.TestTimeout.Seconds()))
		assert.Equal(t, 1, config.RetryAttempts)
		assert.False(t, config.VerboseOutput)
		assert.True(t, config.MockResponses)
		assert.True(t, config.CleanupOnExit)
	})

	t.Run("Global config initialization", func(t *testing.T) {
		// Reset global config
		globalConfig = nil

		config := InitTestConfig()
		assert.NotNil(t, config)
		assert.Equal(t, DefaultTestConfig().TestTimeout, config.TestTimeout)

		// Get should return the same instance
		config2 := GetTestConfig()
		assert.Equal(t, config, config2)
	})

	t.Run("SetTestConfig", func(t *testing.T) {
		// Reset global config
		globalConfig = nil

		customConfig := &TestConfig{
			TestTimeout:   60 * time.Second,
			RetryAttempts: 3,
			VerboseOutput: true,
		}
		SetTestConfig(customConfig)

		config := GetTestConfig()
		assert.Equal(t, 60, int(config.TestTimeout.Seconds()))
		assert.Equal(t, 3, config.RetryAttempts)
		assert.True(t, config.VerboseOutput)
	})

	t.Run("Environment variable helpers", func(t *testing.T) {
		// Test GetEnv
		value := GetEnv("NON_EXISTENT_VAR", "default-value")
		assert.Equal(t, "default-value", value)

		// Test GetEnvAsBool
		boolValue := GetEnvAsBool("NON_EXISTENT_BOOL", false)
		assert.False(t, boolValue)

		// Test GetEnvAsInt
		intValue := GetEnvAsInt("NON_EXISTENT_INT", 42)
		assert.Equal(t, 42, intValue)

		// Test GetEnvAsDuration
		duration := GetEnvAsDuration("NON_EXISTENT_DURATION", 10*time.Second)
		assert.Equal(t, 10*time.Second, duration)
	})

	t.Run("CleanupTestResources", func(t *testing.T) {
		// This should not panic
		CleanupTestResources()
	})

	t.Run("AssertErrorContains", func(t *testing.T) {
		t.Run("error contains substring", func(t *testing.T) {
			err := errors.New("this is an error message")
			// This should not fail
			AssertErrorContains(t, err, "error message")
		})

		t.Run("error does not contain substring", func(t *testing.T) {
			// Create a failing test to verify it fails when it should
			testingT := &testing.T{}
			// We'll test this functionality more indirectly
			// For now, just verify the function doesn't panic
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("AssertErrorContains should not panic: %v", r)
				}
			}()
			AssertErrorContains(testingT, errors.New("test error"), "different")
		})

		t.Run("nil error test", func(t *testing.T) {
			// Test that nil error causes the expected behavior
			testingT := &testing.T{}
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("AssertErrorContains should not panic: %v", r)
				}
			}()
			AssertErrorContains(testingT, nil, "any message")
		})
	})
}

// TestMockServiceHelpers tests the service helper functions
func TestMockServiceHelpers(t *testing.T) {
	t.Run("NewTestAuthServiceWithMocks", func(t *testing.T) {
		service, err := NewTestAuthServiceWithMocks()
		require.NoError(t, err)
		require.NotNil(t, service)

		// Verify service has mock user repository
		assert.NotNil(t, service.GetUserRepository())
	})
}

// TestMockConnectionManager_WithConnection pins the mock's
// WithConnection behavior: fn runs once with a connection context and its
// error crosses untouched; an injected acquisition error short-circuits
// fn and crosses as-is (the 502 mapping itself is the production
// manager's job, pinned once in the connections package tests).
func TestMockConnectionManager_WithConnection(t *testing.T) {
	t.Run("runs fn with a connection context and propagates its error", func(t *testing.T) {
		mock := NewMockConnectionManager()

		var sawCtx context.Context
		fnErr := errors.New("engine exploded")
		err := mock.WithConnection(context.Background(), "user-1", "host-1", "volumes.List", func(connCtx context.Context) error {
			sawCtx = connCtx
			return fnErr
		})

		assert.ErrorIs(t, err, fnErr, "fn's error must cross untouched")
		assert.NotNil(t, sawCtx, "fn must receive a connection context")
	})

	t.Run("injected acquisition error short-circuits fn", func(t *testing.T) {
		acquireErr := errors.New("dial failed")
		mock := NewMockConnectionManagerWithAcquireError(acquireErr)

		fnRan := false
		err := mock.WithConnection(context.Background(), "user-1", "host-1", "volumes.List", func(context.Context) error {
			fnRan = true
			return nil
		})

		assert.Equal(t, acquireErr, err, "the injected acquisition error crosses as-is")
		assert.False(t, fnRan, "fn must not run without a connection")
	})
}

// TestMockSSHExec tests the MockSSHExec functionality across its three
// construction modes (default, fixed-result, fixed-error, custom-func) plus
// the call-recording helpers. Mirrors the MockEncryptionService test shape.
func TestMockSSHExec(t *testing.T) {
	ctx := context.Background()
	errBoom := errors.New("boom")

	t.Run("default returns zero-exit empty result with no error", func(t *testing.T) {
		m := NewMockSSHExec()

		res, err := m.Exec(ctx, "m", "u", "uptime", sshexec.ExecOptions{})

		assert.NoError(t, err)
		assert.Equal(t, sshexec.ExecResult{Stdout: "", Stderr: "", ExitCode: 0}, res)
	})

	t.Run("WithResult returns the configured result", func(t *testing.T) {
		want := sshexec.ExecResult{Stdout: "active\n", ExitCode: 0}
		m := NewMockSSHExecWithResult(want)

		res, err := m.Exec(ctx, "m", "u", "systemctl", sshexec.ExecOptions{Args: []string{"is-active", "web"}})

		assert.NoError(t, err)
		assert.Equal(t, want, res)
	})

	t.Run("WithError returns the configured error", func(t *testing.T) {
		wantErr := errors.New("dial tcp: connection refused")
		m := NewMockSSHExecWithError(wantErr)

		res, err := m.Exec(ctx, "m", "u", "uptime", sshexec.ExecOptions{})

		assert.ErrorIs(t, err, wantErr)
		assert.Equal(t, sshexec.ExecResult{}, res)
	})

	t.Run("WithFunc delegates each call to the function", func(t *testing.T) {
		calls := 0
		m := NewMockSSHExecWithFunc(func(_ context.Context, _, _, cmd string, _ sshexec.ExecOptions) (sshexec.ExecResult, error) {
			calls++
			if cmd == "fail" {
				return sshexec.ExecResult{ExitCode: 1, Stderr: "nope"}, nil
			}
			return sshexec.ExecResult{ExitCode: 0, Stdout: cmd}, nil
		})

		r1, err := m.Exec(ctx, "m", "u", "ok", sshexec.ExecOptions{})
		assert.NoError(t, err)
		assert.Equal(t, 0, r1.ExitCode)
		assert.Equal(t, "ok", r1.Stdout)

		r2, err := m.Exec(ctx, "m", "u", "fail", sshexec.ExecOptions{})
		assert.NoError(t, err)
		assert.Equal(t, 1, r2.ExitCode)
		assert.Equal(t, "nope", r2.Stderr)

		assert.Equal(t, 2, calls, "the custom func must be invoked for every Exec call")
	})

	t.Run("records every Exec invocation for assertion", func(t *testing.T) {
		m := NewMockSSHExec()
		opts := sshexec.ExecOptions{Args: []string{"start", "web.service"}}

		_, _ = m.Exec(ctx, "host-1", "user-1", "systemctl", opts)
		_, _ = m.Exec(ctx, "host-2", "user-2", "echo", sshexec.ExecOptions{})

		assert.Equal(t, 2, m.CallCount())

		calls := m.Calls()
		assert.Len(t, calls, 2)
		assert.Equal(t, "host-1", calls[0].HostID)
		assert.Equal(t, "user-1", calls[0].UserID)
		assert.Equal(t, "systemctl", calls[0].Command)
		assert.Equal(t, []string{"start", "web.service"}, calls[0].Opts.Args)

		assert.Equal(t, "host-2", calls[1].HostID)
		assert.Equal(t, "echo", calls[1].Command)
		assert.Empty(t, calls[1].Opts.Args)
	})

	t.Run("Calls returns a defensive copy", func(t *testing.T) {
		m := NewMockSSHExec()
		_, _ = m.Exec(ctx, "m", "u", "echo", sshexec.ExecOptions{})

		calls := m.Calls()
		calls[0].Command = "mutated"

		// The mock's internal record must not be affected by callers
		// mutating the returned slice.
		fresh := m.Calls()
		assert.Equal(t, "echo", fresh[0].Command)
	})

	t.Run("ExecStream falls back to the configured result and emits its lines", func(t *testing.T) {
		m := NewMockSSHExecWithResult(sshexec.ExecResult{
			Stdout:   "line 1\nline 2\n",
			Stderr:   "warn\n",
			ExitCode: 3,
		})

		var lines []sshexec.OutputLine
		res, err := m.ExecStream(ctx, "host-1", "user-1", "systemctl", sshexec.ExecOptions{}, func(line sshexec.OutputLine) {
			lines = append(lines, line)
		})

		assert.NoError(t, err)
		assert.Equal(t, 3, res.ExitCode)
		assert.Equal(t, []sshexec.OutputLine{
			{Stream: sshexec.StdoutStream, Text: "line 1"},
			{Stream: sshexec.StdoutStream, Text: "line 2"},
			{Stream: sshexec.StderrStream, Text: "warn"},
		}, lines)
	})

	t.Run("ExecStream records calls with emitted lines", func(t *testing.T) {
		m := NewMockSSHExec()
		opts := sshexec.ExecOptions{Args: []string{"auto-update", "web"}}

		_, _ = m.ExecStream(ctx, "host-1", "user-1", "podman", opts, func(sshexec.OutputLine) {})

		calls := m.StreamCalls()
		assert.Len(t, calls, 1)
		assert.Equal(t, "host-1", calls[0].HostID)
		assert.Equal(t, "user-1", calls[0].UserID)
		assert.Equal(t, "podman", calls[0].Command)
		assert.Equal(t, []string{"auto-update", "web"}, calls[0].Opts.Args)
		assert.Empty(t, calls[0].Lines, "no lines for a result with empty output")

		// StreamCalls returns a defensive copy.
		calls[0].Command = "mutated"
		assert.Equal(t, "podman", m.StreamCalls()[0].Command)
	})

	t.Run("ExecStream with a configured stream result and stream error", func(t *testing.T) {
		result := sshexec.ExecResult{Stdout: "report\n", ExitCode: 0}
		m := NewMockSSHExecWithStreamResult(result)

		var lines []string
		res, err := m.ExecStream(ctx, "m", "u", "podman", sshexec.ExecOptions{}, func(line sshexec.OutputLine) {
			lines = append(lines, line.Text)
		})
		assert.NoError(t, err)
		assert.Equal(t, result, res)
		assert.Equal(t, []string{"report"}, lines)

		wantErr := errors.New("stream dial failed")
		me := NewMockSSHExecWithStreamError(wantErr)
		res, err = me.ExecStream(ctx, "m", "u", "podman", sshexec.ExecOptions{}, func(sshexec.OutputLine) {})
		assert.ErrorIs(t, err, wantErr)
		assert.Equal(t, sshexec.ExecResult{}, res)
	})

	t.Run("ExecStream delegates to a configured stream func", func(t *testing.T) {
		m := &MockSSHExec{streamFunc: func(_ context.Context, _, _, _ string, _ sshexec.ExecOptions, onLine func(sshexec.OutputLine)) (sshexec.ExecResult, error) {
			onLine(sshexec.OutputLine{Stream: sshexec.StdoutStream, Text: "from func"})
			return sshexec.ExecResult{ExitCode: 2}, errBoom
		}}

		var lines []string
		_, err := m.ExecStream(ctx, "m", "u", "podman", sshexec.ExecOptions{}, func(line sshexec.OutputLine) {
			lines = append(lines, line.Text)
		})

		assert.ErrorIs(t, err, errBoom)
		assert.Equal(t, []string{"from func"}, lines)
		assert.Len(t, m.StreamCalls(), 1)
		assert.Equal(t, []sshexec.OutputLine{{Stream: sshexec.StdoutStream, Text: "from func"}}, m.StreamCalls()[0].Lines)
	})

	t.Run("satisfies sshexec.SSHExecInterface", func(t *testing.T) {
		// Compile-time check: any code accepting sshexec.SSHExecInterface
		// can be handed a *MockSSHExec in tests. This is the whole point
		// of the mock — pinning it as a test prevents accidental drift.
		var _ sshexec.SSHExecInterface = (*MockSSHExec)(nil)
	})
}
