# Historian

The Historian stores the history of your machine data, so you can see what a machine did last night,
last week or during the last batch. It keeps the values from the Unified Namespace in a TimescaleDB
database, a PostgreSQL database built for time series. You read them back with SQL and build Grafana
dashboards on them.

You connect the database once per instance. Then you choose which data models to store. Each stored
data model gets its own Historian bridge, which writes that model's data contract into the database.
Data models you do not choose are not stored, so the disk holds only data someone needs.

{% hint style="info" %}
**Early access.** The Historian is not switched on by default. If you want to use it, get in touch
with us on [Discord](https://discord.com/invite/F9mqkZnm9d) or through your usual UMH contact, and we will
show you how to get started.
{% endhint %}

To set it up step by step, follow [Use the Historian](use-the-historian.md).

## In the Management Console

### Write flows must be enabled

A Historian bridge is a bridge with only a write flow, which moves data from the Unified Namespace to
the database. Write flows must be enabled in the Management Console before you can save a data model
to the Historian. While they are disabled, **Save to Historian** stays greyed out and says so. This
is part of the early-access setup.

The write flow shows the warning "Write flows do not guarantee delivery yet". A message can be lost
when the destination rejects it or is unreachable. Do not use the Historian for critical data
infrastructure yet.

### Plugins tab

The instance's **Plugins** tab shows the Historian card once a connection is configured. It shows
the database, host, port, user and latency, and the health of the connection.

- **Add plugin** → **Historian** opens the connection form. The fields are described in
  [Connection](connection.md#connection-fields).
- **Open plugin** opens the overview page.
- The card's menu has **Monitor**, **Edit**, **Schema** and **Delete**. **Delete** removes the
  connection; see [Connection](connection.md#editing-and-removing).

### Overview page

The **Database connection** card shows the connection settings and three fields that the instance
updates once per second:

| Field | Meaning | What to do |
|---|---|---|
| **Reachable** | `Yes` when the database answered, also when it rejects the login. `No` on a network fault or timeout. | On `No`, check the host and port, and that the database runs and is reachable from the umh-core container. |
| **Auth** | Whether the server accepted the user, password and database name. `Unknown` when the database did not answer. | When the login is rejected, click **Edit Config** and correct the user, password or database. |
| **Latency** | Round-trip time of the check query. | |

The **Database status** card shows what the database holds. It appears once the connection is
configured and the instance reports its status.

| Field | Meaning |
|---|---|
| **Database disk used** | Disk used by the whole database. The tooltip shows the part used by the Historian. |
| **PostgreSQL version**, **TimescaleDB version** | The versions the database runs. |
| **Historian tables** | Number of tables in the `umh` schema. |
| **Historian history covered** | Time between the oldest and the newest stored value. |
| **Historian growth** | Disk the Historian uses per day, from its disk use and the history covered. |
| **Historian failing jobs** | Background jobs whose last run failed. The tooltip shows the total number of jobs. |

When **Historian growth** fills the disk sooner than planned, set a retention for the next data
model, or reduce the volume; see [Historian Sizing](sizing.md). When a job fails, open the Monitor
page.

**Monitor database** opens the Monitor page. **Database schema** opens the
[Database Schema page](database-schema.md#database-schema-page).

### Monitor page

The Monitor page ("Historian monitoring") reads its figures from the database when it opens, and
again when you refresh it.

The **Historian Hypertables** table has one row per table:

| Column | Meaning |
|---|---|
| **Table** | The table name. |
| **Size**, **Rows** | Disk use and number of rows. |
| **Compression**, **Before compression**, **After compression** | How much compression saves. |
| **Chunks**, **Compressed chunks** | Number of chunks, and how many of them are compressed. |
| **Earliest timestamp**, **History covered** | The oldest stored value, and the time from it to the newest. |
| **Chunk interval**, **Compress after**, **Retention** | The [settings applied once](settings-applied-once.md), as the database holds them. |

The **Background Jobs** table lists TimescaleDB's compression and retention jobs, with their
**Status**, **Job type**, **Table**, **Runs** (how often the job runs), **Last success** and
**Next run**. A failing job means that data is not compressed or old data is not deleted, so the disk
fills faster. The job's error is in the TimescaleDB logs.

### Historian column on Data Models

The **Data Models** page has a **Historian** column:

| The column shows | Meaning | What to do |
|---|---|---|
| **add** | The data model is not stored, and it can be. | Click it to open a new Historian bridge for the data model, as in [Use the Historian](use-the-historian.md#step-3-save-the-data-model-to-the-historian). |
| The bridge's health | A Historian bridge stores the data model. The tooltip names the bridge and the versions it stores. | Click it to open the bridge. When it is degraded, see [Troubleshoot a Historian bridge](troubleshoot-a-bridge.md). |
| Greyed out | The data model cannot be stored. The tooltip says why, for example that the instance is offline or that write flows are disabled. | Fix the cause the tooltip names. |

### SQL query tab in the Topic Browser

For a tag of a stored data contract, the **SQL query** tab gives a ready-to-copy query for Grafana and
one for TimescaleDB. The Management Console runs none of these queries. Otherwise the tab says why
there is nothing to query and links to the fix; see
[Troubleshoot a Historian bridge](troubleshoot-a-bridge.md#the-sql-query-tab-has-no-query). The
queries are described in [Database schema](database-schema.md#queries-in-the-sql-query-tab).

## Configuration

The connection is the `historian:` section of the instance's `config.yaml`. The Management Console
writes it when you save the connection form.

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

A change needs no restart and no redeploy. umh-core reloads `config.yaml` and renders every Historian
bridge again with the new values. Removing the `historian:` section stops the Historian bridges; see
[Connection](connection.md#editing-and-removing).

## Metrics

umh-core checks the Historian connection once per second. The check exists only while `config.yaml`
has a `historian:` section. It runs on FSMv2 (see
[Which framework runs a component](../instances/agent/metrics.md#which-framework-runs-a-component))
and reports the standard [FSMv2 series](../instances/agent/metrics.md#fsmv2-metrics). Its
`hierarchy_path` ends in `timescale-001(historian-timescale)`. The state is `running` while the check
succeeds and `degraded` while it fails, for example when TimescaleDB is unreachable or rejects the
login. To read the metrics, see [Read the metrics](../instances/agent/metrics.md#read-the-metrics).

This alert fires when the connection has been down for more than a minute:

```promql
umh_fsmv2_state_duration_seconds{hierarchy_path=~".*\\(historian-timescale\\)", state="degraded"} > 60
```

A rising `umh_fsmv2_state_transitions_total` for this path shows a connection that keeps dropping.
Both state series are defined in
[`fsmv2/supervisor/metrics/metrics.go`](https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/staging/umh-core/fsmv2/supervisor/metrics/metrics.go).
The database size and the job details appear only in the Management Console.

Historian bridges report like any other bridge. See [Bridges: Metrics](../data-flows/bridges.md#metrics).

### Historian output counters

The Historian output in each bridge counts its own writes, such as `historian_value_rows_written`,
`historian_rows_poisoned` and `messages_dropped`. The bridge's Benthos process keeps these counters.
Neither port 8080 nor the Management Console's Metrics view shows them. The output reference's
[Metrics](https://docs.umh.app/benthos-umh/output/historian#metrics) section lists them.

## Details for IT

**The connection check.** The check is one `SELECT 1` over one pooled connection. It shows that the
database is reachable and the login works. It does not show whether a bridge writes. Each bridge's
health, throughput and errors are in the **Data Flows** bridge list. Connections are reopened every
five minutes, so a password changed on the server shows as an authentication failure within five
minutes.

**Bridge health follows the connection.** A bridge counts as a Historian bridge when its write flow's
destination protocol is `historian`. umh-core checks its health against the instance's connection
instead of a host and port entered on the bridge. When the database is unreachable, every Historian
bridge on the instance shows it.

**Template variables.** Bridge templates read the connection through the reserved scope
`{{ .historian.timescale.* }}`: `host`, `port`, `database`, `username`, `password`, `sslmode`,
`sslrootcert`, `sslcert` and `sslkey`. A bridge variable named `historian` is ignored, because
umh-core reserves the name. See [Variables](../../reference/variables.md).

**Database requirements.** PostgreSQL 16 or newer with the `timescaledb` and `ltree` extensions, and a
login role created before the first bridge starts. See [Connection](connection.md#requirements).
