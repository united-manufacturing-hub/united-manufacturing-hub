# SQL Query Returns No Rows

A query from the **SQL query** tab in the Topic Browser returns no rows. The tab shows queries for
every time-series tag, stored or not.

## Cause

- The instance has no Historian connection.
- No Historian bridge stores the tag's data contract.
- The bridge has not written any data yet.
- The bridge does not read the tag, for example because the tag's version is not in its source topic.

## What to do

Check the **Historian** column on **Data Models**. Save the data model if it is not stored; see
[Use the Historian](../use-the-historian.md#step-3-save-the-data-model-to-the-historian). Otherwise
check the bridge's status and logs.
