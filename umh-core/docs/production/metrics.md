# Metrics

`http://<device-ip>:8080/metrics` (Prometheus format) exposes:

## Currently Available ✅

* Agent tick & FSM timings (each full reconcile loop < 100 ms by design)
* Per-DFC counters: processed, error, latency, active / idle flag
* Redpanda I/O and disk-utilisation stats
* CPU health evidence (preview)

## CPU health evidence (preview)

Every gauge below is exposed as the series `umh_fsmv2_worker_<gauge name>`, carrying a `hierarchy_path` label that names the worker that published it. A rule written against the bare name matches nothing, and Prometheus reports no error for it.

On a tick that could not measure, every gauge on this page keeps being scraped at its previous value, and nothing marks it stale. The age of `cpu_last_sample_unix` is the only thing that reveals such a freeze.

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
