# Topic Browser

Real-time exploration of your Unified Namespace data via Management Console or GraphQL API.

## Management Console

Access via `management.umh.app` → **Topic Browser** (not the Classic version).

### Interface Overview
- **Left panel**: Hierarchical topic tree following ISA-95 structure
- **Right panel**: Details for selected topic (metadata, values, history)
- **Auto-aggregation**: Combines data from all your UMH instances
- **Live updates**: Real-time data without manual refresh

### Key Features

**Topic Tree Navigation:**
- Expand/collapse hierarchy levels (Enterprise → Site → Area → Line → Equipment)
- See data contracts (`_raw`, `_pump_v1`) and virtual paths
- Search bar for filtering by topic name or metadata

**Topic Details Panel:**
When selecting a topic, view:
- **Location**: Physical hierarchy path
- **Data Contract**: Applied schema (`_raw`, `_pump_v1`, etc.)
- **Current Value**: Latest data with timestamp
- **Metadata**: Including original device tags (see [Metadata and Tracing](metadata-and-tracing.md))
- **History**: Last 100 values as chart (time-series) or table (boolean/string)

![Topic Browser showing DB1.DW20](../../getting-started/images/2-topic-browser-DB1.DW20.png)

## GraphQL API

Programmatic access at `http://localhost:8090/graphql` (per instance).

{% hint style="info" %}
**Early Access.** We built the GraphQL API for developers who want to query the Unified Namespace programmatically. It's experimental, so it's off by default. See the instructions below and give it a try. Tell us what you think.
{% endhint %}

### Configuration

```yaml
internal:
  topicbrowser:
    desiredState: "active"  # Enable/disable

agent:
  graphql:
    enabled: true           # Enable GraphQL API (required!)
    port: 8090             # API port
    debug: false           # Set true for GraphiQL UI
```

### Basic Query
```graphql
{
  topics(filter: {
    text: "temperature",
    meta: [{ key: "data_contract", eq: "_pump_v1" }]
  }) {
    topic
    metadata {
      key
      value
    }
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

### cURL Example
```bash
curl -X POST http://localhost:8090/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ topics(limit: 10) { topic } }"}'
```

## Common Use Cases

### Finding Specific Data
1. Use search bar for partial matches (e.g., "temp" finds all temperature topics)
2. Filter by metadata like data contract or location
3. Check metadata section for original device tags

### Verifying Data Flow
1. Select topic to see current value
2. Check "Produced At" timestamp for freshness
3. Review history chart for patterns
4. Inspect metadata for source bridge

### Debugging Issues
1. Verify topic exists in tree
2. Check metadata for `bridged_by` to identify source
3. Look for original tag names (e.g., `opcua_tag_name`, `s7_address`)
4. Compare timestamps to detect delays

## Performance Notes

- Topic Browser stores last 100 values per topic
- Extremely rapid updates are batched for display
- Large namespaces (>10,000 topics) may load progressively
- Each UMH instance has its own Topic Browser service

## Metrics

The **Topic Browser** card on the instance details page shows one value. **Topics** is the number of different topics the instance has seen since umh-core started. A topic stays in the count after it stops receiving messages. The count starts again from zero when umh-core restarts. The **Metrics** button on the card opens the counters of the Benthos process that reads the messages in the Kafka topic `umh.messages` from [Redpanda](redpanda.md).

The badge on the card shows Active while that process runs. Check the badge tooltip for failures. The tooltip shows `benthos degraded` when the Benthos process fails, and `redpanda degraded` when Redpanda is degraded. For a Redpanda failure, the Redpanda card shows the cause. See [When Redpanda is degraded](redpanda.md#when-redpanda-is-degraded).

On port 8080, the Topic Browser runs on FSMv1 and uses the standard [FSMv1 metrics](../instances/agent/metrics.md#fsmv1-metrics). [Metrics](../instances/agent/metrics.md) explains how to read them. The Topic Browser has `component="topic_browser_instance"` with `instance="topic-browser"`, and its Benthos process has `component="benthos_instance"` with `instance="topicbrowser-topic-browser"`. In Prometheus, write `exported_instance` for `instance`.

To alert when the Benthos process of the Topic Browser is degraded, use:

```promql
umh_core_service_current_state{component="benthos_instance", exported_instance="topicbrowser-topic-browser"} == 5
```

The definitions are in [`pkg/metrics/metrics.go`](https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/staging/umh-core/pkg/metrics/metrics.go).

## Next Steps

- [**Metadata and Tracing**](metadata-and-tracing.md) - Understanding metadata fields