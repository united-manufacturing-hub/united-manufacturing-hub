# Metrics

`http://<device-ip>:8080/metrics` (Prometheus format) exposes:

## Currently Available ✅

* Agent tick & FSM timings (each full reconcile loop < 100 ms by design)
* Per-DFC counters: processed, error, latency, active / idle flag
* Redpanda I/O and disk-utilisation stats
* CPU health evidence (preview): usage, throttling, PSI pressure, host headroom and capacity. Each measurement that can be unreadable has a companion flag reading 1 or 0, because the measurement itself reports 0 when its signal is absent — read the flag before trusting a zero