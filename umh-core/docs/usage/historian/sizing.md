# Historian Sizing

This page tells you what to add to a machine for the Historian and how much disk it needs per year. The figures assume the Historian output's standard settings and no downsampling.

{% hint style="info" %}
**Early access.** The Historian is not switched on by default. See [Historian](README.md) for how to get started.
{% endhint %}

## Extra CPU and RAM

The Historian's database can run on the same machine as UMH Core. Add this to the machine in the [Sizing Guide](../../production/sizing-guide.md):

* about **4 more CPU cores**
* about **8 GB more RAM**
* disk for your retention, from the table below

## Disk per year

Disk use depends on how many values per second you store and what the values look like. TimescaleDB compresses[^1] steady or repeating values far better than noisy ones.

| Data shape | Disk per year, per 1,000 values/s |
| --- | --- |
| States, counters and status text | ≈ 100 GB |
| Analog sensors | ≈ 200 GB |
| Noisy, high-resolution signals | ≈ 350 GB |

Add about **120 GB per 1,000 values/s** for the most recent two weeks, which are not compressed yet.

Example: 2,000 analog values per second need about 400 GB for one year, plus about 240 GB for the uncompressed two weeks.

## Set retention

By default the Historian keeps every value and deletes nothing. The `retention` setting is empty unless you set it. The disk grows until you set a retention period or it runs full.

Set `retention` before the bridge writes its first value. It applies only to a table that holds no data yet. To change retention on a table that already holds data, see the [Historian output reference](https://docs.umh.app/benthos-umh/output/historian#changing-compression-or-retention).

## Reduce the volume

The [downsampler](https://docs.umh.app/benthos-umh/processing/downsampler) stores fewer values by dropping samples that carry no new information, using deadband or swinging-door compression.

[^1]: Measured with the Historian output's own tables on TimescaleDB 2.24 and PostgreSQL 17, 17.3 million values per data shape. Compressed, a value took 3 bytes for counters, states and short text, 6 bytes for smooth analog signals and 11 bytes for noisy ones, against about 100 bytes before compression.
