# Metrics

If you are the power user, hand the Docker-host and Prometheus steps on this page to IT. IT needs access to port 8080 on the Docker host and to your Prometheus setup.

Every UMH Core instance serves Prometheus metrics in the Prometheus text format on port 8080, at the path `/metrics`. The metrics come from umh-core, the process inside the UMH Core container. Use them to monitor an instance with Prometheus and Grafana. Because the endpoint runs inside the container, you can read it when the Management Console cannot reach the instance.

The component pages list the metrics of one component. See [Metrics by component](#metrics-by-component).

## Read the metrics

To read the metrics once, run this on the machine that runs the container:

```bash
docker exec <container name> wget -qO- http://localhost:8080/metrics
```

To collect the metrics with Prometheus, publish port 8080 when you start the container. Add `-p 8080:8080` to your `docker run` command. The install command from the Management Console does not publish this port. For a running container, remove the container and start it again with the same `docker run` command plus `-p 8080:8080`. `docker restart` keeps the old port mappings.

The endpoint has no authentication. Publish it only on a trusted network, for example with `-p <host-ip>:8080:8080`.

Then add a scrape job to your Prometheus configuration:

```yaml
scrape_configs:
  - job_name: umh-core
    static_configs:
      - targets: ["<device-ip>:8080"]
```

Prometheus sets its own `instance` label on every scrape. With the default `honor_labels: false`, it renames umh-core's `instance` label to `exported_instance`. In Prometheus queries, write `exported_instance="Core"` where the component pages write `instance="Core"`. Alternatively, set `honor_labels: true` in the scrape job.

The port inside the container is always 8080. umh-core sets `agent.metricsPort` in `config.yaml` back to 8080 on every start. To use another port on the host, change the host side of the mapping, for example `-p 9100:8080`.

Most series appear only after umh-core records their first value. The FSMv2 counters, for example, appear after their first count. A missing series therefore does not mean the component is missing. `umh_core_errors_total` is the exception. It exists at 0 from the moment umh-core creates the component.

When you remove a component, such as a bridge, most of its series stay on `/metrics` with their last value. The series disappear when umh-core restarts. umh-core deletes only `umh_fsmv2_state_duration_seconds` together with the component.

## Which framework runs a component

The agent is the umh-core process. A framework is the part of the agent that starts a component, watches it and restarts it when it fails. umh-core has two frameworks. FSMv1 is the older one, and FSMv2 is the newer one. Each framework names its metrics differently. To find the metrics of a component, first find its framework in this table.

| Component | Framework | Switch |
|---|---|---|
| Communicator | FSMv2 | none |
| Persistence and configuration components (internal parts of umh-core) | FSMv2 | none |
| Historian (the connection to TimescaleDB) | FSMv2 | none |
| Bridge connection check | FSMv2 by default | `NMAP_BACKEND=fsmv1` switches to FSMv1 |
| CPU health | FSMv1 by default | `USE_FSMV2_CPU=true` switches to FSMv2 |
| Bridges, stand-alone flows, stream processors | FSMv1 | none |
| Topic Browser, Redpanda, agent monitor | FSMv1 | none |
| Container memory and disk | FSMv1 | none |

A switch is an environment variable of the container. umh-core reads it once at startup. To change it, remove the container and start it again with the same `docker run` command plus the variable. `docker restart` keeps the old environment. See [Environment Variables](../../../reference/environment-variables.md).

## FSMv2 metrics

Components that run under FSMv2 report two kinds of series:

- Standard series, named `umh_fsmv2_<name>`. Every FSMv2 component can report them.
- Component series, named `umh_fsmv2_worker_<name>`. Only the component that records the value reports them. The component pages list them.

Both kinds carry the label `hierarchy_path`. It names the component that reports the series. FSMv2 components form a tree. A component can start child components, and each child can start its own children. The path lists every component from the top of the tree down to the one that reports, separated by `/`. Each step is written as `<id>(<type>)`. For example, the part of the communicator that sends messages to the Management Console has this path:

```
application-fsmv2(application)/communicator-001(communicator)/transport-001(transport)/push-001(push)
```

To select one component, match the end of the path with a regular expression. This query returns the time the communicator has spent in its current state:

```promql
umh_fsmv2_state_duration_seconds{hierarchy_path=~".*\\(communicator\\)"}
```

Prometheus matches a regular expression against the whole label value. This expression therefore selects the communicator and none of its children.

Not every component reports every standard series. A simple component such as the bridge connection check or the historian runs no actions. It reports no `action_*` and no `stuck_action_*` series. A component without children never reports `umh_fsmv2_circuit_open`.

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
| `umh_fsmv2_reconciliation_total` | counter | Reconciles. The label `result` always reads `success`. A cycle that fails before the reconcile records nothing, and the rate drops. Use the rate only to confirm that reconciles run. |

### Circuit breaker and panics

A panic is a crash inside umh-core that umh-core caught and recovered from. When a component stops after repeated panics, every component above it pauses. This pause is called an open circuit.

| Name | Type | Reports |
|---|---|---|
| `umh_fsmv2_circuit_open` | gauge | 1 on a component and on every component above it, up to the top of the tree, while a component below has stopped after repeated panics. The component that panicked shows up in `umh_fsmv2_panic_recovery_total`. The series appears the first time the circuit opens. |
| `umh_fsmv2_panic_recovery_total` | counter | Panics, by component. Label `panic_type` is `error_panic`, `string_panic` or `unknown_panic`. A panic inside an action is counted as `status="panic"` on `umh_fsmv2_action_execution_duration_seconds` instead. |

Any increase of `umh_fsmv2_panic_recovery_total` is a defect in umh-core. Report it to UMH support.

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

Many components have sub-states, such as a bridge that waits for its connection. -1 means the component is in a sub-state (starting, degraded or stopping steps). Check the state in the Management Console.

### Errors and reconcile time

| Name | Type | Reports |
|---|---|---|
| `umh_core_errors_total` | counter | Errors umh-core hit while it managed the component. |
| `umh_core_reconcile_duration_milliseconds` | histogram | Duration of one reconcile, in milliseconds. |
| `umh_core_reconcile_starved_total_seconds` | counter | Rises when umh-core's control loop has not run for more than 15 seconds. It has no labels. |

umh-core counts one error at several layers, for example under the bridge and again under the part of umh-core that manages all bridges. Compare `umh_core_errors_total` over time for one `component` value. Do not add up all values.

`umh_core_reconcile_duration_milliseconds` with `component="control_loop"` and `instance="main"` is the duration of one whole cycle of umh-core's control loop. A cycle should finish within 100 milliseconds. A longer cycle writes a warning to the agent log. Other `instance` values time single steps inside that cycle. A component's step names start with the component's name. The filter `instance=~"<name>(\\..*)?"` selects all of them. Do not add the steps up. They are nested inside each other, and a sum counts time twice.

`umh_core_reconcile_starved_total_seconds` adds more than one second for each second of a stall. Only an increase above zero is meaningful. It means the control loop did not run for more than 15 seconds.

## Metrics by component

Each component page has a Metrics section that lists the series of that component and how to read them.

| Component | Metrics |
|---|---|
| Agent | [Agent](README.md#metrics) |
| Container | [Container](../container/README.md#metrics) |
| CPU health | [CPU Health](../container/cpu-health/README.md) |
| Communicator | [Communicator](../communicator/README.md#metrics) |
| Redpanda | [Redpanda](../../unified-namespace/redpanda.md#metrics) |
| Topic Browser | [Topic Browser](../../unified-namespace/topic-browser.md#metrics) |
| Bridges | [Bridges](../../data-flows/bridges.md#metrics) |
| Stand-alone flows | [Stand-alone Flows](../../data-flows/stand-alone-flow.md#metrics) |
| Stream processors | [Stream Processors](../../data-flows/stream-processor.md#metrics) |
| Historian | [Historian](../../historian/README.md#metrics) |

Each bridge, stand-alone flow and stream processor runs in Benthos, the engine that moves the data. Benthos and Redpanda keep their own metrics. umh-core does not copy them to port 8080. Per-bridge message counts are therefore not on port 8080. The Metrics view of a bridge in the Management Console shows them.

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
- `umh_fsmv2_adapter_dropped_messages_total`
- `umh_fsmv2_worker_compaction_deltas_deleted_total`, `umh_fsmv2_worker_compaction_cycles_total`, `umh_fsmv2_worker_last_compaction_duration_ms`, `umh_fsmv2_worker_maintenance_cycles_total` and `umh_fsmv2_worker_last_maintenance_duration_ms`, reported by the persistence component
- `umh_communicator_action_panics_total`
- `umh_core_filesystem_ops_total`
- `umh_core_filesystem_ops_duration_seconds`
