# Historian Bridge Topics Rejected

After an upgrade to umh-core 0.45.0, a Historian bridge shows as degraded, or its deploy fails with
"the write flow topics need changes".

## Cause

Historian bridges created before umh-core 0.45.0 read every topic in the Unified Namespace and kept
only their own data contract. From 0.45.0, a write flow may read only one data contract at the
bridge's location and below, so umh-core does not start these bridges.

## What to do

1. Open the bridge, select **Edit Config** and the **Write Flow** tab.
2. In **UMH Topics**, select the data contract the bridge stores and its versions.
3. Click **Save & Deploy**.

If the message names another problem, see
[Write Flow Topics Rejected](../../data-flows/troubleshooting/write-flow-topics-rejected.md), which
explains each message.
