# Metrics

`http://<device-ip>:8080/metrics` (Prometheus format) exposes:

* Agent tick & FSM timings (each full reconcile loop < 100 ms by design)
* Per-DFC counters: processed, error, latency, active / idle flag
* Redpanda I/O and disk-utilisation stats
