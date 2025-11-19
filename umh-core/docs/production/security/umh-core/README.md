# umh-core Security

Security documentation for umh-core edge deployments.

## Documentation

| Document | Description |
|----------|-------------|
| [deployment-security.md](./deployment-security.md) | Container isolation, volume mounts, environment variables |
| [bridge-access-model.md](./bridge-access-model.md) | Bridge permissions, deployment modes, protocol considerations |
| [network-configuration.md](./network-configuration.md) | Outbound connections, corporate firewalls, proxy settings |

## Quick Security Checklist

### Container Deployment

- [ ] Mount only `/data` volume (avoid mounting additional host paths)
- [ ] Use standard network mode (not `--network=host`) unless Layer 2 protocols required
- [ ] Set `AUTH_TOKEN` environment variable for Management Console authentication
- [ ] Never set `ALLOW_INSECURE_TLS=true` in production unless behind trusted corporate firewall

### Network Configuration

- [ ] Allow outbound HTTPS to `management.umh.app`
- [ ] Configure proxy settings if required (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`)
- [ ] Add corporate CA certificates if TLS inspection is performed

### Bridge Configuration

- [ ] Review bridge access requirements before enabling host network mode
- [ ] Understand protocol-specific security implications (OPC UA certificates, Modbus network access)
