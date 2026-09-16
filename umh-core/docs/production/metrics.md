# Metrics

`http://<device-ip>:8080/metrics` (Prometheus format) exposes:

## Currently Available ✅

* Agent tick & FSM timings (each full reconcile loop < 100 ms by design)
* Per-DFC counters: processed, error, latency, active / idle flag
* Redpanda I/O and disk-utilisation stats
* CPU health evidence (preview), described below

## CPU health evidence (preview)

Every CPU gauge is scraped under the prefix `umh_fsmv2_worker_`, with a `hierarchy_path` label naming
the worker that published it. The gauge `cpu_avg_usage_cores` therefore arrives as the series
`umh_fsmv2_worker_cpu_avg_usage_cores{hierarchy_path="..."}`. A rule written against the bare gauge
name matches nothing, and Prometheus reports no error for it.

### Check the sample age first

`cpu_last_sample_unix` carries the unix seconds of the last tick that measured. A tick that fails to
measure republishes the previous values, so every CPU series keeps arriving at its last reading with
nothing to mark it stale. Alert on the age of this gauge before trusting anything beside it:

```promql
time() - umh_fsmv2_worker_cpu_last_sample_unix > 120
```

### Measurements

| Gauge | Reports |
|---|---|
| `cpu_avg_usage_cores` | this container's 60s mean usage, in cores |
| `cpu_avg_usage_fraction` | that mean as a 0..1 fraction of the CPUs this container may run on |
| `cpu_throttle_ratio` | the 60s share of scheduling periods that were throttled, 0..1 |
| `cpu_pressure_avg60_ratio` | the kernel's PSI cpu-some avg60 as a 0..1 fraction |
| `cpu_host_headroom_cores` | cores free on the host after the reserve; negative on a full machine |
| `cpu_avg_host_busy_cores` | the whole machine's 60s mean busy time, in cores |
| `cpu_capacity_cores` | the ceiling the verdict judged against, in cores |
| `cpu_reserve_cores` | the part of that ceiling held back, in cores |
| `cpu_host_cpus` | the machine's CPU count |

### Flags

A flag gauge qualifies the measurement beside it. Each flag qualifies one measurement:

| Flag | Qualifies |
|---|---|
| `cpu_usage_ring_active` | `cpu_avg_usage_cores` |
| `cpu_host_busy_ring_active` | `cpu_avg_host_busy_cores` |
| `cpu_throttle_signal_ready` | `cpu_throttle_ratio` |
| `cpu_pressure_signal_ready` | `cpu_pressure_avg60_ratio` |

The two families call for opposite actions. A `_ring_active` of 0 means the 60s window has not
filled. A non-zero mean beside it is a real average over fewer samples, so keep it and read it as
covering less than 60s. A 0 beside it is ambiguous: the window may instead be empty because nothing
could be read, and the flag does not separate those two cases. A `_signal_ready` of 0 means the
signal could not be read at all, so the value beside it is a 0 nobody measured and you should
discard it.

A measurement that no flag qualifies gives you no way to separate an absent signal from a measured 0.
Alert on such a gauge only where both readings call for the same action.

`cpu_host_headroom_available` is not a flag. It reports whether this container sees the whole machine,
and reads 0 when the container is pinned to a subset of CPUs.

### Not published yet

The CPU verdict also judges steal time, and this preview publishes no gauge for it. On a virtual
machine with a noisy neighbour the verdict can turn degraded while every CPU gauge above reads normal.
