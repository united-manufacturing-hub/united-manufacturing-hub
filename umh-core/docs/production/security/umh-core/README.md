# umh-core Security

## Quick Checklist

Things to keep in mind when deploying umh-core:

**Container Setup:**
- Mount `/data` for persistent storage - mount extra paths only when you need them
- Configure your firewall to only give the container access to the IP addresses it needs
- Set `AUTH_TOKEN` for Management Console connection
- Only use `ALLOW_INSECURE_TLS=true` if you're behind a corporate firewall with TLS inspection

**Network:**
- Allow outbound HTTPS to `management.umh.app`
- Configure proxy if needed (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`)
- Add corporate CA certs if your firewall does TLS inspection
