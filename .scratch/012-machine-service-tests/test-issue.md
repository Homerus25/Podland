---
title: Implement Machine Service Unit Tests with Validation
date: 2026-07-02
status: completed
labels:
  - needs-triage
  - testing
  - machines
author: vibe-agent
---

## What to build

Implement unit tests for machine service focusing on machine creation, validation, PEM parsing, and business logic.

## Acceptance criteria

- [x] Test machine creation with valid data
- [x] Test machine validation logic
- [x] Test PEM key parsing (valid and invalid formats)
- [x] Test default value handling (ports, socket paths)
- [x] Test error cases and edge conditions
- [ ] Test machine update and deletion logic (Note: Update and Delete methods not yet implemented in service)
- [x] Achieve >85% code coverage for machine service methods
- [x] All tests pass with mock dependencies

## Blocked by

- Issue: `.scratch/010-test-infrastructure/test-issue.md` (test infrastructure)

## Technical details

### Test Cases to Implement:

1. **Machine Creation Tests**:
   - `TestCreateMachine_ValidData` - Valid machine creation
   - `TestCreateMachine_MissingRequiredFields` - Required field validation
   - `TestCreateMachine_InvalidHostname` - Hostname format validation
   - `TestCreateMachine_DuplicateName` - Duplicate name handling
   - `TestCreateMachine_InvalidSSHKey` - Invalid SSH key formats

2. **PEM Parsing Tests**:
   - `TestIsValidPEM_ValidRSAKey` - Valid RSA private key
   - `TestIsValidPEM_ValidOpenSSHKey` - Valid OpenSSH private key
   - `TestIsValidPEM_ValidECKey` - Valid EC private key
   - `TestIsValidPEM_ValidEd25519Key` - Valid Ed25519 private key
   - `TestIsValidPEM_InvalidKeyTooShort` - Too short to be valid
   - `TestIsValidPEM_InvalidKeyMissingBegin` - Missing BEGIN marker
   - `TestIsValidPEM_InvalidKeyMissingEnd` - Missing END marker
   - `TestIsValidPEM_InvalidKeyNotPrivate` - Public key instead of private
   - `TestIsValidPEM_EmptyKey` - Empty key string
   - `TestIsValidPEM_WhitespaceOnly` - Whitespace-only key
   - `TestIsValidPEM_ValidKeyWithWhitespace` - Valid key with surrounding whitespace

3. **Default Value Tests**:
   - `TestCreateMachine_DefaultPort` - Default SSH port (22)
   - `TestCreateMachine_CustomPort` - Custom SSH port
   - `TestCreateMachine_DefaultSocketPath` - Default Podman socket path
   - `TestCreateMachine_CustomSocketPath` - Custom Podman socket path

4. **Business Logic Tests**:
   - `TestCreateMachine_EncryptionSuccess` - SSH key encryption works
   - `TestCreateMachine_EncryptionFailure` - Encryption failure handling
   - `TestGetMachine_Existing` - Get existing machine
   - `TestGetMachine_NonExisting` - Get non-existing machine
   - `TestUpdateMachine_Valid` - Update machine successfully
   - `TestDeleteMachine_Valid` - Delete machine successfully

5. **Error Handling Tests**:
   - `TestCreateMachine_DatabaseError` - Database error handling
   - `TestCreateMachine_EncryptionError` - Encryption error handling
   - `TestCreateMachine_InvalidMachineID` - Invalid machine ID handling

### Test Structure Example:

```go
func TestMachineService(t *testing.T) {
    // Setup
    mockRepo := testutils.NewMockMachineRepository()
    encryption := testutils.NewMockEncryptionService()
    service := machines.NewMachineService(mockRepo, encryption)
    
    t.Run("CreateValidMachine", func(t *testing.T) {
        req := &machines.CreateMachineRequest{
            Name:          "test-machine",
            Hostname:      "example.com",
            SSHUsername:   "root",
            SSHPrivateKey: testutils.GenerateValidPEMKey(),
            Port:          22,
        }
        
        machine, err := service.CreateMachine(context.Background(), req)
        assert.NoError(t, err)
        assert.NotNil(t, machine)
        assert.Equal(t, "test-machine", machine.Name)
        assert.Equal(t, "example.com", machine.Hostname)
    })
    
    t.Run("PEMValidation", func(t *testing.T) {
        testCases := []struct {
            name     string
            key      string
            expected bool
        }{
            {"valid RSA", testutils.GenerateValidPEMKey(), true},
            {"invalid short", "short", false},
            {"empty", "", false},
        }
        
        for _, tc := range testCases {
            t.Run(tc.name, func(t *testing.T) {
                result := machines.IsValidPEM(tc.key)
                assert.Equal(t, tc.expected, result)
            })
        }
    })
    
    // More test cases...
}
```

## Implementation approach

1. **Use existing test patterns** from machines service test file
2. **Extend PEM validation tests** with comprehensive test cases
3. **Add business logic tests** for create, update, delete operations
4. **Use table-driven tests** for PEM validation (already started)
5. **Mock external dependencies** using test infrastructure
6. **Test both happy paths and error conditions**
7. **Ensure tests are deterministic** and fast

## Success criteria

- All machine service functionality is thoroughly tested
- PEM validation covers all key formats and edge cases
- Default value handling is properly tested
- Error conditions are well covered
- Tests provide good documentation of expected behavior
- Code coverage meets quality standards