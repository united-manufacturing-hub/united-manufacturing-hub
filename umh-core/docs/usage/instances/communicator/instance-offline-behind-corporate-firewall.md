# Instance Stays Offline Behind a Corporate Firewall

An instance behind a corporate firewall that never shows as online in the Management Console usually has one of three causes. The firewall blocks the connection, the network requires a proxy, or the firewall inspects TLS. A firewall that inspects TLS decrypts HTTPS traffic. It replaces the certificate of each server with one that your company's own certificate authority signs, the corporate CA.

## Which error the instance reports

The Management Console shows a red alert, "Instance Offline", once 15 seconds pass without a message from the instance. The cards keep showing the last status the instance sent, if it sent one. They do not show why the connection fails. The error counters of the communicator count failed requests by cause, with one series per worker. The failed logins are the series whose `hierarchy_path` ends in `(transport)`. [Metrics](../agent/metrics.md#read-the-metrics) explains how to read the metrics and how to collect them with Prometheus. [Communicator](README.md#errors) explains each counter. To read the three counters once, run this on the Docker host:

```bash
docker exec <container name> wget -qO- http://localhost:8080/metrics | grep -E 'network_errors_total|proxy_block_errors_total|auth_failures_total'
```

Find the counter that keeps rising in this table.

| Counter | Meaning |
|---|---|
| `umh_fsmv2_worker_network_errors_total` | The login failed without an answer that umh-core can classify. Either it never reached the Management Console, or something on the way answered with a status umh-core does not classify. A proxy that requires a login answers with status 407, and that counts here too. The counter does not say which case applies. Continue with [Find the cause](#find-the-cause). |
| `umh_fsmv2_worker_proxy_block_errors_total` | A proxy answered with a block page instead of the Management Console. Ask your IT department to allow `management.umh.app` on the proxy. |
| `umh_fsmv2_worker_auth_failures_total` | The login was answered with status 401 or 403. Usually the Management Console rejected the `AUTH_TOKEN` from your `docker run` command. A proxy or Cloudflare can also answer 403. If the token is correct, continue with [Find the cause](#find-the-cause). |

## Find the cause

Run this command on the Docker host. It connects to `management.umh.app` and prints who issued the certificate it receives. If the host uses a proxy, add `-proxy <proxy host>:<port>` before `-connect`.

```bash
echo | openssl s_client -connect management.umh.app:443 -servername management.umh.app 2>/dev/null | grep '^issuer='
```

| What you see | Cause | What to do |
|---|---|---|
| A public certificate authority, for example `issuer=C=US, O=Let's Encrypt, …` | Nothing on the path replaces the certificate | Check that the instance uses the same proxy as the host. See [Network Configuration](../../../production/security/umh-core/network-configuration.md#proxy-configuration) |
| An issuer that names your company or your firewall product | The firewall inspects TLS | [Add your corporate CA to the container](#add-your-corporate-ca-to-the-container) |
| No output | The connection failed. The firewall blocks `management.umh.app`, or the network requires a proxy | Ask your IT department which one applies. Then allow the connections in [Network Configuration](../../../production/security/umh-core/network-configuration.md#outbound-connections), or set the proxy variables in [Network Configuration](../../../production/security/umh-core/network-configuration.md#proxy-configuration) |

A network that needs a proxy often inspects TLS too. Set the proxy first, then check the counters again.

## Add your corporate CA to the container

umh-core trusts only public certificate authorities. It rejects the certificate that the firewall puts in place, and `umh_fsmv2_worker_network_errors_total` rises. It always checks the certificate when it logs in to the Management Console. `ALLOW_INSECURE_TLS` does not turn this check off (see [TLS and Certificates](../../../reference/tls-and-certificates.md#allow_insecure_tls)).

1. Get the corporate CA certificate file from your IT department in PEM format. A PEM file is text and starts with `-----BEGIN CERTIFICATE-----`. If you get a binary `.cer` or `.der` file instead, convert it:

   ```bash
   openssl x509 -inform der -in corporate-ca.cer -out corporate-ca.pem
   ```

2. Put the file in a directory on the Docker host. The container runs as user ID 1000. That user must be able to read the file:

   ```bash
   sudo mkdir -p /opt/umh/certs
   sudo cp corporate-ca.pem /opt/umh/certs/
   sudo chmod 644 /opt/umh/certs/corporate-ca.pem
   ```

3. Stop and remove the container. `docker restart` is not enough, because it keeps the flags the container was created with.

   {% hint style="warning" %}
   Removing the container stops umh-core and every bridge in it until you start it again in the next step. A bridge is a connection that reads data from a machine. Values the bridges would read during this time are not collected. The configuration and data in the `/data` volume stay. Plan this for a maintenance window.
   {% endhint %}

   ```bash
   docker rm -f <container name>
   ```

4. Start the container again with the same `docker run` command as before, and add two flags. The first mounts the directory. The second tells umh-core to load certificates from it. In the example, `<your original flags>` stands for the flags of your original command. Replace it with those flags, one per line and each line ending in `\`, or delete the line if there are none.

   ```bash
   docker run \
     -v /opt/umh/certs:/certs:ro \
     -e SSL_CERT_DIR=/etc/ssl/certs:/certs \
     <your original flags> \
     management.umh.app/oci/united-manufacturing-hub/umh-core:<VERSION>
   ```

   The bridges run in the same container and receive the same `SSL_CERT_DIR`.

5. Wait a minute. The instance shows as online in the Management Console, and `umh_fsmv2_worker_network_errors_total` stops rising.

## Image pulls fail with an x509 error

The Docker host pulls the umh-core image from `management.umh.app` through the same firewall. If the firewall inspects TLS, `docker pull` fails with an error that ends in `x509: certificate signed by unknown authority`. The steps above do not fix this, because Docker has its own trust store.

On Linux, put the corporate CA certificate file at `/etc/docker/certs.d/management.umh.app/ca.crt` on the Docker host. On Docker Desktop, add it to the trust store of the operating system and restart Docker Desktop.
