# Documentation Guidelines

These docs are a GitBook space, published at docs.umh.app. Follow GitBook's editing reference for syntax, blocks, `SUMMARY.md` and `.gitbook.yaml`: <https://gitbook.com/docs/skill/write-docs.md>

Write and review pages with the `writing-standards:umh-documentation` skill. It holds the page shapes, readers, linking and evidence rules. This file holds only what is specific to these docs.

## Page types

Each page has one [Diátaxis](https://diataxis.fr/) type: tutorial, how-to, reference or explanation. Getting Started is tutorials, Reference is reference, and Production is how-to and explanation. A usage page is the one deliberate mix, the skill's troubleshooting/feature shape, so a reader in the middle of a problem needs to open only one page. Child pages under a usage page have a single type, usually how-to or troubleshooting.

## Readers

Unless a page says otherwise, its primary reader is the power user ([Who we build for](https://engineering.umh.app/product/who-we-build-for.md)). For Bridges it is the OT team. For Metrics and for the steps on the Docker host it is IT. Those pages open with two sentences that tell the power user when to hand the page to IT.

## Usage pages

The usage pages mirror the instance details page in the Management Console. That page shows one card per component: Agent, Container, Communicator, Data Flows, Redpanda and Topic Browser. Each card has its own usage page. The Data Flows card leads to the Data Flows section, with one page each for Bridges, Stand-alone Flows and Stream Processors.

Subpages follow the component's internal structure in umh-core, the FSMv1 or FSMv2 state machines that run it. The Management Console follows the same structure. For example, a bridge consists of a connection, a read flow and a write flow, and the UI shows each of them. Name a subpage the way the UI names that part. Explain a part that has no subpage through what it does for the user and the metrics it produces.

Leave out any section that doesn't apply, and keep this order:

1. Opening in the primary reader's words, with no internals.
2. What the Management Console shows, and what to do about it.
3. `## Configuration`: the `config.yaml` keys and environment variables. For each, give the default and whether a change needs a restart or a new container (`docker restart` keeps the old environment).
4. `## Metrics`: link [Read the metrics](usage/instances/agent/metrics.md#read-the-metrics) with a path relative to the current page, then list only this component's series as `| Name | Type | Reports |`. If the component can run under either framework, show two visible tables: the default first, then the other one with its switch named.
5. Details for IT or support.

## Config snippets

`config.yaml` is parsed strictly: one unknown key rejects the whole file. A few keys are silently ignored in the wrong place instead, such as `debug_level` inside `protocolConverterServiceConfig`. Check every snippet against the structs in `umh-core/pkg/config` on `staging`.

## Terms

- **Permission grant**: what the Management Console issues so that a user or an instance can access instances. Never call it a certificate. "Certificate" means TLS only. Describe what the user sees on screen: "permission grant" or "unlocks access to your X".
