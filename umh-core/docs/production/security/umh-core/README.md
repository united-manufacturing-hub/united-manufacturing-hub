# umh-core Security

Security documentation for umh-core edge deployments.

## Topics

- [Deployment Security](./deployment-security.md) - Container isolation, volume mounts, network modes
- [Network Configuration](./network-configuration.md) - Corporate firewalls and proxies

## Quick Checklist

Things to keep in mind when deploying umh-core:

**Container Setup:**
- Mount `/data` for persistent storage - mount extra paths only when you need them
- Use standard network mode unless you're doing Modbus RTU or other Layer 2 protocols
- Set `AUTH_TOKEN` for Management Console connection
- Only use `ALLOW_INSECURE_TLS=true` if you're behind a corporate firewall with TLS inspection

**Network:**
- Allow outbound HTTPS to `management.umh.app`
- Configure proxy if needed (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`)
- Add corporate CA certs if your firewall does TLS inspection

**Protocols:**
- OPC UA needs certificate files mounted
- Modbus RTU needs host network mode
- Most TCP/IP protocols work in standard mode
