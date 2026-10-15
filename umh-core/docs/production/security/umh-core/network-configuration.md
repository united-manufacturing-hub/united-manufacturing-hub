# Network Configuration

Network requirements and configuration for umh-core edge deployments.

## Outbound Connections

umh-core requires outbound connectivity to the Management Console:

| Destination | Protocol | Port | Purpose |
|-------------|----------|------|---------|
| `management.umh.app` | HTTPS | 443 | Configuration sync, status reporting, action retrieval |
| `management.umh.app` | HTTPS | 443 | Container image pulls by the Docker host |

No inbound connections are required from the internet.

Allowlist the hostname in your firewall. Do not allowlist IP addresses or certificate fingerprints, because both can change without notice. See [TLS and Certificates](../../../reference/tls-and-certificates.md#hosts-and-certificates) for the certificates and TLS versions.

## Corporate Firewall Configuration

### TLS Inspection (MITM)

A firewall that inspects TLS replaces the certificate of `management.umh.app` with one that your corporate certificate authority (CA) signs. Until you add your corporate CA to the container, umh-core rejects that certificate and the instance stays offline in the Management Console. [Instance Stays Offline Behind a Corporate Firewall](../../../usage/instances/communicator/instance-offline-behind-corporate-firewall.md) shows how to confirm this and gives the steps.

## Proxy Configuration

If your network requires a proxy, add the proxy variables to the `docker run` command. An existing container must be removed and started again with the new flags, as in [step 3](../../../usage/instances/communicator/instance-offline-behind-corporate-firewall.md#add-your-corporate-ca-to-the-container) of the corporate CA steps. The container's bridges stop until it is back up.

```bash
docker run \
  -e HTTP_PROXY=http://proxy.company.com:8080 \
  -e HTTPS_PROXY=http://proxy.company.com:8080 \
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

In most corporate environments, a proxy and TLS inspection go together. In that case the container needs both the proxy variables above and your corporate CA. [Instance Stays Offline Behind a Corporate Firewall](../../../usage/instances/communicator/instance-offline-behind-corporate-firewall.md#add-your-corporate-ca-to-the-container) gives the steps for the CA.
