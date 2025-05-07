# UMH Core

*A lightweight, single-container gateway that connects your machines to the United Manufacturing Hub (UMH) Unified Namespace.*

---

## 1  What is UMH Core?

UMH Core bundles three things into **one Docker container** that runs happily on a Raspberry Pi, an industrial PC, or any AMD64/ARM64 box:

| Inside the container | What it does                                                                                            |
| -------------------- | ------------------------------------------------------------------------------------------------------- |
| **Agent** (Go)       | Reads the YAML config, starts / stops services, talks to the UMH Management Console, and watches health |
| **[Benthos-UMH](https://github.com/United-Manufacturing-Hub/benthos-umh)**      | Streams, converts, and routes data (every pipeline is called a **[Data Flow Component](https://www.umh.app/product/data-flow-component) — DFC**)           |
| **Redpanda**         | Kafka-compatible broker for store-and-forward buffering when the network blinks                         |

Everything is orchestrated by **[S6 Overlay](https://github.com/just-containers/s6-overlay)**, so services start in the right order and restart automatically if they crash — *no Kubernetes required*.

---

## 2  TL;DR — Spin it up

The Management Console generates a command similar to this:

```bash
sudo docker run -d \
  --restart unless-stopped \
  -v "$(pwd)/umh-core-data":/data \
  -e AUTH_TOKEN=<YOUR_TOKEN> \
  -e RELEASE_CHANNEL=stable \
  -e LOCATION_0="My-Plant---Line-A" \
  -e API_URL=https://management.umh.app/api \
  management.umh.app/oci/united-manufacturing-hub/umh-core:latest
```

| Variable          | Purpose                                                                                                 |
| ----------------- | ------------------------------------------------------------------------------------------------------- |
| `AUTH_TOKEN`      | **Required.** Authorises the agent with the cloud console (easy to revoke/rotate there)                 |
| `RELEASE_CHANNEL` | `stable` (default) \| `nightly` \| `enterprise` – controls automatic updates shipped with the container |
| `LOCATION_0..n`   | Human-readable hierarchy shown in the console (e.g. `Company-Plant-Line`)                               |
| `API_URL`         | Where the agent calls home; defaults to the public SaaS URL                                             |
| `LOGGING_LEVEL`   | Optional: `DEBUG`, `INFO` (default), `WARN`, `ERROR`                                                    |

You can get the AUTH_TOKEN from the Management Console. You can run umh-core without it, but then you will lack the option to supervise and control your components via the web UI.

When the container starts it looks for **`/data/config.yaml`**. If the file is missing, the agent creates an empty skeleton that you (or the console) can later fill. When you use the Management Console and add there a new DFC, the agent will automatically update that config file as well. So you can use umh-core via CI/CD pipelines, as well as via the Management Console.

---

## 3  Folder & Volume Layout

```
/data
 ├─ config.yaml           # Desired-state YAML (watched continuously)
 ├─ logs/                 # Rolling logs for agent, every DFC, Redpanda …
 ├─ redpanda/             # Broker data & WALs (backup-worthy)
 └─ hwid                  # Device fingerprint sent to the console
```

Mount **one persistent volume** (e.g. `umh-core-data`) to `/data` and you’re done.

---

## 4  Configuration File (what *you* edit)

```yaml
agent:
  metricsPort: 8080
  communicator:
    apiUrl: https://management.umh.app/api
  releaseChannel: stable
  location:
    0: My-Enterprise
    1: My-Plant
    2: My-Line

dataFlow:
  - name: hello-world-dfc
    desiredState: active
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

### 4.1  What’s **not** in the file?

Everything under the top-level key `internal:` is reserved for UMH engineers (built-in services, monitors, etc.).
You should never touch it; in the console UI those fields stay hidden.

---

## 5  Data Flow Components (DFCs)

A **DFC** is a Benthos pipeline wrapped with health checks and lifecycle management. Use them to

* Convert protocols (OPC UA → MQTT, REST → Kafka …)
* Bridge data between brokers
* Enrich or clean messages on the fly

### 5.1  Lifecycle states

| State        | Meaning                                                                      |
| ------------ | ---------------------------------------------------------------------------- |
| **Stopped**  | Config exists, pipeline is off                                               |
| **Starting** | S6 starts Benthos, config loads, waiting for health OK & clean log window    |
| **Idle**     | Running but **below activity threshold** (no messages processed for ≈ 2 min) |
| **Active**   | Running **and** processing data                |
| **Degraded** | Running **but** health probes fail **or** recent logs/metrics show errors    |
| **Stopping** | Graceful shutdown in progress                                                |

### 5.2  What flips a DFC to *Degraded*?

*Evaluated over the last 10 min **or** 10 000 log lines — whichever is shorter.*

1. **/ping or /ready** return non-OK (checked once per second, curl timeout 1 s)
2. **Logs** show a fatal prefix, any `error` level, or a `warn` containing *failed to*, *connection lost*, *unable to*
3. **Metrics** have `error > 0` on any output **or** any processor

When all three are green again the DFC auto-returns to **Idle** (or **Active** if messages flow).

### 5.3  How *Idle* ⇄ *Active* is decided

* After ≈ 120 s without a message → **Idle**
* First new message → back to **Active**

### 5.4  Redpanda store-and-forward layer

Every UMH Core instance ships with **one embedded Redpanda node** that listens internally on **`localhost:9092`**.

| Feature                  | Default / behaviour                                                                                                                    | Why it matters                                                                                |
| ------------------------ | -------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| Always-on buffering      | Redpanda starts automatically (`desiredState: active`)                                                                                 | Ride out network outages without data loss                                                    |
| Zero mandatory wiring    | DFCs **don’t write to Redpanda unless you say so** – set `output.kafka.addresses: ["localhost:9092"]`                                  | Lets a PoC stay simple; you choose when to buffer                                             |
| Architecture forward-fit | Forthcoming *Protocol Converter* DFCs will *always* write to Redpanda; you add a *Bridge DFC* to fan out (HTTP, MQTT, another Kafka …) | Standardises buffering & replay, avoids vendor lock-in                                        |

---

## 6  Startup timeline (from code)

```
Stopped ──▶ Starting
            • S6 service is up
            • Wait 5 s uptime to deem “config loaded”
            • /ping & /ready healthy
            • No critical logs/metrics for 10 s
            ──▶ Idle      (≤ 15 s total or retries kick in)
Idle ──▶ Active as soon as throughput rises
```

---

## 7  Sizing guide

**Start with → 2 vCPU · 4 GB RAM · 40 GB SSD**

### What that box handles

* **≈ 9 protocol-converter DFCs** (e.g. OPC UA ➜ Redpanda) **plus one bridge DFC** that forwards from the local Redpanda to an external MQTT broker
* **≈ 900 tags at 1 message / second each**
* Keeps **seven days** of history under the default cluster retention (`log_retention_ms = 7 days`)
* Runs comfortably below 70 % CPU and I/O on a Hetzner CAX21 / CX32-class VM or Raspberry Pi 4

### Disk usage in practice

Redpanda writes **128 MiB segments**; a segment can be deleted only after it is closed.
With Snappy compression, a typical 200 B JSON payload shrinks to ≈ 20 B.
Allowing a 5 GB safety buffer, a 40 GB SSD gives **≈ 35 GB usable history ≙ \~2.8 billion messages**.

*Need more?*
Shorten retention (either during install with `internal.redpanda.redpandaServiceConfig.defaultTopicRetentionMs` or later on the topic level using `rpk`) or enlarge the disk.

### Memory

| Component           | Rule of thumb                                    |
| ------------------- | ------------------------------------------------ |
| Redpanda            | ≈ 2 GB · cores + 1.5 GB head-room (Seastar rule) |
| Agent + supervision | ≈ 150 MB                                         |
| Each extra DFC      | ≈ 100 MB                                         |

### CPU

*One core* is kept busy by Redpanda. *One additional core* comfortably covers the agent and the first dozen DFCs doing light transforms. Heavy parsing, encryption, or synchronous HTTP calls may warrant more cores or a faster CPU.

### Easy vertical scaling

UMH Core is stateless besides the **`/data`** volume. To grow:

1. Stop the container
2. Move or resize the volume / attach it to a bigger VM
3. Start the same image — no reinstall or re-configuration required

---

## 8  Health checks & self-healing

* **Heartbeat:** every 1 s the monitor fetches `/ping`, `/ready`, `/version`, `/metrics` (1 s HTTP timeout)
* **Automatic restarts:** crash → immediate restart; after any *rate-limited* action the agent waits before repeating it
* **Back-off:** repeated failures increase the delay (constant + jitter) but retries never stop until config changes

---

## 9  Metrics

`http://<device-ip>:8080/metrics` (Prometheus format) exposes:

* Agent tick & FSM timings (each full reconcile loop < 100 ms by design)
* Per-DFC counters: processed, error, latency, active / idle flag
* Redpanda I/O and disk-utilisation stats

---

## 10 Updating

To update umh-core, simply stop the container, pull the latest image, and start the container again.

```bash
# stop + delete the old container (data is preserved)
sudo docker stop umh-core
sudo docker rm umh-core

# pull the latest image and re-create
sudo docker run -d \
  --restart unless-stopped \
  -v "$(pwd)/umh-core-data":/data \
  management.umh.app/oci/united-manufacturing-hub/umh-core:<NEW_VERSION>
```

Need to roll back? Just start the previous tag against the same `/data` volume.

You can find the latest version on the [Releases](https://github.com/united-manufacturing-hub/united-manufacturing-hub/releases) page.
