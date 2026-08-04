# Historian

The Historian stores data from the Unified Namespace in a TimescaleDB database, so you can query
history with SQL and build Grafana dashboards on it. The UNS keeps the live value of every tag; the
Historian keeps the past.

{% hint style="info" %}
**Early access.** The Historian is not switched on by default. If you want to use it, get in touch
with us on [Discord](https://discord.gg/F9mqkZnm8U) or through your usual UMH contact, and we will
show you how to get started.
{% endhint %}

## How it fits together

```text
Bridges / Stream Processors
        │
        ▼
  Unified Namespace  ──►  Historian bridge  ──►  TimescaleDB  ──►  Grafana
                          (one per contract)      umh schema
```

You configure the database connection **once per instance**, in the instance's Plugins tab. Every
Historian bridge on that instance then reuses it, so no credentials are copied into bridge configs,
and changing the connection updates the bridges that reference it.

Three steps get you from a running UNS to a dashboard:

1. **Set up the connection** (this page). Tell the instance where its database is.
2. **[Save data to the Historian](save-to-historian.md).** Create one bridge per data contract you
   want to keep.
3. **[Query the data](querying.md).** Copy queries out of the topic browser into Grafana or psql.

## Prerequisites

- **A TimescaleDB database reachable from the umh-core instance.** If you don't have one,
  [Recommended UMH Stack](../../production/deployment/docker-compose/additional-services/recommended-umh-stack.md)
  brings up umh-core, PgBouncer, TimescaleDB, and Grafana together. To add just the database to a
  running instance, see
  [TimescaleDB](../../production/deployment/docker-compose/additional-services/timescaledb.md).
- **PostgreSQL 16 or newer**, with the `timescaledb` and `ltree` extensions available. Version 16 is
  the floor because older `ltree` labels reject hyphens, and location paths such as `line-1` contain
  them.
- **A login role, created before the first bridge starts.** The bridge logs in as this role and
  creates the `umh` schema it owns; it cannot create the role itself. A database-level grant is
  enough, so no privileges on `public` are needed:

  ```sql
  CREATE ROLE umh_owner WITH LOGIN PASSWORD 'change-me';
  GRANT CREATE, CONNECT ON DATABASE umh TO umh_owner;
  ```

- **Data flowing in the UNS.** The Historian archives what is already in the namespace; it does not
  read from devices itself.

## Set up the connection

1. Open the instance, go to the **Plugins** tab, and add the **Historian** plugin.
2. Fill in the TimescaleDB connection:

   | Field | Default | Notes |
   |---|---|---|
   | **Host** | required | Hostname or IP, resolved *from inside the umh-core container*. |
   | **Port** | `5432` | Point this at PgBouncer if you use one. |
   | **Database** | `umh` | Must already exist. |
   | **User** | `umh_owner` | The login role from the prerequisites. |
   | **Password** | required | Stored in `config.yaml`, hidden in the UI, redacted in logs. |
   | **SSL Mode** | `require` | `require` or `disable`. |

3. Save. The instance opens a connection and reports the result on the plugin's overview card.

`require` encrypts the connection but does not verify the server certificate. `disable` turns TLS
off entirely.

Full certificate verification (`verify-full`) is not offered yet: it needs a CA and client
certificate inside the container, and there is no way to upload those from the Management Console.
A connection that already sets `sslmode: verify-full` in `config.yaml` keeps working, along with its
`sslrootcert`, `sslcert`, and `sslkey` paths.

### Reading the connection status

The overview card shows the connection settings plus three live fields, refreshed once per second:

| Field | Meaning |
|---|---|
| **Reachable** | The endpoint answered. It stays `true` even when the server rejects the credentials. Only a network fault or timeout makes it `false`. |
| **Auth** | Whether the server accepted the login role, password, and database name. `unknown` when nothing answered, so authentication could not be checked. |
| **Latency** | Round-trip time of the check query. |

The check is a single `SELECT 1` over one pooled connection. It tells you the database is reachable
and the credentials work; it says nothing about whether a particular bridge is writing. Per-bridge
throughput, health, and errors stay in the **Data Flows** bridge list.

Connections are recycled every five minutes, so a password rotated on the server surfaces as an
authentication failure within that window rather than being masked by a long-lived session.

### Editing and removing

Editing the connection updates every bridge that references it, so you don't touch the bridges. Leave
the password field blank to keep the stored one; the Management Console never receives it back, so
an empty value means "unchanged", not "clear".

Adding a Historian connection is create-only. If one already exists, edit it instead; a repeated
add is rejected rather than silently overwriting your settings.

## Where it is stored

The connection lives in the instance's `config.yaml` as a single shared block:

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

This block is the one place the credentials are written. Every Historian bridge on the instance reads
them from here instead of carrying its own copy.

Bridge templates read it through the reserved `{{ .historian.timescale.* }}` scope: `host`, `port`,
`database`, `username`, `password`, `sslmode`, `sslrootcert`, `sslcert`, and `sslkey`. See
[Variables](../../reference/variables.md).

A bridge counts as a Historian bridge when its write flow's destination protocol is `historian`.
umh-core then targets the bridge's health check at the shared connection rather than at a host and
port entered on the bridge, so the check follows the connection whenever you change it.

## Next steps

- [Save data to the Historian](save-to-historian.md)
- [Query the Historian](querying.md)
