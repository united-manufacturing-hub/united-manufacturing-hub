# FAQ – UMH Core vs UMH Classic

## Why a single Docker container?

* **Deployment complexity**

  * *UMH Classic*: many pods, sidecars, service meshes → a forest of YAML.
  * *UMH Core*: **one image, one command**, sub-second startup.

* **Observability & recovery**

  * *Classic*: error clues spread across pod logs, `kubectl` events, load-balancers. Kubernetes states (PodPending, Running, etc.) is usually not want the user woudl expect (Running does not mean the pod is healthy)
  * *Core*: **each component has well-defined states** and is recovered if the state is not healthy (e.g., incl. watching out for error or warning logs, metrics checks, etc.); S6 restarts failed processes instantly.

* **Version management**

  * *Classic*: drift between Node-RED plug-ins, broker, bridges.
  * *Core*: *bump one line in the Dockerfile → ship vX.Y* — everything already integration-tested.

* **Edge resource footprint**

  * *Classic*: edge boxes fight the kube-control-plane for CPU/RAM.
  * *Core*: no kubelet, no sidecars, no HiveMQ ⇒ **lighter RAM/CPU usage**.

We collapse the stack into a single supervised image, giving enterprises less integration pain and users deterministic behaviour. Inside that image the **Agent, Bridges, Redpanda, and Benthos** still run as separate S6-managed processes, so you keep modularity without the network overhead.

##  Isn't that just going back to a monolith?  *(Micro-services vs Monolith)*

A "modular monolith" is the sweet spot for most factories:

* **Micro-services tax** – separate repos, CI/CD pipelines, service discovery, network latency – only pays off when you reach FAANG-scale org size.
* **UMH Core** keeps clear boundaries (each Bridge, each pipeline, its own process) **inside one runtime**. Start simple, split when it hurts – industry trend is swinging back that way.

---

##  "So you ditched Kubernetes?  I thought you were cloud-native!"

We didn't ditch it—**we de-coupled from it**.

* **UMH Core is *more* k8s-friendly** because it's self-contained. Drop the same image into k3s, OpenShift, EKS or bare Docker; no hidden assumptions about CNIs, StorageClasses, or LBs.
* With one container, those variables disappear. **Kubernetes becomes an optional scheduler, not a hard dependency.**

---

##  Do we lose Kubernetes advantages?

No – the two big ones are still there:

**Scalability**: Run multiple `umh-core` replicas behind a Service/LB. Future: Redpanda clustering to shard partitions
**Fail-over**: Shared volume (NFS/Longhorn) lets a standby replay Redpanda's log; any scheduler (k8s, Portainer, systemd-docker) restarts the pod/container.

Manufacturing rarely needs layer-7 load-balancing (100 k msg/s is peanuts vs IT), but it's still possible if you want it.


##  Why is Kafka (Redpanda) embedded instead of its own container?

The UNS needs a **policy wall**. Every byte passes schema validation before it hits the log. Exposing raw Redpanda ports would let clients inject malformed data, bypassing Bridges. Embedding gives us:

* On-the-fly topic/ACL/retention re-config from the Agent.
* Enforcement of Matthew Parris' **portal / bridge / firewall** model.

---

##  Where is MQTT?

1. **Bring-your-own broker** (HiveMQ CE, Mosquitto) in a side-car, then connect via a Bridge.
2. **UMH MQTT plug-in** (road-map) – a slim container that wraps Redpanda with an MQTT façade plus schema enforcement.

##  Where is Benthos running?

Each Bridge launches its own Benthos pipeline **inside** the image; S6 supervises it. If that flow crashes, S6 restarts just that pipeline, leaving the rest untouched.

##  Where is Node-RED?

Outside the image, on purpose. Node-RED plug-ins break often; letting plant IT run their preferred image means **we don't accidentally nuke custom flows** during an update.

---

## Why not ship "just a Helm chart"?

Kubernetes distros vary wildly (OpenShift SCCs, Rancher CNIs, air-gapped k3s, …). One universal chart devolves into a spaghetti of `if/else` we can't certify. **A single image sidesteps 90 % of those edge-case failures.**

---

## Why not let users own the Helm chart (self-managed)?

Our value prop is **curated integration + SLA**. If every customer patches their own chart, swaps Grafana versions, tweaks sidecars, we lose traceability and can't guarantee hot-fixes.

---

##  How do I migrate from UMH Classic?

