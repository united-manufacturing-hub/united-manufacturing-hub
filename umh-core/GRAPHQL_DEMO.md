# GraphQL API Demo

Quick setup to test the GraphQL API with mock data.

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
    latestEvent {
      timestamp
      value
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
    latestEvent {
      timestamp
      value
      unit
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
    latestEvent {
      timestamp
      value
      unit
      scalarType
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
    latestEvent {
      timestamp
      value
      unit
    }
  }
}
```

## Mock Data Included

The mock data includes realistic UNS topics:

- `umh.v1.acme.cologne.packaging.station1.temperature` (23.9°C)
- `umh.v1.acme.cologne.packaging.station1.pressure` (1.19 bar)  
- `umh.v1.acme.cologne.packaging.station1.count` (1454 pieces)
- `umh.v1.acme.cologne.assembly.robot1.cycle_time` (12.4 seconds)
- `umh.v1.acme.cologne.quality.vision1.defect_rate` (0.6%)
- `umh.v1.acme.cologne.energy.main.power` (146.9 kW)
- `umh.v1.acme.cologne.maintenance.pump1.vibration` (2.2 mm/s)

Each topic includes realistic metadata (sensor IDs, locations, alarm levels, etc.). 