# HTTP API Reference

{% hint style="info" %}
**Early Access.** We built the GraphQL API for developers who want to query the Unified Namespace programmatically. It's experimental, so it's off by default. Switch it on with `graphql.enabled: true` and give it a try. Tell us what you think.
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