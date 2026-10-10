# Network Configuration

Network requirements and configuration for umh-core edge deployments.

## Outbound Connections

umh-core requires outbound connectivity to the Management Console:

| Destination | Protocol | Port | Purpose |
|-------------|----------|------|---------|
| `management.umh.app` | HTTPS | 443 | Configuration sync, status reporting, action retrieval (API under `/api`) |
| `management.umh.app` | HTTPS | 443 | Container image pulls by the Docker host (registry under `/oci`) |

No inbound connections are required from the internet.

Allowlist the hostname in your firewall, not IP addresses or certificate fingerprints. Both can change without notice. See [TLS and Certificates](../../../reference/tls-and-certificates.md#hosts-and-certificates) for the certificates and TLS versions.

## Corporate Firewall Configuration

### TLS Inspection (MITM)

A firewall that inspects TLS replaces the public certificate of `management.umh.app` with a certificate that your corporate certificate authority (CA) signs. umh-core trusts only the public CAs by default, so it refuses this connection. The umh-core logs then show an error like this:

```
tls: failed to verify certificate: x509: certificate signed by unknown authority
```

umh-core always validates the certificate. You cannot turn this validation off. `ALLOW_INSECURE_TLS` has no effect since v0.44.42. Add your corporate CA certificate to the container instead:

1. Get the corporate CA certificate from your IT department in PEM format. A PEM file starts with `-----BEGIN CERTIFICATE-----`.
2. Put the file in a directory on the host, for example `/opt/umh/certs/corporate-ca.pem`. The container runs as UID 1000, so this user must be able to read the file.
3. Mount the directory into the container and add it to `SSL_CERT_DIR`:

   ```bash
   docker run \
     -v /opt/umh/certs:/certs:ro \
     -e SSL_CERT_DIR=/etc/ssl/certs:/certs \
     ...
     management.umh.app/oci/united-manufacturing-hub/umh-core:<VERSION>
   ```

   Keep `/etc/ssl/certs` in the list, so that umh-core continues to trust the public CAs.
4. Restart the container and make sure that the instance shows as online in the Management Console.

`SSL_CERT_DIR` also applies to the bridges, because they run inside the same container. They then trust servers that your corporate CA signs.

The Docker host pulls the image from `management.umh.app/oci` through the same firewall, so the Docker daemon must also trust the corporate CA. Put the certificate at `/etc/docker/certs.d/management.umh.app/ca.crt` on the host, or add it to the trust store of the host operating system and restart Docker.

## Proxy Configuration

If your network requires a proxy:

```bash
docker run \
  -e HTTP_PROXY=http://proxy.company.com:8080 \
  -e HTTPS_PROXY=https://proxy.company.com:8080 \
  -e NO_PROXY=localhost,127.0.0.1,.local \
  management.umh.app/oci/united-manufacturing-hub/umh-core:<VERSION>
```

Supported environment variables: `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` (and their lowercase variants).

### Authenticated Proxies

Include credentials in the proxy URL:

```bash
-e HTTP_PROXY=http://username:password@proxy.company.com:8080
```

Supported proxy types: HTTP and HTTPS.

## Common Configuration

In most corporate environments, proxy usage and TLS inspection go together. If you need to configure a proxy, you'll likely also need to add your corporate CA certificate as described in [TLS Inspection (MITM)](#tls-inspection-mitm). A complete command then looks like this:

```bash
docker run \
  -e HTTP_PROXY=http://proxy.company.com:8080 \
  -e HTTPS_PROXY=http://proxy.company.com:8080 \
  -e NO_PROXY=localhost,127.0.0.1,.local \
  -v /opt/umh/certs:/certs:ro \
  -e SSL_CERT_DIR=/etc/ssl/certs:/certs \
  ...
  management.umh.app/oci/united-manufacturing-hub/umh-core:<VERSION>
```
