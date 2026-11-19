# Why a Data Flow Is Degraded

A data flow shows "degraded" in the Management Console when its Benthos instance reported a failure recently, and its status shows the reason. A new flow stays in "starting" for the same reasons. This page explains the two checks behind the reason and how to keep expected failures out of the counters. A flow can also show degraded because its connection or its Redpanda is down, without a Benthos failure; its reason then names the connection or the broker, and this page does not cover that.

This applies to bridges, stand-alone flows and stream processors alike, because each one runs a Benthos instance.

## The two checks

umh-core checks each Benthos instance in two ways:

- **The log check** reads the instance's log. An `error` line, or a `warning` line that says "failed to", "connection lost" or "unable to", counts as a failure. The reason reads `found error (clears in 4m12s): <the log line>`.
- **The metric check** reads the instance's error counters: output errors and processor errors. The reason names the counter, for example `benthos reported 3 output errors`.

{% hint style="info" %}
**Early access:** With `USE_FSMV2_BENTHOS_METRICS=true` (see [Configuration](#configuration)), metric-check errors clear 10 minutes after the last one, without a restart, and the metric check also reports inputs and outputs that are not connected until Benthos reconnects them. The reason names errors before connections, for example `benthos reported 3 output errors in the last 10m, last one 4m ago` or `benthos reports 1 input not connected`. The log check's reason is shown first, and Benthos logs most outages, so the metric reason appears only while the log stays clean. An output that lost its connection counts as connected again once Benthos has written the message it was holding when the connection dropped. Some inputs and outputs report a broken connection as an error instead, so it shows through the log check. Two reasons exist only with the flag on; see [When umh-core cannot read the metrics](#when-umh-core-cannot-read-the-metrics).
{% endhint %}

## When a failure stops counting

The log check stops counting a failure 10 minutes after it happened. The flow then leaves degraded on its own when no new failure arrived and the metric check passes too; nothing needs to be redeployed, but the flow is idle until it processes data again. A new failure marks the flow degraded again at once, even if the previous one had just cleared.

Without the early-access flag above, a metric-check error keeps the flow degraded until Benthos restarts. Both checks start fresh when Benthos restarts, which happens after any change to the flow's configuration. Failures from before the restart no longer count.

## Keep expected failures out of the counters

Some failures are part of normal operation, for example a cache lookup for a key that is not there yet. Counted as failures, they keep the flow degraded.

- Check for the key before `cache get`: the `cache` processor's `exists` operator writes `true` or `false` into the message and never fails, so a missing key cannot count as a processor error. It also writes `false` when the cache itself fails, so do not use it where a cache outage must not look like a missing key. (`cached` cannot take `cache get`'s place here: it stores processor results, and cannot read values the flow did not store through it.)
- A failure inside `catch` still counts. `catch` handles the message afterwards, but the processor error is already counted.

## When umh-core cannot read the metrics

With the early-access flag on, two reasons mean the metric check has no reading to judge rather than an error to report. The flow may well be moving data; what is missing is umh-core's view of it.

- `no Benthos metrics yet` — umh-core has no reading for the flow. This is normal for the first seconds after a flow starts or after umh-core restarts. If it stays, the Benthos process may not be serving its metrics port: read the flow's logs, which usually show why it did not start. If the flow's logs are clean, umh-core itself may be failing to collect the metrics: look for errors about Benthos metrics in the agent's log, `/data/logs/umh-core/current`.
- `no fresh Benthos metrics` — readings arrived before and have now stopped, or the newest is more than ten seconds old. Read the flow's logs to see whether Benthos is still running, and check whether the instance is short of CPU, which delays the readings.

In both cases the logs are the next place to look, not the status.

## What to do

1. Read the reason on the flow's status.
2. For the log check, find the log line in the flow's logs and fix what it names.
3. For the metric check, the reason names a count and the kind of error, not the output or processor behind it. Find the failing one in the flow's logs, which usually show the error behind the counter, and the input or output that lost its connection.
4. If the reason says umh-core has no metrics, see [When umh-core cannot read the metrics](#when-umh-core-cannot-read-the-metrics).
5. Wait until the failure stops counting (see [When a failure stops counting](#when-a-failure-stops-counting)). A log-check failure clears on its own. A metric-check error clears only when Benthos restarts, so redeploy the flow after the fix, unless the early-access flag above is on.

## Configuration

- `USE_FSMV2_BENTHOS_METRICS` (environment variable, default `false`): turns on the early-access metric check described above, for example with `-e USE_FSMV2_BENTHOS_METRICS=true` on the `docker run` command. It is read at startup, so changing it means recreating the container; `docker restart` keeps the old environment.
