# Historian

The Historian stores the values of the Unified Namespace in TimescaleDB, a PostgreSQL database for
time series, so you can query them with SQL and chart them in Grafana. You connect the database once
per instance, then save each data model you want to keep. Each saved data model gets its own
Historian bridge.

To set it up, follow [Use the Historian](use-the-historian.md).

## In the Management Console

### Plugins tab

Once a connection is configured, the **Plugins** tab shows the Historian card with the connection
settings, the latency and the connection health.

- **Add plugin** → **Historian** opens the connection form. The fields are described in
  [Connection](connection.md#connection-fields).
- **Open plugin** opens the overview page.
- The card's menu has **Monitor**, **Edit**, **Schema** and **Delete**. **Delete** removes the
  connection; see [Connection](connection.md#editing-and-removing).

### Overview page

The **Database connection** card shows the connection settings and three fields, updated every second:

| Field | Meaning | What to do |
|---|---|---|
| **Reachable** | `Yes` when the database answered, also when it rejects the login. `No` on a network fault or timeout. | On `No`, check the host and port, and that the database runs and is reachable from the umh-core container. |
| **Auth** | Whether the server accepted the user, password and database name. `Unknown` when the database did not answer. | When the login is rejected, click **Edit Config** and correct the user, password or database. |
| **Latency** | Round-trip time of the check query. | |

The **Database status** card shows what the database holds:

| Field | Meaning |
|---|---|
| **Database disk used** | Disk used by the whole database. The tooltip shows the part used by the Historian. |
| **PostgreSQL version**, **TimescaleDB version** | The versions the database runs. |
| **Historian tables** | Number of tables in the `umh` schema. |
| **Historian history covered** | Time between the oldest and the newest stored value. |
| **Historian growth** | Disk the Historian uses per day, from its disk use and the history covered. |
| **Historian failing jobs** | Background jobs whose last run failed. The tooltip shows the total number of jobs. |

If the disk fills faster than planned, see [Historian Sizing](sizing.md). **Monitor database** opens
the Monitor page. **Database schema** opens the [Database Schema page](database-schema.md#database-schema-page).

### Monitor page

The Monitor page reads its figures when it opens and when you refresh it. The **Historian
Hypertables** table has one row per table:

| Column | Meaning |
|---|---|
| **Table** | The table name. |
| **Size**, **Rows** | Disk use and number of rows. |
| **Compression**, **Before compression**, **After compression** | How much compression saves. |
| **Chunks**, **Compressed chunks** | Number of chunks, and how many of them are compressed. |
| **Earliest timestamp**, **History covered** | The oldest stored value, and the time from it to the newest. |
| **Chunk interval**, **Compress after**, **Retention** | The [Historian parameters](historian-parameters.md), as the database holds them. |

The **Background Jobs** table lists TimescaleDB's compression and retention jobs, with their
**Status**, **Job type**, **Table**, **Runs** (how often the job runs), **Last success** and
**Next run**. A failing job leaves data uncompressed or undeleted. Its error is in the TimescaleDB
logs.

### Historian column on Data Models

The **Data Models** page has a **Historian** column:

| The column shows | Meaning | What to do |
|---|---|---|
| **add** | The data model is not stored, and it can be. | Click it to open a new Historian bridge for the data model, as in [Use the Historian](use-the-historian.md#step-3-save-the-data-model-to-the-historian). |
| The bridge's health | A Historian bridge stores the data model. The tooltip names the bridge and the versions it stores. | Click it to open the bridge. When it is degraded, see [Troubleshoot a Historian bridge](troubleshoot-a-bridge.md). |
| A dash (—) | The data model cannot be stored. Hover over it to see why, for example that the instance is offline or that the name contains a dash. | Fix the cause the tooltip names. |

### SQL query tab in the Topic Browser

For every time-series tag, the **SQL query** tab gives a query for Grafana and one for TimescaleDB.
The queries return rows once the Historian stores the tag's data contract; see
[Database schema](database-schema.md#queries-in-the-sql-query-tab).

## Configuration

The connection form writes the `historian:` section of `config.yaml`:

```yaml
historian:
  timescale:
    host: timescaledb.example.com
    port: 5432
    database: umh
    username: umh_owner
    password: change-me
    sslmode: require
```

| Key | Default | Notes |
|---|---|---|
| `timescale.host` | required | Hostname or IP address, resolved from inside the umh-core container. |
| `timescale.port` | `5432` | |
| `timescale.database` | `umh` | |
| `timescale.username` | `umh_owner` | |
| `timescale.password` | required | A literal value. Environment variable references such as `${PGPASSWORD}` are not supported. |
| `timescale.sslmode` | `require` | `require`, `disable` or `verify-full`. The Management Console offers only `require` and `disable`. |
| `timescale.sslrootcert`, `timescale.sslcert`, `timescale.sslkey` | empty | Certificate paths inside the container. Allowed only with `sslmode: verify-full`. |

A change applies without a restart: umh-core reloads `config.yaml` and restarts every Historian bridge
with the new values. Removing the section stops the Historian bridges.

## Metrics

While `config.yaml` has a `historian:` section, umh-core checks the connection every second. The
check runs on FSMv2 and reports the standard [FSMv2 series](../instances/agent/metrics.md#fsmv2-metrics),
with a `hierarchy_path` ending in `timescale-001(historian-timescale)`. Its state is `running` while
the check succeeds and `degraded` while it fails. See
[Read the metrics](../instances/agent/metrics.md#read-the-metrics).

This alert fires when the connection has been down for more than a minute:

```promql
umh_fsmv2_state_duration_seconds{hierarchy_path=~".*\\(historian-timescale\\)", state="degraded"} > 60
```

A rising `umh_fsmv2_state_transitions_total` for this path means the connection keeps dropping. Both
series are defined in
[`fsmv2/supervisor/metrics/metrics.go`](https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/staging/umh-core/fsmv2/supervisor/metrics/metrics.go).
Historian bridges report like any other bridge; see [Bridges: Metrics](../data-flows/bridges.md#metrics).

### Historian output counters

Each bridge's Historian output counts its writes, for example `historian_value_rows_written`,
`historian_rows_poisoned` and `messages_dropped`. These counters stay in the bridge's Benthos process
and do not appear on port 8080 or in the Management Console. The output reference lists them under
[Metrics](https://docs.umh.app/benthos-umh/output/historian#metrics).

## Details for IT

The connection check is one `SELECT 1` over a pooled connection. It shows that the database answers
and the login works, not that a bridge writes; bridge health is in the **Data Flows** list.
Connections are reopened every five minutes, so a password changed on the server shows as an
authentication failure within five minutes.

A bridge whose write flow has the destination protocol `historian` is a Historian bridge. umh-core
checks its health against the instance's connection, so an unreachable database degrades every
Historian bridge on the instance.

Bridge templates read the connection through the reserved variable `{{ .historian.timescale.* }}`
with the keys `host`, `port`, `database`, `username`, `password`, `sslmode`, `sslrootcert`, `sslcert`
and `sslkey`. See [Variables](../../reference/variables.md).
