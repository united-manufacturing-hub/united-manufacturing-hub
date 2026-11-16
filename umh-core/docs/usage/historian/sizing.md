<!-- Proposed path: umh-core/docs/usage/historian/sizing.md -->
# Historian Sizing

This page tells you how much disk the Historian needs per year for your data rate. It also says what to add when the Historian shares a machine with UMH Core. The figures assume the Historian output's standard settings and no downsampling.

{% hint style="info" %}
**Early access.** The Historian is not switched on by default. See [Historian](README.md) for how to get started.
{% endhint %}

## Disk per year

Disk use depends on how many values per second you store and on what the values look like. TimescaleDB compresses[^1] steady or repeating values far better than noisy ones.

| Data shape | Disk per year, per 1,000 values/s |
| --- | --- |
| States, counters and status text | ≈ 100 GB |
| Analog sensors | ≈ 200 GB |
| Noisy, high-resolution signals | ≈ 350 GB |

On top of that, add about **120 GB per 1,000 values/s** for the most recent two weeks. Those values are not compressed yet. With the standard settings, data is stored in 7-day chunks. A chunk is compressed once it is older than 7 days (`compress_after: 168h`), so up to 14 days of data stay uncompressed.

For example, 2,000 analog values per second need about 400 GB for one year of history, plus about 240 GB for the uncompressed two weeks.

## Retention defaults to forever

By default the Historian keeps every value and deletes nothing. The `retention` setting is empty unless you set it, so the disk grows until you set a retention period or it runs full.

Set `retention` before the bridge writes its first value. It is applied only to a table that holds no data yet, so setting it later has no effect on a table that already holds data. The [Historian output reference](https://docs.umh.app/benthos-umh/output/historian#changing-compression-or-retention) shows how to change retention on a table that already holds data.

## Historian and UMH Core on the same machine

The Historian's database can run on the same machine as UMH Core. Then add its needs to the machine in the [Sizing Guide](../../production/sizing-guide.md): about **4 more vCPU and 8 GB more RAM**. Add the disk for your chosen retention from the table above.

## Reduce the volume

The [downsampler](https://docs.umh.app/benthos-umh/processing/downsampler) stores fewer values by dropping samples that carry no new information. Its deadband mode drops repeated values and changes below a threshold. Its swinging-door mode drops values that stay within an error bound around the trend between the values it keeps.

[^1]: Measured with the Historian output's own tables on TimescaleDB 2.24 and PostgreSQL 17, 17.3 million values per data shape. Compressed, a value took 3 bytes for counters, states and short text, 6 bytes for smooth analog signals and 11 bytes for noisy ones, against about 100 bytes before compression.
