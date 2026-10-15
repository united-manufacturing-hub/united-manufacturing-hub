# Agent

The agent is the `umh-core` process inside the UMH Core container. It starts, watches and restarts the components inside the container. These components are the bridges, the stand-alone flows, the stream processors and Redpanda. Two frameworks inside the agent do this work: FSMv1, the older one, and FSMv2, the newer one. See [Which framework runs a component](metrics.md#which-framework-runs-a-component).

## What the agent does

The agent works in a control loop. Every 100 milliseconds, one cycle of the loop compares each component with the [config file](../config-file.md). The agent then starts, stops or restarts the component until it matches the config file. A Kubernetes controller and the scan cycle of a PLC work the same way.

The agent also checks the health of each component:

- It reads the last 10 minutes of the log of each bridge, stand-alone flow and stream processor. An error in the log marks the component as degraded. So does a warning that contains `failed to`, `connection lost` or `unable to`. The error counters of Benthos, the program that runs these components, also mark a component as degraded.
- For a bridge, it checks whether the port of the device is reachable. While the port is not reachable, the bridge reports the connection as the reason it is degraded. The errors of the bridge's read and write flows are not reported then.
- When a component's process stops, s6 starts it again. s6 is the process supervisor inside the container.
- When a component stays stuck for about 60 seconds while the agent creates or removes it, the agent removes it and creates it again. A component that waits in a starting step, such as a bridge that waits for its connection, is not removed this way.

Each bridge, stand-alone flow and stream processor runs as its own process. A crash in one of them does not stop the others. They still share the CPU and memory of the container.

### When the agent process stops

Bridges, stand-alone flows, stream processors and Redpanda run as processes separate from the agent. They keep running while the agent process is down. s6 starts the agent again. When the agent starts again, it may restart them once.

### Health checks in Kubernetes

UMH Core has no health or readiness endpoint. Port 8080 serves `/metrics` and an internal debug page. For a liveness check, use the running state of the container. An answer from `/metrics` shows that the agent process runs. It does not show that the components are healthy.

## The Agent card

The Agent card on the instance details page shows the identity and the version of the instance.

| Field | Shows |
|---|---|
| Status badge | The health of the agent. Hover over it to see the reason. See [When the Agent shows Degraded](#when-the-agent-shows-degraded). |
| Name | The name of the instance. The **Rename** button changes it. |
| Location | The location path of the instance, one level per line. It is set when you create the instance and cannot be changed on the card. |
| Core Version | The version of UMH Core. Hover over it to see the versions of S6 Overlay, Benthos and Redpanda. A marker next to the version shows when the release channel has a newer version. |
| Release Channel | The release channel the instance follows, from `agent.releaseChannel` in the config file: `stable`, `nightly` or `enterprise`. It shows `n/a` when `agent.releaseChannel` is missing or has another value. |

The **Logs** button opens the logs of the agent. The agent writes them to `/data/logs/umh-core/`. See [Container Layout](../../../reference/container-layout.md#logs). The **Metrics** button is not available yet.

## When the Agent shows Degraded

The Agent shows Degraded while the emergency setting *Turn off bridge admission* is on. The badge reason then starts with `Bridge admission is off (agent.enableResourceLimitBlocking: false).` New bridges start without waiting for healthy CPU, memory and disk, and without the bridge limit.

To clear it, fix the resource problem first. Then set `agent.enableResourceLimitBlocking: true` in the config file. See [Emergency settings: Turn off bridge admission](../../data-flows/bridges-do-not-start.md#emergency-settings-turn-off-bridge-admission).

A degraded Agent also marks the whole instance as degraded. In the instance list, hover over the status dot of the instance to see the line `Agent: <reason>`.

## Metrics

umh-core exposes the metrics of the agent on its Prometheus endpoint, port 8080 at `/metrics`. [Read the metrics](metrics.md#read-the-metrics) explains how to reach the endpoint. The agent series come from FSMv1 and use the standard [FSMv1 metrics](metrics.md#fsmv1-metrics).

### Control loop and agent monitor

The `component` and `instance` labels select the control loop or the agent monitor. The agent monitor is the part of the agent that reports the health shown on the Agent card. In Prometheus, the `instance` label usually arrives as `exported_instance`. [Read the metrics](metrics.md#read-the-metrics) explains why.

| Name | Type | Reports |
|---|---|---|
| `umh_core_reconcile_duration_milliseconds{component="control_loop",instance="main"}` | histogram | Duration of one cycle of the control loop, in milliseconds. A cycle should finish within 100 ms. A longer cycle writes a warning to the agent log. |
| `umh_core_reconcile_starved_total_seconds` | counter | Grows while the control loop has not run for more than 15 seconds. During one stall, it adds more than one second for each second of the stall. Alert on any increase. |
| `umh_core_errors_total{component="control_loop",instance="main"}` | counter | Errors that stopped the control loop. |
| `umh_core_errors_total{component="control_loop",instance="config_permanent_failure"}` | counter | Times the agent gave up reading the config file after its retries. This also stops the control loop. |
| `umh_core_service_current_state{component="agent_monitor",instance="Core"}` | gauge | State of the agent monitor: 3 = active, 5 = degraded. -1 means the agent monitor is in a sub-state, such as its starting or stopping steps. See [State](metrics.md#state). |
| `umh_core_service_desired_state{component="agent_monitor",instance="Core"}` | gauge | State the agent monitor should reach, with the same values. |
| `umh_core_errors_total{component="agent_monitor",instance="Core"}` | counter | Errors while the agent monitor collected the status of the agent. |
| `umh_core_reconcile_duration_milliseconds{component="agent_monitor",instance="Core"}` | histogram | Duration of one update of the agent monitor, in milliseconds. |

Other monitors also write series with `component="agent_monitor"`, each with its own `instance` value. Filter by `instance="Core"` to get the agent alone.
