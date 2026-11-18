# New Data Model Version Not Stored

Devices publish a new version of a data model, such as `_pump_v3`, but the Historian does not store
it.

## Cause

The source topic lists the versions that existed when the bridge was saved.

## What to do

Hover over the **Historian** column on **Data Models** to see the stored versions. Add the new
version to the source topic in Code mode, for example `_pump_v[1-3]\.`.
