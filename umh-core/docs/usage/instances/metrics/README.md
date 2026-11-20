# Metrics

This page lists the Prometheus metrics that a UMH Core instance serves on port 8080, and explains how to read them. The metrics come from umh-core, the process inside the UMH Core container. [Collect Metrics with Prometheus](collect-with-prometheus.md) explains how to reach the endpoint.

The component pages explain what one component reports and give alert queries for it. See [Metrics by component](#metrics-by-component).

## Read the metrics

[Collect Metrics with Prometheus](collect-with-prometheus.md) explains how to read the endpoint once and how to set up a Prometheus scrape job.

Prometheus renames umh-core's `instance` label to `exported_instance`, unless the scrape job sets `honor_labels: true`. In Prometheus queries, write `exported_instance="Core"` where these pages write `instance="Core"`.

Most series appear only after umh-core records their first value. The FSMv2 counters, for example, appear after their first count. A missing series therefore does not mean the component is missing. `umh_core_errors_total` is the exception. It exists at 0 from the moment umh-core creates the component.

When you remove a component, such as a bridge, most of its series stay on `/metrics` with their last value. The series disappear when umh-core restarts. umh-core deletes only `umh_fsmv2_state_duration_seconds` together with the component.

## Which framework runs a component

The agent is the umh-core process. A framework is the part of the agent that starts a component, watches it and restarts it when it fails. umh-core has two frameworks. FSMv1 is the older one, and FSMv2 is the newer one. Each framework names its metrics differently. To find the metrics of a component, first find its framework in this table.

| Component | Framework | Switch |
|---|---|---|
| Communicator | FSMv2 | none |
| Persistence and configuration components (internal parts of umh-core) | FSMv2 | none |
| Historian (the connection to TimescaleDB) | FSMv2 | none |
| Bridge connection check | FSMv2 | none. `NMAP_BACKEND` is ignored |
| CPU health | FSMv2 from v0.44.46. On v0.44.37 to v0.44.45, FSMv1 unless `USE_FSMV2_CPU=true` | `USE_FSMV2_CPU=false` switches to FSMv1 on v0.44.46 and later. See [Container: Configuration](../container/README.md#configuration) |
| Bridges, stand-alone flows, stream processors | FSMv1 | none |
| Reading the Benthos metrics of bridges, stand-alone flows and stream processors | FSMv1, or FSMv2 with `USE_FSMV2_BENTHOS_METRICS=true` | `USE_FSMV2_BENTHOS_METRICS=true` switches the reading to FSMv2. See [Environment Variables](../../../reference/environment-variables.md) |
| Topic Browser, Redpanda, agent monitor | FSMv1 | none |
| Container memory and disk | FSMv1 | none |
| Memory health (preview) | FSMv2 | off unless `USE_FSMV2_MEMORY_MONITOR=true`. See [Container: Configuration](../container/README.md#configuration) |

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
| `umh_fsmv2_reconciliation_total` | counter | Reconciles, by label `result`. A cycle that fails before the reconcile records nothing, and the rate drops. Use the rate to confirm that reconciles run. |

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

`umh_core_reconcile_starved_total_seconds` adds more than one second for each second of a stall. Only an increase above zero is meaningful. It means the control loop did not run for more than 15 seconds.

## Metrics by component

Each component page has a Metrics section. It explains what the component reports and gives alert queries for it.

| Component | Metrics |
|---|---|
| Agent | [Agent](../agent/README.md#metrics) |
| Container | [Container](../container/README.md#metrics) |
| CPU health | [CPU gauges](#cpu-gauges) |
| Memory health (preview) | [Memory gauges](#memory-gauges) |
| Communicator | [Communicator](../communicator/README.md#metrics) |
| Redpanda | [Redpanda](../../unified-namespace/redpanda.md#metrics) |
| Topic Browser | [Topic Browser](../../unified-namespace/topic-browser.md#metrics) |
| Bridges | [Bridges](../../data-flows/bridges.md#metrics) |
| Stand-alone flows | [Stand-alone Flows](../../data-flows/stand-alone-flow.md#metrics) |
| Stream processors | [Stream Processors](../../data-flows/stream-processor.md#metrics) |
| Historian | [Historian](../../historian/README.md#metrics) |

Each bridge, stand-alone flow and stream processor runs in Benthos. Benthos and Redpanda keep their own metrics. umh-core does not copy them to port 8080. Per-bridge message counts are therefore not on port 8080. The Metrics view of a bridge in the Management Console shows them.

## CPU gauges

With the CPU health check on, UMH Core publishes the gauges below on the Prometheus endpoint on port 8080. UMH Core measures the CPU once per second and updates the gauges after each measurement. Each gauge is exposed as `umh_fsmv2_worker_<name>`, with a `hierarchy_path` label that ends in `(cpu)`. Select them with `{hierarchy_path=~".*\\(cpu\\)"}`. A query that uses the name without the `umh_fsmv2_worker_` prefix matches nothing, and Prometheus reports no error for it.

When UMH Core cannot read a file it needs for the CPU usage or capacity (the `CPU not measured` message on the [Instance Details Page](../instance-details-page.md#cpu-status)), every gauge keeps its previous value, and nothing marks it stale. Only the age of `umh_fsmv2_worker_cpu_last_sample_unix` shows such a freeze.

### CPU measurements

| Name | Type | Reports |
|---|---|---|
| `umh_fsmv2_worker_cpu_avg_usage_cores` | gauge | This container's mean usage over 60 seconds, in cores |
| `umh_fsmv2_worker_cpu_avg_usage_fraction` | gauge | That mean as a fraction (0 to 1) of the CPUs this container may run on |
| `umh_fsmv2_worker_cpu_throttle_ratio` | gauge | The share of scheduling periods in the last 60 seconds that were throttled, 0 to 1 |
| `umh_fsmv2_worker_cpu_pressure_avg60_ratio` | gauge | The kernel's PSI cpu-some avg60, as a fraction from 0 to 1 |
| `umh_fsmv2_worker_cpu_host_headroom_cores` | gauge | Cores free on the machine after the reserve, in cores. Negative on a full machine. |
| `umh_fsmv2_worker_cpu_avg_host_busy_cores` | gauge | The whole machine's mean busy time over 60 seconds, in cores |
| `umh_fsmv2_worker_cpu_capacity_cores` | gauge | The ceiling that CPU health judges against, in cores |
| `umh_fsmv2_worker_cpu_reserve_cores` | gauge | The part of that ceiling held back, in cores |
| `umh_fsmv2_worker_cpu_host_cpus` | gauge | The machine's CPU count |
| `umh_fsmv2_worker_cpu_last_sample_unix` | gauge | The Unix time, in seconds, of the last successful measurement |

### CPU flags

Each flag reads 1 or 0. A 0 means the gauge it covers is not worth acting on right now. That happens in two cases: the gauge is a mean over too few samples, or the gauge reads 0 because this machine has no source for it or the reading failed. Both cases set the flag to 0, and nothing in the metrics tells them apart.

| Name | Type | Covers |
|---|---|---|
| `umh_fsmv2_worker_cpu_usage_ring_active` | gauge | `cpu_avg_usage_cores` |
| `umh_fsmv2_worker_cpu_host_busy_ring_active` | gauge | `cpu_avg_host_busy_cores` |
| `umh_fsmv2_worker_cpu_host_busy_cores_available` | gauge | `cpu_avg_host_busy_cores` and `cpu_host_headroom_cores` |
| `umh_fsmv2_worker_cpu_throttle_signal_ready` | gauge | `cpu_throttle_ratio` |
| `umh_fsmv2_worker_cpu_pressure_signal_ready` | gauge | `cpu_pressure_avg60_ratio` |
| `umh_fsmv2_worker_cpu_host_headroom_available` | gauge | `cpu_host_headroom_cores` |

The flags come in three kinds:

- A `_ring_active` flag covers one 60-second window. It reads 0 until that window holds enough samples to trust.
- A `_signal_ready` flag covers a whole signal. It reads 0 while the window fills and when the signal could not be read. It also reads 0 for as long as the container runs on a machine with no source for that signal, such as a bare-metal host without cgroup throttle counters.
- An `_available` flag reads 0 when the latest measurement could not supply the figure. `cpu_host_headroom_available` reads 0 when the container is pinned to a subset of the machine's CPUs. It also reads 0 when the machine's CPU count could not be read. The first is a deployment choice. The second is a read failure worth investigating.

`cpu_host_headroom_cores` has no flag of its own. Trust it only when `cpu_host_headroom_available`, `cpu_host_busy_cores_available` and `cpu_host_busy_ring_active` all read 1.

## Memory gauges

With the [memory monitor preview](../container/how-memory-health-is-judged.md) on, umh-core publishes the gauges below. They use the same `umh_fsmv2_worker_<gauge name>` series and `hierarchy_path` label as the CPU gauges, with a `hierarchy_path` that ends in `(memory)`, and they also keep their previous value on a tick that could not measure. On such a tick, the age of `memory_last_sample_unix` is the only thing that reveals the freeze.

### Memory measurements

| Gauge | Reports |
|---|---|
| `memory_avg_used_bytes` | the 60s mean of the container's memory in use, in bytes |
| `memory_total_bytes` | the limit that usage is judged against: the container's memory limit, or the machine's total memory when the container has none |
| `memory_avg_used_fraction` | the 60s mean of the memory in use as a 0..1 fraction of `memory_total_bytes` |
| `memory_avg_host_used_bytes` | the whole machine's 60s mean used memory, in bytes. It works like `cpu_avg_host_busy_cores` in [CPU Health](#cpu-measurements) |
| `memory_host_total_bytes` | the machine's total memory, in bytes |
| `memory_pressure_avg60_ratio` | the kernel's PSI memory-some avg60 as a 0..1 fraction |
| `memory_oom_kills_last_hour` | the OOM kills in the container within the last hour |
| `memory_last_sample_unix` | the unix seconds of the last tick that measured |

### Memory flags

| Flag | Applies to |
|---|---|
| `memory_used_ring_active` | `memory_avg_used_bytes` and `memory_avg_used_fraction` |
| `memory_host_ring_active` | `memory_avg_host_used_bytes` |
| `memory_pressure_signal_ready` | `memory_pressure_avg60_ratio` |
| `memory_oom_signal_ready` | `memory_oom_kills_last_hour` |
| `memory_used_from_cgroup` | `memory_avg_used_bytes` |
| `memory_total_is_cgroup_limit` | `memory_total_bytes` and `memory_avg_used_fraction` |

The `_ring_active` and `_signal_ready` flags work like the [CPU flags](#cpu-flags). `memory_host_ring_active` also reads 0 while the machine's memory is unreadable. `memory_pressure_signal_ready` stays 0 on cgroup v1 and without PSI. `memory_oom_signal_ready` reads 0 before the second read of the OOM counter and while that counter is unreadable.

The two source flags say where a number came from, not whether to trust it. `memory_used_from_cgroup` reads 0 when the container's cgroup was unreadable. `memory_avg_used_bytes` and `memory_avg_used_fraction` then have no value, `memory_used_ring_active` reads 0, and the machine's gauges carry the reading. `memory_total_is_cgroup_limit` reads 0 when the container has no memory limit and usage is judged against the machine's total memory.

## Communicator error counters

The communicator counts each failed request to the Management Console once, under its cause. The series carry a `hierarchy_path`. The failed logins are the series whose `hierarchy_path` ends in `(transport)`. A counter appears only after its first count. [Communicator: Metrics](../communicator/README.md#metrics) gives the alert query.

| Name | Type | Reports |
|---|---|---|
| `umh_fsmv2_worker_network_errors_total` | counter | Requests without an answer that umh-core can classify. This includes a refused connection, a timeout and a rejected certificate. It also counts answers with a status no other counter takes, such as 400 or a proxy's 407 Proxy Authentication Required. |
| `umh_fsmv2_worker_proxy_block_errors_total` | counter | Error answers that look like the block page of a proxy. Ask your IT department to allow `management.umh.app` on the proxy. |
| `umh_fsmv2_worker_auth_failures_total` | counter | Answers with status 401 or 403. Under the transport series, a rise usually means the Management Console did not accept the `AUTH_TOKEN`. A firewall, a proxy or Cloudflare that answers 403 also counts here, unless umh-core recognizes the answer as a proxy block page, which counts in `umh_fsmv2_worker_proxy_block_errors_total`. |
| `umh_fsmv2_worker_instance_deleted_total` | counter | Answers with status 404. Usually the instance was deleted in the Management Console. A wrong `API_URL` or a 404 from a proxy also counts here. |
| `umh_fsmv2_worker_server_errors_total` | counter | Requests the Management Console answered with a status of 500 or higher. |
| `umh_fsmv2_worker_backend_rate_limit_errors_total` | counter | Requests the Management Console rejected with status 429, because the instance sent too many in a short time. |
| `umh_fsmv2_worker_cloudflare_errors_total` | counter | Answers with status 429 that carry a Cloudflare challenge page. |

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
- `umh_fsmv2_adapter_dropped_messages_total`
- `umh_fsmv2_worker_compaction_deltas_deleted_total`, `umh_fsmv2_worker_compaction_cycles_total`, `umh_fsmv2_worker_last_compaction_duration_ms`, `umh_fsmv2_worker_maintenance_cycles_total` and `umh_fsmv2_worker_last_maintenance_duration_ms`, reported by the persistence component
- `umh_communicator_action_panics_total`
- `umh_core_filesystem_ops_total`
- `umh_core_filesystem_ops_duration_seconds`
