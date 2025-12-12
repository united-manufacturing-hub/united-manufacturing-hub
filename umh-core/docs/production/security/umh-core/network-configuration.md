# Network Configuration

Network requirements and configuration for umh-core edge deployments.

## Outbound Connections

umh-core requires outbound connectivity to the Management Console:

| Destination | Protocol | Port | Purpose |
|-------------|----------|------|---------|
| `management.umh.app` | HTTPS | 443 | Configuration sync, status reporting, action retrieval |

No inbound connections are required from the internet.

## Corporate Firewall Configuration

### TLS Inspection (MITM)

If your corporate firewall performs TLS inspection, you may see certificate errors. Solutions:

**Option 1: Add Corporate CA Certificate (Recommended)**

Add your corporate CA certificate to the container's trusted certificates.

**Option 2: Disable Certificate Validation (Last Resort)**

```yaml
# config.yaml
agent:
  communicator:
    allowInsecureTLS: true  # WARNING: Only use if corporate firewall blocks secure connections
```

Or via environment variable:

```bash
docker run -e ALLOW_INSECURE_TLS=true management.umh.app/oci/united-manufacturing-hub/umh-core:latest
```

**Security Warning:** The `allowInsecureTLS` option disables certificate validation. Only use this if:

- You're behind a corporate firewall that you trust
- You cannot add your corporate CA certificate
- You understand the security implications

## Proxy Configuration

If your network requires a proxy:

```bash
docker run \
  -e HTTP_PROXY=http://proxy.company.com:8080 \
  -e HTTPS_PROXY=https://proxy.company.com:8080 \
  -e NO_PROXY=localhost,127.0.0.1,.local \
  management.umh.app/oci/united-manufacturing-hub/umh-core:latest
```

Supported environment variables: `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` (and their lowercase variants).

### Authenticated Proxies

Include credentials in the proxy URL:

```bash
-e HTTP_PROXY=http://username:password@proxy.company.com:8080
```

Supported proxy types: HTTP and HTTPS.

## Common Configuration

In most corporate environments, proxy usage and TLS inspection go together. If you need to configure a proxy, you'll likely also need to add your corporate CA certificate to handle TLS inspection.
