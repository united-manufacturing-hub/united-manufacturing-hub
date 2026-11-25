# Communicator Shows Degraded

Follow this page when the Communicator card on the instance details page shows Degraded. [Communicator states](README.md#communicator-states) explains each reason. If the instance never shows as online, see [Instance Stays Offline Behind a Corporate Firewall](instance-offline-behind-corporate-firewall.md). Some steps need access to the Docker host.

| Reason starts with | What to do |
|---|---|
| `Messages are held because the last login could not be verified.` | umh-core logs in again by itself, and the reason clears after a login that it can verify. If the reason stays, send the agent log to UMH support. The **Logs** button on the Agent card opens it. |
| `Outbound queue …` | The queue holds the messages that wait to be sent to the Management Console. Each open Management Console tab that watches the instance adds a copy of every status message to it. Close the tabs you do not use. |
| `Authentication with the Management Console failed` | If `umh_fsmv2_worker_auth_failures_total` rises for the login (its `hierarchy_path` ends in `(transport)`, see [Communicator: Metrics](README.md#metrics)), the Management Console did not accept the `AUTH_TOKEN` of the instance. Check the token. If `umh_fsmv2_worker_instance_deleted_total` rises instead, the instance was usually deleted in the Management Console, and a new token does not fix that. |
| `Connection to the Management Console is degraded` | The reason names the last error type. Find its counter in [Error counters](#error-counters). |
| `Communicator status is stale` | umh-core did not update the communicator status in time. Check the agent log for errors. If the reason stays, send the agent log to UMH support. |

## Error counters

Each failed request to the Management Console raises the counter of its cause, so the counter that rises shows what to fix. [Communicator: Error counters](README.md#error-counters) says what each one counts.

| Counter | What to do |
|---|---|
| `umh_fsmv2_worker_network_errors_total` | See [Instance Stays Offline Behind a Corporate Firewall](instance-offline-behind-corporate-firewall.md). |
| `umh_fsmv2_worker_proxy_block_errors_total` | Ask your IT department to allow `management.umh.app` on the proxy. |
| `umh_fsmv2_worker_auth_failures_total` | Check the `AUTH_TOKEN` the container was started with. If the token is correct, see [Instance Stays Offline Behind a Corporate Firewall](instance-offline-behind-corporate-firewall.md). |
| `umh_fsmv2_worker_instance_deleted_total` | Check whether the instance still exists in the Management Console, and check `API_URL`. |
| `umh_fsmv2_worker_server_errors_total` | Wait. umh-core retries. If the counter keeps rising, contact UMH support. |
| `umh_fsmv2_worker_backend_rate_limit_errors_total` | umh-core retries. If the counter keeps rising, contact UMH support. |
| `umh_fsmv2_worker_cloudflare_errors_total` | umh-core retries. If the counter keeps rising, contact UMH support. |
