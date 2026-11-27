# Product Features and Supported Paths

For some jobs, UMH Core offers more than one way to get them done. This page says which way we intend you to use, and what we promise about each of the others, so that you can tell a route we will keep working from one you will have to leave later.

Three kinds of thing appear in this documentation.

## Product features

A product feature is the intended way to do something. It has its place in the Management Console, UMH Core checks what you give it, we keep building on it, and you can ask us to change it.

Most of this documentation describes product features. Where a page says nothing about status, it describes a product feature.

## Supported paths

A supported path is not a product feature. It is a documented way to do a job that no product feature covers.

We keep a supported path working, and we do not build new capability on it. It does not get its own screen in the Management Console, and it does not grow new options. A fault in it is still a fault, and we still fix it.

If a supported path is not enough for what you need, tell us what you are trying to do. That is a request for the product feature that would do the job properly, and that is the request we can act on.

When such a feature ships, the supported path keeps working. Moving to the feature is your choice, not something we make you do.

### Supported paths today

**Stand-alone flows.** A [stand-alone flow](usage/data-flows/stand-alone-flow.md) gives you direct access to Benthos for a job that bridges and stream processors do not cover, such as reading from a database or writing a file. We will keep stand-alone flows working. The product features we build for data flows are bridges and stream processors.

## Provisional paths

A provisional path works today and may change. If it changes, you may have to move your setup to something else.

Use a provisional path when you need it, and tell us when you do, so that we know who to warn. Where we already know what will end one, the list below says so.

### Provisional paths today

**Unchecked data contracts.** A write flow needs a data contract, and today only time-series contracts exist. An instance started with `BYPASS_WRITE_FLOW_VALIDATION=true` accepts a contract name it does not know, so a write flow can carry data that no data model describes. The bridge stays degraded for as long as it uses one, and the Management Console names the contracts it could not check. This path ends when custom data contracts ship, and setups that use it will have to move to a real contract.

**Linking two UMH Core instances with Kafka bridges.** Two instances can exchange data over Kafka today. This path ends when the `umh-core-API` plugin ships, which is the intended way to connect one instance to another.

## Escape hatches

An escape hatch is a setting that lets you past a check UMH Core makes. A path is a way of working, so the two are different things, and something can be both: you reach the unchecked-data-contract path above by setting an escape hatch.

An escape hatch stays available for as long as the check does, because the two exist together. While one is set, the instance tells you so, which is how it does not stay set by accident. [Turn off bridge admission](usage/data-flows/bridges-do-not-start.md) is one.

## Early access features

An early access feature is a product feature we are still finishing, not a path. It can change or break without notice, and you switch it on yourself under **Settings**, **Early Access Features** in the [Management Console](management-console/README.md).

The difference matters when you decide what to build on. An early access feature is going to become the intended way to do its job. A path is not, however long we keep it working.
