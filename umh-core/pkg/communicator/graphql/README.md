# GraphQL Browse API

A production-ready GraphQL API for browsing topics and events from the United Manufacturing Hub topic browser cache.

## Features

- 🚀 **Real-time topic browsing** - Query live topics and their latest events
- 🔍 **Advanced filtering** - Text search and metadata-based filtering  
- 📊 **Multi-format support** - Time series and relational event types
- 🛡️ **Production ready** - CORS, middleware, error handling, graceful shutdown
- ⚡ **High performance** - Optimized for 1000+ topics with early termination
- 🧪 **Fully tested** - Unit tests, integration tests, and linter compliance

## Quick Start

### Integration in main.go

```go
import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/graphql"

// Simple 3-line setup:
resolver := &graphql.Resolver{TopicBrowserCache: topicBrowserCache}
graphQLServer, err := graphql.StartGraphQLServer(resolver, cfg.GraphQL, logger)
// Server runs in background with graceful shutdown
```

### Configuration

```go
type GraphQLConfig struct {
    Enabled     bool     `yaml:"enabled" default:"false"`
    Port        int      `yaml:"port" default:"8080"`
    Debug       bool     `yaml:"debug" default:"false"`
    CORSOrigins []string `yaml:"corsOrigins" default:"[]"`
}
```

## GraphQL Schema

### Queries

#### Topics Query
Browse all topics with optional filtering and pagination:

```graphql
query {
  topics(filter: { text: "temperature" }, limit: 10) {
    topic
    metadata {
      key
      value
    }
    lastEvent {
      ... on TimeSeriesEvent {
        producedAt
        scalarType
        stringValue
        numericValue
        booleanValue
      }
      ... on RelationalEvent {
        producedAt
        json
      }
    }
  }
}
```

#### Single Topic Query
Get details for a specific topic:

```graphql
query {
  topic(topic: "enterprise.site.area.line.workstation.sensor.temperature") {
    topic
    metadata {
      key
      value
    }
    lastEvent {
      ... on TimeSeriesEvent {
        producedAt
        stringValue
      }
    }
  }
}
```

### Filtering Options

```graphql
input TopicFilter {
  text: String        # Search in topic path and metadata
  meta: [MetaExpr!]   # Exact metadata key-value matching
}

input MetaExpr {
  key: String!
  eq: String          # Exact value match
}
```

## Usage Examples

### Using curl

```bash
# Query topics
curl -X POST http://localhost:8080/graphql \
  -H "Content-Type: application/json" \
  -d '{
    "query": "{ topics(limit: 5) { topic metadata { key value } } }"
  }'

# Filter by text
curl -X POST http://localhost:8080/graphql \
  -H "Content-Type: application/json" \
  -d '{
    "query": "{ topics(filter: { text: \"temperature\" }) { topic } }"
  }'
```

### Using GraphiQL Playground

When debug mode is enabled, visit `http://localhost:8080/` for the interactive GraphiQL playground.

## Development

### Running Tests

```bash
# Unit tests
go test ./pkg/communicator/graphql/...

# Integration tests (starts real HTTP server)
go test ./pkg/communicator/graphql/... -tags=integration -v

# Linting
golangci-lint run ./pkg/communicator/graphql/...
```

### Building

```bash
go build ./pkg/communicator/graphql/...
```

## Architecture

### Components

- **`server.go`** - HTTP server with Gin, CORS, middleware, graceful shutdown
- **`resolver.go`** - GraphQL resolvers with optimized topic processing
- **`config.go`** - Configuration structures and adapters  
- **`helpers.go`** - Convenience functions for main.go integration
- **`schema.go`** - GraphQL schema definition (generated)

### Design Principles

- **Performance First** - Early termination, efficient filtering, context cancellation
- **Production Ready** - Comprehensive error handling, logging, monitoring hooks
- **Clean Architecture** - Testable interfaces, dependency injection, separation of concerns
- **Developer Experience** - Simple integration, clear documentation, debugging support

## Error Handling

The API provides comprehensive error handling:

- **GraphQL Errors** - Returned in standard GraphQL error format
- **HTTP Errors** - Proper status codes and CORS headers
- **Logging** - Structured logging with context (UNS tree ID, payload info)
- **Sentry Integration** - Automatic error reporting in production

## Performance

- **Optimized for Scale** - Handles 1000+ topics efficiently
- **Early Termination** - Stops processing when limit is reached
- **Context Aware** - Respects cancellation for long-running operations
- **Memory Efficient** - Direct cache access, minimal allocations

## Production Deployment

### Health Checks

The server provides health endpoints for orchestration:

- **GraphQL Endpoint** - `POST /graphql` 
- **Playground** - `GET /` (debug mode only)
- **Options** - `OPTIONS /graphql` (CORS preflight)

### Monitoring

Configure structured logging and Sentry reporting:

```go
logger := zap.NewProduction()
server, err := graphql.NewServer(resolver, config, logger.Sugar())
```

### CORS Configuration

For web applications, configure CORS origins:

```yaml
graphql:
  corsOrigins:
    - "http://localhost:3000"
    - "https://your-app.com"
```

## Contributing

1. All changes must pass tests: `go test ./...`
2. Run linter: `golangci-lint run ./...`
3. Add tests for new functionality
4. Update documentation as needed

## License

Licensed under the Apache License, Version 2.0.