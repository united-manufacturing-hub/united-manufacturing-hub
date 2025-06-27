# State Machines

**State machines are the core orchestration mechanism in UMH Core.** Every component is managed by a finite state machine (FSM) with clearly defined states and transitions. This provides predictable, observable behavior and enables reliable error handling and recovery.

## How It Works

UMH Core uses hierarchical state machines where components build upon each other:

- **Bridge** (formerly Protocol Converter) = Connection + Source Flow + Sink Flow
- **Flow** (DataFlow Component) = Benthos instance with lifecycle management  
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

## 4 — DataFlow Component (Bridge)

### Aggregate Bridge FSM

| State                | Verified | Meaning                                                 | Enter                                            | Exit                                                                                           |
| -------------------- | -------- | ------------------------------------------------------- | ------------------------------------------------ | ---------------------------------------------------------------------------------------------- |
| **stopped**          | ✅        | All sub‑services stopped.                               | *stop\_done* or after create.                    | *start* → **starting**                                                                         |
| *starting*           | ✅        | Launching source & sink Benthos + connection monitor.   | *start*                                          | *start\_done* → **idle**<br>*start\_failed* → **starting\_failed**                             |
| **starting\_failed** | ✅        | At least one sub‑service failed during start.           | *start\_failed*                                  | Manual retry (*start*) or removal                                                              |
| **idle**             | ✅        | Sub‑services healthy, **no payload for 30 s**.          | *start\_done*, *no\_data\_received*, *recovered* | *data\_received* → **active**<br>*benthos\_degraded* → **degraded**<br>*stop* → **stopping**   |
| **active**           | ✅        | Data moving through at least one flow.                  | *data\_received*                                 | *no\_data\_received* → **idle**<br>*benthos\_degraded* → **degraded**<br>*stop* → **stopping** |
| ⚠️ **degraded**      | ✅        | ≥1 sub‑FSM degraded/down (connection lost, flow error). | *benthos\_degraded*                              | *benthos\_recovered* → **idle**<br>*stop* → **stopping**                                       |
| *stopping*           | ✅        | Stopping Benthos + connection monitor.                  | *stop*                                           | *stop\_done* → **stopped**                                                                     |

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

> **Idle/Active timeout:** default 30 s (`DFC_IDLE_WINDOW`).

---

### Quick Defaults

| Parameter              | Default | Source Const / Env     |
| ---------------------- | ------- | ---------------------- |
| Idle window (Redpanda) | 30 s    | `REDPANDA_IDLE_WINDOW` |
| Idle window (Bridge)   | 30 s    | `DFC_IDLE_WINDOW`      |
| Container CPU limit    | 85 %    | `CONTAINER_CPU_LIMIT`  |
| Container RAM limit    | 90 %    | `CONTAINER_RAM_LIMIT`  |
| Container Disk limit   | 90 %    | `CONTAINER_DISK_LIMIT` |