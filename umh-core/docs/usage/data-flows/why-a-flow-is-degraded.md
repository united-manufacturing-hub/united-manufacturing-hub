# Why a Data Flow Is Degraded

A data flow shows "degraded" in the Management Console when its Benthos instance reported a failure recently, and its status shows the reason. A new flow stays in "starting" for the same reasons. This page explains the two checks behind the reason and how to keep expected failures out of the counters.

This applies to bridges, stand-alone flows and stream processors alike, because each one runs a Benthos instance.

{% hint style="info" %}
**Early access:** With `-e USE_FSMV2_BENTHOS_METRICS=true` on the `docker run` command that starts the container, the metric check works differently:

- **Errors with a time span:** an error clears 10 minutes after the last one, without a restart. The reason reads, for example, `benthos reported 3 output errors in the last 10 minutes`.
- **Connections:** inputs and outputs that are not connected count until Benthos reconnects them. The reason reads, for example, `benthos reports 1 input not connected`. An output that lost its connection counts as connected again once Benthos has written the message it was holding when the connection dropped.
- **Blocked outputs:** the flow's outputs have sent no message for 2 minutes while at least three writes failed, the latest within those 2 minutes. The reason reads, for example, `benthos output blocked: no message sent for 3 minutes`. Check that the output's target accepts writes. Only failures since the last sent message count, not those from before umh-core started watching the instance or before a Benthos restart. One stuck output among several that still send is not reported as blocked.

The reason names blocked outputs first, then errors, then connections. Some inputs and outputs report a broken connection as an error, not as a lost connection, so their connection problems show up through the log check instead. The flag is read at startup, so changing it means recreating the container.
{% endhint %}

## The two checks

umh-core checks each Benthos instance in two ways:

- **The log check** reads the instance's log. An `error` line, or a `warning` line that says "failed to", "connection lost" or "unable to", counts as a failure. The reason reads `found error (clears in 4m12s): <the log line>`.
- **The metric check** reads the instance's error counters: output errors and processor errors. The reason names the counter, for example `benthos reported 3 output errors`.

## When a failure stops counting

The log check stops counting a failure 10 minutes after it happened. The flow returns to active on its own when no new failure arrived in those 10 minutes; nothing needs to be redeployed. A new failure marks the flow degraded again at once, even if the previous one had just cleared.

A metric-check error keeps the flow degraded until Benthos restarts. Both checks start fresh when Benthos restarts, which happens after any change to the flow's configuration. Failures from before the restart no longer count.

## Keep expected failures out of the counters

Some failures are part of normal operation, for example a cache lookup for a key that is not there yet. Counted as failures, they keep the flow degraded.

- Use the `cached` processor instead of `try` around `cache get`. A miss in `cached` is not a failure.
- A failure inside `catch` still counts. `catch` handles the message afterwards, but the processor error is already counted.

## What to do

1. Read the reason on the flow's status.
2. For the log check, find the log line in the flow's logs and fix what it names.
3. For the metric check, fix the output or processor the reason names.
4. Wait until the failure stops counting (see [When a failure stops counting](#when-a-failure-stops-counting)). A log-check failure clears on its own. A metric-check error clears only when Benthos restarts, so redeploy the flow after the fix, unless the early-access flag above is on.
