# Bridge Access Model

Security model for bridges (protocol converters) running inside umh-core.

## Overview

Bridges run as Benthos processes inside the umh-core container. Their access to the host system depends on the deployment mode.

## Deployment Modes

### Standard Deployment (Default)

```bash
docker run -v /path/on/host:/data umh-core:latest
```

**Network Access:**
- Container network isolation
- Bridges can reach external IPs through container networking
- Cannot access host-only network interfaces

**Filesystem Access:**
- Only `/data` directory is persistent
- No access to host filesystem outside mount

**Use Cases:** Most TCP/IP-based protocols (OPC UA, MQTT, Modbus TCP)

### Layer 2 / Host Network Deployment

```bash
docker run --network=host -v /path/on/host:/data umh-core:latest
```

**Network Access:**
- Full host network stack access
- Can access all host network interfaces
- Required for protocols that need Layer 2 access or specific interface binding

**Filesystem Access:**
- Same as standard deployment (only `/data`)

**Security Implications:**
- Bridges can see all host network traffic
- Can bind to any port on the host
- No network isolation from other host processes

**Use Cases:** Modbus RTU over serial, protocols requiring specific network interfaces

## Protocol-Specific Considerations

| Protocol | Network Mode | Security Notes |
|----------|--------------|----------------|
| **OPC UA** | Standard | Requires certificate management; bridges access certificate files in `/data` |
| **Modbus TCP** | Standard | Consider network segmentation for PLC networks |
| **Modbus RTU** | Host Network | Requires serial device access; mount `/dev/ttyUSB*` if needed |
| **S7** | Standard | Industrial network access; firewall rules recommended |
| **MQTT** | Standard | TLS certificates stored in `/data`; broker credentials in config |

## Risk Assessment Matrix

| Deployment Mode | Network Access | Filesystem Access | Risk Level | Use When |
|-----------------|----------------|-------------------|------------|----------|
| Standard | Container isolated | `/data` only | Low | Default for most deployments |
| Host Network | Full host stack | `/data` only | Medium | Layer 2 protocols, specific interface binding |
| Additional Mounts | Varies | Extended host access | High | Avoid unless absolutely necessary |

## Recommendations

1. **Default to standard deployment** - Use container network isolation unless protocol requires host network
2. **Document host network usage** - If `--network=host` is required, document why
3. **Segment industrial networks** - Keep PLC networks separate from IT networks
4. **Review serial device access** - Only mount specific devices needed (e.g., `/dev/ttyUSB0`)
5. **Manage certificates properly** - Store OPC UA and TLS certificates securely in `/data`
6. **Use least privilege** - Only grant the access each bridge actually needs
