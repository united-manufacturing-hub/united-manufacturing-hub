# Communicator Shows Degraded

Follow this page when the Communicator card on the instance details page shows Degraded. [Communicator states](README.md#communicator-states) explains each reason. If the instance never shows as online, see [Instance Stays Offline Behind a Corporate Firewall](instance-offline-behind-corporate-firewall.md). Some steps need access to the Docker host.

| Reason starts with | What to do |
|---|---|
| `Messages are held because the last login could not be verified.` | umh-core logs in again by itself, and the reason clears after a login that it can verify. If the reason stays, send the agent log to UMH support. The **Logs** button on the Agent card opens it. |
| `Outbound queue …` | The queue holds the messages that wait to be sent to the Management Console. Each open Management Console tab that watches the instance adds a copy of every status message to it. Close the tabs you do not use. |
| `Authentication with the Management Console failed` | If `umh_fsmv2_worker_auth_failures_total` rises under the transport worker, the Management Console usually did not accept the `AUTH_TOKEN` of the instance. Check the token. If `umh_fsmv2_worker_instance_deleted_total` rises instead, the login got status 404. Usually the instance was deleted in the Management Console, and a new token does not fix that. |
| `Connection to the Management Console is degraded` | The reason names the last error type. Find its counter in [Error counters](#error-counters). |
| `Communicator status is stale` | umh-core did not update the communicator status in time. Check the agent log for errors. If the reason stays, send the agent log to UMH support. |

## Error counters

The communicator counts each failed request to the Management Console once, under its cause. A counter that rises shows the cause. [Read the metrics](../agent/metrics.md#read-the-metrics) explains how to read them. To find the counter that rises without Prometheus, read the metrics once, wait a minute and read them again. The failed logins are the series whose `hierarchy_path` ends in `(transport)`. A counter appears only after its first count.

| Counter | Counts | What to do |
|---|---|---|
| `umh_fsmv2_worker_network_errors_total` | Requests without an answer that umh-core can classify, such as a refused connection, a timeout or a rejected certificate. It also counts answers with a status no other counter takes, such as 400 or a proxy's 407 Proxy Authentication Required. | See [Instance Stays Offline Behind a Corporate Firewall](instance-offline-behind-corporate-firewall.md). |
| `umh_fsmv2_worker_proxy_block_errors_total` | Error answers that look like the block page of a proxy. | Ask your IT department to allow `management.umh.app` on the proxy. |
| `umh_fsmv2_worker_auth_failures_total` | Answers with status 401 or 403. A firewall, a proxy or Cloudflare that answers 403 also counts here, unless umh-core recognizes the answer as a proxy block page. | Check the `AUTH_TOKEN` of your `docker run` command. If the token is correct, see [Instance Stays Offline Behind a Corporate Firewall](instance-offline-behind-corporate-firewall.md). |
| `umh_fsmv2_worker_instance_deleted_total` | Answers with status 404. A wrong `API_URL` or a 404 from a proxy also counts here. | Check whether the instance still exists in the Management Console, and check `API_URL`. |
| `umh_fsmv2_worker_server_errors_total` | Answers with a status of 500 or higher. | Wait. umh-core retries. If the counter keeps rising, contact UMH support. |
| `umh_fsmv2_worker_backend_rate_limit_errors_total` | Requests the Management Console rejected with status 429, because the instance sent too many in a short time. | umh-core retries. If the counter keeps rising, contact UMH support. |
| `umh_fsmv2_worker_cloudflare_errors_total` | Answers with status 429 that carry a Cloudflare challenge page. | umh-core retries. If the counter keeps rising, contact UMH support. |
