# GraphQL Package

This package provides a GraphQL API for browsing UMH Core topic browser data.

## Features

- **Complete GraphQL server** with Gin framework integration
- **Interactive GraphiQL playground** for development and testing
- **Mock data mode** with 7 realistic UNS manufacturing topics
- **CORS support** for web development
- **Filtering and pagination** for topic queries
- **Rich metadata** including sensor IDs, locations, and alarm levels

## Architecture

- `server.go` - GraphQL server setup and configuration
- `resolver.go` - GraphQL query resolvers
- `models.go` - GraphQL type definitions  
- `schema.graphqls` - GraphQL schema definition
- `mock_data.go` - Mock UNS data for testing/demo

## Start Server

### Recommended: Using Makefile (Docker)
```bash
cd umh-core
make help            # Show all available commands
make test-graphql    # Start GraphQL server with mock data
```

### Alternative: Direct Go execution
```bash
go run cmd/main.go --config config-graphql-demo.yaml
```

> **📍 Important:** The `make test-graphql` command must be run from the `umh-core/` directory, not the repository root.

## Access GraphQL

- **GraphiQL Playground**: http://localhost:8090/
- **GraphQL Endpoint**: http://localhost:8090/graphql

## Sample Queries

### List all topics
```graphql
query {
  topics {
    topic
    lastEvent {
      producedAt
      ... on TimeSeriesEvent {
        scalarType
        numericValue
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

### Filter topics by text
```graphql
query {
  topics(filter: { text: "temperature" }) {
    topic
    lastEvent {
      producedAt
      ... on TimeSeriesEvent {
        scalarType
        numericValue
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
  topic(topic: "..umh.v1.acme.cologne.packaging.station1.temperature") {
    topic
    lastEvent {
      producedAt
      ... on TimeSeriesEvent {
        scalarType
        numericValue
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

### Limit results
```graphql
query {
  topics(limit: 3) {
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

## Mock Data Included

The mock data includes 7 realistic UNS manufacturing topics:

- `..umh.v1.acme.cologne.packaging.station1.temperature` (23.9°C)
- `..umh.v1.acme.cologne.packaging.station1.pressure` (1.19 bar)  
- `..umh.v1.acme.cologne.packaging.station1.count` (1454 pieces)
- `..umh.v1.acme.cologne.assembly.robot1.cycle_time` (12.4 seconds)
- `..umh.v1.acme.cologne.quality.vision1.defect_rate` (0.6%)
- `..umh.v1.acme.cologne.energy.main.power` (146.9 kW)
- `..umh.v1.acme.cologne.maintenance.pump1.vibration` (2.2 mm/s)

Each topic includes realistic metadata:
- **Sensor IDs** (temp_001, press_001, etc.)
- **Physical locations** (Packaging Station 1, Assembly Line, etc.)
- **Alarm levels** (high/low thresholds, warning levels)
- **Target values** (daily counts, cycle times, performance targets) 