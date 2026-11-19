# Communicator

The communicator connects your instance to the Management Console. Use this page when the instance shows as offline or its Communicator card is degraded.

If the instance never shows as online, see [Instance Stays Offline Behind a Corporate Firewall](../troubleshooting/instance-offline-behind-corporate-firewall.md).

## The Communicator card

The card shows the health of the connection, how full the outbound queue is, and how many Management Console sessions watch the instance. [Instance Details Page: Communicator](../instance-details-page.md#communicator) lists every field and every badge reason.

The card shows the status that the instance sent last. When the instance stops sending, every card keeps showing that last status. After 15 seconds without a message, the Management Console shows a red alert, "Instance Offline", above the tabs. The card does not show why the connection failed. The [metrics](#metrics) show the cause.

## When the Communicator shows Degraded

| Reason starts with | What to do |
|---|---|
| `Outbound queue …` | The queue holds the messages that wait to be sent to the Management Console. Each open Management Console tab that watches the instance adds a copy of every status message to it. Close the tabs you do not use. |
| `Authentication with the Management Console failed` | If `umh_fsmv2_worker_auth_failures_total` rises under the transport worker, the Management Console usually did not accept the `AUTH_TOKEN` of the instance. Check the token. If `umh_fsmv2_worker_instance_deleted_total` rises instead, the login got status 404. Usually the instance was deleted in the Management Console, and a new token does not fix that. |
| `Connection to the Management Console is degraded` | The reason names the last error type. Look up its counter in [Metrics: Communicator error counters](../metrics/README.md#communicator-error-counters). For an instance behind a corporate firewall, see [Instance Stays Offline Behind a Corporate Firewall](../troubleshooting/instance-offline-behind-corporate-firewall.md). |

## Metrics

The communicator reports on port 8080 how many of its requests to the Management Console failed, by cause. [Read the metrics](../metrics/README.md#read-the-metrics) explains how to reach the endpoint, and [FSMv2 metrics](../metrics/README.md#fsmv2-metrics) explains the `hierarchy_path` label.

### Errors

Each failed request is counted once, under the counter of its cause. To alert on all failed requests of one cause, add up the counter across the series. A counter series appears with its first error, so it starts at 1, and `rate()` and `increase()` do not count that first step. Add a second part that matches a series that was not there 5 minutes ago. This example covers `umh_fsmv2_worker_network_errors_total`, and the same form works for the other counters, including `umh_fsmv2_worker_auth_failures_total`:

```promql
sum without (hierarchy_path) (increase(umh_fsmv2_worker_network_errors_total[5m])) > 0
or
sum without (hierarchy_path) (umh_fsmv2_worker_network_errors_total unless umh_fsmv2_worker_network_errors_total offset 5m) > 0
```

The `offset 5m` part also fires without a new error after a failed scrape, a short scrape outage, or the first scrape of an instance whose counters are already above zero.

[Metrics: Communicator error counters](../metrics/README.md#communicator-error-counters) lists every counter and the cause it counts.

The communicator also reports the standard [FSMv2 series](../metrics/README.md#fsmv2-metrics). To alert when the communicator stays in a bad state, compare `umh_fsmv2_state_duration_seconds{hierarchy_path=~".*\\(communicator\\)"}` for that state with a threshold.

The communicator is part of the Management Console connection module, which is not in this repository. Its exact series names come from that module, so check the `/metrics` output of your version.
