# Communicator

The communicator connects your instance to the Management Console. Use this page when the instance shows as offline or its Communicator card is degraded. The communicator logs in with the `AUTH_TOKEN` of the instance, sends the status and the replies to actions, and fetches the actions you start in the Management Console.

If the instance never shows as online, see [Instance Stays Offline Behind a Corporate Firewall](instance-offline-behind-corporate-firewall.md).

## The Communicator card

The card shows the status that the instance sent last. When the instance stops sending, every card keeps showing that last status. After 15 seconds without a message, the Management Console shows a red alert, "Instance Offline", above the tabs. The card does not show why the connection failed. The [metrics](#metrics) show the cause.

| Field | Shows |
|---|---|
| Status badge | The health of the connection. Hover over it to see the reason, listed below. |
| Usage | How full the outbound queue was over the last 30 seconds, as the 95th percentile. The outbound queue holds the messages that wait to be sent to the Management Console. |
| Peak Usage | The highest fill level of the outbound queue in the last 30 seconds. |
| Active Viewers | The number of Management Console sessions that receive updates from this instance. A session stays counted for a few minutes after you close it. |

The Usage and Peak Usage rows appear while the badge is Active or Degraded. Each viewer adds a copy of every status message to the queue, so many open tabs can fill it.

| Badge | Reason | What it means |
|---|---|---|
| Active | `Outbound queue …% full …, peak …% …` | The connection works and the queue has room. |
| Degraded | `Outbound queue … Status updates and action replies may be lost` | The queue is close to full. This happens when Usage rises above 50% or Peak Usage above 90%. Each figure that rose above its mark must fall below its own lower mark before the badge clears: 30% for Usage, 70% for Peak Usage. Close Management Console tabs you do not use. |
| Degraded | `Authentication with the Management Console failed` | The login failed in a way that another attempt cannot fix. If `umh_fsmv2_worker_auth_failures_total` rises under the transport worker, the Management Console usually did not accept the `AUTH_TOKEN` of the instance. Check the token. If `umh_fsmv2_worker_instance_deleted_total` rises instead, the login got status 404. Usually the instance was deleted in the Management Console, and a new token does not fix that. See [Errors](#errors). |
| Degraded | `Connection to the Management Console is degraded: <n> consecutive <type> errors` | Requests failed since the last successful login. `<n>` counts the failed logins, sends and fetches since that login. `<type>` names the last error, and each type has an [error counter](#errors). `invalid_token` counts in `umh_fsmv2_worker_auth_failures_total`, and `cloudflare_challenge` in `umh_fsmv2_worker_cloudflare_errors_total`. `unknown`, `network` and `channel_full` count in `umh_fsmv2_worker_network_errors_total`. Every other type matches the start of its counter name, for example `server_error` and `umh_fsmv2_worker_server_errors_total`. |
| Degraded | `Communicator status is stale` | umh-core has not updated the communicator status for more than 10 seconds. |
| Neutral | `Connecting to the Management Console` | The communicator is starting. |
| Neutral | `Measuring outbound queue usage` | The connection works, and umh-core is still collecting samples of the queue. |
| Neutral | `Communicator not running` | The transport worker is stopped or stopping. |
| Neutral | `Communicator status unknown` | umh-core could not read the communicator status. |

## Metrics

The communicator reports on port 8080 how many of its requests to the Management Console failed, by cause. [Read the metrics](../agent/metrics.md#read-the-metrics) explains how to reach the endpoint, and [FSMv2 metrics](../agent/metrics.md#fsmv2-metrics) explains the `hierarchy_path` label. The failed logins are the series whose `hierarchy_path` ends in `(transport)`. A counter appears only after its first count.

### Errors

Each failed request is counted once, under the cause below. To alert on all failed requests of one cause, add up the counter across the series, for example `sum without (hierarchy_path) (rate(umh_fsmv2_worker_network_errors_total[5m])) > 0`.

| Name | Type | Reports |
|---|---|---|
| `umh_fsmv2_worker_network_errors_total` | counter | Requests without an answer that umh-core can classify. This includes a refused connection, a timeout and a rejected certificate. It also counts answers with a status no other counter takes, such as 400 or a proxy's 407 Proxy Authentication Required. |
| `umh_fsmv2_worker_proxy_block_errors_total` | counter | Error answers that look like the block page of a proxy. Ask your IT department to allow `management.umh.app` on the proxy. |
| `umh_fsmv2_worker_auth_failures_total` | counter | Answers with status 401 or 403. Under the transport series, a rise usually means the Management Console did not accept the `AUTH_TOKEN`. A firewall, a proxy or Cloudflare that answers 403 also counts here. |
| `umh_fsmv2_worker_instance_deleted_total` | counter | Answers with status 404. Usually the instance was deleted in the Management Console. A wrong `API_URL` or a 404 from a proxy also counts here. |
| `umh_fsmv2_worker_server_errors_total` | counter | Requests the Management Console answered with a status of 500 or higher. |
| `umh_fsmv2_worker_backend_rate_limit_errors_total` | counter | Requests the Management Console rejected with status 429, because the instance sent too many in a short time. |
| `umh_fsmv2_worker_cloudflare_errors_total` | counter | Answers with status 429 that carry a Cloudflare challenge page. |

The communicator also reports the standard [FSMv2 series](../agent/metrics.md#fsmv2-metrics). To alert when the communicator stays in a bad state, compare `umh_fsmv2_state_duration_seconds{hierarchy_path=~".*\\(communicator\\)"}` for that state with a threshold.

The communicator is part of the Management Console connection module, which is not in this repository. Its exact series names come from that module, so check the `/metrics` output of your version.
