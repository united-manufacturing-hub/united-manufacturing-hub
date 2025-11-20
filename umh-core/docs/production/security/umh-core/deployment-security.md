# Deployment Security

Understanding what umh-core can access and why that's intentional.

## The Core Principle

umh-core is an edge gateway that connects to your factory devices and data sources. To do its job, it NEEDS:
- Network access to PLCs, SCADA systems, OPC UA servers, MQTT brokers
- Filesystem access to read configuration files, CSV data, logs

## What Your Data Flows Actually Do

When you create a bridge (protocol converter) or data flow, you're telling umh-core to connect to specific devices, files, or network services and pull data from them. This requires network and filesystem permissions.

## Container Isolation Model

umh-core runs in a Docker container with standard isolation.

### What's Isolated
- Separate process namespace (can't see host processes)
- Isolated network namespace (unless you explicitly use `--network=host`)
- Limited filesystem access (only what you mount)

### What Data Flows CAN Access

**Inside the container:**
- Entire container filesystem (ephemeral, resets on restart)
- `/data` directory (persistent storage for configs, logs, certificates)
- Network connections as defined by your Docker network mode

**Why this is necessary:**

Bridges run as benthos-umh processes (see [Bridges documentation](../../../usage/data-flows/bridges.md)) that need:

- **File access**: Reading CSV/JSON/XML files requires mounted directories. See [Benthos-UMH inputs](https://docs.umh.app/benthos-umh/input) for supported file formats.
- **Network access**: Industrial protocols like [OPC UA](https://docs.umh.app/benthos-umh/input/opc-ua-input), [Modbus](https://docs.umh.app/benthos-umh/input/modbus), and [Siemens S7](https://docs.umh.app/benthos-umh/input/siemens-s7) need TCP/IP connections to your equipment.
- **Certificate handling**: Security protocols need access to certificate files for authentication.

This is intentional - umh-core exists to connect your factory to the cloud.

**Example use case:**
```bash
# User mounts a shared directory to read production reports
docker run -v /mnt/production-data:/data/production umh-core:latest
```

Then creates a data flow that reads CSV files from `/data/production/*.csv` - this is intentional and documented.

### What Data Flows CANNOT Access

In standard deployment (without `--network=host` or extra mounts):
- Host filesystem outside mounted paths
- Host process namespace
- Host-only network interfaces (localhost services)
- Docker socket or container management

## Process Isolation and Security Trade-offs

### How umh-core Runs

All umh-core components run as a single non-root user (UID 1000, `umhuser`):
- The main umh-core agent
- All bridges (protocol converters)
- All data flows
- Redpanda and other services

### Why Not Per-Bridge Isolation?

**Technical constraint**: In non-root containers, processes cannot switch users. This PR investigated privilege dropping but found:
- s6-setuidgid requires CAP_SETUID capability (only available to root)
- s6-applyuidgid requires CAP_SETGID capability (only available to root)
- Adding these capabilities would partially defeat the security benefits of non-root containers

**Security trade-off**: We chose non-root container security over per-bridge isolation:
- ✅ Container cannot escalate to root privileges
- ✅ Standard Docker security model
- ✅ Compatible with restricted Kubernetes environments
- ❌ No isolation between bridges within the container

### What This Means for Security

**Shared access within container:**
- All bridges can read `/data/config.yaml` (contains AUTH_TOKEN)
- All bridges share access to mounted directories
- All bridges can see environment variables

**Container boundary still enforced:**
- Bridges cannot access host filesystem (except mounted paths)
- Bridges cannot see host processes
- Network isolation applies (unless using --network=host)

**Best practices:**
- Only deploy trusted bridge configurations
- Use network segmentation at the infrastructure level
- Mount only necessary directories with read-only where possible
- Monitor bridge activity through logs and metrics

## When to Mount Additional Paths

**Common scenarios where you SHOULD mount extra paths:**
- Reading log files from other systems (`-v /var/log/plc:/data/plc-logs:ro`)
- Importing data files from network shares (`-v /mnt/nas:/data/imports:ro`)

**Use read-only mounts when possible:** Add `:ro` suffix to prevent umh-core from writing to host

## Network Modes Explained

### Standard Mode (Recommended)

```bash
docker run -v /data:/data umh-core:latest
```

**What it does:**
- Container gets its own network stack
- Can reach external IPs/hostnames through Docker networking
- Cannot access host-only services (e.g., `localhost:5000`)

**Use for:**
- TCP/IP protocols: [OPC UA](https://docs.umh.app/benthos-umh/input/opc-ua-input), [Modbus TCP](https://docs.umh.app/benthos-umh/input/modbus), HTTP, MQTT
- Cloud services
- Network-attached devices with routable IPs

### Host Network Mode (Special Cases)

```bash
docker run --network=host -v /data:/data umh-core:latest
```

**What it does:**
- Container shares host network stack
- Can access host-only services
- Can use Layer 2 protocols (MAC addresses, broadcast)

**Use for:**
- Modbus RTU over serial ports
- Protocols requiring broadcast (some industrial protocols)
- Services only listening on `localhost`

**Trade-off:** Reduces network isolation, but necessary for certain protocols

## Environment Variables

### AUTH_TOKEN
**Required** for connecting to Management Console:
```bash
-e AUTH_TOKEN=your_token_here
```

### ALLOW_INSECURE_TLS
**Only use behind corporate firewalls** that perform TLS inspection:
```bash
-e ALLOW_INSECURE_TLS=true
```

⚠️ **Warning:** This disables certificate validation. Only use if you trust your corporate firewall and cannot add corporate CA certificates.

### Proxy Configuration
If your network requires a proxy:
```bash
-e HTTP_PROXY=http://proxy.company.com:8080 \
-e HTTPS_PROXY=https://proxy.company.com:8080 \
-e NO_PROXY=localhost,127.0.0.1,.local
```

For authenticated proxies:
```bash
-e HTTP_PROXY=http://username:password@proxy.company.com:8080
```

## Security Best Practices

### Do's
- Mount only the paths you need
- Use read-only mounts (`:ro`) when possible
- Configure your firewall to only give the container access to the IP addresses it needs
- Only deploy trusted bridge configurations (all bridges share the same user context)
- Set `AUTH_TOKEN` environment variable for Management Console auth
- Use standard network mode unless you specifically need host networking

### Don'ts
- Don't mount `/` (entire host filesystem) - there's no legitimate use case
- Don't use `ALLOW_INSECURE_TLS=true` in production (unless behind corporate firewall)
- Don't expose umh-core ports to the internet - it's designed as edge-only

## FAQ

**Q: Why does the security scanner flag network access?**
A: Because umh-core has network capabilities. That's intentional - it's an edge gateway that connects to factory equipment.

**Q: Should I be concerned about filesystem access?**
A: Only mount what you need. If you're mounting `/data` and a CSV import directory, that's normal and safe.

**Q: Is `--network=host` dangerous?**
A: It reduces isolation, but it's necessary for certain protocols. If you need Modbus RTU or Layer 2 protocols, use it. Otherwise, stick with standard mode.

**Q: Can data flows access my host system?**
A: Only through paths you explicitly mount and network interfaces you explicitly enable. Docker isolation is still active.

## Related Documentation

- [Network Configuration](./network-configuration.md) - Firewalls, proxies, TLS inspection
- [Bridges Documentation](../../../usage/data-flows/bridges.md) - How bridges work
- [Benthos-UMH Inputs](https://docs.umh.app/benthos-umh/input) - 50+ supported protocols
