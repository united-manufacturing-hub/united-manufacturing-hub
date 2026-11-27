# Metrics

This page explains how to read the Prometheus metrics that a UMH Core instance serves on port 8080, and how umh-core names them. The metrics come from umh-core, the process inside the UMH Core container. The steps need access to the Docker host.

## Read the metrics

Every instance serves its metrics on port 8080, at the path `/metrics`, in the Prometheus text format. The endpoint runs inside the container, so you can read it when the Management Console cannot reach the instance. To read the metrics once, run `wget -qO- -Y off http://localhost:8080/metrics` inside the container, the way your [deployment](../../../production/deployment/README.md) runs a command in a container. The `-Y off` keeps `wget` from sending the request through the proxy of the container. With Docker:

```bash
docker exec <container name> wget -qO- -Y off http://localhost:8080/metrics
```

The install command from the Management Console names the container `umh-core` and does not publish port 8080. For Prometheus to scrape the instance, remove the container and start it again with the same `docker run` command plus `-p <host-ip>:8080:8080`. `docker restart` keeps the old port mappings. The endpoint has no authentication, so give `<host-ip>` an address on a trusted network, for example `127.0.0.1` when Prometheus runs directly on the same host. When Prometheus runs in a container on a Docker bridge network, `127.0.0.1` is that container itself, so give an address of the host that the Prometheus container can reach. Without it, Docker publishes the port on every interface of the host.

The port inside the container is always 8080, because umh-core sets `agent.metricsPort` in `config.yaml` back to 8080 on every start. To use another port on the host, change the host side of the mapping, for example `-p <host-ip>:9100:8080`.

Prometheus renames umh-core's `instance` label to `exported_instance`, unless the scrape job sets `honor_labels: true`. With the default setting, write `exported_instance="Core"` in Prometheus queries where the component pages write `instance="Core"`. With `honor_labels: true`, keep `instance`.

Most series appear only after umh-core records their first value. The FSMv2 counters, for example, appear after their first count. A missing series therefore does not mean the component is missing. `umh_core_errors_total` is the exception. It exists at 0 from the moment umh-core creates the component.

## Which framework runs a component

The agent is the umh-core process. A framework is the part of the agent that starts a component, watches it and restarts it when it fails. umh-core has two frameworks. FSMv1 is the older one, and FSMv2 is the newer one. Each framework names its metrics differently. To find the metrics of a component, first find its framework in this table.

