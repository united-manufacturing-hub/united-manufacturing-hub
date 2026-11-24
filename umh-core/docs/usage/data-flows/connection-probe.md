# Connection Probe

The connection probe is the part of a [bridge](bridges.md) that checks whether the external system is reachable.

Before umh-core starts a bridge, it checks that the external system answers on its host and port. This connection probe runs on FSMv2. The `NMAP_BACKEND` environment variable that once switched it to FSMv1 is ignored and can be removed.

When the external system stops answering, the bridge's own state changes. It stays in `starting_connection` if it has not started yet. It moves to `degraded_connection` if it was already running. To see whether the external system answers, read the bridge's state.

## Reference

### Configuration

The connection probe is the `connection` key of a bridge. The full bridge configuration is in [Bridges](bridges.md#configuration).

```yaml
connection:
  nmap:
    target: "{{ .IP }}"    # Uses IP variable
    port: "{{ .PORT }}"    # Uses PORT variable
```

| Field | Description |
|---|---|
| `protocolConverterServiceConfig.config.connection.nmap` | Connection probe: a TCP check of whether the external system is reachable. |

### Metrics

For a bridge named `mixing-station`, the connection's series carry these labels. [Bridges: Metrics](bridges.md#metrics) explains the labels of the other parts of the bridge.

| Part of the bridge | `component` | `instance` |
|---|---|---|
| Its connection | `connection_instance` | `protocolconverter-mixing-station` |

The connection reports errors and reconcile time, but no state.

The connection probe's series carry a `hierarchy_path` that ends in `connection-protocolconverter-<bridge>-001(nmap)`.
