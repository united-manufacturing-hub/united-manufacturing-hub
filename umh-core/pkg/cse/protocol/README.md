# pkg/cse/protocol

## Layer 3: Network and Security Interfaces

This package provides the transport, cryptographic, and authorization interfaces for the Client-Side Encryption (CSE) system.

## Transport Interface

**Purpose**: Abstract network layer for CSE sync protocol

**Key Design**:
- `RawMessage` struct with UNVERIFIED sender field (must verify via crypto)
- 5 typed errors for precise error handling:
  * `NetworkError` - Connection/transmission failures
  * `OverflowError` - Message queue capacity exceeded
  * `ConfigError` - Invalid configuration
  * `TimeoutError` - Operation timeout
  * `AuthError` - Authentication failures

**Testing**: 18 contract tests verify interface behavior
- `MockTransport` provides in-memory testing implementation
- All errors tested for correct type and context

**Example**:
```go
transport := protocol.NewMockTransport()
rawMsg, err := transport.ReceiveMessage(ctx, subscriptionID)
if protocol.IsNetworkError(err) {
    // Handle network failure
}
```

## Crypto Interface

**Purpose**: E2E encryption with sender authentication

**Key Design**:
- Combined `EncryptAndSign()` and `DecryptAndVerify()` prevent unsafe usage patterns
- Idempotent encryption via random nonce (same plaintext → different ciphertext)
- Based on patent EP4512040A2 (blind relay architecture)
- Relay cannot read/modify messages (E2E encrypted)

**Security Model**:
- Sender authentication built into encryption (not separate step)
- Fail-closed: Decryption fails if sender cannot be verified
- No separate "sign then encrypt" or "encrypt then sign" (combined operation)

**Testing**: 17 contract tests verify cryptographic properties
- `MockCrypto` uses XOR cipher for deterministic testing
- Tests verify idempotency, sender verification, error handling

**Example**:
```go
crypto := protocol.NewMockCrypto()
encrypted, err := crypto.EncryptAndSign(ctx, plaintext, recipientID)
plaintext, senderID, err := crypto.DecryptAndVerify(ctx, encrypted)
// senderID is VERIFIED (crypto vouches for it)
```

## Authorizer Interface

**Purpose**: Attribute-Based Access Control (ABAC)

**Three Permission Levels**:
1. `CanSubscribe(userID, collection)` - Collection-level access
2. `CanReadField(userID, collection, field)` - Field-level granular control
3. `CanWriteTransaction(userID, collection)` - Write operation control

**Security Model**:
- Fail-closed: Deny by default
- Attributes checked before granting access
- Supports both collection-wide and field-granular permissions

**Testing**: 29 contract tests verify authorization logic
- `MockAuthorizer` allows policy configuration for testing
- Tests verify deny-by-default, policy enforcement, edge cases

**Example**:
```go
authorizer := protocol.NewMockAuthorizer()
authorizer.SetPolicy(userID, "workers", protocol.PolicyReadWrite)

if !authorizer.CanReadField(userID, "workers", "status") {
    return errors.New("access denied")
}
```

## Contract Test Pattern

All three interfaces use contract tests to verify behavior:

1. **Interface contract**: Each interface defines expected behavior
2. **Test suite**: Generic tests verify contract compliance
3. **Mock implementations**: Provide testable implementations
4. **Type safety**: Compile-time verification of interface compliance

**Benefits**:
- Future implementations automatically validated
- Consistent behavior across implementations
- Clear interface contracts
- Easy to test higher layers

## Mock Implementations

### MockTransport
- In-memory message queue
- Configurable capacity and timeouts
- Error injection for testing

### MockCrypto
- XOR cipher (deterministic, reversible)
- Nonce tracking for idempotency verification
- Sender ID validation

### MockAuthorizer
- Policy-based access control
- Three policy levels: None/ReadOnly/ReadWrite
- Configurable per-user, per-collection

## Testing

Run all protocol tests:
```bash
ginkgo -r ./pkg/cse/protocol
```

Current: 64 specs (18 Transport + 17 Crypto + 29 Authorizer)

## Dependencies

- **External**: None (pure Go, no external crypto libraries)
- **Internal**: None (lowest layer in CSE stack)

## References

- **Patent EP4512040A2**: E2E encryption with blind relay architecture
- **ABAC**: Attribute-Based Access Control model
- **CSE RFC (ENG-3622)**: Complete design documentation
