# Why a Data Flow Is Degraded

A data flow shows degraded in the Management Console when umh-core finds a failure in the Benthos process that runs it. This page explains which checks find the failure, which reason the status shows, what the error counts in that reason mean and how the flow recovers. It applies to bridges, stand-alone flows and stream processors, because each of them runs its flow in a Benthos process.

A bridge can also be degraded because of its connection, Redpanda or inconsistent states of its parts. Its reason then starts with `connection degraded:`, `redpanda degraded:` or `other degraded:`. A stream processor can likewise show `degraded_redpanda` or `degraded_other`. This page does not cover these cases. A degraded container is a different status: see [Container Shows Degraded](../instances/container/container-shows-degraded.md).

## Where the reason shows

| Data flow | What the status shows |
|---|---|
| Stand-alone flow | The reason of the check that failed, for example a message that ends in `benthos reported 6484 processor errors`. |
| Bridge | A message that contains `flow degraded:` and the reason of the read flow, for example `Bridge data flow component is degraded (port is open) - flow degraded: degraded: benthos reported 6484 processor errors`. The reason of the write flow shows when the read flow is healthy. |
| Stream processor | Only the state, for example `Stream processor degraded_dfc`. The status does not show the reason or the error count. Find the cause in the stream processor's **Metrics** and logs, as described in [What to do](#what-to-do). |

## The checks

umh-core runs these checks on the flow's Benthos process, in this order. The status shows the reason of the first check that fails. A later check's failure shows only once the checks before it pass.

| Check | Fails when | Example reason |
|---|---|---|
| Process | The Benthos process is not running. umh-core starts it again. | `restarting service` |
| Uptime | The process has run for less than 10 seconds, for example right after a restart. | `uptime 4 s (< 10 s threshold)` |
| Logs | In the last 10 minutes of the current process, Benthos logged an `error` line, a `warning` line that contains "failed to", "connection lost" or "unable to", or a configuration or start-up error. | `found error (clears in 4m12s): <the log line>` |
| Output errors | The flow's outputs counted at least one error. | `benthos reported 3 output errors` |
| Processor errors | The flow's processors counted at least one error. | `benthos reported 6484 processor errors` |
| Health checks | Benthos does not report itself live and ready, or has reported it for less than 5 seconds. | `healthchecks did not pass: live=true, ready=false` |

A new flow runs the same uptime, log and error checks before it leaves starting. Their reason then follows `waiting for service to remain running:`.

## Error counts

Benthos counts errors for every output and every processor of the flow, by its path in the configuration. Processors nested inside other processors, for example inside a `switch` or a `branch`, have their own path. The reason shows one total:

- **Processor errors** are the sum over all processor paths. If one processor counted 6 errors and a nested one 6,478, the reason reads `benthos reported 6484 processor errors`.
- **Output errors** are the sum over all output paths, for example over the outputs of a `switch` or a `broker`.
- **Output errors come first.** While the outputs have counted any error, the reason shows only the output errors, even if processors also failed. The processor count shows when the outputs have counted none.

A count is the number of errors, not the number of messages that failed. A message that fails in two processors counts twice, so the total can be higher than the number of failed messages.

Benthos counts from the start of its process, and a count never goes down while the process runs. The count in the reason therefore grows with every new error and stays the same when no new error happens.

These counts are not the `umh_core_errors_total` series on port 8080. That series counts errors umh-core hit while it managed the component, not errors of the messages. See [Metrics](../instances/agent/metrics.md#errors-and-reconcile-time).

## When the flow recovers

The flow leaves degraded once every check passes. It then shows idle until it processes data again.

- **Logs:** a log line stops counting 10 minutes after Benthos logged it. The reason shows the time until then, for example `clears in 4m12s`.
- **Output and processor errors:** messages that succeed later do not clear the counts. A flow with one counted error stays degraded until its Benthos process restarts.

The Benthos process of a flow restarts when you save a change to that flow's configuration, when you stop and start the flow, when the process exits and umh-core starts it again, and when the container restarts. A bridge runs one process for its read flow and one for its write flow, so a change to one of them resets only its own counts. The counts then start again at 0, and log lines from before the restart no longer count.

## What to do

1. Read the reason on the flow's status. For a stream processor, the status shows only the state. Continue with step 3.
2. If the reason quotes a log line, find it in the flow's logs and fix what it names.
3. If the reason counts errors, open the flow's **Metrics**. For a stand-alone flow, it shows the errors of the input, the processors and the output, so you can see which part counted them. The flow's logs usually show the error behind the count. For a stand-alone flow, [`debug_level: true`](stand-alone-flow.md#enable-debug-logging) logs more detail.
4. Fix the configuration or the external system. Saving the changed configuration restarts the flow's Benthos process and resets its counts. If you fixed something outside the flow, stop and start the flow to reset them.
5. Watch the status. If the flow goes back to degraded, the cause is still there, and the new count shows how many errors happened since the restart.
