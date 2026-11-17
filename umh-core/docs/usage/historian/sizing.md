# Historian Sizing

This page tells you what to add to a machine for the Historian and how much disk it needs per year. The figures assume the Historian output's standard settings and no downsampling.

{% hint style="info" %}
**Early access.** The Historian is not switched on by default. See [Historian](README.md) for how to get started.
{% endhint %}

## Extra CPU and memory

The Historian's database can run on the same machine as UMH Core. As a starting point, add this to the machine in the [Sizing Guide](../../production/sizing-guide.md):

* about **4 more vCPU**
* about **8 GB more memory**
* disk for your retention, from the table below

Actual usage depends on the applications you build on top of the Historian, such as dashboards, and on the queries your users run. The CPU figures assume vCPUs as fast as those of the Sizing Guide's example instances, not overcommitted, with no other software on the machine. See [Overcommitting, slow CPUs and other software](../../production/sizing-guide.md#overcommitting-slow-cpus-and-other-software).

## Disk per year

Disk use depends on how many values per second you store and what the values look like. TimescaleDB compresses[^1] steady or repeating values far better than noisy ones. The figures below come from a benchmark and are meant for orientation. Your own data can compress better or worse.

| Data shape | Disk per year, per 1,000 values/s |
| --- | --- |
| States, counters and status text | ≈ 100 GB |
| Analog sensors | ≈ 200 GB |
| Noisy, high-resolution signals | ≈ 350 GB |

Add about **120 GB per 1,000 values/s** for the most recent two weeks, which are not compressed yet.

Example: 2,000 analog values per second need about 400 GB for one year, plus about 240 GB for the uncompressed two weeks.

## Set retention

By default the Historian deletes nothing, so its disk grows until it is full. With `retention` set, the Historian deletes all values older than the retention. Set `retention` in the Historian bridge before it writes its first value, because it applies only when the bridge creates its tables. To change it later, see the [Historian output reference](https://docs.umh.app/benthos-umh/output/historian#changing-compression-or-retention).

## Reduce the volume

The [downsampler](https://docs.umh.app/benthos-umh/processing/downsampler) stores fewer values and so needs less disk, at the cost of some detail.

[^1]: Measured with the Historian output's own tables on TimescaleDB 2.24 and PostgreSQL 17, 17.3 million values per data shape. Compressed, a value took 3 bytes for counters, states and short text, 6 bytes for smooth analog signals and 11 bytes for noisy ones, against about 100 bytes before compression.
