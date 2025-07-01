# Corporate Firewalls

If you're behind a corporate firewall that performs TLS inspection (MITM), you might see certificate errors. In this case, you can:

1. **Recommended:** Add your corporate CA certificate to the container's trusted certificates
2. **Last Resort:** Set `allowInsecureTLS: true` in your config or use the `ALLOW_INSECURE_TLS=true` environment variable:

**Option A: Config file**
```yaml
agent:
  communicator:
    allowInsecureTLS: true  # WARNING: Only use if corporate firewall blocks secure connections
```

**Option B: Environment variable**
```bash
docker run -e ALLOW_INSECURE_TLS=true umh-core:latest
```

⚠️ **Security Warning:** The `allowInsecureTLS` option disables certificate validation. Only use this if:

* You're behind a corporate firewall that you trust
* You cannot add your corporate CA certificate
* You understand the security implications

## Proxy Configuration

If your network requires a proxy, add these environment variables to your Docker run command:

```bash
-e HTTP_PROXY=http://proxy.company.com:8080 \
-e HTTPS_PROXY=https://proxy.company.com:8080 \
-e NO_PROXY=localhost,127.0.0.1,.local
```

Supported proxy environment variables: `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` (and their lowercase variants).

For authenticated proxies, include credentials in the URL:
```bash
-e HTTP_PROXY=http://username:password@proxy.company.com:8080
```

Supported proxy types: HTTP and HTTPS.

## Common Configuration

In most corporate environments, proxy usage and TLS interception go together. If you need to configure a proxy, you'll likely also need to add your corporate CA certificate to handle TLS inspection. See both sections above for complete configuration.
