# Gatekeeper Package Implementation Plan

## 1. Architecture Overview

Gatekeeper serves as a security and protocol middleware layer that sits between the Transport layer (Puller/Pusher) and the Business Logic layer (Router/ActionHandler/SubscriberRegistry).

```
+------------------+     +------------------+     +------------------+
|    Transport     |     |    Gatekeeper      |     |  Business Logic  |
|                  |     |                  |     |                  |
| +------------+   |     | +------------+   |     | +------------+   |
| |   Puller   |---+---->| | Inbound    |---+---->| |   Router   |   |
| +------------+   |     | | Processor  |   |     | +------------+   |
|                  |     | +------------+   |     |                  |
| +------------+   |     | +------------+   |     | +------------+   |
| |   Pusher   |<--+-----| | Outbound   |<--+-----| |  Actions   |   |
| +------------+   |     | | Processor  |   |     | +------------+   |
+------------------+     +------------------+     +------------------+
```

### Responsibilities

Gatekeeper handles:
- **Protocol detection and versioning** (v0 legacy, cse_v1)
- **Message encryption/decryption**
- **Message compression/decompression**
- **Permission validation** (Linux-like permissions model)
- **Certificate handling and caching**

## 2. Package Structure

```
pkg/gatekeeper/
├── gatekeeper.go                    # Main orchestrator (Gatekeeper struct)
├── gatekeeper_test.go               # Unit tests for orchestrator
├── options.go                       # Configuration options
│
├── protocol/
│   ├── interface.go                 # ProtocolHandler, Detector interfaces
│   ├── detector.go                  # Protocol version detector implementation
│   ├── detector_test.go
│   ├── cache/
│   │   ├── cache.go                 # Protocol version cache (per-sender)
│   │   └── cache_test.go
│   └── versions/
│       ├── v0.go                    # Legacy protocol handler
│       └── cse_v1.go                # CSE v1 protocol handler
│
├── validator/
│   ├── interface.go                 # Validator interface (moved from pkg/permissions/)
│   ├── validator_noop.go            # NoopValidator for OSS builds
│   ├── validator_impl.go            # CryptoValidator (build tag: cryptolib)
│   ├── wrapper.go                   # Action-specific validation wrapper
│   ├── wrapper_test.go
│   ├── actions.go                   # Action type to permission mapping
│   └── helpers.go                   # GetLocationString, GetLocationFromConfig
│
├── compression/
│   ├── interface.go                 # Compressor interface
│   ├── zstd.go                      # Zstd implementation
│   └── zstd_test.go
│
├── encryption/
│   ├── interface.go                 # Encryptor interface
│   ├── noop.go                      # No-op for v0 protocol
│   ├── cse_v1.go                    # Client-Side Encryption v1 impl
│   └── encryption_test.go
│
└── certificatehandler/
    ├── interface.go                 # Handler interface
    ├── handler.go                   # Handler implementation
    ├── handler_test.go
    └── cache/
        ├── cache.go                 # Certificate cache
        └── cache_test.go
```

**Note**: The current `pkg/permissions/` directory will be moved into `pkg/gatekeeper/validator/`. This consolidates all validation logic under the gatekeeper package.

## 3. Interface Definitions

### 3.1 Main Gatekeeper Interface

```go
// pkg/gatekeeper/gatekeeper.go

package gatekeeper

// MessageContentWithSender represents a decrypted, validated message
// ready for business logic processing.
type MessageContentWithSender struct {
    Content     models.UMHMessageContent
    SenderEmail string
    Certificate *x509.Certificate
    Metadata    *MessageMetadata
}

// Gatekeeper is the main orchestrator for message processing.
type Gatekeeper struct {
    // Dependencies
    instanceUUID        uuid.UUID
    validator           permissions.Validator
    protocolCache       *protocol.Cache
    certificateHandler  *certificatehandler.Handler
    compressor          compression.Compressor

    // Channels - Input from Transport
    inboundChannel      chan *models.UMHMessageWithAdditionalInfo
    outboundChannel     chan *models.UMHMessage

    // Channels - Output to Business Logic
    inboundVerifiedChan chan *MessageContentWithSender
    outboundVerifiedChan chan *MessageContentWithSender

    // State
    rootCA              *x509.Certificate
    intermediateCerts   []*x509.Certificate
    authTokenHash       string

    logger              *zap.SugaredLogger
}

// NewGatekeeper creates a new Gatekeeper instance.
func NewGatekeeper(opts ...Option) *Gatekeeper

// Start begins processing messages in both directions.
func (m *Gatekeeper) Start(ctx context.Context) error

// Stop gracefully shuts down the Gatekeeper.
func (m *Gatekeeper) Stop() error

// GetInboundVerifiedChannel returns the channel for business logic to receive messages.
func (m *Gatekeeper) GetInboundVerifiedChannel() <-chan *MessageContentWithSender

// GetOutboundVerifiedChannel returns the channel for business logic to send messages.
func (m *Gatekeeper) GetOutboundVerifiedChannel() chan<- *MessageContentWithSender

// SetRootCA sets the root CA for permission validation.
func (m *Gatekeeper) SetRootCA(rootCA *x509.Certificate, intermediateCerts []*x509.Certificate)
```

