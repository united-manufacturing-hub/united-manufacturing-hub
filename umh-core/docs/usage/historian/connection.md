# Connection

This page describes the Historian connection: what the database needs and the fields of the
connection form. Each instance has one connection, and every
Historian bridge on the instance uses it.

## Requirements

- **TimescaleDB on PostgreSQL 16 or newer**, reachable from the umh-core container. Version 16 is the
  minimum because older `ltree` labels reject hyphens, and location paths such as `line-1` contain
  them.
- **The `timescaledb` and `ltree` extensions** available in the database.
- **The database** already exists. The Historian does not create it.
- **A login role**, created before the first Historian bridge starts. The bridge logs in as this role
  and creates the `umh` schema, which the role then owns. The bridge cannot create the role itself. A
  database-level grant is enough, so no privileges on `public` are needed:

  ```sql
  CREATE ROLE umh_owner WITH LOGIN PASSWORD 'change-me';
  GRANT CREATE, CONNECT ON DATABASE umh TO umh_owner;
  ```

The [Recommended UMH Stack](../../production/deployment/docker-compose/additional-services/recommended-umh-stack.md),
which the Management Console calls the Getting Started Stack, meets these requirements. It creates
the `umh` database and logs in as the `postgres` user, so it needs no `umh_owner` role. To add only
the database to a running instance, see
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

`require` encrypts the connection but does not verify the server certificate. `disable` turns TLS
off. Use `disable` only when the database or PgBouncer has no TLS set up, as in the Getting Started
Stack.

Full certificate verification (`verify-full`) cannot be selected in the Management Console. A
connection whose `config.yaml` already sets `sslmode: verify-full` keeps working, together with its
`sslrootcert`, `sslcert` and `sslkey` paths.

## PgBouncer

Every Historian bridge opens its own connections to the database. PostgreSQL accepts about 100
connections by default. PgBouncer pools the connections of all bridges onto a few database
connections, so put it in front of the database and enter its host and port in the form. The Getting
Started Stack includes PgBouncer.

## Editing and removing

Editing the connection updates every Historian bridge on the instance. You do not edit the bridges.
Leave the password field empty to keep the stored password. The Management Console never receives
the stored password, so an empty field means "unchanged", not "clear".

An instance has at most one connection. Adding a second one is rejected. Edit the existing one
instead.

Removing the plugin (**Delete** in the menu of the Historian card) deletes the `historian:` section
from `config.yaml`. Historian bridges on the instance then fail with an error saying that no valid
`historian:` section is configured. The data already stored stays in the database.

## Where the connection is stored

The connection is stored in the `historian:` section of the instance's `config.yaml`. Its keys,
defaults and reload behaviour are listed in [Historian: Configuration](README.md#configuration).
