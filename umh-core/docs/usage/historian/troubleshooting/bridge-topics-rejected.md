# Historian Bridge Topics Rejected

A Historian bridge shows degraded, or its deploy fails, with a message about its source topic.

## Cause

umh-core rejected the source topic. Historian bridges created before umh-core 0.45.0 read the whole
Unified Namespace, which 0.45.0 rejects.

## What to do

Open the bridge, pick one data contract in the topic table and save. For a topic written by hand,
see [Write Flow Topics Rejected](../../data-flows/troubleshooting/write-flow-topics-rejected.md).
