# Metrics

`http://<device-ip>:8080/metrics` (Prometheus format) exposes:

## Currently Available ✅

* Agent tick & FSM timings (each full reconcile loop < 100 ms by design)
* Per-DFC counters: processed, error, latency, active / idle flag
* Redpanda I/O and disk-utilisation stats
* CPU health evidence (preview): usage, throttling, PSI pressure, host headroom and capacity. Several of these read 0 when their signal is absent, so check the companion flag gauge before trusting a zero