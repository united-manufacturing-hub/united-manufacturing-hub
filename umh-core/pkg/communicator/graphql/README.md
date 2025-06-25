# GraphQL API

The GraphQL package provides a complete GraphQL server implementation for querying unified namespace data and system information.

## Architecture

The GraphQL server provides read-only access to:
- **Unified Namespace Topics**: Manufacturing data organized in hierarchical topics
- **System Snapshots**: Configuration and health information
- **Event Data**: Latest values and time-series information

### Dependencies

The GraphQL resolver requires two main dependencies injected from `main.go`:

```go
graphqlResolver := &graphql.Resolver{
    TopicBrowserCache: communicationState.TopicBrowserCache,  // UNS data access
    SnapshotManager:   systemSnapshotManager,                 // System state access
}
```

**TopicBrowserCache**: Provides access to unified namespace data
- Contains topic hierarchy (e.g., `enterprise.site.area.line.workstation.sensor.temperature`)
- Stores latest events/values for each topic
- Provides the core manufacturing data that GraphQL queries expose

**SnapshotManager**: Provides access to system configuration and state
- System-wide configuration snapshots
- Service health and status information
- Administrative data about the system

## Quick Start Demo

### Using Docker (Recommended)
```bash
cd umh-core
make test-graphql    # Start GraphQL server with mock data
```

### Direct Go execution
```bash
# Create a demo config file
cat > config-demo.yaml << EOF
metricsPort: 8080
graphql:
  enabled: true
  port: 8090
  debug: true
  mockData: true
  corsOrigins: ["*"]
communicator:
  mqttHost: "localhost"
  mqttPort: 1883
  mqttTopic: "umh/v1/+/+/+/+/+"
  mqttClientID: "umh-core-graphql-demo"
  enableKafka: false
  enableRedis: false
location:
  0: "acme"
  1: "cologne"
  2: "packaging"
  3: "station1"
releaseChannel: "stable"
allowInsecureTLS: true
EOF

# Start server
go run cmd/main.go --config config-demo.yaml
```

### Access Points

- **GraphiQL Playground**: http://localhost:8090/
- **GraphQL Endpoint**: http://localhost:8090/graphql

## Sample Queries

### List all topics
```graphql
query {
  topics {
    topic
    lastEvent {
      ... on TimeSeriesEvent {
        producedAt
        scalarType
        stringValue
        doubleValue
      }
    }
    metadata {
      key
      value
    }
  }
}
```

### Filter topics by text
```graphql
query {
  topics(filter: { text: "temperature" }) {
    topic
    lastEvent {
      ... on TimeSeriesEvent {
        producedAt
        doubleValue
        stringValue
      }
    }
    metadata {
      key
      value
    }
  }
}
```

### Get specific topic
```graphql
query {
  topic(topic: "umh.v1.acme.cologne.packaging.station1.temperature") {
    topic
    lastEvent {
      ... on TimeSeriesEvent {
        producedAt
        scalarType
        doubleValue
      }
    }
    metadata {
      key
      value
    }
  }
}
```

### Limit and paginate results
```graphql
query {
  topics(limit: 10, offset: 0) {
    topic
    lastEvent {
      ... on TimeSeriesEvent {
        producedAt
        doubleValue
      }
    }
  }
}
```

## Mock Data

When `mockData: true` is enabled, the system includes realistic UNS topics:

- `umh.v1.acme.cologne.packaging.station1.temperature` (23.9°C)
- `umh.v1.acme.cologne.packaging.station1.pressure` (1.19 bar)  
- `umh.v1.acme.cologne.packaging.station1.count` (1454 pieces)
- `umh.v1.acme.cologne.assembly.robot1.cycle_time` (12.4 seconds)
- `umh.v1.acme.cologne.quality.vision1.defect_rate` (0.6%)
- `umh.v1.acme.cologne.energy.main.power` (146.9 kW)
- `umh.v1.acme.cologne.maintenance.pump1.vibration` (2.2 mm/s)

Each topic includes realistic metadata (sensor IDs, locations, alarm levels, etc.).

## Configuration

```yaml
graphql:
  enabled: true              # Enable GraphQL server
  port: 8090                # HTTP port for GraphQL endpoint
  debug: true               # Enable GraphiQL playground
  mockData: false           # Use real data (default)
  corsOrigins:              # CORS configuration
    - "http://localhost:3000"
    - "https://dashboard.example.com"
```

## Package Structure

- `server.go`: Complete GraphQL server with middleware (CORS, logging, recovery)
- `resolver.go`: GraphQL resolvers for queries and data access
- `config.go`: Configuration management and adapters
- `helpers.go`: Convenience functions for integration
- `graphql_test.go`: Unit tests with mocks
- `schema.graphql`: GraphQL schema definition

## Development

### Running Tests
```bash
go test ./pkg/communicator/graphql/...
```

### Adding New Queries

1. Update `schema.graphql` with new query definitions
2. Regenerate GraphQL code: `go generate ./...`
3. Implement resolver methods in `resolver.go`
4. Add tests in `graphql_test.go`

### Performance Considerations

- Topics are processed with early termination when limits are reached
- Large datasets use pagination to avoid memory issues
- Context cancellation is supported for long-running queries
- JSON parsing errors are handled gracefully with logging