### 3.2 Protocol Interface

```go
// pkg/gatekeeper/protocol/protocol.go

package protocol

// ProtocolVersion represents the message protocol version.
type ProtocolVersion string

const (
    ProtocolV0     ProtocolVersion = ""        // Legacy, unencrypted
    ProtocolCSEV1  ProtocolVersion = "cse_v1"  // Client-Side Encryption v1
)

// ProtocolHandler handles encoding/decoding for a specific protocol version.
type ProtocolHandler interface {
    // Version returns the protocol version this handler supports.
    Version() ProtocolVersion

    // DecodeMessage decodes an incoming message.
    DecodeMessage(content string, cert *x509.Certificate) (models.UMHMessageContent, error)

    // EncodeMessage encodes an outgoing message.
    EncodeMessage(content models.UMHMessageContent, recipientCert *x509.Certificate) (string, error)

    // IsEncrypted returns true if this protocol uses encryption.
    IsEncrypted() bool
}

// Detector detects the protocol version from a message.
type Detector interface {
    DetectVersion(content string) ProtocolVersion
}
```

### 3.3 Protocol Cache Interface

```go
// pkg/gatekeeper/protocol/cache/cache.go

package cache

// Cache stores protocol version preferences per sender.
// The frontend selects which protocol to use, and we cache it.
type Cache struct {
    cache *expiremap.ExpireMap[string, protocol.ProtocolVersion]
}

func NewCache(ttl, cleanupInterval time.Duration) *Cache
func (c *Cache) Get(senderEmail string) (protocol.ProtocolVersion, bool)
func (c *Cache) Set(senderEmail string, version protocol.ProtocolVersion)
```

### 3.4 Validator Interface and Wrapper

```go
// pkg/gatekeeper/validator/interface.go

package validator

// Validator interface for certificate-based permission validation.
// This is moved from pkg/permissions/interface.go
type Validator interface {
    ValidateUserPermissions(
        userCert *x509.Certificate,
        action string,
        location string,
        rootCA *x509.Certificate,
        intermediateCerts []*x509.Certificate,
    ) (bool, error)

    DecryptRootCA(encryptedCA string, keyMaterial string, salt string) (string, error)
}

// NewValidator creates a Validator (noop or crypto based on build tags)
func NewValidator() Validator
```

```go
// pkg/gatekeeper/validator/wrapper.go

package validator

// Wrapper wraps the Validator with action-specific logic and message type mapping.
type Wrapper struct {
    inner           Validator
    locationPath    string
}

func NewWrapper(validator Validator, locationPath string) *Wrapper

// ValidateInbound validates an incoming message.
func (w *Wrapper) ValidateInbound(
    userCert *x509.Certificate,
    messageType models.MessageType,
    actionType models.ActionType,
    rootCA *x509.Certificate,
    intermediateCerts []*x509.Certificate,
) (bool, error)

// ValidateOutbound validates an outgoing message.
func (w *Wrapper) ValidateOutbound(
    recipientEmail string,
    messageType models.MessageType,
) (bool, error)
```

### 3.5 Compression Interface

```go
// pkg/gatekeeper/compression/compression.go

package compression

// Compressor handles message compression/decompression.
type Compressor interface {
    Compress(data []byte) ([]byte, error)
    Decompress(data []byte) ([]byte, error)
    IsCompressed(data []byte) bool
}

// ZstdCompressor implements Compressor using zstd.
type ZstdCompressor struct {
    threshold int // Minimum size to compress (default: 1024)
}
```

### 3.6 Encryption Interface

