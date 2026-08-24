# HTTP API Reference

{% hint style="info" %}
**Early Access.** The GraphQL API lets developers query the Unified Namespace programmatically. It's experimental and off by default. To turn it on, set `agent.graphql.enabled: true` in the `agent:` block of your `umh-core` instance configuration. See [Configuration](topic-browser-graphql.md#configuration) for all options. Tell us what you think.
{% endhint %}

## GraphQL API
- **Default:** Disabled; enable with `agent.graphql.enabled: true` (port 8090)
- **Endpoint:** `POST /graphql`
- **GraphiQL:** Available at `/` when debug enabled
- **Purpose:** Query Unified Namespace topics

## Topic Browser GraphQL
- [Complete GraphQL API Reference](topic-browser-graphql.md)
- [Schema Documentation](topic-browser-graphql.md#schema)
- [Query Examples](topic-browser-graphql.md#examples)

## Monitoring
- **Metrics:** See [Production Metrics](../../production/metrics.md)
- **Health:** Via FSM state monitoring 