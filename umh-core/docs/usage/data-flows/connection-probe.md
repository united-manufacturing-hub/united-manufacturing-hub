# Connection Probe

The connection probe is the part of a [bridge](bridges.md) that checks whether the external system is reachable. Every second, it opens a TCP connection to the external system and closes it again. The Management Console shows its result as the bridge's connection.

A bridge's flows start only once the connection is up. [How the connection affects the bridge](#how-the-connection-affects-the-bridge) describes what happens when it goes down.

## What the Management Console shows

The bridge's overview page shows the connection as its own card, next to the read flow and the write flow. The card shows a badge. The icon next to the badge shows the connection's state, or, when the last probe failed, the reason. For example, the external system refused the connection on its port, or the host name could not be found.

| State | Badge | Meaning |
|---|---|---|
| `up` | Active | The external system accepts connections on its port. |
| `down` | Degraded | The external system does not accept connections on its port. |
| `degraded` | Degraded | The probe has stopped returning results, or its result changed back and forth within the last few checks. |
| `starting` | Neutral | The probe has no result yet. |
| `stopped` | Degraded | The bridge is stopped. |

## How the connection affects the bridge

A bridge starts in this order: first the connection, then Redpanda, then the bridge's flows. While the connection is not `up`, the bridge stays in the state `starting_connection`, and neither flow starts. If the connection drops during a later step, the bridge returns to `starting_connection`.

When the connection of a running bridge leaves `up`, umh-core checks whether a flow's own connection to the external system is still open. If one is, the bridge moves to `degraded_other`, and the flows keep running. Its status says that the connection is down but the flow is still connected.

Once no flow has an open connection to the external system, the bridge moves to `degraded_connection`, and umh-core stops both flows. When the connection is `up` again, umh-core starts the flows. The bridge shows `degraded_dfc` while they start, then `idle`, and `active` once data moves. `config.yaml` does not change.

A connection that is `up` means only that the port accepts TCP connections. It does not mean that the read flow can read. Wrong credentials, a wrong rack and slot, or a wrong OPC UA endpoint path show up in the read flow, not in the connection. An external system that accepts no TCP connections on the configured port never stays `up`.

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
| `connection.nmap.target` | none | The host name or IP address of the external system. The Management Console requires it, fills it from the **IP Address** field and stores the value as the variable `IP`. |
| `connection.nmap.port` | none, required | The TCP port of the external system, a number from 1 to 65535. The Management Console fills it from the **Port** field and stores the value as the variable `PORT`. |

A bridge that writes to the [Historian](../historian/README.md) uses the same `connection.nmap` key. The Management Console sets `target` to `{{ .historian.timescale.host }}` and `port` to `{{ .historian.timescale.port }}`, the TimescaleDB host and port of the Historian. It shows them as read-only **IP Address** and **Port** fields.

Changes to `connection.nmap` take effect without a restart of the container.

### Metrics

To alert on an unreachable external system, alert on the bridge, as described in the [Bridges metrics section](bridges.md#metrics). While the bridge waits for its connection, or its connection is degraded, the bridge's state reads -1. The rule also fires for a bridge that is degraded for other reasons, such as a failed flow. The connection card in the Management Console shows whether the probe failed, and why.

[Read the metrics](../instances/agent/metrics.md#read-the-metrics) explains the state values and the label names. The probe is defined in [`pkg/fsmv2/nmap`](https://github.com/united-manufacturing-hub/united-manufacturing-hub/tree/staging/umh-core/pkg/fsmv2/nmap).
