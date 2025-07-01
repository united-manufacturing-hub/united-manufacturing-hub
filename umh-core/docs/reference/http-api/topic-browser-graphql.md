# Topic Browser GraphQL API

## Overview
Query Unified Namespace topics and metadata in real-time.

- **Endpoint:** `http://localhost:8090/graphql`
- **Default:** Enabled (`graphql.enabled: true`)
- **Authentication:** None (open by default)

## Schema

### Queries
```graphql
type Query {
  topics(filter: TopicFilter, limit: Int): [Topic!]!
  topic(topic: String!): Topic
}
```

### Types
```graphql
type Topic {
  topic: String!
  metadata: [MetadataEntry!]!
  lastEvent: Event    # Latest event only
}

union Event = TimeSeriesEvent | RelationalEvent

type TimeSeriesEvent {
  producedAt: String!
  scalarType: String!
  numericValue: Float
  stringValue: String
  booleanValue: Boolean
}

type RelationalEvent {
  producedAt: String!
  json: String!
}

input TopicFilter {
  text: String
  meta: [MetaExpr!]
}

input MetaExpr {
  key: String!
  eq: String!
}
```

## Example Queries

### All topics:
```graphql
{ topics { topic metadata { key value } } }
```

### Filter by text:
```graphql
{ topics(filter: { text: "temperature" }) { topic } }
```

### Filter by metadata:
```graphql
{
  topics(filter: { 
    meta: [{ key: "data_contract", eq: "_pump" }] 
  }) {
    topic
    lastEvent {
      ... on TimeSeriesEvent {
        producedAt
        numericValue
      }
    }
  }
}
```

### Single topic:
```graphql
{
  topic(topic: "umh.v1.acme.plant1.line4.sensor1._raw.temperature") {
    metadata { key value }
    lastEvent {
      ... on TimeSeriesEvent {
        producedAt
        numericValue
        scalarType
      }
    }
  }
}
```

## Using curl

### Basic query:
```bash
curl -X POST http://localhost:8090/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ topics(limit: 3) { topic } }"}'
```

### Filter query:
```bash
curl -X POST http://localhost:8090/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ topics(filter: { text: \"pump\" }) { topic lastEvent { ... on TimeSeriesEvent { numericValue } } } }"}'
```

## Limitations
- **History:** Latest event only (no historical data)
- **Subscriptions:** Not supported (queries only)
- **Rate Limiting:** None (use responsibly)
- **Scope:** Single UMH instance only

## Multi-Instance Behavior
- Each UMH Core instance has its own Topic Browser
- GraphQL API returns topics from local instance only
- Management Console aggregates all instances into unified view

## Configuration
```yaml
agent:
  graphql:
    enabled: true    # Default: true
    port: 8090      # Default: 8090
    debug: false    # Set true for GraphiQL UI
    corsOrigins: [] # Default: allows all origins
```

## Security
- **Authentication:** Currently open (no tokens required)
- **CORS:** Configurable via `corsOrigins` setting
- **Network Security:** Recommended for production deployments
- **Port Access:** Ensure port 8090 is appropriately secured 