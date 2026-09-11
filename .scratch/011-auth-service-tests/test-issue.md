---
title: Implement Comprehensive Auth Service Unit Tests
date: 2026-07-02
status: open
labels:
  - needs-triage
  - testing
  - auth
author: vibe-agent
---

## What to build

Implement comprehensive unit tests for the auth service covering JWT token generation, validation, error handling, and edge cases.

## Acceptance criteria

- [ ] Test JWT token generation with valid user IDs
- [ ] Test JWT token validation with valid tokens
- [ ] Test expired token handling and error messages
- [ ] Test invalid token formats and signatures
- [ ] Test token issuer validation
- [ ] Test malformed token handling
- [ ] Achieve >80% code coverage for auth service
- [ ] All tests pass in CI environment

## Blocked by

- Issue: `.scratch/010-test-infrastructure/test-issue.md` (test infrastructure)

## Technical details

### Test Cases to Implement:

1. **Token Generation Tests**:
   - `TestGenerateToken_ValidUserID` - Valid user ID generates token
   - `TestGenerateToken_EmptyUserID` - Empty user ID handling
   - `TestGenerateToken_SpecialCharacters` - User IDs with special characters
   - `TestGenerateToken_LongUserID` - Very long user ID handling

2. **Token Validation Tests**:
   - `TestValidateToken_ValidToken` - Valid token validation
   - `TestValidateToken_ExpiredToken` - Expired token detection
   - `TestValidateToken_InvalidSignature` - Tampered token detection
   - `TestValidateToken_WrongIssuer` - Issuer mismatch detection
   - `TestValidateToken_Malformed` - Malformed token handling

3. **Error Handling Tests**:
   - `TestValidateToken_ExpiredErrorMessage` - Correct expired error message
   - `TestValidateToken_InvalidErrorMessage` - Correct invalid error message
   - `TestValidateToken_ParseError` - Token parsing errors
   - `TestValidateToken_NilToken` - Nil token handling

4. **Edge Case Tests**:
   - `TestValidateToken_JustExpired` - Token that expired 1 second ago
   - `TestValidateToken_AboutToExpire` - Token expiring in 1 second
   - `TestValidateToken_NoExpiration` - Token with no expiration
   - `TestValidateToken_VeryLongExpiration` - Token with very long expiration

### Test Structure:

```go
func TestAuthService(t *testing.T) {
    // Setup - use test infrastructure
    config := testutils.NewTestJWTConfig()
    mockRepo := testutils.NewMockUserRepository()
    service, err := auth.NewAuthService(config, mockRepo)
    require.NoError(t, err)
    
    t.Run("GenerateValidToken", func(t *testing.T) {
        token, err := service.GenerateToken("test-user-123")
        assert.NoError(t, err)
        assert.NotEmpty(t, token)
        assert.True(t, strings.HasPrefix(token, "ey")) // JWT prefix
    })
    
    t.Run("ValidateExpiredToken", func(t *testing.T) {
        expiredToken := testutils.CreateExpiredTestToken(service)
        _, err := service.ValidateToken(expiredToken)
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "token expired")
    })
    
    // More test cases...
}
```

## Implementation approach

1. **Use test infrastructure** from slice 1 for mocks and helpers
2. **Create test JWT keys** for signing and validation
3. **Test happy paths** first, then error cases
4. **Use table-driven tests** for similar test cases
5. **Add test coverage** measurement
6. **Ensure tests are fast** (<100ms per test)
7. **Add proper test cleanup**

## Success criteria

- All auth service functionality is covered by tests
- Edge cases and error conditions are properly tested
- Tests run quickly and reliably
- Code coverage meets quality standards
- Tests can run in CI/CD pipeline