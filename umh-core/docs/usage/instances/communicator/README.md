# Communicator

The communicator is the part of umh-core that connects an instance to the Management Console. It logs in with the `AUTH_TOKEN` of the instance. It then sends the status of the instance and the replies to actions, and it fetches the actions you start in the Management Console. Sending is called push. Fetching is called pull.

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

umh-core exposes the communicator metrics on its Prometheus endpoint. [Metrics](../agent/metrics.md#read-the-metrics) explains how to read the endpoint once and how to collect it with Prometheus. The communicator runs on FSMv2, the newer of umh-core's two frameworks (see [Which framework runs a component](../agent/metrics.md#which-framework-runs-a-component)). [FSMv2 metrics](../agent/metrics.md#fsmv2-metrics) explains the names and the `hierarchy_path` label.

Three parts of the communicator record metrics. umh-core calls each part a worker. The transport worker logs in to the Management Console, the push worker sends messages to it, and the pull worker fetches messages from it. The `hierarchy_path` label names the worker that recorded a series, so to select one worker, match the end of the path.

| Worker | `hierarchy_path` ends in | Example selector |
|---|---|---|
| Transport | `transport-001(transport)` | `{hierarchy_path=~".*\\(transport\\)"}` |
| Push | `push-001(push)` | `{hierarchy_path=~".*\\(push\\)"}` |
| Pull | `pull-001(pull)` | `{hierarchy_path=~".*\\(pull\\)"}` |

A counter appears only after its first count (see [Read the metrics](../agent/metrics.md#read-the-metrics)).

### Errors

Each worker counts its own failed requests by cause: the transport worker counts failed logins, the push worker failed sends, and the pull worker failed fetches. The push and pull workers send the session token that the transport worker received at its last login, not the `AUTH_TOKEN`. To alert on all failed requests of one cause, add up the counter across the three workers, for example `sum without (hierarchy_path) (rate(umh_fsmv2_worker_network_errors_total[5m]))`.

| Name | Type | Reports |
|---|---|---|
| `umh_fsmv2_worker_network_errors_total` | counter | Requests without an answer that umh-core can classify. This includes a refused connection, a timeout and a rejected certificate. It also counts answers with a status no other counter takes, such as 400 or a proxy's 407 Proxy Authentication Required, and answers umh-core cannot decode. |
| `umh_fsmv2_worker_proxy_block_errors_total` | counter | Error answers that look like the block page of a proxy. umh-core searches the answer for texts that common proxies show, such as `Zscaler`, `BlueCoat`, `Websense`, `FortiGuard` or `Access Denied`. A match counts here whatever the status and the sender, unless the answer is a Cloudflare challenge. |
| `umh_fsmv2_worker_auth_failures_total` | counter | Answers with status 401 or 403, and logins answered without a session token. Under the transport worker, a rise usually means the Management Console did not accept the `AUTH_TOKEN`. Under the push or pull worker, it means the session token was rejected, for example because it expired. A firewall, a proxy or Cloudflare that answers 403 also counts here. |
| `umh_fsmv2_worker_instance_deleted_total` | counter | Answers with status 404. Usually the instance was deleted in the Management Console. A wrong `API_URL` or a 404 from a proxy also counts here. |
| `umh_fsmv2_worker_server_errors_total` | counter | Requests the Management Console answered with a status of 500 or higher. |
| `umh_fsmv2_worker_backend_rate_limit_errors_total` | counter | Requests the Management Console rejected with status 429, because the instance sent too many in a short time. |
| `umh_fsmv2_worker_cloudflare_errors_total` | counter | Answers with status 429 that carry a Cloudflare challenge page. Cloudflare protects `management.umh.app`. A challenge with status 403 counts under `umh_fsmv2_worker_auth_failures_total`. |

### Push

The push worker records these series.

| Name | Type | Reports |
|---|---|---|
| `umh_fsmv2_worker_push_ops` | counter | Send attempts. A batch counts once. Each resend of a waiting message counts once. |
| `umh_fsmv2_worker_push_success` | counter | Send attempts that succeeded |
| `umh_fsmv2_worker_push_failures` | counter | Send attempts that failed |
| `umh_fsmv2_worker_messages_pushed` | counter | Messages sent |
| `umh_fsmv2_worker_bytes_pushed` | counter | Size of the messages sent, in bytes. It counts the length of the message content and its identifiers. It does not count the bytes on the network, such as HTTP headers or TLS. |
| `umh_fsmv2_worker_pending_messages` | gauge | Messages that wait to be sent on a later attempt. While sends keep failing, umh-core also moves new messages from the outbound queue here, so some of them were never sent. At most 1000 wait. |
| `umh_fsmv2_worker_messages_dropped` | counter | Messages lost before they were sent. A message is lost when more than 1000 wait, when the communicator resets the connection after repeated errors, or when a resend fails with an error that another resend cannot fix, such as a deleted instance. |
| `umh_fsmv2_worker_last_push_latency_ms` | gauge | Duration of the last send attempt, in milliseconds |

### Pull

The pull worker records these series. `umh_fsmv2_worker_pending_messages` and `umh_fsmv2_worker_messages_dropped` also exist under the push worker, with a different meaning. Select the worker by `hierarchy_path` to tell them apart.

| Name | Type | Reports |
|---|---|---|
| `umh_fsmv2_worker_pull_ops` | counter | Fetch attempts. See the caveat below the table. |
| `umh_fsmv2_worker_pull_success` | counter | Fetch attempts that succeeded. See the caveat below the table. |
| `umh_fsmv2_worker_pull_failures` | counter | Fetches from the Management Console that failed |
| `umh_fsmv2_worker_messages_pulled` | counter | Messages fetched |
| `umh_fsmv2_worker_bytes_pulled` | counter | Size of the messages fetched, in bytes. It counts the same way as `umh_fsmv2_worker_bytes_pushed`. |
| `umh_fsmv2_worker_pending_messages` | gauge | Fetched messages that wait to be handed on inside umh-core. At most 1000 messages wait. |
| `umh_fsmv2_worker_pending_messages_delivered` | counter | Waiting messages handed on in a handover that emptied the waiting list. A partial handover is not counted here. |
| `umh_fsmv2_worker_partial_deliveries` | counter | Handovers where only part of the messages fit. The rest waits. |
| `umh_fsmv2_worker_messages_dropped` | counter | Fetched messages lost, because more than 1000 waited or the communicator reset the connection after repeated errors |
| `umh_fsmv2_worker_backpressure_active` | gauge | 1 while fetching is paused because umh-core cannot take more messages, 0 otherwise |
| `umh_fsmv2_worker_backpressure_entry_total` | counter | How often fetching was paused |
| `umh_fsmv2_worker_backpressure_skips` | counter | Fetch attempts skipped while fetching was paused |
| `umh_fsmv2_worker_last_pull_latency_ms` | gauge | Duration of the last fetch from the Management Console, in milliseconds |

`umh_fsmv2_worker_pull_ops` and `umh_fsmv2_worker_pull_success` also count attempts that only hand on waiting messages. Those attempts do not contact the Management Console. While messages wait, `umh_fsmv2_worker_pull_success` can rise although the Management Console is unreachable. To check that fetching works, watch `umh_fsmv2_worker_pull_failures` and the error counters under the pull worker.
