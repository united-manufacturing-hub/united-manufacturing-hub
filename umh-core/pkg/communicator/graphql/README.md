# GraphQL Topic Browser API

This package provides a comprehensive GraphQL API for browsing and querying Unified Namespace (UNS) data in UMH Core. The API serves as a real-time interface to the topic browser cache, enabling sophisticated queries, filtering, and data exploration of manufacturing data following ISA-95 hierarchies and UNS conventions.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [GraphQL Fundamentals](#graphql-fundamentals)
- [UNS Integration](#uns-integration)
- [Architecture](#architecture)
- [Data Model](#data-model)
- [Schema Reference](#schema-reference)
- [Query Examples](#query-examples)
- [Real-Time Data Flow](#real-time-data-flow)
- [Configuration](#configuration)
- [Development](#development)

## Overview

The GraphQL Topic Browser API bridges the gap between UMH Core's internal data structures and external applications that need to query manufacturing data. It provides:

- **Real-time access** to UNS topic data with sub-second latency
- **Structured querying** with filtering, pagination, and field selection
- **Type-safe data access** with GraphQL's strong typing system
- **Interactive exploration** via GraphiQL playground
- **Standards compliance** with ISA-95 hierarchies and UNS conventions

### Key Benefits

- **Unified Data Access**: Single endpoint for all UNS topics across your manufacturing environment
- **Efficient Querying**: Request only the fields you need, reducing bandwidth and processing
- **Real-Time Updates**: Live data from the simulator or production systems
- **Developer Experience**: Self-documenting API with built-in schema introspection
- **Manufacturing Context**: Native understanding of ISA-95 hierarchies and industrial data patterns

## Quick Start

Get the GraphQL API running in under 2 minutes:

```bash
# 1. Navigate to umh-core directory
cd umh-core

# 2. Start GraphQL server with built-in simulator
make test-graphql

# 3. Open GraphiQL playground in your browser
open http://localhost:8090/

# 4. Try your first query
```

**First Query** (paste into GraphiQL):
```graphql
{
  topics {
    topic
    lastEvent {
      producedAt
      ... on TimeSeriesEvent {
        numericValue
      }
    }
  }
}
```

You should see 2 simulator topics with values that update every second. The GraphQL API is now serving real-time manufacturing data following UNS conventions!

## GraphQL Fundamentals

### What is GraphQL?

GraphQL is a query language and runtime for APIs that provides a complete and understandable description of the data in your API. Unlike REST APIs with multiple endpoints, GraphQL uses a single endpoint with a flexible query language.

#### Key GraphQL Concepts

**Schema**: Defines the structure of your API - what data is available and how it's organized
```graphql
type Topic {
  topic: String!
  lastEvent: Event
  metadata: [MetadataKv!]!
}
```

**Queries**: Read-only operations to fetch data
```graphql
query {
  topics {
    topic
    lastEvent {
      producedAt
    }
  }
}
```

**Types**: Define the shape of data objects
- **Scalar Types**: `String`, `Int`, `Float`, `Boolean`, `ID`
- **Object Types**: Custom types like `Topic`, `Event`
- **Union Types**: Types that can be one of several types (e.g., `Event`)
- **Interfaces**: Shared fields across multiple types

**Fields**: Individual pieces of data within a type
```graphql
{
  topic        # String field
  lastEvent {  # Object field
    producedAt # Nested scalar field
  }
}
```

**Arguments**: Parameters passed to fields to modify their behavior
```graphql
{
  topics(filter: { text: "temperature" }, limit: 10) {
    topic
  }
}
```

### GraphQL vs REST for Manufacturing Data

| Aspect | REST | GraphQL |
|--------|------|---------|
| **Endpoints** | Multiple (`/topics`, `/events`, `/metadata`) | Single (`/graphql`) |
| **Data Fetching** | Over-fetching or under-fetching common | Fetch exactly what you need |
| **Versioning** | URL versioning (`/v1/topics`) | Schema evolution with deprecation |
| **Real-time** | Requires polling or WebSockets | Built-in subscription support |
| **Discovery** | Documentation separate | Self-documenting schema |
| **Manufacturing Context** | Generic HTTP patterns | Domain-specific query language |

## UNS Integration

### Unified Namespace Overview

The Unified Namespace (UNS) is an event-driven architecture that flips the traditional ISA-95 "automation pyramid" by having every device, PLC, or application publish its own events as they happen. This creates a standardized, hierarchical topic structure that enables:

- **Event-driven data flow**: Producers publish regardless of consumers
- **Standard topic hierarchy**: ISA-95-compatible keys with data contracts
- **Stream processing**: Real-time data contextualization and modeling
- **Publish-regardless mentality**: Adding dashboards becomes a reading exercise, not a reprogramming project

### Topic Convention

The GraphQL API exposes topics following the UNS topic convention:

```
umh.v1.<location_path>.<data_contract>[.<virtual_path>].<tag_name>
```

#### Topic Segments Explained

| Segment | Description | Example | Filled by |
|---------|-------------|---------|-----------|
| **umh.v1** | Constant product/schema generation prefix | `umh.v1` | System |
| **location_path** | Dot-separated ISA-95 hierarchy (up to 6 levels) | `acme.cologne.line1.station3` | `msg.meta.location_path` |
| **data_contract** | Logical model starting with underscore | `_historian`, `_setpoint` | `msg.meta.data_contract` |
| **virtual_path** | Optional sub-segments for sub-models/folders | `motor.diagnostics` | `msg.meta.virtual_path` |
| **tag_name** | Leaf field inside the contract | `temperature`, `pressure` | `msg.meta.tag_name` |

#### ISA-95 Location Hierarchy

The `location_path` follows ISA-95 manufacturing hierarchy:

```
<enterprise>.<site>.<area>.<line>.<workCell>.<unit>
```

**Example**: `acme-inc.cologne-plant.cnc-line.station-1.robot-arm.gripper`

- **Enterprise** (Level 0): Company or organization (`acme-inc`)
- **Site** (Level 1): Physical manufacturing location (`cologne-plant`)
- **Area** (Level 2): Production area within site (`cnc-line`)
- **Line** (Level 3): Manufacturing line (`station-1`)
- **Work Cell** (Level 4): Work cell or machine group (`robot-arm`)
- **Unit** (Level 5): Individual equipment unit (`gripper`)

### Data Contracts

Data contracts define the logical model and payload structure:

#### Built-in Contracts

- **`_historian`**: Time-series sensor data (temperature, pressure, counts)

#### Custom Contracts

Custom contracts can be defined for specific equipment or processes:
- `TemperatureSensor`: Specialized temperature monitoring
- `Pump`: Pump-specific parameters and diagnostics
- `ConveyorBelt`: Conveyor control and status

### Payload Formats

The GraphQL API supports three payload formats as defined in UMH Core:

#### 1. Time-Series / Tags
**Purpose**: Sensor readings, counters, states
**Structure**: One numeric/boolean value + timestamp
**Use Case**: Data from PLCs, sensors, IoT devices

```json
{
  "value": 23.4,
  "timestamp_ms": 1717083000000
}
```

#### 2. Relational / JSON
**Purpose**: Business records, complex data structures
**Structure**: Self-contained JSON documents
**Use Case**: Orders, recipes, batch reports, alarms

```json
{
  "order_id": "ORD-12345",
  "product": "Widget-A",
  "quantity": 1000,
  "timestamp_ms": 1717083000000
}
```

#### 3. Binary Blob
**Purpose**: Files and binary data
**Structure**: File pointers or binary payloads
**Use Case**: Images, PDFs, CNC programs, documentation

### UNS Tree ID Generation

Topics are internally indexed using xxHash-based UNS Tree IDs for efficient storage and retrieval:

```go
// Hash generation with null-byte delimiters
hasher := xxhash.New()
hasher.Write([]byte(enterprise + "\x00"))
hasher.Write([]byte(site + "\x00"))
hasher.Write([]byte(area + "\x00"))
// ... continue for all levels
hasher.Write([]byte(dataContract + "\x00"))
hasher.Write([]byte(tagName + "\x00"))
treeId := hex.EncodeToString(hasher.Sum(nil))
```

This prevents hash collisions between different segment combinations (e.g., `["ab","c"]` vs `["a","bc"]`).

## Architecture

### Component Overview

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Simulator     │    │ Topic Browser   │    │   GraphQL API   │
│                 │───▶│     Cache       │───▶│                 │
│ Generates UNS   │    │                 │    │ Serves Queries  │
│ Data Every 1s   │    │ Stores Latest   │    │ via HTTP/JSON   │
└─────────────────┘    │ Event Per Topic │    └─────────────────┘
                       └─────────────────┘              │
                                │                       │
                                ▼                       ▼
                       ┌─────────────────┐    ┌─────────────────┐
                       │  Event Storage  │    │   GraphiQL      │
                       │                 │    │   Playground    │
                       │ Key: UNS Tree   │    │                 │
                       │ Value: Latest   │    │ Interactive     │
                       │ Event Data      │    │ Query Builder   │
                       └─────────────────┘    └─────────────────┘
```

### File Structure

```
pkg/communicator/graphql/
├── README.md           # This comprehensive documentation
├── schema.graphqls     # GraphQL schema definition
├── models.go          # Generated GraphQL types from schema
├── resolver.go        # Query resolver implementations
├── server.go          # HTTP server and middleware setup
├── simple_test.go     # Unit tests for resolver logic
└── graphql_suite_test.go # Ginkgo test suite runner
```

#### File Responsibilities

**`schema.graphqls`**: Defines the GraphQL schema using Schema Definition Language (SDL)
- Type definitions for `Topic`, `Event`, `MetadataKv`
- Query operations and their parameters
- Union types for different event formats

**`models.go`**: Auto-generated Go types from the GraphQL schema
- Struct definitions matching GraphQL types
- Enum definitions for scalar types
- Interface definitions for unions

**`resolver.go`**: Implements the business logic for GraphQL queries
- `Topics()`: Lists and filters topics from the cache
- `Topic()`: Retrieves a specific topic by name
- Helper methods for data transformation and filtering

**`server.go`**: Sets up the HTTP server and GraphQL endpoint
- Gin framework integration for HTTP handling
- GraphQL handler configuration with playground
- CORS middleware for web development
- Request logging and error handling

### Data Flow

1. **Data Generation**: Simulator generates UNS-compliant data every second
2. **Cache Update**: Topic browser cache receives and processes new data
3. **Event Storage**: Latest event per topic stored with UNS Tree ID as key
4. **Topic Mapping**: UNS map maintains topic metadata and structure
5. **GraphQL Query**: Client sends GraphQL query to `/graphql` endpoint
6. **Resolver Execution**: Resolver fetches data from cache and transforms it
7. **Response**: JSON response returned with requested fields only

### Concurrency and Performance

**Thread Safety**: Topic browser cache uses read-write mutexes for concurrent access
**Memory Efficiency**: Only latest event per topic stored, not full history
**Query Optimization**: GraphQL field selection reduces data transfer
**Caching Strategy**: In-memory cache with configurable retention policies

## Data Model

### Core Types

#### Topic
Represents a single UNS topic with its latest event and metadata.

```graphql
type Topic {
  topic: String!           # Full UNS topic name
  lastEvent: Event        # Latest event (nullable if no data)
  metadata: [MetadataKv!]! # Topic metadata as key-value pairs
}
```

**Fields Explained**:
- `topic`: Complete UNS topic following convention (e.g., `umh.v1.acme.plant1._historian.temperature`)
- `lastEvent`: Most recent event data, can be null for topics without events
- `metadata`: Additional information about the topic (units, sensor IDs, etc.)

#### Event (Union Type)
Events can be one of several types depending on the payload format.

```graphql
union Event = TimeSeriesEvent | RelationalEvent
```

##### TimeSeriesEvent
For sensor data, measurements, and scalar values.

```graphql
type TimeSeriesEvent {
  producedAt: Time!           # When the event was produced
  sourceTs: Time!             # Original timestamp from source
  scalarType: ScalarType!     # Data type of the value
  numericValue: Float         # Numeric value (if scalar type is NUMERIC)
  stringValue: String         # String value (if scalar type is STRING)
  booleanValue: Boolean       # Boolean value (if scalar type is BOOLEAN)
  headers: [MetadataKv!]!     # Kafka headers and metadata
}
```

**Scalar Types**:
- `NUMERIC`: Floating-point numbers (temperature, pressure, counts)
- `STRING`: Text values (status messages, part numbers)
- `BOOLEAN`: True/false values (alarm states, switch positions)

##### RelationalEvent
For structured data and business records.

```graphql
type RelationalEvent {
  producedAt: Time!           # When the event was produced
  headers: [MetadataKv!]!     # Kafka headers and metadata
  json: JSON!                 # Structured JSON payload
}
```

#### MetadataKv
Key-value pairs for metadata and headers.

```graphql
type MetadataKv {
  key: String!
  value: String!
}
```

**Common Metadata Keys**:
- `unit`: Measurement unit (°C, bar, pieces, kW)
- `sensor_id`: Unique sensor identifier
- `location`: Physical location description
- `source`: Data source system

### Query Parameters

#### TopicFilter
Filters topics based on text matching.

```graphql
input TopicFilter {
  text: String  # Substring match against topic name
}
```

**Filter Behavior**:
- Case-insensitive substring matching
- Searches the full topic name
- Returns topics containing the filter text

**Examples**:
```graphql
# Find all temperature-related topics
topics(filter: { text: "temperature" })

# Find topics from a specific location
topics(filter: { text: "acme.plant1" })

# Find historian data
topics(filter: { text: "_historian" })
```

#### Pagination
Limit the number of results returned.

```graphql
topics(limit: Int)
```

**Behavior**:
- Default limit: 100 topics
- Maximum limit: 100 topics (enforced)
- Results are not guaranteed to be in any specific order

### Data Relationships

```
Topic
├── topic (String)              # "umh.v1.acme.plant1._historian.temperature"
├── lastEvent (Event)           # Latest event data
│   ├── producedAt (Time)       # 2024-06-25T10:30:00Z
│   ├── sourceTs (Time)         # 2024-06-25T10:29:59Z
│   ├── scalarType (Enum)       # NUMERIC
│   ├── numericValue (Float)    # 23.4
│   └── headers (MetadataKv[])  # [{"key": "unit", "value": "°C"}]
└── metadata (MetadataKv[])     # [{"key": "unit", "value": "°C"}]
```

## Schema Reference

### Complete GraphQL Schema

```graphql
# Scalar Types
scalar Time
scalar JSON

# Enums
enum ScalarType {
  NUMERIC
  STRING
  BOOLEAN
}

# Input Types
input TopicFilter {
  text: String
}

# Object Types
type Topic {
  topic: String!
  lastEvent: Event
  metadata: [MetadataKv!]!
}

type TimeSeriesEvent {
  producedAt: Time!
  sourceTs: Time!
  scalarType: ScalarType!
  numericValue: Float
  stringValue: String
  booleanValue: Boolean
  headers: [MetadataKv!]!
}

type RelationalEvent {
  producedAt: Time!
  headers: [MetadataKv!]!
  json: JSON!
}

type MetadataKv {
  key: String!
  value: String!
}

# Union Types
union Event = TimeSeriesEvent | RelationalEvent

# Root Query Type
type Query {
  topics(filter: TopicFilter, limit: Int): [Topic!]!
  topic(topic: String!): Topic
}
```

### Schema Evolution

The GraphQL schema supports evolution through:

**Field Addition**: New fields can be added without breaking existing clients
```graphql
type Topic {
  topic: String!
  lastEvent: Event
  metadata: [MetadataKv!]!
  # New field added
  createdAt: Time
}
```

**Field Deprecation**: Old fields can be marked as deprecated
```graphql
type Topic {
  topic: String!
  lastEvent: Event
  metadata: [MetadataKv!]!
  # Deprecated field
  oldField: String @deprecated(reason: "Use newField instead")
}
```

**Type Extension**: New event types can be added to the union
```graphql
union Event = TimeSeriesEvent | RelationalEvent | BinaryEvent
```

## Query Examples

### Quick Start Examples

Try these queries immediately after starting the GraphQL server with `make test-graphql`:

#### List All Simulator Topics
```graphql
query GetSimulatorTopics {
  topics {
    topic
    lastEvent {
      producedAt
      ... on TimeSeriesEvent {
        scalarType
        numericValue
      }
    }
  }
}
```

**Expected Response** (values change every second):
```json
{
  "data": {
    "topics": [
      {
        "topic": "corpA.plant-1.line-4.pump-41.uns.topic1.uns.topic1",
        "lastEvent": {
          "producedAt": "1970-01-01T00:00:00.034Z",
          "scalarType": "NUMERIC",
          "numericValue": 0.0729175118083637
        }
      },
      {
        "topic": "corpA.plant-1.line-4.pump-42.uns.topic2.uns.topic2",
        "lastEvent": {
          "producedAt": "1970-01-01T00:00:00.034Z",
          "scalarType": "NUMERIC",
          "numericValue": 0.8251994434710742
        }
      }
    ]
  }
}
```

#### Filter Simulator Topics
```graphql
query FilterByPump {
  topics(filter: { text: "pump-41" }) {
    topic
    lastEvent {
      ... on TimeSeriesEvent {
        numericValue
      }
    }
  }
}
```

#### Get Single Simulator Topic
```graphql
query GetSpecificTopic {
  topic(topic: "corpA.plant-1.line-4.pump-41.uns.topic1.uns.topic1") {
    topic
    lastEvent {
      producedAt
      ... on TimeSeriesEvent {
        numericValue
      }
    }
  }
}
```

### Production Examples

#### List All Topics
```graphql
query GetAllTopics {
  topics {
    topic
    lastEvent {
      producedAt
      ... on TimeSeriesEvent {
        scalarType
        numericValue
        stringValue
        booleanValue
      }
    }
    metadata {
      key
      value
    }
  }
}
```

**Response**:
```json
{
  "data": {
    "topics": [
      {
        "topic": "umh.v1.acme.plant1._historian.temperature",
        "lastEvent": {
          "producedAt": "2024-06-25T10:30:00Z",
          "scalarType": "NUMERIC",
          "numericValue": 23.4,
          "stringValue": null,
          "booleanValue": null
        },
        "metadata": [
          {"key": "unit", "value": "°C"}
        ]
      }
    ]
  }
}
```

#### Get Specific Topic
```graphql
query GetSpecificTopic {
  topic(topic: "umh.v1.acme.plant1._historian.temperature") {
    topic
    lastEvent {
      producedAt
      ... on TimeSeriesEvent {
        numericValue
        headers {
          key
          value
        }
      }
    }
  }
}
```

### Advanced Queries

#### Filter by Location
```graphql
query GetPlantTopics {
  topics(filter: { text: "plant1" }) {
    topic
    lastEvent {
      producedAt
      ... on TimeSeriesEvent {
        scalarType
        numericValue
      }
    }
  }
}
```

#### Filter by Data Contract
```graphql
query GetHistorianData {
  topics(filter: { text: "_historian" }) {
    topic
    lastEvent {
      ... on TimeSeriesEvent {
        numericValue
        scalarType
      }
    }
    metadata {
      key
      value
    }
  }
}
```

#### Paginated Results
```graphql
query GetLimitedTopics {
  topics(limit: 5) {
    topic
    lastEvent {
      producedAt
    }
  }
}
```

#### Multiple Filters and Selections
```graphql
query GetTemperatureData {
  topics(filter: { text: "temperature" }, limit: 10) {
    topic
    lastEvent {
      producedAt
      sourceTs
      ... on TimeSeriesEvent {
        numericValue
        scalarType
        headers {
          key
          value
        }
      }
    }
    metadata {
      key
      value
    }
  }
}
```

### Fragment Usage

Fragments allow reusing common field selections:

```graphql
fragment EventDetails on TimeSeriesEvent {
  producedAt
  sourceTs
  scalarType
  numericValue
  stringValue
  booleanValue
}

fragment TopicInfo on Topic {
  topic
  metadata {
    key
    value
  }
}

query GetTopicsWithFragments {
  topics(filter: { text: "sensor" }) {
    ...TopicInfo
    lastEvent {
      ...EventDetails
    }
  }
}
```

### Conditional Queries

Use GraphQL variables for dynamic queries:

```graphql
query GetFilteredTopics($filterText: String, $maxResults: Int) {
  topics(filter: { text: $filterText }, limit: $maxResults) {
    topic
    lastEvent {
      producedAt
      ... on TimeSeriesEvent {
        numericValue
      }
    }
  }
}
```

**Variables**:
```json
{
  "filterText": "temperature",
  "maxResults": 5
}
```

### Error Handling

GraphQL queries can return partial results with errors:

```json
{
  "data": {
    "topics": [
      {
        "topic": "umh.v1.acme.plant1._historian.temperature",
        "lastEvent": null
      }
    ]
  },
  "errors": [
    {
      "message": "Failed to decode event data",
      "path": ["topics", 0, "lastEvent"],
      "extensions": {
        "code": "DECODE_ERROR"
      }
    }
  ]
}
```

## Real-Time Data Flow

### Simulator Integration

The GraphQL API integrates with UMH Core's built-in simulator for real-time data:

#### Simulator Characteristics
- **Update Frequency**: Every 1 second
- **Data Generation**: Random numeric values simulating sensor readings
- **Topic Structure**: Follows UNS convention with proper ISA-95 hierarchy
- **UNS Tree IDs**: Uses xxHash for efficient topic indexing

#### Sample Simulator Topics
```
uns.topic1 → corpA.plant-1.line-4.pump-41.uns.topic1.uns.topic1
uns.topic2 → corpA.plant-1.line-4.pump-42.uns.topic2.uns.topic2
```

#### Data Generation Process
1. **Tick Generation**: Simulator increments tick counter every second
2. **Value Generation**: Random float64 values between 0.0 and 1.0
3. **Event Creation**: TimeSeriesEvent with NUMERIC scalar type
4. **Cache Update**: Topic browser cache receives new events
5. **GraphQL Access**: API serves latest cached data

### Cache Architecture

#### Topic Browser Cache
The cache maintains the latest event for each topic:

```go
type Cache struct {
    eventMap           map[string]*EventTableEntry  // UNS Tree ID → Latest Event
    unsMap             *TopicMap                     // Topic Name → Topic Info
    lastCacheTimestamp int64                         // Last update time
    lastSentTimestamp  int64                         // Last sent time
}
```

#### Update Process
1. **Buffer Processing**: New events arrive in observed state buffer
2. **Timestamp Filtering**: Only process events newer than last cache timestamp
3. **Event Upsert**: Replace older events with newer ones for same topic
4. **UNS Map Update**: Update topic metadata and structure
5. **Timestamp Tracking**: Update last processed timestamp

#### Data Consistency
- **Read-Write Locks**: Concurrent access protection
- **Atomic Updates**: Cache updates are atomic per buffer
- **Timestamp Ordering**: Events processed in timestamp order
- **Idempotency**: Repeated updates with same data are safe

### Performance Characteristics

#### Latency
- **Simulator to Cache**: Sub-second (typically <100ms)
- **Cache to GraphQL**: Microseconds (in-memory access)
- **GraphQL Response**: Milliseconds (JSON serialization)
- **End-to-End**: Typically <1 second from data generation to API response

#### Throughput
- **Topics Supported**: Thousands of concurrent topics
- **Query Rate**: Hundreds of queries per second
- **Data Volume**: Configurable retention and compression
- **Memory Usage**: Scales linearly with topic count

#### Scalability Considerations
- **Memory**: One event per topic (not full history)
- **CPU**: Hash-based lookups for O(1) topic access
- **Network**: GraphQL field selection reduces bandwidth
- **Storage**: Optional persistence for cache state

## Configuration

### GraphQL Server Configuration

The GraphQL server is configured through the main UMH Core configuration file:

```yaml
agent:
  # GraphQL API Configuration
  graphql:
    enabled: true        # Enable GraphQL server
    port: 8090          # GraphQL endpoint at http://localhost:8090/graphql
    debug: true         # Enable GraphiQL playground and debug logging
    corsOrigins:        # Allow specific origins for CORS
      - "http://localhost:3000"
      - "https://dashboard.example.com"
    authRequired: false  # Require authentication (future feature)
```

#### Configuration Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | `bool` | `false` | Enable/disable GraphQL server |
| `port` | `int` | `8090` | HTTP port for GraphQL endpoint |
| `debug` | `bool` | `false` | Enable GraphiQL playground and verbose logging |
| `corsOrigins` | `[]string` | `["*"]` | CORS allowed origins for web development |
| `authRequired` | `bool` | `false` | Require authentication (planned feature) |

### Environment Variables

Override configuration with environment variables:

```bash
# GraphQL Configuration
UMH_GRAPHQL_ENABLED=true
UMH_GRAPHQL_PORT=8090
UMH_GRAPHQL_DEBUG=true

# General Configuration
UMH_AUTH_TOKEN=your-api-token
UMH_LOG_LEVEL=info
```

### Docker Configuration

When running in Docker, map the GraphQL port:

```bash
docker run -p 8090:8090 -p 8081:8080 \
  -v $(pwd)/config.yaml:/data/config.yaml \
  umh-core:latest
```

### Production Considerations

#### Security
- **CORS Configuration**: Restrict origins in production
- **Authentication**: Enable when auth system is implemented
- **Rate Limiting**: Consider reverse proxy with rate limiting
- **HTTPS**: Use TLS termination at load balancer

#### Performance
- **Query Complexity**: Monitor and limit complex queries
- **Caching**: Consider Redis for distributed caching
- **Load Balancing**: Multiple GraphQL instances for high availability
- **Monitoring**: Track query performance and error rates

#### Observability
- **Metrics**: Prometheus metrics on port 8081
- **Logging**: Structured logging with correlation IDs
- **Tracing**: Distributed tracing for query execution
- **Health Checks**: Endpoint health monitoring

## Development

### Local Development Setup

#### Prerequisites
- Go 1.21 or later
- Docker and Docker Compose
- Make (for build automation)

#### Quick Start
```bash
# Clone repository
git clone https://github.com/united-manufacturing-hub/united-manufacturing-hub.git
cd united-manufacturing-hub/umh-core

# Start GraphQL server with simulator
make test-graphql

# Access GraphiQL playground
open http://localhost:8090/
```

#### Development Workflow
```bash
# Build application
make build

# Run tests
go test ./pkg/communicator/graphql/...

# Generate GraphQL types (if schema changes)
go generate ./pkg/communicator/graphql/...

# Lint code
golangci-lint run ./pkg/communicator/graphql/...
```

### Testing

#### Unit Tests
```bash
# Run GraphQL package tests
go test ./pkg/communicator/graphql/ -v

# Run with coverage
go test ./pkg/communicator/graphql/ -cover
```

#### Integration Tests
```bash
# Start test environment
make test-graphql

# Run integration tests
curl -X POST http://localhost:8090/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ topics { topic } }"}'
```

#### Test Data
The simulator provides consistent test data:
- 2 topics with random numeric values
- Updates every second
- Proper UNS structure
- Realistic manufacturing context

### Schema Development

#### Modifying the Schema
1. Edit `schema.graphqls` with new types or fields
2. Run `go generate` to update Go types
3. Update resolvers in `resolver.go`
4. Add tests for new functionality
5. Update documentation

#### Schema Best Practices
- **Nullable Fields**: Use nullable types for optional data
- **Descriptive Names**: Clear, descriptive field and type names
- **Backwards Compatibility**: Avoid breaking changes
- **Documentation**: Add descriptions to schema elements
- **Validation**: Input validation for arguments

#### Example Schema Addition
```graphql
# Add new field to Topic type
type Topic {
  topic: String!
  lastEvent: Event
  metadata: [MetadataKv!]!
  # New field
  createdAt: Time
}

# Add new query
type Query {
  topics(filter: TopicFilter, limit: Int): [Topic!]!
  topic(topic: String!): Topic
  # New query
  topicsByLocation(location: String!): [Topic!]!
}
```

### Debugging

#### Debug Mode
Enable debug mode for detailed logging:

```yaml
agent:
  graphql:
    debug: true
```

#### Log Analysis
```bash
# View GraphQL logs
docker logs <container-id> | grep GraphQL

# Monitor query performance
docker logs <container-id> | grep "GraphQL POST"
```

#### Common Issues

**No Data Returned**
- Check simulator is running
- Verify cache is populated
- Check topic name spelling

**Performance Issues**
- Monitor query complexity
- Check cache size and memory usage
- Analyze slow queries

**Schema Errors**
- Validate GraphQL syntax
- Check type compatibility
- Verify resolver implementations

### Contributing

#### Code Style
- Follow Go conventions and formatting
- Use meaningful variable and function names
- Add comments for complex logic
- Write tests for new features

#### Pull Request Process
1. Create feature branch from main
2. Implement changes with tests
3. Update documentation
4. Submit pull request with description
5. Address review feedback

#### Documentation Updates
When adding features:
- Update this README
- Add examples to query section
- Update schema reference
- Consider adding to main docs

---

This comprehensive documentation provides everything needed to understand, use, and develop with the GraphQL Topic Browser API. For additional support or questions, please refer to the main UMH Core documentation or open an issue in the repository.