```go
// pkg/gatekeeper/encryption/encryption.go

package encryption

// Encryptor handles message encryption/decryption.
type Encryptor interface {
    Encrypt(data []byte, recipientCert *x509.Certificate) ([]byte, error)
    Decrypt(data []byte, cert *x509.Certificate) ([]byte, error)
}

// NoopEncryptor provides no encryption (for v0 protocol).
type NoopEncryptor struct{}

// CSEV1Encryptor provides client-side encryption v1.
type CSEV1Encryptor struct{}
```

### 3.7 Certificate Handler Interface

```go
// pkg/gatekeeper/certificatehandler/handler.go

package certificatehandler

// Handler manages certificate retrieval and caching.
type Handler struct {
    cache          *Cache
    apiURL         string
    insecureTLS    bool
    validator      permissions.Validator
    authTokenHash  string
    instanceUUID   string
    logger         *zap.SugaredLogger
}

func NewHandler(opts ...HandlerOption) *Handler
func (h *Handler) GetCertificate(ctx context.Context, email string, jwt string) (*x509.Certificate, error)
func (h *Handler) GetRootCA(ctx context.Context, encryptedCA string) (*x509.Certificate, error)

// Cache is the certificate cache.
type Cache struct {
    userCerts *expiremap.ExpireMap[string, *x509.Certificate]
    ttl       time.Duration
}
```

## 4. Data Flow Diagrams

### 4.1 Incoming Message Flow

```
+------------------+
|  Puller (raw)    |
|   UMHMessage     |
+--------+---------+
         |
         v
+------------------+
| Gatekeeper.process |
|  InboundMessage  |
+--------+---------+
         |
    +----+----+
    |         |
    v         v
+-------+  +-------+
|Detect |  | Fetch |
|Protocol| |  Cert |
+---+---+  +---+---+
    |          |
    +----+-----+
         |
         v
+------------------+
|  Protocol Handler|
|  (v0 or cse_v1)  |
+--------+---------+
         |
    +----+----+
    |         |
    v         v
+--------+ +--------+
|Decrypt | |Decomp- |
|(if enc)| |ress    |
+---+----+ +----+---+
    |           |
    +-----+-----+
          |
          v
+------------------+
| Validate         |
| Permissions      |
+--------+---------+
         |
    pass |  fail
    +----+----+
    |         |
    v         v
+--------+ +--------+
| Output | | Drop   |
| Channel| | & Log  |
+--------+ +--------+
         |
         v
+------------------+
| inboundVerified  |
|   Channel        |
| (to Router)      |
+------------------+
```

### 4.2 Outgoing Message Flow

```
+------------------+
| outboundVerified |
|   Channel        |
| (from Business)  |
+--------+---------+
         |
         v
+------------------+
| Gatekeeper.process |
|  OutboundMessage |
+--------+---------+
         |
         v
+------------------+
| Determine        |
| Protocol Version |
| (from cache)     |
+--------+---------+
         |
         v
+------------------+
| Validate         |
| Permissions      |
+--------+---------+
         |
    +----+----+
    |         |
    v         v
+--------+ +--------+
|Compress| | Lookup |
| Data   | | Cert   |
+---+----+ +---+----+
    |          |
    +----+-----+
         |
         v
+------------------+
|  Protocol Handler|
|  (v0 or cse_v1)  |
+--------+---------+
         |
    +----+----+
    |         |
    v         v
+--------+ +--------+
|Encrypt | |Add     |
|(if enc)| |Headers |
+---+----+ +---+----+
    |          |
    +----+-----+
         |
         v
+------------------+
|  outboundChannel |
| (to Pusher)      |
+------------------+
```

## 5. Implementation Phases

### Phase 1: Core Structure and Protocol Detection

**Goal**: Establish package structure and basic protocol detection.

**Tasks**:
1. Create package directory structure
2. Define `ProtocolIdentifier` struct in `pkg/models/action_models.go`
3. Implement `protocol/detector.go` with detection logic
4. Implement `protocol/cache/cache.go` using expiremap
5. Implement `protocol/versions/v0.go` (wraps existing encoding)
6. Write unit tests for protocol detection

**Files to Create**:
- `pkg/gatekeeper/protocol/protocol.go`
- `pkg/gatekeeper/protocol/detector.go`
- `pkg/gatekeeper/protocol/cache/cache.go`
- `pkg/gatekeeper/protocol/versions/v0.go`

**Dependencies**: None

### Phase 2: Compression Layer

**Goal**: Extract compression from encoding package into standalone module.

