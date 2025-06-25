# GraphQL Package

This package provides a GraphQL API for browsing UMH Core topic browser data.

## Features

- **Complete GraphQL server** with Gin framework integration
- **Interactive GraphiQL playground** for development and testing
- **Mock data mode** with 7 realistic UNS manufacturing topics
- **CORS support** for web development
- **Filtering and pagination** for topic queries
- **Rich metadata** including units, sensor IDs, locations, and alarm levels
- **Proper UNS structure** following topic convention with data contracts

## Architecture

- `server.go` - GraphQL server setup and configuration
- `resolver.go` - GraphQL query resolvers
- `models.go` - GraphQL type definitions  
- `schema.graphqls` - GraphQL schema definition
- `mock_data.go` - Mock UNS data for testing/demo with xxhash UNS tree IDs

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
  topic(topic: "acme.cologne.packaging.station1._historian.temperature") {
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

The mock data includes 7 realistic UNS manufacturing topics following proper UNS convention with data contracts:

- `acme.cologne.packaging.station1._historian.temperature` (unit: °C, 23.9°C)
- `acme.cologne.packaging.station1._historian.pressure` (unit: bar, 1.19 bar)  
- `acme.cologne.packaging.station1._historian.count` (unit: pieces, 1454 pieces)
- `acme.cologne.assembly.robot1._historian.cycle_time` (unit: seconds, 12.4 seconds)
- `acme.cologne.quality.vision1._historian.defect_rate` (unit: %, 0.6%)
- `acme.cologne.energy._historian.power` (unit: kW, 146.9 kW)
- `acme.cologne.maintenance.pump1._historian.diagnostics.vibration` (unit: mm/s, 2.2 mm/s)

### Topic Structure

Topics follow the UNS convention: `<enterprise>.<location_path>.<data_contract>[.<virtual_path>].<tag_name>`

- **Enterprise**: `acme`
- **Location Path**: `cologne.packaging.station1`, `cologne.assembly.robot1`, etc.
- **Data Contract**: `_historian` (for time-series data)
- **Virtual Path**: `diagnostics` (optional, used for vibration topic)
- **Tag Name**: `temperature`, `pressure`, `count`, etc.

### Metadata Structure

Each topic includes comprehensive metadata:
- **Unit**: Physical unit of measurement (°C, bar, pieces, etc.)
- **Sensor IDs**: Unique identifiers (temp_001, press_001, etc.)
- **Physical Locations**: Human-readable locations (Packaging Station 1, Assembly Line, etc.)
- **Alarm Levels**: Operational thresholds (high/low limits, warning levels)
- **Target Values**: Performance targets (daily counts, cycle times, etc.)

### UNS Tree ID Calculation

The mock data uses proper xxhash calculation for UNS tree IDs, computed from topic elements separated by null delimiters, matching production behavior. 