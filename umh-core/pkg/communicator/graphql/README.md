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

## Development & Testing

### 🎯 Test Types Overview

#### 1. **Unit Tests** 
- **Purpose**: Test individual components and functions in isolation
- **Dependencies**: None (uses mocks)
- **Speed**: Fast (~1-2 seconds)

#### 2. **Integration Tests**
- **Purpose**: Test real HTTP requests and GraphQL queries  
- **Dependencies**: Requires actual server startup
- **Speed**: Medium (~5-10 seconds)

#### 3. **Live Testing**
- **Purpose**: Test with real-time changing data
- **Dependencies**: Docker container with simulator
- **Speed**: Manual/Interactive

### 🧪 Unit Tests

**What They Test:**
- GraphQL resolver functions (`Topics`, `Topic`)
- Server creation and configuration
- Mock data handling
- Schema validation

**How to Run:**
```bash
# Run all unit tests
cd umh-core
go test ./pkg/communicator/graphql/...

# Run with verbose output
go test -v ./pkg/communicator/graphql/...

# Run specific test
go test -v ./pkg/communicator/graphql/... -run TestResolver_Topics
```

### 🔗 Integration Tests (Ginkgo/Gomega)

**What They Test:**
- Real HTTP server startup/shutdown
- Actual GraphQL queries over HTTP
- CORS functionality
- Error handling with real responses
- Server configuration

**How to Run:**
```bash
# Run integration tests only
cd umh-core
go test -tags=integration -v ./pkg/communicator/graphql/...

# Run with Ginkgo directly for better output
ginkgo -v --tags=integration ./pkg/communicator/graphql/...
```

**Test Structure:**
```
GraphQL Server Integration Tests
├── When starting the server
│   ├── Should create server successfully
│   └── Should serve GraphQL on correct port
├── When querying GraphQL endpoint  
│   ├── Should handle topics queries
│   ├── Should handle filtered queries
│   └── Should handle single topic queries
├── When handling CORS
│   └── Should include proper CORS headers
└── When handling errors
    ├── Should handle malformed JSON (400)
    └── Should handle invalid GraphQL (422)
```

### 🚀 Live Testing with Real Data

**What It Tests:**
- **Real-time data changes**: Simulator updates every second
- **Complete end-to-end flow**: Docker → UMH Core → GraphQL → Client
- **Production-like environment**: Full container with all services
- **Performance**: Response times with realistic data loads

#### **Step 1: Start GraphQL Server**
```bash
cd umh-core
make test-graphql
```

This starts:
- **GraphQL API**: `http://localhost:8090/graphql`
- **GraphiQL Playground**: `http://localhost:8090/`
- **Metrics**: `http://localhost:8081/metrics`
- **Built-in Simulator**: Generates UNS topics every second

#### **Step 2: Test with curl**

**Basic Topics Query:**
```bash
curl -X POST http://localhost:8090/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ topics(limit: 3) { topic metadata { key value } } }"}'
```

**Query with Events (Changing Data):**
```bash
curl -X POST http://localhost:8090/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ topics(limit: 2) { topic lastEvent { producedAt ... on TimeSeriesEvent { numericValue } } } }"}'
```

**Filtered Query:**
```bash
curl -X POST http://localhost:8090/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ topics(filter: {text: \"pump\"}) { topic } }"}'
```

#### **Step 3: Verify Data Changes**

Run the same query multiple times to see values change:
```bash
# Test 1
curl -s -X POST http://localhost:8090/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ topics(limit: 1) { lastEvent { ... on TimeSeriesEvent { numericValue } } } }"}' \
  | jq '.data.topics[0].lastEvent.numericValue'

# Wait 3 seconds
sleep 3

# Test 2 - value should be different
curl -s -X POST http://localhost:8090/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ topics(limit: 1) { lastEvent { ... on TimeSeriesEvent { numericValue } } } }"}' \
  | jq '.data.topics[0].lastEvent.numericValue'
```

**Expected Output:**
```
Test 1: 23.55
Test 2: 41.52  (different value = ✅ data changing)
```

#### **Step 4: Use GraphiQL Playground**

Open `http://localhost:8090/` in your browser for interactive testing.

### 🛠️ Development Workflow

#### **1. Code Changes → Unit Tests**
```bash
# Quick feedback loop for development
cd umh-core
go test ./pkg/communicator/graphql/...
```

#### **2. New Features → Integration Tests**
```bash
# Test HTTP integration and GraphQL schema
go test -tags=integration -v ./pkg/communicator/graphql/...
```

#### **3. End-to-End → Live Testing**
```bash
# Test complete system with realistic data
make test-graphql
# Then test with curl/browser
```

#### **4. CI Pipeline → All Tests**
```bash
# What CI should run
go test ./pkg/communicator/graphql/...                    # Unit tests
go test -tags=integration ./pkg/communicator/graphql/...  # Integration tests
golangci-lint run ./pkg/communicator/graphql/...         # Linting
go vet ./pkg/communicator/graphql/...                    # Static analysis
```

### 🐛 Troubleshooting

**GraphQL Server Not Starting:**
```bash
# Check Docker container logs
docker logs umh-core

# Verify ports are free
netstat -tlnp | grep 8090

# Check config file
cat data/config.yaml | grep -A5 graphql
```

**No Data Changing:**
```bash
# Verify simulator is running (look for log entries)
docker logs umh-core | grep "simulator\|topic"

# Check if GraphQL resolver can access data
curl -s http://localhost:8090/graphql -d '{"query":"{topics{topic}}"}' -H "Content-Type: application/json"
```

### ✅ Test Checklist

Before committing changes, verify:

- [ ] **Unit tests pass**: `go test ./pkg/communicator/graphql/...`
- [ ] **Integration tests pass**: `go test -tags=integration ./pkg/communicator/graphql/...`
- [ ] **Linting clean**: `golangci-lint run ./pkg/communicator/graphql/...`
- [ ] **Live server starts**: `make test-graphql` (no errors in logs)
- [ ] **Data changes**: Values update every ~1 second
- [ ] **GraphiQL accessible**: `http://localhost:8090/` loads

**🎉 All tests passing = Ready for production!**

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