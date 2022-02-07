---
title: "Why we chose timescaleDB over InfluxDB"
linkTitle: "timescaleDB vs InfluxDB"
author: Jeremy Theocharis
weight: 5
description: >
    TimescaleDB is better suited for the Industrial IoT than InfluxDB, because it is stable, mature and failure resistant, it uses the very common SQL as a query language and you need a relational database for manufacturing anyway
---

## Introduction

The introduction and implementation of an Industrial IoT strategy is already complicated and tedious. There is no need to put unnecessary obstacles in the way through lack of stability, new programming languages, or more databases than necessary. You need a piece of software that you can trust with your company’s most important data.

We are often asked why we chose [timescaleDB](https://www.timescale.com/) instead of [InfluxDB](https://www.influxdata.com/). Both are time-series databases suited for large amounts of machine and sensor data (e.g., vibration or temperature).

We started with InfluxDB (probably due to its strong presence in the home automation and Grafana communities) and then ended up with timescaleDB based on three arguments. In this article, we would like to explain our decision and provide background information on why timescaleDB makes the most sense for the United Manufacturing Hub.

## Argument 1: Reliability & Scalability

A central requirement for a database: it cannot lose or corrupt your data. Furthermore, as a central element in an Industrial IoT stack, it must scale with growing requirements.

### TimescaleDB

TimescaleDB is built on PostgreSQL, which has been continuously developed for over 25 years and has a central place in the architecture of many large companies like [Uber, Netflix, Spotify or reddit](https://stackshare.io/postgresql). This has created a fault-tolerant database that can scale horizontally across multiple servers. In short: it is boring and works.

### InfluxDB

In contrast, InfluxDB is a relatively young startup that was funded at [**119.9 M USD**](https://www.crunchbase.com/organization/influxdb) (as of 2021-05-03) but still doesn't have 25+ years of expertise to fall back on. 

On the contrary: Influx has completely rewritten the database twice in the last 5 years [^rewrite1] [^rewrite2]. Rewriting software can improve fundamental issues or add exciting new features. However, it is usually associated with breaking changes in the API and new unintended bugs. This results in additional migration projects, which take time and risk system downtime or data loss.

Due to its massive funding, we get the impression that they add quite a lot of exciting new features and functionalities (e.g., [an own visualization tool](https://www.influxdata.com/how-to-visualize-time-series-data/)). However, after testing, we noticed that the stability suffers under these new features.

In addition, Influx only offers the horizontally scalable version of the database in the [paid version](https://docs.influxdata.com/influxdb/v1.8/high_availability/clusters/), which will scare off companies wanting to use it on a larger scale as you will be fully dependent on the provider of that software (vendor lock-in).

### Summary

With databases, the principle applies: **Better boring and working than exciting and unreliable.**

*We can also strongly recommend an article by [timescaleDB](https://blog.timescale.com/blog/timescaledb-vs-influxdb-for-time-series-data-timescale-influx-sql-nosql-36489299877/).*

## Argument 2: SQL is better known than flux

The second argument refers to the query language, i.e., the way information can be retrieved from the database.

### SQL (timescaleDB)

TimescaleDB, like PostgreSQL, relies on SQL, the de facto standard language for relational databases. Advantages: A programming language established for over 45 years, which almost every programmer knows or has used at least once. Any problem? No problem, just Google it, and some smart person has already solved it on Stack Overflow. Integration with PowerBI? A standard interface that’s already integrated!

```SQL
SELECT time, (memUsed / procTotal / 1000000) as value
FROM measurements
WHERE time > now() - '1 hour';
```
*Example SQL code to get the average memory usage for the last hour.*

### flux (InfluxDB)

*InfluxDB*, on the other hand, relies on the homegrown [**flux**](https://www.influxdata.com/products/flux/), which is supposed to simplify time-series data queries. It sees time-series data as `a continuous stream upon which are applied functions, calculations and transformations`[^flux]. 

Problem: as a programmer, you have to rethink a lot because the language is [flow-based](https://www.influxdata.com/blog/why-were-building-flux-a-new-data-scripting-and-query-language/) and not based on relational algebra. It takes some time to get used to it, but it is still an unnecessary hurdle for those not-so-tech-savvy companies who already struggle with Industrial IoT.

From some experience, we can also say that the language quickly reaches its limits. In the past, we worked with additional Python scripts that extract the data from InfluxDB via Flux, then process it and then play it back again.

```
// Memory used (in bytes)
memUsed = from(bucket: "telegraf/autogen")
  |> range(start: -1h)
  |> filter(fn: (r) =>
    r._measurement == "mem" and
    r._field == "used"
  )

// Total processes running
procTotal = from(bucket: "telegraf/autogen")
  |> range(start: -1h)
  |> filter(fn: (r) =>
    r._measurement == "processes" and
    r._field == "total"
    )

// Join memory used with total processes and calculate
// the average memory (in MB) used for running processes.
join(
    tables: {mem:memUsed, proc:procTotal},
    on: ["_time", "_stop", "_start", "host"]
  )
  |> map(fn: (r) => ({
    _time: r._time,
    _value: (r._value_mem / r._value_proc) / 1000000
  })
)
```
*Example Flux code for the same SQL code.*

### Summary
**In summary, InfluxDB puts unnecessary obstacles in the way of not-so-tech-savvy companies with flux, while PostgreSQL relies on SQL, which just about every programmer knows.**

*We can also strongly recommend the [blog post by timescaleDB on exactly this topic](https://blog.timescale.com/blog/sql-vs-flux-influxdb-query-language-time-series-database-290977a01a8a/).*

## Argument 3: relational data

Finally, the argument that is particularly important for production: Production data is more relational than time-series based.

Relational data is, simply put, all table-based data that you can store in Excel in a meaningful way, for example, shift schedules, orders, component lists, or inventory.

{{< imgproc Relational Fit "1200x1200" >}}Relational data. Author: AutumnSnow, License: CC BY-SA 3.0{{< /imgproc >}}

TimescaleDB provides this by default through the PostgreSQL base, whereas with InfluxDB, you always have to run a second relational database like PostgreSQL in parallel.

If you have to run two databases anyway, you can reduce complexity and directly use PostgreSQL/timescaleDB.

## Not an argument: Performance for time-series data

Often the duel between timescaleDB and InfluxDB is fought on the performance level. Both databases are efficient, and 30% better or worse does not matter if both databases are 10x-100x faster [^timescaledbperformance] than classical relational databases like PostgreSQL or MySQL.

Even if it is not important, there is strong evidence that timescaleDB is actually more performant. Both databases regularly compare their performance against other databases, and InfluxDB never compares itself to timescaleDB. However, timescaleDB has provided [a detailed performance guide of influxDB](https://blog.timescale.com/blog/timescaledb-vs-influxdb-for-time-series-data-timescale-influx-sql-nosql-36489299877/).
 
## Summary

**Who do you trust more? The nerdy and boring, or the good-looking accountant, with 25 new exciting tools?**

The same goes for databases: **Boring is awesome.**

[^rewrite1]: https://www.influxdata.com/blog/new-storage-engine-time-structured-merge-tree/ 
[^rewrite2]: https://www.influxdata.com/blog/influxdb-2-0-open-source-beta-released/
[^flux]: https://www.influxdata.com/blog/why-were-building-flux-a-new-data-scripting-and-query-language/
[^timescaledbperformance]: https://docs.timescale.com/latest/introduction/timescaledb-vs-postgres
