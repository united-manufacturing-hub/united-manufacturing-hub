# Historian bridge

A Historian bridge has only a write flow. It reads one data contract from the Unified Namespace and
writes it into TimescaleDB. Its output is described in the
[Historian output reference](https://docs.umh.app/benthos-umh/output/historian).

## Which topics the bridge reads

The bridge reads one data contract at its location and the levels below it. The topic table picks the
latest version of the data contract; later versions are not added on their own. The topic follows
the rules in [Topic validation](../data-flows/write-flow.md#topic-validation).

## Data models with a dash in the name

A data model with a dash in its name, such as `pump-a`, cannot be stored, because its name becomes part
of the PostgreSQL table names. Older data models can have one; new names reject dashes.

## Destination fields

The fields in **Destination** are the settings of the
[Historian output](https://docs.umh.app/benthos-umh/output/historian#configuration), which lists
their defaults and accepted values. Two fields are read-only: **Historian Connection** comes from the
instance's [connection](setup.md), and **Data Contract** follows the source topic.

### Settings applied once

**Compress After**, **Retention** and the two chunk intervals apply only when the bridge creates its
tables, and are read-only after that. Before the first deployment, Advanced Settings shows **Table
settings apply once**; afterwards it shows **Table settings are fixed**. See
[Historian parameters](how-the-historian-stores-data.md#historian-parameters).

## What the bridge writes

For the data contract `pump`, the bridge writes two tables in the `umh` schema:

| Table | Contents |
|---|---|
| `umh.value_pump` | One row per tag and timestamp. Numbers and booleans in `value_num`, strings and JSON in `value_text`. |
| `umh.attribute_pump` | The message metadata as JSON, written when a tag's metadata changes and after the bridge reconnects to the database. |

All versions of the data contract write to these two tables. See [Database schema](database-schema.md).
