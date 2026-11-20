# Documentation Guidelines

These docs are a GitBook space, published at docs.umh.app. Follow GitBook's editing reference for syntax, blocks, `SUMMARY.md` and `.gitbook.yaml`: <https://gitbook.com/docs/skill/write-docs.md>

Write and review pages with the `writing-standards:umh-documentation` skill. It holds the page shapes, readers, linking and evidence rules. This file holds only what is specific to these docs. For usage pages, the shape in [Usage pages](#usage-pages) replaces the skill's feature shape.

## Page types

Each page has one [Diátaxis](https://diataxis.fr/) type: tutorial, how-to, reference or explanation. Getting Started is tutorials, Reference is reference, and Production is how-to and explanation. A usage page is the one deliberate mix: the explanation and the reference of one component or part, so a reader looks up a part on one page. [Child pages](#child-pages) lists what goes below it.

## Readers

Unless a page says otherwise, its primary reader is the person who installed and runs the instance ([Who we build for](https://engineering.umh.app/product/who-we-build-for.md)). For Bridges it is the OT team. For Metrics and for the steps on the Docker host it is IT. Those pages open with what they explain and what access the steps need, for example "This page explains how to collect UMH Core metrics with Prometheus. The steps need access to port 8080 on the Docker host."

## Usage pages

The usage pages mirror the instance details page in the Management Console. That page shows one card per component: Agent, Container, Communicator, Data Flows, Redpanda and Topic Browser. Each card has its own usage page. The Data Flows card leads to the Data Flows section, with one page each for Bridges, Stand-alone Flows and Stream Processors.

Subpages follow the component's internal structure in umh-core, the FSMv1 or FSMv2 state machines that run it. The Management Console follows the same structure. For example, a bridge consists of a connection, a read flow and a write flow, and the UI shows each of them. Name a subpage the way the UI names that part. Explain a part that has no subpage through what it does for the user and the metrics it produces.

A component page, such as Bridges, and a part page, such as Read Flow or Write Flow, hold no steps. Pages written before this rule, Bridges among them, still do. Move their steps when you rework the page, and do not copy their shape. Leave out any section that doesn't apply, and keep this order:

1. What the component or part is, in the primary reader's words, with no internals.
2. What the Management Console shows. One sentence on what a status means is fine. The steps to fix it go to a troubleshooting child page.
3. The rules of the component or part: what it accepts, what it rejects and how it behaves. Details for IT or support go last in this section.
4. `## Reference`, the last section of the page, with these subsections. Only a short list of next steps may follow it.
   - `### Configuration`: the `config.yaml` keys and environment variables. For each, give the default and whether a change needs a restart or a new container (`docker restart` keeps the old environment).
   - `### Metrics`: what the component or part tells the operator, one or two alert queries, and a link to where the metrics are defined in code. Link to the "Read the metrics" section of `usage/instances/agent/metrics.md` with a path relative to the current page. Do not list every series, because names and help texts change with the code. Explain the frameworks, the `exported_instance` rename, Benthos and the `-1` sub-state once, on the agent Metrics page, and link there.

Do not describe known defects on any page. Add the docs update to the ticket that fixes them.

### Child pages

- Steps live in a guide, a child of the part the steps change. A guide that changes several parts, such as creating a bridge, is a child of the component.
- A troubleshooting page is a child of the page whose screen shows the symptom.
- The title tells the kind of page, because the sidebar has no group headings: a noun is something to understand ("Write Flow"), an imperative is something to do ("Set up a write flow"), a symptom is something that went wrong ("Bridges Do Not Start").
- Order the children the same way: parts first, then explanations, then guides, then troubleshooting.

## Config snippets

`config.yaml` is parsed strictly: one unknown key rejects the whole file. A few keys are silently ignored in the wrong place instead, such as `debug_level` inside `protocolConverterServiceConfig`. Check every snippet against the structs in `umh-core/pkg/config` on `staging`.

## Terms

Use one word per concept on every page. Literal UI labels, error messages, YAML keys and code identifiers stay as they are, even when they use a word the table replaces. Quote them as labels or code, and use the table's term in the prose around them.

### Bridges and flows

<!-- vale off -->

| Concept | Use | Instead of |
|---|---|---|
| What a bridge talks to | **external system**; **device** only when it is hardware, such as a PLC or a sensor | connection, destination, target, data source, endpoint |
| The reachability check | **connection** for what the Management Console shows; **connection probe** for what umh-core does (the nmap TCP check and its states) | connection check, health check, liveness check, network probe; never the external system itself |
| Direction | a flow is named after what it does **at the external system**: a read flow reads from it, a write flow writes to it | brings, sends, pushes, pulls, ingests, forwards |
| Picking UNS topics | the write flow **subscribes to** topics | receives |
| A regular expression over topics | **topic pattern**; a **topic** is one concrete UNS topic | "a topic is a regular expression" |
| Location | **bridge location** = instance location plus the bridge's levels | agent location; mixing Level and Location |

<!-- vale on -->

The words in the table that are wrong in every context are also [Vale](https://vale.sh) rules in `.vale/styles/UMH/`. Change the table and the rules together. To check your pages, run `vale .` from `umh-core/docs`, for example with `mise exec vale -- vale .`. CI adds the same warnings to a pull request, on the lines it adds. Words that are right in some contexts, such as device, endpoint, receives or connection, have no rule. Check them in review.

- The read flow publishes what it reads to the UNS, and the write flow writes to the external system what it subscribed to. "Reads from the PLC" and "writes to the MES" need no direction words.
- **Instance location** is `agent.location` in `config.yaml`, or the `LOCATION_<n>` environment variables. The bridge's levels are `protocolConverterServiceConfig.location`, set in the **Level _n_** fields of the bridge. They extend the instance location and do not overwrite its levels. A **location path** is the dotted form of a location, such as `acme.cologne.packaging`, and the value of `{{ .location_path }}`.
- **Destination** and **Output** are two words on purpose. Destination is the visual layer: the **Destination** card of a write flow and the `destination:` key. Input, processing and output are the Redpanda Connect layer, which a user edits in code mode and in Benthos YAML. Do not use one for the other.
- "Connection" also keeps its network meaning, for example a TCP connection, a database connection or the connection to the Management Console. Only the thing at the other end of it is the external system.
- "Device model" is a data modeling term and stays.

### Access

- **Permission grant**: what the Management Console issues so that a user or an instance can access instances. Never call it a certificate. "Certificate" means TLS only. Describe what the user sees on screen: "permission grant" or "unlocks access to your X".
