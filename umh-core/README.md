# UMH Core

*A lightweight, single‑container gateway that connects your machines to the United Manufacturing Hub (UMH) Unified Namespace.*

---

## 1  What is UMH Core?

UMH Core bundles three things into **one Docker container** running happily on a Raspberry Pi, an industrial PC, or any x86/ARM box:

| Inside the container | What it does                                                                                          |
| -------------------- | ----------------------------------------------------------------------------------------------------- |
| **Agent** (Go)       | Reads a YAML config, starts/stops pipelines, talks to the UMH Management Console, and watches health. |
| **Benthos‑UMH**      | Streams, converts, and routes data (each pipeline is called a **Data Flow Component — DFC**).         |
| **Redpanda**         | Kafka‑compatible broker for store‑and‑forward buffering when the network blinks.                      |

Everything is orchestrated by **S6 Overlay** so services start in the right order and are restarted if they crash — no Kubernetes required.

---

## 2  TL;DR — Spin it up

The Management Console will generate a command similar to this:

```bash
sudo docker run -d \
  --restart unless-stopped \
  -v $(pwd)/umh-core-data:/data \
  -e AUTH_TOKEN=YOUR_TOKEN_HERE \
  -e RELEASE_CHANNEL=stable             # nightly / enterprise are also available
  -e LOCATION_0="My‑Plant---Line‑A"      # optional, add LOCATION_1…n for hierarchies
  -e API_URL=https://management.umh.app/api \
  management.umh.app/oci/united-manufacturing-hub/umh-core:latest
```

| Variable          | Purpose                                                                                                    |
| ----------------- | ---------------------------------------------------------------------------------------------------------- |
| `AUTH_TOKEN`      | **Required.** Authorises the agent with the cloud console. Easily revoke/rotate there.                     |
| `RELEASE_CHANNEL` | `stable` (default), `nightly`, or `enterprise`. Controls automatic updates shipped with the container.     |
| `LOCATION_0..n`   | Human‑readable hierarchy used by the console (e.g. `Company‑Plant‑Line`).                                  |
| `API_URL`         | Where the agent should call home. Defaults to the public SaaS URL; you can point it to an on‑prem console. |
| `LOGGING_LEVEL`   | Optional. `DEBUG`, `INFO` (default), `WARN`, `ERROR`.                                                      |

When the container starts it looks for **`/data/config.yaml`**. If the file is missing, the agent creates an empty skeleton that you or the console can later fill.

---

## 3  Folder & Volume Layout

```
/data
 ├─ config.yaml           # Your desired‑state YAML (watched continuously)
 ├─ logs/                 # Rolling logs for agent, each DFC, Redpanda …
 ├─ redpanda/             # Broker data & WALs (safe to back up)
 └─ hwid                  # Device fingerprint sent to the console
```

Mount **one persistent volume** (e.g. `umh-core-data`) to `/data` and you’re done.

---

## 4  Configuration File (what *you* edit)

A minimal user‑managed config looks like:

```yaml
agent:
  metricsPort: 8080                    # optional (default 8080 inside container)
  communicator:
    apiUrl: https://management.umh.app/api
    # authToken comes from the AUTH_TOKEN env var
  releaseChannel: stable               # stable | nightly | enterprise
  location:
    0: My‑Plant
    1: Line A

dataFlow:
  - name: hello-world-dfc
    desiredState: active               # active | stopped
    dataFlowComponentConfig:
      benthos:
        input:
          generate:
            mapping: root = "hello world from DFC!"
            interval: 1s
        pipeline:
          processors:
            - bloblang: root = content()
        output:
          stdout: {}
```

### 4.1  What’s **not** in the file?

Everything under `internal:` is reserved for UMH engineers — those entries let us run built‑in services like Redpanda or Benthos Monitors. You never have to touch them and they stay hidden in the console UI.

---

## 5  Data Flow Components (DFCs)

A **DFC** is a Benthos pipeline wrapped with health checks and lifecycle management. Use them to:

* Convert protocols (OPC UA → MQTT, REST → Kafka, …)
* Bridge data between brokers
* Enrich or clean messages on the fly

### 5.1  Lifecycle States

| State        | What it means                                                                            |
| ------------ | ---------------------------------------------------------------------------------------- |
| **Stopped**  | Config exists, pipeline is off.                                                          |
| **Starting** | S6 starts Benthos, config loads, waiting for health OK & log window.                     |
| **Idle**     | Running but **throughput below activity threshold** (no messages processed for ≈ 2 min). |
| **Active**   | Running **and** processing messages (metrics flag `IsActive=true`).                      |
| **Degraded** | Running **but** health checks fail or recent logs/metrics show errors.                   |
| **Stopping** | Graceful shutdown in progress.                                                           |


### 5.2  What puts a DFC in *Degraded*?

A DFC flips from *Idle*/**Active** to **Degraded** if **any** of the following tests fail during the last 10 minutes **or** the last 10 000 log lines — whichever is shorter:

1. **Health endpoints** `/ping` or `/ready` return non‑OK (checked once per second, 1 s curl timeout).
2. **Logs** contain:

   * Fatal prefixes: `configuration file read error:`, `failed to create logger:`, `Config lint error:`.
   * Any line with level `error`.
   * A `warn` line that includes *failed to*, *connection lost*, or *unable to*.
3. **Metrics** report `error > 0` on **any output** *or* on **any processor**.

If all three tests become green again (next health ping is OK, log window clean, error counters back to 0) the DFC auto‑returns to **Idle** (or **Active** if messages are flowing).

### 5.3  How *Idle* ⇄ *Active* works

* Benthos‑Monitor tags the pipeline **Active** whenever at least one message was processed in the last second.
* If **no message** is seen for ≈ 120 s the agent sends `EventBenthosNoDataReceived` → *Idle*.
* As soon as a message arrives (`EventBenthosDataReceived`) it flips back to *Active*.

No data loss happens — this is just a status badge so you can tell silent pipelines from busy ones.

---

## 6  Startup Timeline (numbers from code)

```
Stopped ──▶ Starting
            • S6 service up
            • Wait 5 s uptime to deem "config loaded"
            • /ping & /ready healthy
            • No critical logs/metrics for 10 s (window = constants.BenthosLogWindow)
            ──▶ Idle      (≤ 15 s total or retries kick in)
Idle ──▶ Active as soon as throughput rises
```

---

## 7  Health Checks & Self‑Healing

* **Heartbeat:** every **1 s** the monitor fetches `/ping`, `/ready`, `/version`, and `/metrics`, each with a 1 s HTTP timeout.
* **Automatic restarts:** on crash the agent restarts immediately; after every *rate‑limited* operation it must wait before repeating the same expensive action.
* **Back‑off:** repeated failures grow the delay (constant + jitter) but there is *no hard cap* — the agent keeps trying forever until a config change

---

## 8  Metrics

`http://<device‑ip>:8080/metrics` exposes Prometheus metrics for:

* Agent ticks & FSM timings (all constants are sized so one loop < 100 ms).
* Each DFC’s message counters, latency, error count
* Redpanda I/O stats.