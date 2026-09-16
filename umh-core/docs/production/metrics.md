# Metrics

`http://<device-ip>:8080/metrics` (Prometheus format) exposes:

## Currently Available ✅

* Agent tick & FSM timings (each full reconcile loop < 100 ms by design)
* Per-DFC counters: processed, error, latency, active / idle flag
* Redpanda I/O and disk-utilisation stats
* CPU health evidence (preview): usage, throttling, CPU pressure, host headroom and capacity

A CPU measurement reads 0 when its signal was absent, so read the flag gauge beside it first:
`cpu_usage_ring_active`, `cpu_host_busy_ring_active`, `cpu_throttle_signal_ready` and
`cpu_pressure_signal_ready`. `cpu_host_headroom_cores` is the one measurement with no such flag.
`cpu_host_headroom_available` does not serve as one: it reports whether this container sees the
whole machine, so it reads 1 on a machine whose core count was unreadable at startup, where
headroom is pinned at 0.
