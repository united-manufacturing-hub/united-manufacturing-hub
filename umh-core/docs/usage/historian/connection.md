# Connection

Each instance has one Historian connection, which every Historian bridge on the instance uses. This
page lists what the database needs and the fields of the connection form.

## Requirements

- TimescaleDB on PostgreSQL 16 or newer, reachable from the umh-core container. Older versions reject
  hyphens in location paths such as `line-1`.
- The `timescaledb` and `ltree` extensions.
- An existing database.
- A login role, created before the first Historian bridge starts. The bridge creates the `umh` schema
  as this role. A database-level grant is enough:

  ```sql
  CREATE ROLE umh_owner WITH LOGIN PASSWORD 'change-me';
  GRANT CREATE, CONNECT ON DATABASE umh TO umh_owner;
  ```

The [Recommended UMH Stack](../../production/deployment/docker-compose/additional-services/recommended-umh-stack.md),
called Getting Started Stack in the Management Console, meets these requirements and uses the
`postgres` user. To add only the database to a running instance, see
[TimescaleDB](../../production/deployment/docker-compose/additional-services/timescaledb.md).

## Connection fields

The form opens from **Plugins** → **Add plugin** → **Historian**, or from **Edit Config** on the
Historian overview page.

| Field | Default | Notes |
|---|---|---|
| **Host** | required | Hostname or IP address, resolved from inside the umh-core container. The Getting Started Stack uses `pgbouncer`. |
| **Port** | `5432` | Point this at PgBouncer if you use one. |
| **Database** | `umh` | Must already exist. |
| **User** | `umh_owner` | The login role from the requirements. The Getting Started Stack uses `postgres`. |
| **Password** | required | Stored in `config.yaml`, hidden in the UI and redacted in logs. |
| **SSL Mode** | `require` | `require` or `disable`. |

`require` encrypts the connection without verifying the server certificate. Use `disable` only when
the database or PgBouncer has no TLS, as in the Getting Started Stack. `verify-full` can only be set
in `config.yaml`; see [Configuration](README.md#configuration).

## PgBouncer

Every Historian bridge opens its own database connections, and PostgreSQL accepts about 100 by
default. Put PgBouncer in front of the database and enter its host and port in the form. The Getting
Started Stack includes it.

## Editing and removing

Editing the connection updates every Historian bridge on the instance. Leave the password field
empty to keep the stored password.

**Delete** in the menu of the Historian card removes the connection. Historian bridges on the
instance then fail, and the stored data stays in the database.
