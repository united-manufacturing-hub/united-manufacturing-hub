# Communicator

The communicator connects your instance to the Management Console. It sends the instance's status and receives the changes you make in the Management Console.

## The Communicator card

The Communicator card on the instance details page shows the health of the connection, how full the outbound queue is, and how many Management Console sessions watch the instance. Like every card, it shows the status that the instance sent last, also after the instance stops sending. After 15 seconds without a message, the Management Console shows a red alert, "Instance Offline", above the tabs. The card does not show why the connection failed. The [metrics](#metrics) show the cause.

| Field | Shows |
|---|---|
| Status badge | The health of the connection. See [Communicator states](#communicator-states). |
| Usage | How full the outbound queue was over the last 30 seconds, as the 95th percentile. The outbound queue holds the messages that wait to be sent to the Management Console. |
| Peak Usage | The highest fill level of the outbound queue in the last 30 seconds. |
| Active Viewers | The number of Management Console sessions that receive updates from this instance. A session stays counted for a few minutes after you close it. |

The Usage and Peak Usage rows appear while the badge is Active or Degraded. Each viewer adds a copy of every status message to the queue, so many open tabs can fill it.

## Communicator states

| Badge | Reason | What it means |
|---|---|---|
| Active | `Outbound queue …% full …, peak …% …` | The connection works and the queue has room. |
| Degraded | `Messages are held because the last login could not be verified.` | umh-core holds every message to and from the Management Console, because it could not verify the last login. While this lasts, the badge shows this reason instead of any other. |
| Degraded | `Outbound queue … Status updates and action replies may be lost` | The queue is close to full. See [Communicator thresholds](#communicator-thresholds). |
| Degraded | `Authentication with the Management Console failed` | The login failed in a way that another attempt cannot fix. See [Communicator](communicator-shows-degraded.md). |
| Degraded | `Connection to the Management Console is degraded: <n> consecutive <type> errors` | Requests failed since the last successful login. `<n>` counts the failed logins, sends and fetches since that login. `<type>` names the last error, and each type has an [error counter](communicator-shows-degraded.md#error-counters). `invalid_token` counts in `umh_fsmv2_worker_auth_failures_total`, and `cloudflare_challenge` in `umh_fsmv2_worker_cloudflare_errors_total`. `unknown`, `network` and `channel_full` count in `umh_fsmv2_worker_network_errors_total`. Every other type matches the start of its counter name, for example `server_error` and `umh_fsmv2_worker_server_errors_total`. |
| Degraded | `Communicator status is stale` | umh-core has not updated the communicator status for more than 10 seconds. |
| Neutral | `Connecting to the Management Console` | The communicator is starting. |
| Neutral | `Measuring outbound queue usage` | The connection works, and umh-core is still collecting samples of the queue. |
| Neutral | `Communicator not running` | The transport worker is stopped or stopping. |
| Neutral | `Communicator status unknown` | umh-core could not read the communicator status. |

[Communicator Shows Degraded](communicator-shows-degraded.md) lists what to do for each degraded reason.

## Communicator thresholds

| Figure | Degraded above | Clears below |
|---|---|---|
| Usage of the outbound queue | 50% | 30% |
| Peak Usage of the outbound queue | 90% | 70% |
| Age of the communicator status | 10 seconds | 10 seconds, once umh-core updates the status again |

Each queue figure that rose above its mark must fall below its own lower mark before the badge clears. The Management Console shows the "Instance Offline" alert after 15 seconds without a message from the instance.

## Reference

### Metrics

The communicator reports on port 8080 how many of its requests to the Management Console failed, with one counter per cause. [Communicator Shows Degraded: Error counters](communicator-shows-degraded.md#error-counters) lists the counters and what to do for each. [Read the metrics](../agent/metrics.md#read-the-metrics) explains how to reach the endpoint, and [FSMv2 metrics](../agent/metrics.md#fsmv2-metrics) explains the `hierarchy_path` label.

A counter series appears with its first error, so it starts at 1, and `increase()` does not count that first step. To alert on failed requests of one cause, add a second part that matches a series that was not there 5 minutes ago. This example covers `umh_fsmv2_worker_network_errors_total`, and the same form works for the other counters:

```promql
sum without (hierarchy_path) (increase(umh_fsmv2_worker_network_errors_total[5m])) > 0
or
sum without (hierarchy_path) (umh_fsmv2_worker_network_errors_total unless umh_fsmv2_worker_network_errors_total offset 5m) > 0
```

The `offset 5m` part also fires without a new error after a failed scrape, a short scrape outage, or the first scrape of an instance whose counters are already above zero.

The communicator is part of the Management Console connection module, which is not in this repository. Its exact series names come from that module, so check the `/metrics` output of your version.