**Tasks**:
1. Create compression interface
2. Migrate zstd compression from `pkg/communicator/pkg/encoding/corev1/encoding.go`
3. Add compression threshold configuration
4. Write unit tests

**Files to Create**:
- `pkg/gatekeeper/compression/compression.go`
- `pkg/gatekeeper/compression/zstd.go`

**Dependencies**: None

### Phase 3: Certificate Handler

**Goal**: Centralize certificate management with caching.

**Tasks**:
1. Create certificate handler with cache
2. Migrate certificate fetching from `pull.go`
3. Migrate root CA decryption logic
4. Implement cache invalidation strategy
5. Write unit tests

**Files to Create**:
- `pkg/gatekeeper/certificatehandler/handler.go`
- `pkg/gatekeeper/certificatehandler/cache/cache.go`

**Dependencies**: Existing `pkg/permissions/` package

### Phase 4: Validator Wrapper

**Goal**: Create permission validation wrapper with action mapping.

**Tasks**:
1. Create validator wrapper that uses existing `permissions.Validator`
2. Implement action-to-permission mapping
3. Add location path handling
4. Write unit tests

**Files to Create**:
- `pkg/gatekeeper/validator/validator.go`
- `pkg/gatekeeper/validator/actions.go`

**Dependencies**: Existing `pkg/permissions/` package

### Phase 5: Encryption Layer

**Goal**: Prepare encryption infrastructure for CSE v1.

**Tasks**:
1. Create encryption interface
2. Implement NoopEncryptor for v0 protocol
3. Create stub for CSEV1Encryptor (depends on cryptolib)
4. Write unit tests

**Files to Create**:
- `pkg/gatekeeper/encryption/encryption.go`
- `pkg/gatekeeper/encryption/noop.go`
- `pkg/gatekeeper/encryption/cse_v1.go`

**Dependencies**: ManagementConsole cryptolib (build tag: cryptolib)

### Phase 6: Gatekeeper Orchestrator

**Goal**: Implement main orchestrator that ties all components together.

**Tasks**:
1. Create Gatekeeper struct with all dependencies
2. Implement channel management
3. Implement inbound message processing pipeline
4. Implement outbound message processing pipeline
5. Add graceful shutdown
6. Write integration tests

**Files to Create**:
- `pkg/gatekeeper/gatekeeper.go`
- `pkg/gatekeeper/options.go`
- `pkg/gatekeeper/gatekeeper_test.go`

**Dependencies**: All previous phases

### Phase 7: Integration with Existing Code

**Goal**: Wire Gatekeeper into the communication state.

**Tasks**:
1. Modify `communication_state.go` to initialize Gatekeeper
2. Update `Puller` to output raw messages only
3. Update `Router` to receive from Gatekeeper's verified channel
4. Update `Pusher` to receive from Gatekeeper's outbound channel
5. Add startup sequencing (Transport -> Gatekeeper)
6. Write integration tests

**Files to Modify**:
- `pkg/communicator/communication_state/communication_state.go`
- `pkg/communicator/api/v2/pull/pull.go`
- `pkg/communicator/router/router.go`
- `pkg/communicator/api/v2/push/push.go`

**Dependencies**: Phase 6 complete

## 6. Dependencies Between Components

```
+------------------+
|    validator     |  (moved from pkg/permissions/ + wrapper)
|  - interface.go  |
|  - noop/impl     |
|  - wrapper.go    |
+--------+---------+
         |
         | uses
         v
+------------------+
| certificatehandler |
+--------+---------+
         |
         | uses
         v
+------------------+
|    protocol      |
+--------+---------+
         |
    +----+----+
    |         |
    v         v
+--------+ +--------+
|compress| |encrypt |
+--------+ +--------+
         |
         | all used by
         v
+------------------+
|    gatekeeper    |  (main orchestrator)
+------------------+
```

All subpackages are under `pkg/gatekeeper/`.

## 7. Integration Points

### 7.1 Transport Layer Integration

**Current (pull.go)**:
```go
msgWithInfo := &models.UMHMessageWithAdditionalInfo{
    UMHMessage:        message,
    RootCA:            p.rootCA,
    IntermediateCerts: p.intermediateCerts,
}
p.inboundMessageChannel <- msgWithInfo
```

**After Gatekeeper**:
```go
// Puller outputs raw messages only
p.inboundMessageChannel <- &message
// Certificate fetching moves to Gatekeeper's certificate handler
```

### 7.2 Router Integration

