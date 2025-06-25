# GraphQL Package

This package provides a GraphQL API for browsing UMH Core topic browser data.

## Features

- **Complete GraphQL server** with Gin framework integration
- **Interactive GraphiQL playground** for development and testing
- **Real-time data** from built-in simulator with UNS manufacturing topics
- **CORS support** for web development
- **Filtering and pagination** for topic queries
- **Rich metadata** including units, sensor IDs, locations, and alarm levels
- **Proper UNS structure** following topic convention with data contracts

## Architecture

- `server.go` - GraphQL server setup and configuration
- `resolver.go` - GraphQL query resolvers
- `models.go` - GraphQL type definitions  
- `schema.graphqls` - GraphQL schema definition

## Data Source

The GraphQL API uses real data from the built-in simulator that:
- **Generates UNS-compliant topics** every second with proper topic structure
- **Provides realistic sensor data** (temperature, pressure, etc.) with continuous updates
- **Automatically populates** the topic browser cache used by GraphQL
- **Uses proper xxhash-based UNS tree IDs** matching production behavior

## Start Server

### Recommended: Using Makefile (Docker)
```bash
cd umh-core
make help            # Show all available commands
make test-graphql    # Start GraphQL server with simulator data
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
  topics(filter: { text: "topic1" }) {
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
  topic(topic: "uns.topic1") {
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

## Simulator Data

The built-in simulator provides 2 UNS manufacturing topics with realistic data:

- `uns.topic1` - Corporate A, Plant 1, Line 4, Pump 41 (random numeric values)
- `uns.topic2` - Corporate A, Plant 1, Line 4, Pump 42 (random numeric values)

### Topic Structure

Topics follow the UNS convention: `<enterprise>.<location_path>.<data_contract>[.<virtual_path>].<tag_name>`

- **Enterprise**: `corpA`
- **Location Path**: `plant-1.line-4.pump-41`, `plant-1.line-4.pump-42`
- **Data Contract**: `uns.topic1`, `uns.topic2`

### Real-Time Updates

The simulator:
- **Updates every second** with new random values
- **Maintains topic structure** with proper UNS tree IDs
- **Provides continuous data** for testing GraphQL queries
- **Uses realistic manufacturing context** (corporate facilities, production lines)

### UNS Tree ID Calculation

The simulator uses proper xxhash calculation for UNS tree IDs, computed from topic elements separated by null delimiters, matching production behavior. 