| Component | Framework | Switch |
|---|---|---|
| Communicator | FSMv2 | none |
| Persistence and configuration components (internal parts of umh-core) | FSMv2 | none |
| Historian (the connection to TimescaleDB) | FSMv2 | none |
| Bridge connection probe | FSMv2 | none. `NMAP_BACKEND` is ignored |
| CPU health | FSMv2 | none. `USE_FSMV2_CPU` is ignored from v0.45.4 |
| Memory health (early access) | FSMv2 | off unless `USE_FSMV2_MEMORY_MONITOR=true`. See [Container: Configuration](../container/README.md#configuration) |
| Container memory and disk | FSMv1 | none |
| Bridges, stand-alone flows, stream processors | FSMv1 | none |
| Reading the Benthos metrics of bridges, stand-alone flows and stream processors | FSMv1, or FSMv2 with `USE_FSMV2_BENTHOS_METRICS=true` | `USE_FSMV2_BENTHOS_METRICS=true` switches the reading to FSMv2. See [Environment Variables](../../../reference/environment-variables.md) |
| Topic Browser, Redpanda, agent monitor | FSMv1 | none |

A switch is an environment variable of the container. umh-core reads it once at startup. To change it, remove the container and start it again with the same `docker run` command plus the variable. `docker restart` keeps the old environment. See [Environment Variables](../../../reference/environment-variables.md).

## FSMv2 metrics

Components that run under FSMv2 report two kinds of series:

- Standard series, named `umh_fsmv2_<name>`. Every FSMv2 component can report them.
- Component series, named `umh_fsmv2_worker_<name>`. Only the component that records the value reports them. The component pages explain them.

Both kinds carry the label `hierarchy_path`. It names the component that reports the series. FSMv2 components form a tree. A component can start child components, and each child can start its own children. The path lists every component from the top of the tree down to the one that reports, separated by `/`. Each step is written as `<id>(<type>)`. For example, the part of the communicator that sends messages to the Management Console has this path:

```
application-fsmv2(application)/communicator-001(communicator)/transport-001(transport)/push-001(push)
```

To select one component, match the end of the path with a regular expression. This query returns the time the communicator has spent in its current state:

```promql
umh_fsmv2_state_duration_seconds{hierarchy_path=~".*\\(communicator\\)"}
```

Prometheus matches a regular expression against the whole label value. This expression therefore selects the communicator and none of its children.

Not every component reports every standard series. A simple component such as the bridge connection probe or the historian runs no actions. It reports no `action_*` and no `stuck_action_*` series. A component without children never reports `umh_fsmv2_circuit_open`.

### State and time in state

Each FSMv2 component is always in one state, such as `Syncing` or `degraded`. The state names belong to the component and are listed on its page.

| Name | Type | Reports |
|---|---|---|
| `umh_fsmv2_state_duration_seconds` | gauge | Seconds the component has spent in its current state. Label `state` holds the state. There is one series per component, and it changes its `state` label on every state change. |
| `umh_fsmv2_state_transitions_total` | counter | State changes, with labels `from_state` and `to_state`. A fast rise of the same pair means the component switches back and forth between two states. |

To alert on a component that stays in a bad state, compare `umh_fsmv2_state_duration_seconds` for that state with a threshold.

### Action errors

An action is one task a component runs in the background, such as sending a batch of messages. The default timeout of an action is 30 seconds. The label `action_type` holds the name of the action. The two `stuck_action_*` counters use the label `action_name` instead.

| Name | Type | Reports |
|---|---|---|
| `umh_fsmv2_action_execution_duration_seconds` | histogram | Duration of each action, in seconds. Label `status` is `success`, `error`, `timeout` or `panic`. |
| `umh_fsmv2_action_timeout_total` | counter | Actions that ran longer than their timeout. |
| `umh_fsmv2_stuck_action_detected_total` | counter | Actions still running after twice their timeout. |
| `umh_fsmv2_stuck_action_force_removed_total` | counter | Actions umh-core stopped tracking after three times their timeout. |
| `umh_fsmv2_stuck_action_leaked_goroutines` | gauge | Estimated number of background tasks (goroutines) that stopped actions left behind. It only rises. A restart resets it. |

The histogram counts failed and timed-out actions by `status`. To get the error rate of the actions, use `rate(umh_fsmv2_action_execution_duration_seconds_count{status!="success"}[5m])`. An action that umh-core stopped tracking after three times its timeout never reaches the histogram. `umh_fsmv2_stuck_action_force_removed_total` counts those.

### Reconcile duration

FSMv2 runs one cycle of its control loop every 100 milliseconds. A reconcile is one step of each cycle. In that step, umh-core compares a component's child components with the configuration and adds, updates or removes children.

| Name | Type | Reports |
|---|---|---|
| `umh_fsmv2_reconciliation_duration_seconds` | histogram | Duration of one reconcile, in seconds. It does not include the rest of the cycle, such as the actions and the cycles of the children. |
| `umh_fsmv2_reconciliation_total` | counter | Reconciles, by label `result`. A cycle that fails before the reconcile records nothing, and the rate drops. Use the rate to confirm that reconciles run. |

### Circuit breaker and panics

A panic is a crash inside umh-core that umh-core caught and recovered from. When a component stops after repeated panics, every component above it pauses. This pause is called an open circuit.

| Name | Type | Reports |
|---|---|---|
| `umh_fsmv2_circuit_open` | gauge | 1 on a component and on every component above it, up to the top of the tree, while a component below has stopped after repeated panics. The component that panicked shows up in `umh_fsmv2_panic_recovery_total`. The series appears the first time the circuit opens. |
| `umh_fsmv2_panic_recovery_total` | counter | Panics, by component. Label `panic_type` is `error_panic`, `string_panic` or `unknown_panic`. A panic inside an action is counted as `status="panic"` on `umh_fsmv2_action_execution_duration_seconds` instead. |

Any increase of `umh_fsmv2_panic_recovery_total` is a defect in umh-core. Report it through [Get Help](../../../get-help.md).

### Configuration errors

| Name | Type | Reports |
|---|---|---|
| `umh_fsmv2_template_rendering_errors_total` | counter | Failed attempts to build the component's settings from the configuration. umh-core retries in every cycle. A broken configuration raises it about 10 times per second. |

## FSMv1 metrics

Components that run under FSMv1 report series named `umh_core_<name>`. They carry two labels:

- `component` names the kind of component, such as `protocol_converter_instance` for a bridge.
- `instance` names the component. For a bridge or a stream processor, this is the name you gave it. Prometheus renames this label, see [Read the metrics](#read-the-metrics).

| `component` value | Component |
|---|---|
| `protocol_converter_instance` | Bridge |
| `dataflow_component_instance` | Stand-alone flow |
| `stream_processor_instance` | Stream processor |
| `topic_browser_instance` | Topic Browser |
| `redpanda_instance` | Redpanda |
| `container_monitor` | Container |
| `agent_monitor` | Agent |

Other `component` values belong to internal layers of umh-core.

Each bridge, stand-alone flow and stream processor runs in Benthos, the engine that moves the data. A bridge runs two flows, a read flow and a write flow. A stand-alone flow and a stream processor run one each. By default, umh-core starts a separate monitoring service for each running flow to read the flow's Benthos metrics. Each of these services reports its own errors and reconcile time under `component="benthos_monitor"`.

With `USE_FSMV2_BENTHOS_METRICS=true`, the umh-core process reads the metrics itself, through one FSMv2 component of type `benthos_metrics` per running flow. The `component="benthos_monitor"` series stop appearing, and these components report the standard FSMv2 series instead. To move an alert:

- **Reconcile time**: use `umh_fsmv2_reconciliation_duration_seconds`, a histogram in seconds rather than milliseconds, for example `histogram_quantile(0.99, rate(umh_fsmv2_reconciliation_duration_seconds_bucket{hierarchy_path=~".*\\(benthos_metrics\\)"}[5m]))`.
- **Errors**: FSMv2 has no counter that matches `umh_core_errors_total`. Alert on the time in state `degraded` instead. A `benthos_metrics` component is `degraded` while it cannot read the metrics. It also starts in `degraded`, each time a flow starts, so the threshold must cover the first read:

```promql
umh_fsmv2_state_duration_seconds{hierarchy_path=~".*\\(benthos_metrics\\)", state="degraded"} > 60
```

Neither the `benthos_monitor` series nor the `benthos_metrics` series carries the flow's message counts. See [Metrics by component](#metrics-by-component).

### State

| Name | Type | Reports |
|---|---|---|
| `umh_core_service_current_state` | gauge | The state of the component, as a number. |
| `umh_core_service_desired_state` | gauge | The state the component should reach, as a number. |

| Value | State |
|---|---|
| 0 | stopped |
| 1 | starting |
| 2 | running |
| 3 | active |
| 4 | idle |
| 5 | degraded |
| -1 | a sub-state |

Many components have sub-states, such as a bridge that waits for its connection. -1 means the component is in a sub-state, such as one of its starting, degraded or stopping steps. A bridge that waits for resources before it can start stays in `to_be_created`, which also reads -1. Check the state in the Management Console. To alert on a component that is not active, idle or stopped, use `umh_core_service_current_state{component="<component value>"} != 3 != 4 != 0` with a `for:` duration, because every start passes through sub-states.

### Errors and reconcile time

| Name | Type | Reports |
|---|---|---|
| `umh_core_errors_total` | counter | Errors umh-core hit while it managed the component. |
| `umh_core_reconcile_duration_milliseconds` | histogram | Duration of one reconcile, in milliseconds. |
| `umh_core_reconcile_starved_total_seconds` | counter | Rises when umh-core's control loop has not run for more than 15 seconds. It has no labels. |

umh-core counts one error at several layers, for example under the bridge and again under the part of umh-core that manages all bridges. Compare `umh_core_errors_total` over time for one `component` value. Do not add up all values.

`umh_core_reconcile_duration_milliseconds` with `component="control_loop"` and `instance="main"` is the duration of one whole cycle of umh-core's control loop. A cycle should finish within 100 milliseconds. A longer cycle writes a warning to the agent log. Other `instance` values time single steps inside that cycle. A component's step names start with the component's name. The filter `instance=~"<name>(\\..*)?"` selects all of them. Do not add the steps up. They are nested inside each other, and a sum counts time twice.

Alert on any increase of `umh_core_reconcile_starved_total_seconds` above zero. It means the control loop did not run for more than 15 seconds.

## Metrics by component

Each component page has a Metrics section. It explains what the component reports and gives alert queries for it.

| Component | Metrics |
|---|---|
| Agent | [Agent](README.md#metrics) |
| Container | [Container](../container/README.md#metrics) |
| Communicator | [Communicator](../communicator/README.md#metrics) |
| Redpanda | [Redpanda](../../unified-namespace/redpanda.md#metrics) |
| Topic Browser | [Topic Browser](../../unified-namespace/topic-browser.md#metrics) |
| Bridges | [Bridges](../../data-flows/bridges.md#metrics) |
| Stand-alone flows | [Stand-alone Flows](../../data-flows/stand-alone-flow.md#metrics) |
| Stream processors | [Stream Processors](../../data-flows/stream-processor.md#metrics) |
| Historian | [Historian](../../historian/README.md#metrics) |

Benthos and Redpanda keep their own metrics. umh-core does not copy them to port 8080. Per-bridge message counts are therefore not on port 8080. The Metrics view of a bridge in the Management Console shows them.

## Where the metrics are defined

The names and help texts come from the code and can change with a release. Check the definitions for the exact list:

- FSMv1 series (`umh_core_*`, `umh_container_*`): [`pkg/metrics/metrics.go`](https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/staging/umh-core/pkg/metrics/metrics.go) and [`pkg/service/container_monitor/metrics.go`](https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/staging/umh-core/pkg/service/container_monitor/metrics.go)
- FSMv2 series (`umh_fsmv2_*`): [`fsmv2/supervisor/metrics/metrics.go`](https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/staging/umh-core/fsmv2/supervisor/metrics/metrics.go) for the standard series such as `state_duration_seconds` and `circuit_open`, and [`fsmv2/deps/metrics.go`](https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/staging/umh-core/fsmv2/deps/metrics.go) for the metrics a worker records itself
- Every series on a running instance, with its help text: the `/metrics` output itself.

## Go runtime metrics

umh-core is written in Go. The series that start with `go_`, `process_` and `promhttp_` are the standard series of the Prometheus Go client. They report memory use, the number of goroutines (background tasks of the process), CPU seconds and open files of the umh-core process.

## Internal diagnostics

The series below are for UMH support and may change without notice. The same port also serves `/debug/fsmv2`, a JSON view of the FSMv2 components for UMH support.

- `umh_fsmv2_action_queued_total`
- `umh_fsmv2_worker_pool_utilization` (label `pool_name` instead of `hierarchy_path`, with the same value)
- `umh_fsmv2_worker_pool_queue_size` (label `pool_name`, as above)
- `umh_fsmv2_infrastructure_recovery_total`
- `umh_fsmv2_infrastructure_recovery_duration_seconds`
- `umh_fsmv2_observation_save_total`
- `umh_fsmv2_observation_save_duration_seconds`
- `umh_fsmv2_template_rendering_duration_seconds`
- `umh_fsmv2_variable_propagation_total`
- `umh_fsmv2_hierarchy_depth`
- `umh_fsmv2_hierarchy_size`
- `umh_fsmv2_worker_compaction_deltas_deleted_total`, `umh_fsmv2_worker_compaction_cycles_total`, `umh_fsmv2_worker_last_compaction_duration_ms`, `umh_fsmv2_worker_maintenance_cycles_total` and `umh_fsmv2_worker_last_maintenance_duration_ms`, reported by the persistence component
- `umh_communicator_action_panics_total`
- `umh_core_filesystem_ops_total`
- `umh_core_filesystem_ops_duration_seconds`
