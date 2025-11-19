# Deployment Security

Container isolation and security boundaries for umh-core deployments.

## Container Isolation Model

umh-core uses standard Docker container isolation:

- Separate process namespace
- Isolated network namespace (unless `--network=host`)
- Limited filesystem access through volume mounts

## Volume Mounts

### Standard Deployment

Only the `/data` volume is mounted:

```bash
docker run -v /path/on/host:/data umh-core:latest
```

### What Bridges CAN Access

Bridges (Benthos processes) run inside the container and can access:

- Entire container filesystem
- `/data` directory (persistent storage, config files, logs)
- Network connections as defined by container network mode

### What Bridges CANNOT Access

In standard deployment, bridges cannot access:

- Host filesystem outside the `/data` mount
- Host process namespace
- Host network interfaces (unless `--network=host` is used)

**Important:** Avoid mounting additional host paths unless absolutely necessary. Each additional mount expands the container's access to the host system.

## Environment Variables

| Variable | Purpose | Security Notes |
|----------|---------|----------------|
| `AUTH_TOKEN` | Management Console authentication | Keep secret, do not log |
| `ALLOW_INSECURE_TLS` | Disable TLS certificate validation | Only for trusted corporate firewalls |
| `HTTP_PROXY` / `HTTPS_PROXY` | Proxy configuration | May contain credentials |
| `LOG_LEVEL` | Logging verbosity | Debug level may expose sensitive data |

### Setting Environment Variables

```bash
docker run \
  -e AUTH_TOKEN=your-secret-token \
  -v /path/on/host:/data \
  umh-core:latest
```

## Recommendations

1. **Minimal mounts** - Only mount `/data`, avoid additional host paths
2. **Protect AUTH_TOKEN** - Use secrets management, don't hardcode in scripts
3. **Avoid insecure TLS** - Only use `ALLOW_INSECURE_TLS` when necessary and understood
4. **Review log levels** - Debug logging may expose sensitive information
5. **Use standard network mode** - Only use `--network=host` when required for Layer 2 protocols
