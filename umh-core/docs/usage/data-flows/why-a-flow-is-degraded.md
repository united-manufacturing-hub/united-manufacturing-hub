# Why a Data Flow Is Degraded

A data flow shows "degraded" in the Management Console when its Benthos instance reported a failure recently, and its status shows the reason. A new flow stays in "starting" for the same reasons. This page explains the two checks behind the reason and how to keep expected failures out of the counters.

This applies to bridges, stand-alone flows and stream processors alike, because each one runs a Benthos instance.

## The two checks

umh-core checks each Benthos instance in two ways:

- **The log check** reads the instance's log. An `error` line, or a `warning` line that says "failed to", "connection lost" or "unable to", counts as a failure. The reason reads `found error (clears in 4m12s): <the log line>`.
- **The metric check** reads two kinds of counters:
  - **Errors:** output errors and processor errors. The reason names the counter, for example `benthos reported 3 output errors in the last 10m0s`.
  - **Connections:** an input or output that failed to connect, or lost its connection, counts until Benthos reconnects it. The reason reads, for example, `benthos reports 1 input not connected`.

When both are present, the reason names errors before connections.

An output is blocked when it has written nothing for 2 minutes, at least 3 of its writes failed in that time, and the last failed write was less than 2 minutes ago. A written message is one that Benthos counts in its `output_sent` counter. The flow then shows degraded with the reason `benthos output blocked: no message written for 2m0s`. The time is how long nothing was written, rounded down to whole minutes: 2m0s, 3m0s, and so on. A blocked output comes before other errors in the reason.

If a write hangs and never returns an error, Benthos counts neither a written message nor an error. umh-core then cannot tell the output is blocked.

## When a failure stops counting

Both checks stop counting a failure 10 minutes after it happened. A connection is the exception: it counts while it is down and stops as soon as Benthos reconnects. The flow returns to active on its own when no new failure arrived in those 10 minutes; nothing needs to be redeployed. A new failure marks the flow degraded again at once, even if the previous one had just cleared.

Both checks start fresh when Benthos restarts, which happens after any change to the flow's configuration. Failures from before the restart no longer count. Failures between the restart and umh-core's first look at the new instance still count.

The metric check works this way only with the environment variable `USE_FSMV2_BENTHOS_MONITOR=true`. Without it, the metric check reads only output and processor errors, and one such error keeps the flow degraded until Benthos restarts.

An output that lost its connection counts as connected again once Benthos has written the message it was holding when the connection dropped. While that message cannot be written, the output stays not connected, even if Benthos reached the target again.

Some inputs and outputs report a broken connection as an error, not as a lost connection. Their connection problems show up through the log check, not as a connection in the metric check.

## Keep expected failures out of the counters

Some failures are part of normal operation, for example a cache lookup for a key that is not there yet. Counted as failures, they keep the flow degraded.

- Use the `cached` processor instead of `try` around `cache get`. A miss in `cached` is not a failure.
- A failure inside `catch` still counts. `catch` handles the message afterwards, but the processor error is already counted.

## What to do

1. Read the reason on the flow's status.
2. For the log check, find the log line in the flow's logs and fix what it names.
3. For the metric check, fix the input, output or processor the reason names. An input or output that is not connected usually means the device or broker is unreachable.
4. For the log check, wait 10 minutes after the last failure; the flow returns to active on its own. With `USE_FSMV2_BENTHOS_MONITOR=true`, metric-check errors also clear 10 minutes after the last one, and connections clear as soon as Benthos reconnects. Without it, a metric-check error clears only when Benthos restarts, for example after a change to the flow's configuration.
