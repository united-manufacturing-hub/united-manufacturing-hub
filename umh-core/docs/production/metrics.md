# Metrics

`http://<device-ip>:8080/metrics` (Prometheus format) exposes:

## Currently Available ✅

* Agent tick & FSM timings (each full reconcile loop < 100 ms by design)
* Per-DFC counters: processed, error, latency, active / idle flag
* Redpanda I/O and disk-utilisation stats
* CPU health evidence (preview)

## CPU health evidence (preview)

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

### Flags

Each flag qualifies one measurement:

| Flag | Qualifies |
|---|---|
| `cpu_usage_ring_active` | `cpu_avg_usage_cores` |
| `cpu_host_busy_ring_active` | `cpu_avg_host_busy_cores` |
| `cpu_throttle_signal_ready` | `cpu_throttle_ratio` |
| `cpu_pressure_signal_ready` | `cpu_pressure_avg60_ratio` |
