# Communicator Shows Degraded

Follow this page when the Communicator card on the instance details page shows Degraded. [Communicator status](README.md#communicator-status) explains each reason. If the instance never shows as online, see [Instance Stays Offline Behind a Corporate Firewall](instance-offline-behind-corporate-firewall.md).

| Reason starts with | What to do |
|---|---|
| `Outbound queue …` | The queue holds the messages that wait to be sent to the Management Console. Each open Management Console tab that watches the instance adds a copy of every status message to it. Close the tabs you do not use. |
| `Authentication with the Management Console failed` | If `umh_fsmv2_worker_auth_failures_total` rises under the transport worker, the Management Console usually did not accept the `AUTH_TOKEN` of the instance. Check the token. If `umh_fsmv2_worker_instance_deleted_total` rises instead, the login got status 404. Usually the instance was deleted in the Management Console, and a new token does not fix that. |
| `Connection to the Management Console is degraded` | The reason names the last error type. Look up its counter in [Metrics: Communicator error counters](../agent/metrics.md#communicator-error-counters). For an instance behind a corporate firewall, see [Instance Stays Offline Behind a Corporate Firewall](instance-offline-behind-corporate-firewall.md). |
