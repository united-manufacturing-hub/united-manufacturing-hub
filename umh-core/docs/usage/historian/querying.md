# Query the Historian

Once a [Historian bridge](save-to-historian.md) is storing a contract, the data is ordinary
TimescaleDB, and any SQL client or Grafana can read it. This page covers the fastest route, copying
a query out of the topic browser, and the schema behind it for when you write your own.

The Management Console never runs these queries. It generates the SQL and you paste it into Grafana
or psql.

## Copy a query from the topic browser

1. Open the **Topic Browser** and select a tag.
2. Choose the **Grafana** or **TimescaleDB** tab.

   The two differ only in the time filter. Grafana emits `$__timeFilter(ts)`, which the dashboard's
   time picker fills in. TimescaleDB emits a fixed window sized to the resolution, so the query
   returns roughly 60 points when pasted straight into psql.

3. Adjust the controls:

   - **Aggregate** buckets the data with `time_bucket()` and returns `avg`, `min`, and `max`. Turn
     it off for the raw rows. Text tags cannot be aggregated, so the toggle is disabled for them and
     the query always returns raw values.
   - **Resolution** sets the bucket width: `$__interval`, `1 second`, `10 seconds`, `1 minute`,
     `1 hour`, or `raw`. Grafana starts at `$__interval`, which lets Grafana pick a width from the
     panel's time range and width; TimescaleDB has no such macro and starts at `1 minute`.

4. Copy it.

The panel is generated from the topic name, not from the database. It appears for every tag, and a
note says so: a tag that no Historian bridge has stored yet produces a valid query that returns no
rows. When the tag's datatype isn't known yet, the query defaults to the numeric column. Switch it
to `value_text` if the tag holds strings.

## Schema

Values are stored per contract; identity is shared across contracts.

```text
umh.value_pump (topic_id, ts, value_num, value_text)
       │ topic_id
       ▼
umh.topic (topic_id, location_id, tag_id)
       │ tag_id            │ location_id
       ▼                   ▼
umh.tag (tag_id, name,   umh.location (location_id, path)
         virtual_path,
         data_contract_name)
```

`umh.attribute_pump` holds the metadata for the same topics, as a JSON object queryable with
`attribute->>'key'` and `attribute @> '{...}'`.

### Resolving a tag

`umh.get_topic_id(location_path, virtual_path, data_contract, tag_name)` hides that join for
single-tag lookups. It is what the generated queries use:

```sql
SELECT ts, value_num
FROM   umh.value_pump
WHERE  topic_id = umh.get_topic_id('enterprise.site.area.line', '', 'pump', 'temperature')
  AND  ts BETWEEN now() - INTERVAL '1 hour' AND now()
ORDER  BY ts;
```

Three things trip up hand-written queries:

- The timestamp column is **`ts`**, a `timestamptz`, not `timestamp` or `time`.
- A tag with no virtual path stores `virtual_path` as the **empty string**, never `NULL`. Passing
  `NULL` matches nothing and returns an empty result with no error.
- The `data_contract` argument is forgiving: `pump`, `_pump`, and `_pump_v1` all resolve to the same
  tag.

Location paths are canonicalized into an `ltree`: characters outside `[A-Za-z0-9_-]` become `_`.
Hyphens survive, so `line-1` and `line_1` are **different** locations with different `topic_id`s.

### Latest value of every tag

```sql
SELECT DISTINCT ON (v.topic_id)
       l.path::text AS location, g.virtual_path, g.name AS tag, v.ts, v.value_num, v.value_text
FROM   umh.value_pump v
JOIN   umh.topic    t ON t.topic_id    = v.topic_id
JOIN   umh.tag      g ON g.tag_id      = t.tag_id
JOIN   umh.location l ON l.location_id = t.location_id
ORDER  BY v.topic_id, v.ts DESC;
```

This scans each topic's history to find its newest row. That is fine for hundreds of tags. For a
dashboard that refreshes it often, back it with a continuous aggregate holding
`last(value_num, ts)` per `topic_id` and query that instead.

## Using it from Grafana

Add the database as a PostgreSQL data source, paste the Grafana-flavored query into a panel, and the
dashboard's time picker drives `$__timeFilter(ts)`. If you don't have Grafana yet,
[Docker Compose Setup](../../production/deployment/docker-compose/setup.md#grafana) brings it up
alongside TimescaleDB.

Point the data source at PgBouncer rather than TimescaleDB directly if your deployment has one.

## Precision

`value_num` is `DOUBLE PRECISION`. That is exact for sensor floats, but loses precision for integer
counters above 2^53 and for exact decimals. Route those tags to a text contract, where the value is
stored verbatim in `value_text`.

The [Historian output reference](https://docs.umh.app/benthos-umh/output/historian) covers the rest
of what the output plugin does: metrics, error classes, throughput tuning, and schema compatibility.