1. **Side-by-side:** deploy `umh-core` next to Classic, point a new Bridge at the old UNS topics.
2. **Cut-over:** switch producers to the new UNS, retire Classic components incrementally (Node-RED flows, Grafana, Timescale).
3. **Clean-up:** decommission the Helm stack once data is stable in UMH Core.

## What happened to Grafana, Timescale (Historian), and other "extras"?

**Short answer:** anything that needs heavy customization or third-party plug-ins now lives **outside** the core image, so you keep full version control.

* **Grafana + TimescaleDB (Historian)**
  Deployed as a separate Docker or Helm package—or via the upcoming *UMH Historian* plug-in. Many plants already run their own Grafana stack or rely on community plug-ins; we don't want to auto-upgrade and break dashboards.

* **Node-RED**
  Lives in its own container for the same reason: community plug-ins change fast, and site teams often pin specific versions.

* **MQTT broker**
  Bring your own HiveMQ CE, Mosquitto, etc., or use the soon-to-ship *UMH MQTT* plug-in, which wraps Redpanda with an MQTT façade plus schema enforcement.

**Rule of thumb:** if a service demands lots of plug-ins or customization, we keep it out of the core image and provide a starter Compose/Helm file instead.


## How does UMH Core track health if it's not using Kubernetes pod states?

* Every component (Bridge, Benthos pipeline, Redpanda, etc.) is wrapped in a **finite-state machine** managed by the Agent: `starting → active → idle → degraded → stopped`.
* The Agent evaluates **metrics + logs** every 100 ms. A Bridge stuck in a retry loop goes to `degraded`, a pipeline with zero throughput stays `idle`, etc.
* States and counters are exported via `/metrics`, so you can alert on "connection *degraded* for > 30 s", not just pod restarts.

This offers **finer-grained insight** than Kubernetes' generic `Running/CrashLoopBackOff`.


## How do I connect my MQTT devices now?

Run your own broker (Mosquitto, HiveMQ CE, EMQX, etc.), then create a Bridge in UMH Core that subscribes to the topics you care about and inserts the messages into the Unified Namespace.
This keeps MQTT exactly where most plants like it—outside the UNS as a "portal" layer—while UMH Core enforces schema and ACLs before data lands in the clean store, and before you forward it to other nodes.

## I don't want Kafka / Redpanda — I want MQTT to be **my** UNS

No problem.
UMH Core's internal Redpanda is just a **hidden workhorse** for buffering and exactly-once processing; you don't interact with it directly.

1. **Keep (or spin up) your own MQTT broker** — Mosquitto, HiveMQ CE, EMQX, or the upcoming UMH MQTT plug-in.
2. **Create a Bridge** in UMH Core that subscribes to—or publishes from—the topics you choose.
3. From an application point of view, your Unified Namespace *is* MQTT.
   Redpanda just ensures messages are durable, ordered, and schema-checked before they reach consumers.

You never have to operate or even "see" Redpanda; UMH Core embeds and manages it automatically.

## How do I connect two UMH Core instances?

**Short answer:** create a **Bridge** on each side that speaks the upcoming *umh-core-API* output/input plug-in.

Why **HTTP**?

* It's the most firewall-friendly protocol in enterprise networks.
* SSL/TLS is easy to terminate and audit.
* A mature tool-chain (proxies, WAFs, tracing) already exists.
* With idempotent writes we achieve MQTT-style **QoS 1** (at-least-once, de-duplication on the receiving side).

*(Until this feature ships, you can already link cores via Kafka Bridges.)*

## How can I **view** or **query** my Unified Namespace?

* **Topic Browser** (road-map) – a point-and-click tree inside the Management Console to inspect live values.
* **REST API** – two endpoints:

  1. `POST /api/v1/ingest` – push messages into UMH Core (used by the core-to-core Bridge).
  2. `GET  /api/v1/uns/tree` – fetch the namespace tree, including latest values and metadata.

These endpoints are secured by mTLS or JWT and respect the same ACLs the Bridges enforce.

---

## What is open-source now?

* **Was:** only the Helm chart was OSS; the Agent and most internals were closed-source.
* **Now:** with UMH Core **everything except the Management Console is Apache-2**.
  * Agent, Bridges, S6 service definitions, Redpanda integration, FSM health logic – all public.
  * The Management Console UI remains free-to-use (Community Edition) but source-available later this year. 