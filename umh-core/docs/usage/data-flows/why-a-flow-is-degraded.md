# Why a Data Flow Is Degraded

A data flow shows "degraded" in the Management Console when its Benthos instance reported a failure recently. The flow's status shows the reason. A new flow stays in "starting" for the same reasons. This page explains the two checks that produce the reason. It also shows how to keep expected failures out of the counters.

A flow can also show degraded when its connection or its Redpanda is down, without a Benthos failure. The reason then names the connection or Redpanda. This page does not cover that case.

The page applies to bridges, stand-alone flows and stream processors. Each of them runs a Benthos instance.

## The two checks

umh-core checks each Benthos instance in two ways:

- **The log check** reads the instance's log. It counts two kinds of lines as a failure: an `error` line, and a `warning` line that contains "failed to", "connection lost" or "unable to". The reason reads `found error (clears in 4m12s): <the log line>`.
- **The metric check** reads the instance's error counters for outputs and processors. The reason names the counter, for example `benthos reported 3 output errors`.

{% hint style="info" %}
**Early access:** These changes apply when `USE_FSMV2_BENTHOS_METRICS=true` is set (see [Configuration](#configuration)):

- A metric-check failure clears 10 minutes after the last one. Benthos does not need to restart.
- The metric check also reports inputs and outputs that are not connected. It reports them until Benthos reconnects them.
- An output that lost its connection counts as connected again once Benthos has written the message it held when the connection dropped.
- The reason names errors before connections, for example `benthos reported 3 output errors in the last 10m, last one 4m ago` or `benthos reports 1 input not connected`.
- The status shows the log check's reason first. Benthos logs most outages, so the metric check's reason appears only while the log is clean.
- Some inputs and outputs log a broken connection as an error instead. The log check then reports it.
- Two more reasons can appear. See [When umh-core cannot read the metrics](#when-umh-core-cannot-read-the-metrics).
{% endhint %}

## When a failure stops counting

The log check stops counting a failure 10 minutes after it happened. If no new failure arrived and the metric check passes, the flow then leaves degraded on its own. You do not need to redeploy it. The flow shows idle until it processes data again. A new failure marks the flow degraded again at once, even if the previous one had just cleared.

Without the early-access flag, a metric-check failure keeps the flow degraded until Benthos restarts. Benthos restarts after any change to the flow's configuration. Both checks then start fresh, and failures from before the restart no longer count.

## Keep expected failures out of the counters

Some failures are part of normal operation. An example is a cache lookup for a key that does not exist yet. The counters still count them, so they keep the flow degraded.

- Check for the key before `cache get`. The `cache` processor's `exists` operator writes `true` or `false` into the message and never fails. A missing key then does not count as a processor error. The operator also writes `false` when the cache itself fails. Do not use it where a cache outage must not look like a missing key.
- `cached` cannot replace `cache get` here. It stores processor results, and it cannot read values that the flow did not store through it.
- A failure inside `catch` still counts. `catch` handles the message afterwards, but the processor error is already counted.

## When umh-core cannot read the metrics

With the early-access flag on, two reasons mean that the metric check has no reading to judge. They do not report a failure. The flow may still be moving data. Only umh-core's view of it is missing.

- `no Benthos metrics yet`: umh-core has no reading for the flow. This is normal for the first seconds after a flow starts or after umh-core restarts. It can also show for a single check when umh-core briefly fails to read the latest reading. It clears at the next check. If the reason stays:
  1. Read the flow's logs. Benthos may not be serving its metrics port, and the logs usually show why it did not start.
  2. If the flow's logs are clean, umh-core may fail to collect the metrics. Look for errors about Benthos metrics in the agent's log, `/data/logs/umh-core/current`.
- `no fresh Benthos metrics`: the newest reading is more than ten seconds old, usually because readings stopped arriving. It also shows after the system clock is set back. Read the flow's logs to see whether Benthos still runs. Also check whether the instance is short of CPU, because that delays the readings.

## What to do

1. Read the reason on the flow's status.
2. For the log check, find the log line in the flow's logs and fix what it names.
3. For the metric check, the reason names a count and the kind of error. It does not name the output or processor. Find it in the flow's logs. They usually show the error behind the counter and the input or output that lost its connection.
4. If the reason says umh-core has no metrics, see [When umh-core cannot read the metrics](#when-umh-core-cannot-read-the-metrics).
5. Wait until the failure stops counting (see [When a failure stops counting](#when-a-failure-stops-counting)). A log-check failure clears on its own. Without the early-access flag, a metric-check failure clears only when Benthos restarts. In that case, redeploy the flow after the fix.

## Configuration

- `USE_FSMV2_BENTHOS_METRICS` (environment variable, default `false`): turns on the early-access metric check described above. Set it on the `docker run` command, for example `-e USE_FSMV2_BENTHOS_METRICS=true`. umh-core reads it at startup, so a change needs a new container. `docker restart` keeps the old environment.
