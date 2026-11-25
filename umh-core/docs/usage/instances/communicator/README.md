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
| Neutral | `Communicator not running` | The communicator is stopped or stopping. |
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

The communicator reports these series on port 8080. All are FSMv2 component series, so each carries the label `hierarchy_path`, which names the part of the communicator that reports it: `(pull)` fetches messages from the Management Console, `(push)` sends them, `(transport)` logs in, and `(communicator)` is the part above them. [Read the metrics](../agent/metrics.md#read-the-metrics) explains how to reach the endpoint, and [FSMv2 metrics](../agent/metrics.md#fsmv2-metrics) explains the `hierarchy_path` label and the standard series every FSMv2 component reports. A counter appears only after its first count.

#### Messages

| Name | Type | Reports |
|---|---|---|
| `umh_fsmv2_worker_pull_ops` | counter | Attempts to fetch messages from the Management Console. |
| `umh_fsmv2_worker_pull_success` | counter | Fetches that succeeded. |
| `umh_fsmv2_worker_pull_failures` | counter | Fetches that failed. The error counters below count the cause. |
| `umh_fsmv2_worker_messages_pulled` | counter | Messages fetched. |
| `umh_fsmv2_worker_bytes_pulled` | counter | Bytes fetched. |
| `umh_fsmv2_worker_last_pull_latency_ms` | gauge | How long the most recent fetch took, in milliseconds, also when it failed. |
| `umh_fsmv2_worker_push_ops` | counter | Attempts to send messages to the Management Console. |
| `umh_fsmv2_worker_push_success` | counter | Sends that succeeded. |
| `umh_fsmv2_worker_push_failures` | counter | Sends that failed. The error counters below count the cause. |
| `umh_fsmv2_worker_messages_pushed` | counter | Messages sent. |
| `umh_fsmv2_worker_bytes_pushed` | counter | Bytes sent. |
| `umh_fsmv2_worker_last_push_latency_ms` | gauge | How long the most recent send took, in milliseconds, also when it failed. |
| `umh_fsmv2_worker_pending_messages` | gauge | Messages that wait to be sent or delivered again after a failure. |
| `umh_fsmv2_worker_pending_messages_delivered` | counter | Waiting messages that were delivered later. |
| `umh_fsmv2_worker_partial_deliveries` | counter | Deliveries in which only some of the fetched messages fit into the queue inside umh-core. |
| `umh_fsmv2_worker_messages_dropped` | counter | Messages that umh-core gave up on and will not send or deliver. |
| `umh_fsmv2_worker_backpressure_active` | gauge | 1 while fetching is paused because umh-core cannot take more messages, 0 otherwise. |
| `umh_fsmv2_worker_backpressure_entry_total` | counter | How often fetching was paused. |
| `umh_fsmv2_worker_backpressure_skips` | counter | Fetches skipped while fetching was paused. |
| `umh_fsmv2_worker_consecutive_errors` | gauge | Failed requests since the last successful one. It is the `<n>` of the `Connection to the Management Console is degraded` reason. |

#### Error counters

Each failed request to the Management Console counts once, under its cause. [Communicator Shows Degraded: Error counters](communicator-shows-degraded.md#error-counters) says what to do for each.

| Name | Type | Counts |
|---|---|---|
| `umh_fsmv2_worker_network_errors_total` | counter | Requests without an answer that umh-core can classify, such as a refused connection, a timeout or a rejected certificate. It also counts answers with a status no other counter takes, such as 400 or a proxy's 407 Proxy Authentication Required. |
| `umh_fsmv2_worker_proxy_block_errors_total` | counter | Error answers that look like the block page of a proxy. |
| `umh_fsmv2_worker_auth_failures_total` | counter | Answers with status 401 or 403. A firewall, a proxy or Cloudflare that answers 403 also counts here, unless umh-core recognizes the answer as a proxy block page. |
| `umh_fsmv2_worker_instance_deleted_total` | counter | Answers with status 404. A wrong `API_URL` or a 404 from a proxy also counts here. |
| `umh_fsmv2_worker_server_errors_total` | counter | Answers with a status of 500 or higher. |
| `umh_fsmv2_worker_backend_rate_limit_errors_total` | counter | Requests the Management Console rejected with status 429, because the instance sent too many in a short time. |
| `umh_fsmv2_worker_cloudflare_errors_total` | counter | Answers with status 429 that carry a Cloudflare challenge page. |

#### Message checks

umh-core checks every message it receives from the Management Console, and every message it sends to a user, before it acts on it or sends it: the encryption, the sender's signature and certificate chain, and the sender's permissions. Each refused or dropped message counts once, under its reason. All series in this table are counters.

| Name | Counts messages refused or dropped because |
|---|---|
| `umh_fsmv2_worker_gatekeeper_refused_not_configured_total` | the check was not set up for this instance. |
| `umh_fsmv2_worker_gatekeeper_refused_unencrypted_after_pin_total` | the message was not encrypted, although the instance accepts only encrypted messages. |
| `umh_fsmv2_worker_gatekeeper_refused_sealed_before_pin_total` | the message was encrypted before the instance accepts encrypted messages. |
| `umh_fsmv2_worker_gatekeeper_refused_unknown_envelope_version_total` | the message has a format version umh-core does not know. |
| `umh_fsmv2_worker_gatekeeper_refused_unknown_class_total` | the message has a message class umh-core does not know. |
| `umh_fsmv2_worker_gatekeeper_refused_open_failed_total` | the message could not be decrypted. |
| `umh_fsmv2_worker_gatekeeper_refused_bad_chain_total` | the sender's certificate chain is not valid. |
| `umh_fsmv2_worker_gatekeeper_refused_bad_signature_total` | the sender's signature is not valid. |
| `umh_fsmv2_worker_gatekeeper_refused_not_verified_total` | the decrypted message could not be verified. |
| `umh_fsmv2_worker_gatekeeper_refused_wrong_instance_total` | the message is addressed to another instance. |
| `umh_fsmv2_worker_gatekeeper_refused_relay_instance_mismatch_total` | the Management Console delivered the message for another instance. |
| `umh_fsmv2_worker_gatekeeper_refused_class_mismatch_total` | the message class does not match its type. |
| `umh_fsmv2_worker_gatekeeper_refused_type_not_permitted_total` | the message type is not allowed in its message class. |
| `umh_fsmv2_worker_gatekeeper_refused_malformed_content_total` | the content of the message is not valid. |
| `umh_fsmv2_worker_gatekeeper_refused_content_not_decodable_total` | the content of an encrypted message could not be decoded. |
| `umh_fsmv2_worker_gatekeeper_refused_plaintext_refused_total` | the message was not encrypted. |
| `umh_fsmv2_worker_gatekeeper_refused_unknown_operation_total` | the message asks for an operation umh-core does not know. |
| `umh_fsmv2_worker_gatekeeper_refused_no_location_role_extension_total` | the sender's permission grant names no role at a location. |
| `umh_fsmv2_worker_gatekeeper_refused_issuer_location_total` | the permission grant gives a role at a location its issuer has no access to. |
| `umh_fsmv2_worker_gatekeeper_refused_no_role_at_location_total` | the sender has no role at the instance's location. |
| `umh_fsmv2_worker_gatekeeper_refused_operation_not_permitted_total` | the sender's role does not permit the operation. |
| `umh_fsmv2_worker_gatekeeper_refused_role_check_failed_total` | the sender's role could not be checked. |
| `umh_fsmv2_worker_gatekeeper_refused_no_sender_total` | an unencrypted message names no sender. |
| `umh_fsmv2_worker_gatekeeper_refused_too_many_waiting_signers_total` | the sender's certificate chain is missing, and messages of too many other senders already wait for theirs. |
| `umh_fsmv2_worker_gatekeeper_refused_too_many_waiting_for_signer_total` | too many messages of the same sender already wait for its certificate chain. |
| `umh_fsmv2_worker_gatekeeper_refused_router_content_failed_total` | umh-core accepted the message but could not pass it on inside umh-core. |
| `umh_fsmv2_worker_gatekeeper_refused_no_reason_total` | the check refused the message without a reason. |
| `umh_fsmv2_worker_gatekeeper_refused_unmapped_reason_total` | the check refused the message for a reason that has no counter of its own. |
| `umh_fsmv2_worker_gatekeeper_refused_refusal_reply_not_sent_total` | umh-core could not send the user the reply that an action was refused. |
| `umh_fsmv2_worker_gatekeeper_refused_outbound_no_sender_total` | a message to a user names no sender. |
| `umh_fsmv2_worker_gatekeeper_refused_outbound_content_not_decodable_total` | the content of a message to a user could not be decoded. |
| `umh_fsmv2_worker_gatekeeper_refused_outbound_too_many_waiting_signers_total` | a message to a user waits for a certificate chain, and messages of too many other users already wait for theirs. |
| `umh_fsmv2_worker_gatekeeper_refused_outbound_too_many_waiting_for_signer_total` | too many messages to the same user already wait for its certificate chain. |
| `umh_fsmv2_worker_gatekeeper_refused_outbound_seal_failed_total` | a message to a user could not be encrypted. |
| `umh_fsmv2_worker_gatekeeper_refused_outbound_type_not_permitted_total` | the message type may not be sent to the user. |
| `umh_fsmv2_worker_gatekeeper_refused_outbound_no_recipient_chain_total` | the user's certificate chain is not known. |
| `umh_fsmv2_worker_gatekeeper_refused_outbound_no_location_role_extension_total` | the user's permission grant names no role at a location. |
| `umh_fsmv2_worker_gatekeeper_refused_outbound_issuer_location_total` | the user's permission grant gives a role at a location its issuer has no access to. |
| `umh_fsmv2_worker_gatekeeper_refused_outbound_no_role_at_location_total` | the user has no role at the instance's location. |
| `umh_fsmv2_worker_gatekeeper_refused_outbound_role_check_failed_total` | the user's role could not be checked. |
| `umh_fsmv2_worker_gatekeeper_dropped_buffer_full_total` | the buffer of messages that wait for a certificate chain was full. |
| `umh_fsmv2_worker_gatekeeper_dropped_not_ready_total` | the check was not ready yet. |
| `umh_fsmv2_worker_gatekeeper_dropped_shutdown_total` | umh-core was shutting down. |
| `umh_fsmv2_worker_gatekeeper_dropped_chain_missing_total` | the certificate chain the message waited for did not arrive. |
| `umh_fsmv2_worker_gatekeeper_dropped_unmapped_reason_total` | of a reason that has no counter of its own. |

#### Alert queries

A counter series appears with its first count, so it starts at 1, and `increase()` does not count that first step. To alert on failed requests of one cause, add a second part that matches a series that was not there 5 minutes ago. This example covers `umh_fsmv2_worker_network_errors_total`, and the same form works for every counter on this page:

```promql
sum without (hierarchy_path) (increase(umh_fsmv2_worker_network_errors_total[5m])) > 0
or
sum without (hierarchy_path) (umh_fsmv2_worker_network_errors_total unless umh_fsmv2_worker_network_errors_total offset 5m) > 0
```

The `offset 5m` part also fires without a new error after a failed scrape, a short scrape outage, or the first scrape of an instance whose counters are already above zero.

The names are defined in [`fsmv2/deps/metrics.go`](https://github.com/united-manufacturing-hub/united-manufacturing-hub/blob/staging/umh-core/fsmv2/deps/metrics.go). The message check counters are defined in the Management Console connection module that umh-core includes, which is not in this repository.
