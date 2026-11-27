# Documentation Guidelines

These docs are a GitBook space, published at docs.umh.app. Follow GitBook's editing reference for syntax, blocks, `SUMMARY.md` and `.gitbook.yaml`: <https://gitbook.com/docs/skill/write-docs.md>

Write and review pages with the `writing-standards:umh-documentation` skill. It holds the page shapes, readers, linking and evidence rules. This file holds only what is specific to these docs. For usage pages, the shape in [Usage pages](#usage-pages) replaces the skill's feature shape.

## Page types

Each page has one [Diátaxis](https://diataxis.fr/) type: tutorial, how-to, reference or explanation. Getting Started is tutorials, Reference is reference, and Production is how-to and explanation. A usage page is the one deliberate mix: the explanation and the reference of one component or part, so a reader looks up a part on one page. [Child pages](#child-pages) lists what goes below it.

## Readers

Unless a page says otherwise, its primary reader is the person who installed and runs the instance ([Who we build for](https://engineering.umh.app/product/who-we-build-for.md)). For Bridges it is the OT team. For Metrics and for the steps on the Docker host it is IT. Those pages open with what they explain and what access the steps need, for example "This page explains how to collect UMH Core metrics with Prometheus. The steps need access to port 8080 on the Docker host."

## Where readers get help

The [Get Help](get-help.md) page is the only page that names where readers turn for help: technical support (Enterprise support and the community on Discord), security reports, and commercial requests. Do not restate these routes or their addresses on another page; link to Get Help. A page's own instructions stay on it: what to collect before asking, any safety warning, and the commercial contact of a licensing or billing step, such as the contact form on the billing page.

## Usage pages

The usage pages mirror the instance details page in the Management Console. That page shows one card per component: Agent, Container, Communicator, Data Flows, Redpanda and Topic Browser. Each card has its own usage page. The Data Flows card leads to the Data Flows section, with one page each for Bridges, Stand-alone Flows and Stream Processors.

Subpages follow the component's internal structure in umh-core, the FSMv1 or FSMv2 state machines that run it. The Management Console follows the same structure. For example, a bridge consists of a connection, a read flow and a write flow, and the UI shows each of them. Name a subpage the way the UI names that part. Explain a part that has no subpage through what it does for the user and the metrics it produces.

A component page, such as Bridges, and a part page, such as Read Flow or Write Flow, hold no steps. Pages written before this rule, Bridges among them, still do. Move their steps when you rework the page, and do not copy their shape. Leave out any section that doesn't apply, and keep this order:

1. What the component or part is, in the primary reader's words, with no internals.
2. What the Management Console shows: the card and its fields.
3. `## <Name> states`: every state or status the badge can show, one table row each, with what it means. One sentence on what a status means is fine. The steps to fix it go to a troubleshooting child page.
4. `## <Name> thresholds`: the values at which a state changes, with the value it degrades at and the value it recovers at. Leave the section out when the component has no thresholds. A reference child page that covers one health check in detail, such as CPU Health, links here for the values.
5. The rules of the component or part: what it accepts, what it rejects and how it behaves. Details for IT or support go last in this section.
6. `## Reference`, the last section of the page, with these subsections. Only a short list of next steps may follow it.
   - `### Configuration`: the `config.yaml` keys and environment variables. For each, give the default and whether a change needs a restart or a new container (`docker restart` keeps the old environment).
   - `### Metrics`: every series the component or part reports, in a table with its name, type and what it reports, then one or two alert queries and a link to where the series are defined in code. Take the list from the code and check it against the `/metrics` output of a running instance. Link to the "Read the metrics" section of `usage/instances/agent/metrics.md` with a path relative to the current page. Explain the frameworks, the standard series every framework component reports, the `exported_instance` rename, Benthos and the `-1` sub-state once, on the agent Metrics page, and link there.

Do not describe known defects on any page. Add the docs update to the ticket that fixes them.

Document what the reader cannot work out alone. State a general rule with the reason behind it instead of listing every case, and leave out what follows from it.

### Child pages

- Steps live in a guide, a child of the part the steps change. A guide that changes several parts, such as creating a bridge, is a child of the component.
- A troubleshooting page is a child of the page whose screen shows the symptom.
- A reference child page holds the detail of one part of the page above it that is too long for that page, such as CPU Health below Container. It follows the same rules as a usage page: no steps and no reasoning about why the design is the way it is.
- The title tells the kind of page, because the sidebar has no group headings: a noun is something to understand or look up ("Write Flow", "CPU Health"), an imperative is something to do ("Set up a write flow"), a symptom is something that went wrong ("Bridges Do Not Start").
- Order the children the same way: parts and reference pages first, then explanations, then guides, then troubleshooting.

## Images

Add an image as a Markdown image, `![alt text](path)`, not as an HTML `<figure>`. The alt text says what the screenshot shows. GitBook shows a Markdown image at the full width of the page, so a screenshot of a small UI element includes enough of the screen around it to read well at that width.

## Config snippets

`config.yaml` is parsed strictly: one unknown key rejects the whole file. A few keys are silently ignored in the wrong place instead, such as `debug_level` inside `protocolConverterServiceConfig`. Check every snippet against the structs in `umh-core/pkg/config` on `staging`.

## Terms

Use one word per concept on every page. Literal UI labels, error messages, YAML keys and code identifiers stay as they are, even when they use a word the table replaces. Quote them as labels or code, and use the table's term in the prose around them.

### Bridges and flows

<!-- vale off -->

| Concept | Use | Instead of |
|---|---|---|
| What a bridge talks to | **external system**; **device** only when it is hardware, such as a PLC or a sensor | connection, destination, target, data source, endpoint |
| The reachability check | **connection** for what the Management Console shows; **connection probe** for what umh-core does (a probe that checks whether the external system is reachable, and its states) | connection check, health check, liveness check, network probe, nmap; never the external system itself |
| Direction | a flow is named after what it does **at the external system**: a read flow reads from it, a write flow writes to it | brings, sends, pushes, pulls, ingests, forwards |
| Picking UNS topics | the write flow **subscribes to** topics | receives |
| A regular expression over topics | **topic pattern**; a **topic** is one concrete UNS topic | "a topic is a regular expression" |
| Location | **bridge location** = instance location plus the bridge's levels | agent location; mixing Level and Location |

<!-- vale on -->

### Product features and paths

<!-- vale off -->

| Concept | Use | Instead of |
|---|---|---|
| A documented way to do a job that no product feature covers, which we keep working | **supported path** | workaround |
| A documented way that works now and may need a migration later | **provisional path** | workaround, temporary solution |
| A setting that lets a user past a check umh-core makes | **escape hatch** | emergency fallback, emergency setting |

<!-- vale on -->

[Product Features and Supported Paths](product-features-and-supported-paths.md) defines all three for readers, and is the only page that does. "Fallback" has no rule, because it is correct in Emergency Access and in "the FSMv1 fallback is gone". Do not use it to name a path.

Every page that names a component or a part of one states its status, because a reader cannot tell an unlabelled page from one nobody has got to yet. A guide or a troubleshooting page below it has the component's status and does not state it.

- A product feature gets one plain sentence after the opening prose, such as "Bridges are a [product feature](../../product-features-and-supported-paths.md)." It gets no hint block, because a product feature needs no warning.
- A supported path, a provisional path or an early access feature gets a `{% hint style="info" %}` block with a bold lead, because each one changes what a reader should build on. Follow the shape `usage/historian/sizing.md` already uses.

An early access notice is the exception to the rule for child pages: it repeats on every page of the feature, guides and troubleshooting pages included, because a reader who arrives from search never passes the parent page.

The words in the table that are wrong in every context, and the direction words, are also [Vale](https://vale.sh) rules in `.vale/styles/UMH/`. Change the table and the rules together. To check your pages, run `make docs-lint` from `umh-core`. It runs Vale in Docker. CI adds the same warnings to a pull request, on the lines it adds. Vale flags the direction words, such as sends or receives, only right after a bridge or a flow. Words that are right in some contexts, such as device, endpoint or connection, have no rule. Check them in review.

- The read flow publishes what it reads to the UNS, and the write flow writes to the external system what it subscribed to. "Reads from the PLC" and "writes to the MES" need no direction words.
- **Instance location** is `agent.location` in `config.yaml`, or the `LOCATION_<n>` environment variables. The bridge's levels are `protocolConverterServiceConfig.location`, set in the **Level _n_** fields of the bridge. They extend the instance location and do not overwrite its levels. A **location path** is the dotted form of a location, such as `acme.cologne.packaging`, and the value of `{{ .location_path }}`.
- **Destination** and **Output** are two words on purpose. Destination is the visual layer: the **Destination** card of a write flow and the `destination:` key. Input, processing and output are the Redpanda Connect layer, which a user edits in code mode and in Benthos YAML. Do not use one for the other.
- "Connection" also keeps its network meaning, for example a TCP connection, a database connection or the connection to the Management Console. Only the thing at the other end of it is the external system.
- "Device model" is a data modeling term and stays.

### Access

- **Permission grant**: what the Management Console issues so that a user or an instance can access instances. Never call it a certificate. "Certificate" means TLS only. Describe what the user sees on screen: "permission grant" or "unlocks access to your X".
