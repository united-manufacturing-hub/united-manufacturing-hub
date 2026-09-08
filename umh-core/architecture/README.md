# Architecture model

A [C4](https://c4model.com) model of umh-core, written in
[LikeC4](https://likec4.dev). One model in `umh-core.c4`, three views generated
from it.

This is internal developer documentation. It sits outside `umh-core/docs/`,
which is the GitBook space published to docs.umh.app and requires every page to
be listed in its `SUMMARY.md`.

## The diagrams

Three views, one per C4 level. Open them all with `npx likec4 start`.

The level-1 view is named `index`, which makes it the landing view. LikeC4
auto-generates a landscape view when a model declares no `index`, and for a
model with a single system in focus that landscape and the C4 context diagram
are the same picture.

### `index` — level 1, 5 boxes

Who talks to umh-core and which side opens the connection. The OT engineer,
ManagementConsole, the industrial device, and whatever external system a bridge
writes to.

Read it for one thing: every arrow to the cloud points outward. The engineer
reaches the gateway through the console, never by connecting to it.

### `containers` — level 2, 15 boxes in three groups

What actually runs inside the Docker container, grouped by how many of each
exist: one per bridge, one per instance, and the three things on disk.

This is the view to show someone new. "One Docker container" and "one process"
sound like the same thing until you see this, and the answer is 73 processes on
a 16-bridge instance. The grouping is where that number comes from: the
per-bridge group is drawn once and instantiated sixteen times. The nmap scanner
carries an `fsmv1-only` tag because it does not exist on the fsmv2 backend,
where the agent dials the target itself.

### `agentComponents` — level 3, 15 boxes in four groups

How the single Go process is organised, plus ManagementConsole, `config.yaml`
and the s6 log directories at the edges to show what the process reads and
writes.

The shape to notice is two independent drivers. The FSMv1 reconcile loop and the
FSMv2 supervisor each run on their own clock, the supervisor on its own
goroutine at a 100 ms tick (`cmd/main.go:311`), and neither calls the other.
They meet only at the triangular store, which the adapter reads on FSMv1's
behalf. That is the hardest thing to discover from the code and it is invisible
at level 2.

The entrypoint and the config manager sit outside both groups, because both
drivers use them.

### Level 4

C4's fourth level is code, and Simon Brown's own guidance is to generate it on
demand rather than maintain it. There is a separate FSMv2 deep-dive page with a
package dependency graph, the tick loop and an end-to-end observation trace. It
is not in this repository and has no permanent home yet.

## Working with it

```sh
cd umh-core/architecture

npx likec4 start        # http://localhost:5173, hot-reloads on save
npx likec4 validate     # exit 1 with file and line on a broken model
npx likec4 gen mermaid -o ./out
npx likec4 export png -o ./out
```

Run `validate` in CI. There is a Docker image, `ghcr.io/likec4/likec4`, if you
would rather not use Node.

## Editing the model

Two rules.

The element kinds are exactly `person`, `softwareSystem`, `container`, `store`
and `component`. LikeC4 does not enforce C4's abstraction levels, so a sixth
kind is how the hierarchy stops being C4.

The legend is generated. Every element kind declares a `notation` line, and
LikeC4 renders those as a key in the viewer and the built site. Change the
`notation` string rather than adding a notation table here.

## Acronyms used in the diagrams

| | |
|---|---|
| **UNS** | Unified Namespace. The Kafka topic `umh.messages` that every bridge publishes into. |
| **FSM** | Finite state machine. FSMv1 and FSMv2 are the two generations of the state-machine layer, both shipping today. |
| **DFC** | Dataflow component. The unit a bridge's read flow and write flow are each made of. |
| **s6** | The supervision suite that runs every process in the container. |
| **PLC** | Programmable logic controller. |
| **OPC UA, Modbus, S7** | Industrial protocols a bridge uses to reach a device. |
| **Bridge** | Also called a protocol converter, including in the code (`protocolconverter`). The two are synonyms. |

## What the model asserts, and how to check it

**Every service container is a real s6 service.** The three stores
(`config.yaml`, `s6 log directories`, `Redpanda storage`) are filesystem
locations, not processes, and never appear under `/run/service`. Every other box
maps to a directory there, and the names are built by the `getS6ServiceName`
functions in `pkg/service/<service>/<service>.go`. To list the live set:

```sh
docker exec <instance> sh -c \
  'for d in /run/service/*/; do printf "%-72s %s\n" "$(basename $d)" "$(/command/s6-svstat $d)"; done'
```

That listing shows two things the model does not draw, both expected.
`s6-linux-init-shutdownd`, `s6rc-fdholder` and `s6rc-oneshot-runner` belong to
the s6 overlay itself. Standalone dataflow components and stream processors
appear only once a user configures one.

**The agent opens every outbound connection.** ManagementConsole never dials
in. The agent logs in and then pulls actions and pushes status against
`/v2/instance/*` (`pkg/communicator/api/v2/http/requester.go`), which is why
`context` has no inbound arrow. The agent does bind `:8080` for Prometheus
metrics, and `:8090` for GraphQL when enabled, but nothing in the control plane
dials them.

**The triangular store is in memory.** It is a component of the agent process,
not a store container, because the only implementation is
`pkg/persistence/memory`. A restart loses it, which is why a freshly started
worker resolves as `NeverObserved` instead of reading back its last known state.

**Benthos configs are rendered to disk.** Each service gets its own
`/run/service/<service>/config/benthos.yaml`, written from `config.yaml` with
variables expanded. They are regenerated rather than edited, so they are not a
source of truth, but they are on disk and readable.

## Counting services

Bridge-scoped processes are drawn once each. How many exist per bridge depends
on the nmap backend: four under `NMAP_BACKEND=fsmv2` (a read flow, a write flow
and a monitor for each), and five under the fsmv1 default, which adds the nmap
scanner. There is no monitor for the scanner; its FSM parses the scanner's own
log.

Constant overhead is six services: the topic browser and its monitor, Redpanda
and its monitor, and the agent and its log. The s6 overlay adds three more
(`s6-linux-init-shutdownd`, `s6rc-fdholder`, `s6rc-oneshot-runner`).

A 16-bridge instance therefore runs 73 services on fsmv2 and 89 on fsmv1.
Verified against a live fsmv2 instance: 16 read flows, 16 write flows, 32 flow
monitors, the topic browser and its monitor, two Redpanda services, two agent
services, three s6 overlay services.
