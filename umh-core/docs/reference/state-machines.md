# State Machines

**State machines are the core orchestration mechanism in UMH Core.** Every component is managed by a finite state machine (FSM) with clearly defined states and transitions. This provides predictable, observable behavior and enables reliable error handling and recovery.

## How It Works

UMH Core uses hierarchical state machines where components build upon each other:

- **Bridge** = Connection + Read Flow + Write Flow
- **Flow** = Benthos instance with lifecycle management  
- **Benthos Flow** = Individual Benthos process with detailed startup phases
- **Connection** = Network probe service (typically nmap-based)

Each component inherits lifecycle states (`to_be_created`, `creating`, `removing`, `removed`) and adds operational states specific to its function. The Agent continuously reconciles desired vs actual state, triggering appropriate transitions based on observed conditions.

## 1 — Redpanda Service

| State           | Verified | What it means                                                                         | How it is entered                                     | How it leaves                                                                        |
| --------------- | -------- | ------------------------------------------------------------------------------------- | ----------------------------------------------------- | ------------------------------------------------------------------------------------ |
| **stopped**     | ✅        | `redpanda` process not running.                                                       | *stop\_done* from **stopping**, or initial create.    | *start* → **starting**                                                               |
| *starting*      | ✅        | S6 launching broker; health checks pending.                                           | *start* event from **stopped**.                       | *start\_done* → **idle**<br>*start\_failed* → **stopped**                            |
| **idle**        | ✅        | Broker healthy, **no data for 30 s** (default idle window).                           | *start\_done* or *no\_data\_timeout* from **active**. | *data\_received* → **active**<br>*degraded* → **degraded**<br>*stop* → **stopping**  |
| **active**      | ✅        | Broker healthy & `BytesIn/OutPerSec` > 0.                                             | *data\_received* from **idle**.                       | *no\_data\_timeout* → **idle**<br>*degraded* → **degraded**<br>*stop* → **stopping** |
| ⚠️ **degraded** | ✅        | Broker running but ≥1 health‑check failing (`disk-space-low`, `cpu-saturated`, etc.). | *degraded* from **idle/active**.                      | *recovered* → **idle**<br>*stop* → **stopping**                                      |
| *stopping*      | ✅        | Graceful shutdown (draining clients).                                                 | *stop* from any running state.                        | *stop\_done* → **stopped**                                                           |

---

## 2 — Container Monitor

| State                | Verified | Meaning                                    | Enter trigger                                                    | Exit trigger                                       |
| -------------------- | -------- | ------------------------------------------ | ---------------------------------------------------------------- | -------------------------------------------------- |
| **active**           | ✅        | CPU < 85 %, RAM < 90 %, Disk < 90 %.       | *metrics\_all\_ok* after monitor start **or** from **degraded**. | *metrics\_not\_ok* → **degraded**                  |
| ⚠️ **degraded**      | ✅        | One of the above limits breached for 15 s. | *metrics\_not\_ok*                                               | *metrics\_all\_ok* → **active**                    |
| monitoring\_stopped  | ✅        | Watchdog disabled.                         | *stop\_monitoring\_done*                                         | *start\_monitoring* → monitoring\_starting         |
| monitoring\_starting | ✅        | Monitor service booting.                   | *start\_monitoring*                                              | *start\_monitoring\_done* → **degraded** (initial) |
| monitoring\_stopping | ✅        | Monitor shutting down.                     | *stop\_monitoring*                                               | *stop\_monitoring\_done* → monitoring\_stopped     |

---

## 3 — Agent Monitor

| State                | Verified | Meaning                                      | Enter                    | Exit                                               |
| -------------------- | -------- | -------------------------------------------- | ------------------------ | -------------------------------------------------- |
| **active**           | ✅        | Agent connected & internal tasks OK.         | *metrics\_all\_ok*       | *metrics\_not\_ok* → **degraded**                  |
| ⚠️ **degraded**      | ✅        | Cloud unreachable / auth error / task panic. | *metrics\_not\_ok*       | *metrics\_all\_ok* → **active**                    |
| monitoring\_stopped  | ✅        | Agent health monitor off.                    | *stop\_monitoring\_done* | *start\_monitoring* → monitoring\_starting         |
| monitoring\_starting | ✅        | Starting health checks.                      | *start\_monitoring*      | *start\_monitoring\_done* → **degraded** (initial) |
| monitoring\_stopping | ✅        | Halting checks.                              | *stop\_monitoring*       | *stop\_monitoring\_done* → monitoring\_stopped     |

---

## 4 — Bridge

### Aggregate Bridge FSM

