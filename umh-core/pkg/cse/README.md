# pkg/cse - Client-Side Encryption

## Overview

This package provides the foundational infrastructure for client-side encryption (CSE) in the United Manufacturing Hub. It implements a three-layer architecture that separates concerns between protocol, orchestration, and persistence.

## Architecture

The CSE package is organized into three subpackages representing different architectural layers:

### Layer 3: Protocol (`pkg/cse/protocol/`)

The lowest layer providing network and security interfaces:
- **Transport interfaces**: Abstract network communication mechanisms
- **Cryptographic interfaces**: Encryption/decryption operations
- **Authorization interfaces**: Access control for encrypted data

This layer is protocol-agnostic and crypto-agnostic, allowing different implementations to be swapped without affecting higher layers.

### Layer 2: Storage (`pkg/cse/storage/`)

The persistence layer managing encrypted data storage:
- **Storage interfaces**: Generic key-value storage for encrypted data
- **Cache management**: Local caching for performance
- **Versioning**: Track data versions for synchronization
- **Metadata management**: Encryption metadata (key IDs, algorithms)

This layer is storage-agnostic, supporting various backends (disk, memory, databases).

### Orchestration: Sync (`pkg/cse/sync/`)

The orchestration layer coordinating synchronization:
- **Sync state management**: Track synchronization status
- **Retry logic**: Handle transient failures
- **Conflict resolution**: Manage concurrent modifications
- **Progress tracking**: Monitor and report sync operations

This layer composes the protocol and storage layers to implement end-to-end sync workflows.

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

## Usage Patterns

### Basic Encryption Flow

1. **Protocol layer**: Encrypt data using `Encryptor` interface
2. **Storage layer**: Persist encrypted data using `Store` interface
3. **Sync layer**: Orchestrate sync to remote using `SyncEngine`

### Reading Encrypted Data

1. **Storage layer**: Retrieve encrypted data from `Cache` or `Store`
2. **Protocol layer**: Decrypt data using `Encryptor` interface
3. **Sync layer**: Handle cache misses by triggering sync from remote

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

## Contributing

When adding new functionality:
1. Identify the appropriate layer (protocol, storage, or sync)
2. Define interfaces before implementations
3. Keep layers independent (no upward dependencies)
4. Add tests at the layer boundary
5. Update this README with architectural changes
