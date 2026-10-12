# Why a Data Flow Is Degraded

A data flow shows "degraded" in the Management Console when its Benthos instance reported a failure recently, and its status shows the reason. A new flow stays in "starting" for the same reasons. This page explains the two checks behind the reason and how to keep expected failures out of the counters.

This applies to bridges, stand-alone flows and stream processors alike, because each one runs a Benthos instance.

## The two checks

umh-core checks each Benthos instance in two ways:

- **The log check** reads the instance's log. An `error` line, or a `warning` line that says "failed to", "connection lost" or "unable to", counts as a failure. The reason reads `found error (clears in 4m12s): <the log line>`.
- **The metric check** reads the instance's failure counters. The reason names the counter, for example `benthos reported 3 output errors in the last 10m0s`. These counters count:
  - output errors and processor errors
  - failed and lost connections, on inputs and on outputs

When failures of several kinds are present, the reason names errors before connection failures.

## When a failure stops counting

Both checks stop counting a failure 10 minutes after it happened. The flow returns to active on its own when no new failure arrived in those 10 minutes; nothing needs to be redeployed. A new failure marks the flow degraded again at once, even if the previous one had just cleared.

Both checks start fresh when Benthos restarts, which happens after any change to the flow's configuration. Failures from before the restart no longer count. Failures between the restart and umh-core's first look at the new instance still count.

The metric check works this way only with the environment variable `USE_FSMV2_BENTHOS_MONITOR=true`. Without it, the metric check reads only output and processor errors, and one such error keeps the flow degraded until Benthos restarts.

## Keep expected failures out of the counters

Some failures are part of normal operation, for example a cache lookup for a key that is not there yet. Counted as failures, they keep the flow degraded.

- Use the `cached` processor instead of `try` around `cache get`. A miss in `cached` is not a failure.
- A failure inside `catch` still counts. `catch` handles the message afterwards, but the processor error is already counted.

## What to do

1. Read the reason on the flow's status.
2. For the log check, find the log line in the flow's logs and fix what it names.
3. For the metric check, fix the input, output or processor the counter names. A connection failure usually means the device or broker is unreachable.
4. For the log check, wait 10 minutes after the last failure; the flow returns to active on its own. The metric check works the same way with `USE_FSMV2_BENTHOS_MONITOR=true`. Without it, a metric-check failure clears only when Benthos restarts, for example after a change to the flow's configuration.