| State                              | Verified | Meaning                                                      | Status Reason Examples                                           | Enter                                            | Exit                                                                                           |
| ---------------------------------- | -------- | ------------------------------------------------------------ | ---------------------------------------------------------------- | ------------------------------------------------ | ---------------------------------------------------------------------------------------------- |
| **stopped**                        | ✅        | All sub‑services stopped.                                    | `"stopped"`                                                      | *stop\_done* or after create.                    | *start* → **starting\_connection**                                                             |
| *starting\_connection*             | ✅        | Waiting for connection to establish.                         | `"starting: waiting for connection"`                            | *start*                                          | *start\_connection\_up* → **starting\_redpanda**                                               |
| *starting\_redpanda*                | ✅        | Connection up, waiting for message broker.                   | `"starting: redpanda not healthy"`                              | *start\_connection\_up*                          | *start\_redpanda\_up* → **starting\_dfc**                                                      |
| *starting\_dfc*                     | ✅        | Connection + Redpanda up, waiting for flow.                  | `"starting: flow not running"`                                  | *start\_redpanda\_up*                            | *start\_dfc\_up* → **idle**<br>*start\_failed\_dfc\_missing* → **starting\_failed\_dfc\_missing** |
| **starting\_failed\_dfc**           | ✅        | Flow component failed to start.                              | `"starting failed: flow in error state"`                        | *start\_failed\_dfc*                             | Manual retry or removal                                                                       |
| **starting\_failed\_dfc\_missing**   | ✅        | No flow configured.                                          | `"starting failed: no flows configured"`                        | *start\_failed\_dfc\_missing*                    | *start\_retry* (when flow added) or removal                                                   |
| **idle**                           | ✅        | All healthy, **no data for 30 s**.                           | `"idling: no messages processed in 60s"`                        | *start\_dfc\_up*, *no\_data\_timeout*, *recovered* | *data\_received* → **active**<br>*degraded* events → **degraded\_***<br>*stop* → **stopping**   |
| **active**                         | ✅        | Processing data through flows.                               | `""` (empty when fully healthy)                                 | *data\_received*                                 | *no\_data\_timeout* → **idle**<br>*degraded* events → **degraded\_***<br>*stop* → **stopping**   |
| ⚠️ **degraded\_connection**        | ✅        | Connection lost/flaky after successful start.                | `"connection degraded: probe timeout after 30s"`                | *connection\_unhealthy*                          | *recovered* → **idle**<br>*stop* → **stopping**                                               |
| ⚠️ **degraded\_redpanda**          | ✅        | Message broker issues after successful start.                | `"redpanda degraded: not responding"`                           | *redpanda\_degraded*                             | *recovered* → **idle**<br>*stop* → **stopping**                                               |
| ⚠️ **degraded\_dfc**               | ✅        | Flow component issues after successful start.                | `"flow degraded: benthos service not running"`                  | *dfc\_degraded*                                  | *recovered* → **idle**<br>*stop* → **stopping**                                               |
| ⚠️ **degraded\_other**             | ✅        | Inconsistent component states detected.                      | `"other degraded: inconsistent states"`                         | *degraded\_other*                                | *recovered* → **idle**<br>*stop* → **stopping**                                               |
| *stopping*                         | ✅        | Stopping all components.                                     | `"stopping"`                                                     | *stop*                                           | *stop\_done* → **stopped**                                                                     |

### 4.1 Connection Service FSM

| State           | Verified | Meaning                         |
| --------------- | -------- | ------------------------------- |
| *starting*      | ✅        | Probe service launching.        |
| **up**          | ✅        | Target reachable.               |
| **down**        | ✅        | Target unreachable.             |
| ⚠️ **degraded** | ✅        | Flaky / intermittent responses. |
| *stopping*      | ✅        | Probe shutting down.            |
| **stopped**     | ✅        | Probe disabled.                 |

### 4.2 Benthos Flow (Source /Sink)

| State                                                  | Verified | Meaning                                                |
| ------------------------------------------------------ | -------- | ------------------------------------------------------ |
| **stopped**                                            | ✅        | Service file present, process not running.             |
| *starting*                                             | ✅        | S6 launched process.                                   |
| *starting\_config\_loading*                            | ✅        | Benthos parsing YAML pipeline.                         |
| *starting\_waiting\_for\_healthchecks*                 | ✅        | Pipeline loaded; waiting for plugin health.            |
| *starting\_waiting\_for\_service\_to\_remain\_running* | ✅        | Stability grace period.                                |
| **idle**                                               | ✅        | Flow running, no msgs for idle window.                 |
| **active**                                             | ✅        | Processing messages.                                   |
| ⚠️ **degraded**                                        | ✅        | Flow running but error state (e.g., endpoint retries). |
| *stopping*                                             | ✅        | Graceful SIGTERM underway.                             |

> **Idle/Active timeout:** default 30 s (`BRIDGE_IDLE_WINDOW`).

---

## 5 — Topic Browser Service

The Topic Browser service manages real-time topic discovery and caching.

| State | Verified | Description | Enter Trigger | Exit Trigger |
|-------|----------|-------------|---------------|--------------|
| **stopped** | ✅ | Service not running | Initial state or *stop_done* | *start* → **starting** |
| *starting* | ✅ | Service initialization | *start* | *benthos_started* → **starting_benthos** |
| *starting_benthos* | ✅ | Benthos starting | *benthos_started* | *redpanda_started* → **starting_redpanda** |
| *starting_redpanda* | ✅ | Redpanda connection | *redpanda_started* | *start_done* → **idle** |
| **idle** | ✅ | Healthy, no active data | *start_done* or *recovered* | *data_received* → **active** |
| **active** | ✅ | Processing topic data | *data_received* | *no_data_timeout* → **idle** |
| ⚠️ **degraded_benthos** | ✅ | Benthos degraded | *benthos_degraded* | *recovered* → **idle** |
| ⚠️ **degraded_redpanda** | ✅ | Redpanda degraded | *redpanda_degraded* | *recovered* → **idle** |
| *stopping* | ✅ | Graceful shutdown | *stop* | *stop_done* → **stopped** |

**Default:** Active (runs automatically)  
**Transitions:** idle ↔ active based on topic activity  
**Recovery:** Automatic from degraded states when underlying services recover

---

### Quick Defaults

| Parameter              | Default | Source Const / Env     |
| ---------------------- | ------- | ---------------------- |
| Idle window (Redpanda) | 30 s    | `REDPANDA_IDLE_WINDOW` |
| Idle window (Bridge)   | 30 s    | `BRIDGE_IDLE_WINDOW`   |
| Container CPU limit    | 85 %    | `CONTAINER_CPU_LIMIT`  |
| Container RAM limit    | 90 %    | `CONTAINER_RAM_LIMIT`  |
| Container Disk limit   | 90 %    | `CONTAINER_DISK_LIMIT` |