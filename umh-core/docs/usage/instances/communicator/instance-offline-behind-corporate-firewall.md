# Instance Stays Offline Behind a Corporate Firewall

This page explains how to find out why an instance behind a corporate firewall never shows as online in the Management Console, and how to fix it. The steps need access to the Docker host, to the firewall and proxy settings, and to the corporate CA certificate, so you may need your IT department.

The cause is usually one of three. The firewall blocks the connection, the network requires a proxy, or the firewall inspects TLS. A firewall that inspects TLS decrypts HTTPS traffic. It replaces the certificate of each server with one that your company's own certificate authority signs, the corporate CA.

## Which error the instance reports

The Management Console does not show why the connection fails (see [Communicator](README.md#the-communicator-card)). The error counters of the communicator do. [Metrics](../agent/metrics.md#read-the-metrics) explains how to collect them with Prometheus, and [Communicator](README.md#errors) explains each counter. To read them once, run this on the Docker host, wait a minute and run it again. A counter that rose between the two runs is the cause:

```bash
docker exec <container name> wget -qO- http://localhost:8080/metrics | grep -E 'network_errors_total|proxy_block_errors_total|auth_failures_total|instance_deleted_total|server_errors_total|rate_limit_errors_total|cloudflare_errors_total'
```

| Counter that rises | What to do |
|---|---|
| `umh_fsmv2_worker_network_errors_total` | Continue with [Find the cause](#find-the-cause). |
| `umh_fsmv2_worker_proxy_block_errors_total` | Ask your IT department to allow `management.umh.app` on the proxy. |
| `umh_fsmv2_worker_auth_failures_total` | Check the `AUTH_TOKEN` of your `docker run` command. If the token is correct, continue with [Find the cause](#find-the-cause). |
| Any other counter | See [Communicator](README.md#errors). |

If no counter rises, read the full `/metrics` output.

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

The proxy variables of the container do not apply here, because the Docker daemon pulls the image, not the container. If the host needs a proxy for the pull, configure it for the Docker daemon (see [Docker Engine](https://docs.docker.com/engine/daemon/proxy/) and [Docker Desktop](https://docs.docker.com/desktop/settings-and-maintenance/settings/#proxies)).

On Linux, put the corporate CA certificate file at `/etc/docker/certs.d/management.umh.app/ca.crt` on the Docker host. On Docker Desktop, add it to the trust store of the operating system and restart Docker Desktop.
