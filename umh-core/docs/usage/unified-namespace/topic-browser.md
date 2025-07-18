# Topic Browser

Real-time exploration of Unified Namespace. There are two ways to access the Topic Browser:
- Management Console (recommended, access via management.umh.app)
- GraphQL API (for programmatic access, prototype)

## Management Console

The Topic Browser in the Management Console provides a comprehensive, real-time view of your Unified Namespace (UNS). It lets you explore all your data **topics/tags** in a hierarchical tree, aggregated from all connected UMH instances, without having to worry about underlying technical details like MQTT topics, Kafka keys, or payload formats. Each **tag** typically represents a data point (e.g. a sensor reading or machine status) in your ISA-95 asset hierarchy. The Topic Browser is the primary tool for navigating this structure, monitoring live values, and inspecting metadata and history for each topic.

## Accessing the Topic Browser

To open the Topic Browser, log in to the Management Console and select **Topic Browser** from the navigation menu (without the `(Classic)`). The interface is split into two main areas: a **topic tree** on the left and a **details panel** on the right. The topic tree displays the hierarchical structure of all your topics, while the details panel shows information about any topic you select. Once opened, the Topic Browser will begin loading the UNS topic structure and live data from all your connected instances automatically.

## Browsing the Hierarchical Topic Tree

On the left, you will see a collapsible tree representing the **hierarchy of topics** in your Unified Namespace. This hierarchy follows the ISA-95 style structure (Enterprise → Site → Area → Line/Cell, etc., down to individual tag names), organizing data in logical levels. You can expand **folder nodes** (groupings like enterprises, sites, or equipment groups) to reveal deeper levels, and ultimately the **leaf nodes** which represent individual data tags.

**Data Aggregation:** The Topic Browser automatically consolidates and merges topics from **all** your UMH instances into this single tree view. This means if you have multiple instances or brokers publishing data, their topics all appear in one unified namespace hierarchy. (If the same topic exists on multiple instances, it will be combined under one entry with an indication of the multiple data sources.)

**Hierarchical Structure:** The tree clearly shows how each tag is categorized by location and context. In other words, you can easily see **which part of the namespace a tag belongs to** – for example, which enterprise and site a sensor value is associated with. Folder/group nodes make it simple to navigate through logical groupings of tags (for example, all tags under a certain machine or production line). You can collapse sections that you're not interested in and expand the ones you want to explore in depth.

Importantly, this topic list is **live** – if a new device comes online and publishes a new tag, you'll see a new node appear in the tree in real time at the appropriate position. The tree auto-refreshes as data changes, so you're always looking at an up-to-date snapshot of your unified namespace.

## Filtering and Searching Topics

When dealing with a large number of topics, the Topic Browser offers a **filter/search** function to quickly find what you need. At the top of the topic tree panel, there is a search bar where you can type a keyword or partial topic name. As you type, the topic tree filters to show only the branches and tags that match your input. This is extremely useful for pinpointing a specific tag without manually expanding every level of the hierarchy. For example, you can type a machine name or tag name (such as "temperature") to instantly highlight and navigate to relevant topics. The filtering applies across the full topic path and even tag metadata, making it easy to locate data points in a complex namespace.

## Real-Time Updates and Auto-Refresh

One of the key benefits of the Topic Browser is that it updates **automatically in real time**. You do not need to refresh the page to get new data – the Management Console continuously streams updates from the UMH backend. Whenever a tag's value changes or a new topic is created, the interface will reflect those changes within seconds. You can literally watch sensor readings or other data points change live on the screen. The Topic Browser ensures you are always looking at current data. (Behind the scenes it processes incoming events in near real-time, so extremely rapid updates may be batched into short intervals, but generally it keeps the view up-to-date continuously.) This live view makes it easy to monitor system behavior and verify that data is flowing as expected.

## Viewing Topic Details and Data Inspection

When you click on a specific topic (a leaf node in the tree), the right-hand **details panel** displays comprehensive information about that topic organized into several sections:

### Topic Details Section
At the top, you'll see core information about the selected topic:
- **Physical Path**: The location hierarchy (e.g., `enterprise-3.site-3.line-1`)
- **Data Contract**: The schema type (e.g., `_historian`, `_raw`, `_oee`)  
- **Virtual Path**: Additional path information if applicable
- **Tag Name**: The specific tag identifier (e.g., `temperature`, `pressure`)

### Last Message Section
This shows the most recent data point:
- **Payload Type**: The [message format](./payload-formats.md) (`Time-Series`, `Relational`, etc.)
- **Timestamp**: The timestamp inside of the message. Not to be confused with the **Produced At** timestamp.   
- **Data Type**: The value type (`string`, `number`, `boolean`)
- **Value**: The actual current value
- **Produced At**: The timestamp of when the message arrived in the UNS. Not to be confused with the **Timestamp** timestamp.

For **Time-Series** payloads, single values are displayed directly. For array data, you'll see the full array of values (e.g., `[10.123, 2.321, 3.313, 4.543, 5.123, 6.12, 7.423, 8.32, 9.12, 10.64]`).

For **Relational** payloads, structured JSON data is displayed in a formatted view, showing complex objects with multiple fields like:
```json
{
  "timestamp_ms": 123123,
  "value1": 123,
  "value2": 234
}
```

### Metadata Section  
Displays message metadata and context information

### History Section
Shows recent data trends and patterns:
- **Time-Series Data**: Displays an interactive line chart showing value changes over time, with timestamps on the x-axis and values on the y-axis
- **Boolean/String Data**: Shows a table with timestamps and true/false values
- Tracks the **last 100 values** for topics that publish frequent updates

## GraphQL API

For programmatic access, the Topic Browser provides a GraphQL API that allows you to query topics, filter by metadata, and retrieve the latest message data. This is ideal for automation, integrations, and custom applications that need to access UNS data programmatically.

### Accessing the GraphQL API

The GraphQL endpoint is available at `http://localhost:8090/graphql` by default. Unlike the Management Console UI which aggregates data from multiple UMH instances, the GraphQL API provides access to topics from a single UMH instance.

### Querying Topics and Metadata

The API supports powerful filtering capabilities that allow you to:
- **Text Search**: Find topics by name or metadata content
- **Metadata Filtering**: Filter topics based on specific metadata key-value pairs
- **Combined Filtering**: Use both text and metadata filters together
- **Latest Events**: Retrieve the most recent message data for each topic

### Example Query

Here's a comprehensive example showing how to filter topics by text and metadata, then retrieve the latest message data:

```graphql
{
  topics(filter: { 
    text: "temperature",
    meta: [{ key: "data_contract", eq: "_historian" }]
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
      ... on RelationalEvent {
        producedAt
        json
      }
    }
  }
}
```

This query finds all topics containing "temperature" that use the `_historian` data contract, returns their metadata, and shows the latest message data with timestamps and values.

### Configuration

The GraphQL API runs by default alongside the Topic Browser:

```yaml
internal:
  topicBrowser:
    desiredState: "active"  # Default

agent:
  graphql:
    enabled: true    # Default
    port: 8090      # Default
    debug: false    # Set true for GraphiQL UI
```

For detailed API documentation and additional query examples, see [GraphQL API Reference](../../reference/http-api/topic-browser-graphql.md). 