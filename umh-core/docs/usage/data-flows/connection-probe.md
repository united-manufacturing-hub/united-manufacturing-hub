# Connection Probe

The connection probe is the part of a [bridge](bridges.md) that checks whether the external system is reachable. Every second, it opens a TCP connection to the external system and closes it again.

A bridge's flows start only once the connection is up. A running flow keeps running while it is still connected to the external system, even when the connection is down.

## What the Management Console shows

The bridge's overview page shows the connection as its own card, next to the read flow and the write flow. The connection has one of these states:

| State | Meaning |
|---|---|
| `up` | The external system accepts connections on its port. |
| `down` | The external system does not accept connections on its port. |
| `degraded` | The probe has stopped returning results, or its result changed twice within a short time. |
| `starting` | The probe has no result yet. |
| `stopped` | The bridge is stopped. |

When the last probe failed, the connection's status message names the reason. For example, the external system refused the connection on its port, or the host name could not be found. The bridge's status adds the state of the connection, such as "connection is down".

## How the connection affects the bridge

A bridge starts in this order: first the connection, then Redpanda, then the bridge's flows. While the connection is not `up`, the bridge stays in the state `starting_connection`, and neither flow starts. If the connection drops during a later step, the bridge returns to `starting_connection`.

When the connection of a running bridge leaves `up`, umh-core checks whether a flow is still connected to the external system. If it is, the bridge moves to `degraded_other`, its status says that the connection is down but the flow is still connected, and the flows keep running. Once no flow is connected anymore, the bridge moves to `degraded_connection`. When the connection is `up` again, the bridge returns to `idle` or `active`. The bridge state shows the effect of the connection. `config.yaml` does not change, and the desired state of each flow stays as you set it.

A connection that is `up` means only that the port accepts TCP connections. It does not mean that the read flow can read. Wrong credentials, a wrong rack and slot, or a wrong OPC UA endpoint path show up in the read flow, not in the connection. An external system that accepts no TCP connections on the configured port never reaches `up`.

The connection probe runs on FSMv2. The `NMAP_BACKEND` environment variable that once switched it to FSMv1 is ignored and can be removed.

## Reference

### Configuration

The connection probe is the `connection.nmap` key of a bridge. It sits in `protocolConverterServiceConfig.config`, or in the bridge's template for a bridge with `templateRef`. The full bridge configuration is in [Bridges](bridges.md#configuration).

```yaml
connection:
  nmap:
    target: "{{ .IP }}"
    port: "{{ .PORT }}"
```

| Key | Default | Description |
|---|---|---|
| `connection.nmap.target` | none, required | The host name or IP address of the external system. The Management Console fills it from the **IP Address** field and stores the value as the variable `IP`. |
| `connection.nmap.port` | none, required | The TCP port of the external system, a number from 0 to 65535. The Management Console fills it from the **Port** field and stores the value as the variable `PORT`. |

A bridge that writes to the [Historian](../historian/README.md) has no **IP Address** and **Port** fields. Its probe checks the TimescaleDB host and port of the Historian, `{{ .historian.timescale.host }}` and `{{ .historian.timescale.port }}`.

Changes take effect without a restart of the container. The probe checks the new host and port from the next second on.

### Metrics

No series on umh-core's Prometheus endpoint tells whether the external system is reachable. To alert on an unreachable external system, alert on the bridge. A bridge that waits for its connection, or whose connection is degraded, reads -1, so the rule in the [Bridges metrics section](bridges.md#metrics) fires for it.

The connection probe reports the [standard FSMv2 series](../instances/agent/metrics.md#fsmv2-metrics). Its state there is `running` while it probes, whether the port answers or not. It is `degraded` before its first result and while a probe cannot run, and `stopped` when the bridge is stopped. For a bridge named `mixing-station`, the series carry a `hierarchy_path` that ends in `connection-protocolconverter-mixing-station-001(nmap)`.

umh-core does not export the probe's result, its latency or the reason a probe failed as metrics. The connection's status message in the Management Console shows the reason.

The connection itself runs on FSMv1. Its series carry `component="connection_instance"` and, in Prometheus, `exported_instance="protocolconverter-mixing-station"`. It reports errors and reconcile time, but no state.

[Read the metrics](../instances/agent/metrics.md#read-the-metrics) explains the label names in Prometheus. The probe is defined in [`pkg/fsmv2/nmap`](https://github.com/united-manufacturing-hub/united-manufacturing-hub/tree/staging/umh-core/pkg/fsmv2/nmap), and the standard series in [`fsmv2/supervisor/metrics/metrics.go`](https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/staging/umh-core/fsmv2/supervisor/metrics/metrics.go).
