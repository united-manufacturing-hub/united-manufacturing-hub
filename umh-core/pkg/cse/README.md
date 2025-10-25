# pkg/cse - Client-Side Encryption

## Overview

This package provides the foundational infrastructure for client-side encryption (CSE) in the United Manufacturing Hub. It implements a three-layer architecture that separates concerns between protocol, orchestration, and persistence.

## Architecture

The CSE package is organized into three subpackages representing different architectural layers:

### Layer 3: Protocol (`pkg/cse/protocol/`)

Network and security interfaces for CSE sync protocol.

**Transport Interface**:
- Abstracts network layer (HTTP pull/push, WebSocket, etc.)
- Defines RawMessage with UNVERIFIED sender
- 5 typed errors: Network/Overflow/Config/Timeout/Auth
- 18 contract tests, MockTransport for testing

**Crypto Interface**:
- E2E encryption with sender authentication
- Combined encryption+auth prevents unsafe usage
- Idempotent encryption via random nonce
- Based on patent EP4512040A2 (blind relay design)
- 17 contract tests, MockCrypto with XOR cipher

**Authorizer Interface**:
- ABAC (Attribute-Based Access Control)
- Three permission levels:
  * CanSubscribe (collection-level)
  * CanReadField (field-level granular)
  * CanWriteTransaction (write operation control)
- Fail-closed security (deny by default)
- 29 contract tests, MockAuthorizer with policy configuration

**Total: 64 protocol tests passing**

### Layer 2: Storage (`pkg/cse/storage/`)

The persistence layer managing encrypted data storage:
- **Storage interfaces**: Generic key-value storage for encrypted data
- **Cache management**: Local caching for performance
- **Versioning**: Track data versions for synchronization
- **Metadata management**: Encryption metadata (key IDs, algorithms)

This layer is storage-agnostic, supporting various backends (disk, memory, databases).

### Orchestration: Sync (`pkg/cse/sync/`)

The orchestration layer coordinating synchronization:
- **Sync state management**: 2-tier tracking (Frontend ↔ Edge)
- **Retry logic**: Handle transient failures
- **Conflict resolution**: Manage concurrent modifications
- **Progress tracking**: Monitor and report sync operations

This layer composes the protocol and storage layers to implement end-to-end sync workflows.

**Note**: Relay is a transparent E2E encrypted proxy, NOT a sync tier. See `storage/ARCHITECTURE.md` for details.

**Total: 29 sync tests passing**

## Design Principles

1. **Separation of Concerns**: Each layer has clear, non-overlapping responsibilities
2. **Interface-Driven**: All layers define interfaces first, implementations second
3. **Composability**: Layers can be mixed and matched with different implementations
4. **Testability**: Each layer can be tested independently with mocks
5. **Zero-Trust**: Assume all external communication is hostile

## Dependency Flow

```
pkg/cse/sync/
    ↓ uses
pkg/cse/storage/  ←→  pkg/cse/protocol/
    ↓ uses              ↓ uses
External Storage    External Network/Crypto
```

- `sync` depends on both `storage` and `protocol`
- `storage` and `protocol` are independent (same layer, different domains)
- Lower layers have no dependencies on upper layers

## Quick Start

### Transport Interface
```go
import "github.com/united-manufacturing-hub/umh-core/pkg/cse/protocol"

transport := protocol.NewMockTransport()
rawMsg, err := transport.ReceiveMessage(ctx, subscriptionID)
if err != nil {
    // Handle typed errors: NetworkError, OverflowError, etc.
}
```

### Crypto Interface
```go
crypto := protocol.NewMockCrypto()
encrypted, err := crypto.EncryptAndSign(ctx, plaintext, recipientID)
plaintext, senderID, err := crypto.DecryptAndVerify(ctx, encrypted)
```

### Authorizer Interface
```go
authorizer := protocol.NewMockAuthorizer()
if !authorizer.CanReadField(userID, collection, field) {
    return errors.New("access denied")
}
```

## Testing

All components use Ginkgo v2 + Gomega:

```bash
ginkgo -r ./pkg/cse/
```

Current test coverage:
- Protocol layer: 64 specs (Transport + Crypto + Authorizer)
- Sync layer: 29 specs (state management)
- Storage layer: 143 specs (Triangular + Registry + Pool + TxCache)
- **Total: 236 specs**

## Development Roadmap

### Phase 1: Interface Definition (Current)
- Define core interfaces for each layer
- Create placeholder implementations
- Establish layer boundaries and contracts

### Phase 2: Reference Implementation
- Implement basic storage backends (memory, disk)
- Implement basic encryption (AES-GCM)
- Implement basic sync engine (immediate mode)

### Phase 3: Production Hardening
- Add comprehensive error handling
- Implement retry logic and backoff
- Add monitoring and observability
- Performance optimization

### Phase 4: Advanced Features
- Key rotation support
- Multi-tenant authorization
- Distributed caching
- Advanced conflict resolution

## Related Documentation

- [Protocol Layer](./protocol/README.md) - Transport, crypto, and authorization interfaces
- [Storage Layer](./storage/README.md) - Persistence patterns and interfaces
- [Sync Layer](./sync/README.md) - Orchestration and synchronization logic

## References

- **Patent EP4512040A2**: E2E encryption with blind relay architecture
- **Linear sync engine**: wzhudev/reverse-linear-sync-engine
- **ENG-3622**: CSE RFC (Linear-quality UX for Kubernetes)
- **ENG-3647**: FSM v2 RFC

## Contributing

When adding new functionality:
1. Identify the appropriate layer (protocol, storage, or sync)
2. Define interfaces before implementations
3. Keep layers independent (no upward dependencies)
4. Add tests at the layer boundary
5. Update this README with architectural changes