**Current (router.go)**:
```go
func (r *Router) router() {
    for {
        select {
        case message := <-r.inboundChannel:
            messageContent, err := encoding.DecodeMessageFromUserToUMHInstance(message.Content)
            // ... handle action/subscribe
        }
    }
}
```

**After Gatekeeper**:
```go
func (r *Router) router() {
    for {
        select {
        case verifiedMsg := <-r.inboundVerifiedChannel:
            // Message already decrypted and validated
            switch verifiedMsg.Content.MessageType {
            case models.Subscribe:
                r.handleSub(verifiedMsg)
            case models.Action:
                r.handleAction(verifiedMsg)
            }
        }
    }
}
```

## 8. What to Keep/Move from Current Implementation

1. **`pkg/permissions/` package** - Move to `pkg/gatekeeper/validator/`:
   - `interface.go` → `pkg/gatekeeper/validator/interface.go`
   - `validator_impl.go` → `pkg/gatekeeper/validator/validator_impl.go`
   - `validator_noop.go` → `pkg/gatekeeper/validator/validator_noop.go`
   - Helper functions → `pkg/gatekeeper/validator/helpers.go`

2. **Encoding utilities** - Keep for backward compatibility:
   - `pkg/communicator/pkg/encoding/` - Keep as fallback during migration
   - Compression logic reused in `gatekeeper/compression/`

3. **ExpireMap usage pattern** - Keep same caching approach

## 9. What to Remove/Replace

1. **`pkg/permissions/` directory**:
   - Move contents to `pkg/gatekeeper/validator/`
   - Delete original directory after migration
   - Update all imports from `pkg/permissions` to `pkg/gatekeeper/validator`

2. **Certificate handling in Puller** (lines 156-223 of `pull.go`):
   - Move to `gatekeeper/certificatehandler/`
   - Puller becomes a simple message fetcher

3. **Message decoding in Router** (lines 96-97 of `router.go`):
   - Move to Gatekeeper's inbound processing
   - Router receives pre-decoded messages

4. **Permission validation scattered across components**:
   - `subscriber/subscribers.go` lines 110-128
   - `router/router.go` lines 147-148
   - Centralize in `gatekeeper/validator/`

5. **Direct encoding calls**:
   - Replace `encoding.DecodeMessageFromUserToUMHInstance()` calls
   - Route through Gatekeeper's protocol handlers

## 10. Startup Sequence

```go
// In communication_state.go or main.go

import (
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/validator"
)

// 1. Start Transport first (gets instance UUID)
instanceUUID := startTransport()

// 2. Start Gatekeeper with instance UUID
gk := gatekeeper.NewGatekeeper(
    gatekeeper.WithInstanceUUID(instanceUUID),
    gatekeeper.WithValidator(validator.NewValidator()),
    gatekeeper.WithInboundChannel(puller.GetOutputChannel()),
    gatekeeper.WithOutboundChannel(pusher.GetInputChannel()),
    gatekeeper.WithAuthTokenHash(authTokenHash),
    gatekeeper.WithLogger(logger),
)
gk.Start(ctx)

// 3. Start business logic with Gatekeeper's verified channels
router := router.NewRouter(
    gk.GetInboundVerifiedChannel(),
    gk.GetOutboundVerifiedChannel(),
    // ...
)
```

## 11. Linux-like Permission Model

The validation follows a Linux-like permission model:

```go
// Example validation call
Validate(
    objectPath,      // "umh.v1.enterprise.site.area" (like file path)
    user,            // "jt@umh.app" (like user/group)
    actionType,      // "add/edit/delete" (like read/write/execute)
    rootCA,          // Trust anchor
    certificateBundle, // User's certificate chain
)
```

| Concept | Linux | Gatekeeper |
|---------|-------|----------|
| Path | `/home/user/file` | `umh.v1.enterprise.site.area` |
| User | `jt` | `jt@umh.app` |
| Permissions | `rwx` | `add/edit/delete` |
| Object | file | bridge, data flow, etc. |

## 12. Testing Strategy

### Unit Tests
- Protocol detection accuracy
- Compression threshold behavior
- Certificate cache expiration
- Validator action mapping

### Integration Tests
- End-to-end message flow through Gatekeeper
- Protocol version negotiation
- Certificate caching behavior under load
- Graceful degradation when cryptolib unavailable

### Performance Benchmarks
- Target: <500ns for small message encode/decode
- Target: <10ms for large message round-trip
- Memory allocation limits per message
