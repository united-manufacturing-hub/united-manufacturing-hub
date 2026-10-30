# Metrics

`http://<device-ip>:8080/metrics` (Prometheus format) exposes:

## Currently Available ✅

* Agent tick & FSM timings (each full reconcile loop < 100 ms by design)
* Per-DFC counters: processed, error, latency, active / idle flag
* Redpanda I/O and disk-utilisation stats
* CPU health evidence (preview)
* Memory evidence (preview)

## CPU health evidence (preview)

Every gauge below is exposed as the series `umh_fsmv2_worker_<gauge name>`, carrying a `hierarchy_path` label that names the worker that published it. A rule written against the bare name matches nothing, and Prometheus reports no error for it.

On a tick that could not measure, every gauge on this page keeps being scraped at its previous value, and nothing marks it stale. The age of `cpu_last_sample_unix`, and of `memory_last_sample_unix` for the memory gauges, is the only thing that reveals such a freeze.

### Measurements

| Gauge | Reports |
|---|---|
| `cpu_avg_usage_cores` | this container's 60s mean usage, in cores |
| `cpu_avg_usage_fraction` | that mean as a 0..1 fraction of the CPUs this container may run on |
| `cpu_throttle_ratio` | the 60s share of scheduling periods that were throttled, 0..1 |
| `cpu_pressure_avg60_ratio` | the kernel's PSI cpu-some avg60 as a 0..1 fraction |
| `cpu_host_headroom_cores` | cores free on the host after the reserve; negative on a full machine |
| `cpu_avg_host_busy_cores` | the whole machine's 60s mean busy time, in cores |
| `cpu_capacity_cores` | the ceiling CPU health judged against, in cores |
| `cpu_reserve_cores` | the part of that ceiling held back, in cores |
| `cpu_host_cpus` | the machine's CPU count |
| `cpu_last_sample_unix` | the unix seconds of the last tick that measured |

### Flags

| Flag | Applies to |
|---|---|
| `cpu_usage_ring_active` | `cpu_avg_usage_cores` |
| `cpu_host_busy_ring_active` | `cpu_avg_host_busy_cores` |
| `cpu_host_busy_cores_available` | `cpu_avg_host_busy_cores` and `cpu_host_headroom_cores` |
| `cpu_throttle_signal_ready` | `cpu_throttle_ratio` |
| `cpu_pressure_signal_ready` | `cpu_pressure_avg60_ratio` |
| `cpu_host_headroom_available` | `cpu_host_headroom_cores` |

Each flag reads 1 or 0. A 0 means the number beside it is not worth acting on this tick. A figure averaged over too few samples and a 0 nobody measured both publish with the flag at 0, and nothing in the exposed metrics tells them apart.

A `_ring_active` flag covers one 60-second window, and reads 0 until that window holds enough samples to trust. A `_signal_ready` flag covers a whole signal, and reads 0 both while the window fills and when the signal could not be read. It also reads 0 for as long as the container runs on a machine with no instrument for that signal, such as a bare-metal host with no cgroup throttle counters. An `_available` flag reads 0 when this tick's sample could not supply the figure. `cpu_host_headroom_available` reads 0 when the container is pinned to a subset of the machine's CPUs, and also when the machine's CPU count could not be read. The first is a deployment choice, the second a read failure worth investigating.

`cpu_host_headroom_cores` has no flag of its own. Trust it only when `cpu_host_headroom_available`, `cpu_host_busy_cores_available` and `cpu_host_busy_ring_active` all read 1.

## Memory evidence (preview)

These gauges appear when the container is started with `-e USE_FSMV2_MEMORY_MONITOR=true`. They use the same `umh_fsmv2_worker_<gauge name>` series and `hierarchy_path` label as the CPU gauges above, and they also keep their previous value on a tick that could not measure.

With the preview on, the instance's memory status comes from this reading. Memory counts as degraded while any of these causes holds:

| Cause | Degraded from | Healthy again below | Gauge |
|---|---|---|---|
| `container-limit-full` | 0.8 (80%) of the container's own memory limit, as a 60s mean. Judged only when the container has a limit | 0.75 | `memory_avg_used_fraction` while `memory_total_is_cgroup_limit` is 1 |
| `host-memory-full` | 0.8 (80%) of the machine's memory, as a 60s mean | 0.75 | `memory_avg_host_used_bytes` / `memory_host_total_bytes` |
| `pressure` | 0.20: processes in the container waited for memory for 20% of the last minute | 0.12 | `memory_pressure_avg60_ratio` |
| `oom-kill` | 1 OOM kill in the container within the last 60s | 0 for a whole 60s | `memory_oom_kills_last_60s` |

Memory also counts as degraded, and bridges are held back, whenever no reading from the last three seconds is available.

`pressure` needs cgroup v2 with pressure stall information (PSI) enabled, the same as CPU pressure. `oom-kill` counts only the kills after umh-core started, so it does not report a kill that restarted umh-core itself.

### Measurements

| Gauge | Reports |
|---|---|
| `memory_avg_used_bytes` | the 60s mean of the memory in use, in bytes: the container's, or the machine's when `memory_used_from_cgroup` is 0 |
| `memory_total_bytes` | the limit that usage is judged against: the container's memory limit, or the machine's total memory when the container has none |
| `memory_avg_used_fraction` | the 60s mean of the memory in use as a 0..1 fraction of `memory_total_bytes` |
| `memory_avg_host_used_bytes` | the whole machine's 60s mean used memory, in bytes. It works like `cpu_avg_host_busy_cores` |
| `memory_host_total_bytes` | the machine's total memory, in bytes |
| `memory_pressure_avg60_ratio` | the kernel's PSI memory-some avg60 as a 0..1 fraction |
| `memory_oom_kills_last_60s` | the OOM kills in the container within the last 60s |
| `memory_last_sample_unix` | the unix seconds of the last tick that measured |

### Flags

| Flag | Applies to |
|---|---|
| `memory_used_ring_active` | `memory_avg_used_bytes` and `memory_avg_used_fraction` |
| `memory_host_ring_active` | `memory_avg_host_used_bytes` |
| `memory_pressure_signal_ready` | `memory_pressure_avg60_ratio` |
| `memory_oom_signal_ready` | `memory_oom_kills_last_60s` |
| `memory_used_from_cgroup` | `memory_avg_used_bytes` |
| `memory_total_is_cgroup_limit` | `memory_total_bytes` and `memory_avg_used_fraction` |

The `_ring_active` and `_signal_ready` flags work like the CPU flags above. `memory_host_ring_active` also reads 0 while the machine's memory is unreadable. `memory_pressure_signal_ready` stays 0 on cgroup v1 and without PSI. `memory_oom_signal_ready` reads 0 before the second read of the OOM counter and while that counter is unreadable.

The two source flags say where a number came from, not whether to trust it. `memory_used_from_cgroup` reads 0 when the container's cgroup was unreadable and the machine's usage was used instead. `memory_total_is_cgroup_limit` reads 0 when the container has no memory limit and usage is judged against the machine's total memory.
