# umh-core Security

## Relevant OWASP Standards

For umh-core as software, the following OWASP standards are relevant:

| Standard | Status | Implementation |
|----------|--------|----------------|
| **OWASP Docker Security #2** | ✅ Compliant | Non-root container (UID 1000, umhuser) |
| **OWASP Docker Security #0** | ✅ Compliant | Regular updates via CI/CD pipeline |
| **OWASP IoT Top 10 I1** | ✅ Compliant | No default passwords (AUTH_TOKEN user-configured) |
| **OWASP IoT Top 10 I9** | ✅ Compliant | Secure defaults (TLS enabled, auth required) |
| **OWASP OT Top 10 #9** | 📋 Documented | Protocol security limitations (Modbus, S7 lack encryption) |
| **OWASP OT Top 10 #10** | ✅ Compliant | Non-root containers, minimal base image |

**Supply chain security**: Container images scanned via Aikido, OSS licenses via FOSSA. ISO 27001 audit in progress. Status: https://trust.umh.app

---

## What umh-core Accesses and Why

### Filesystem Access

**Required**: `/data` directory (persistent storage)
- Configuration files (config.yaml with AUTH_TOKEN)
- Logs (rolling logs for all services)
- Redpanda data (message broker storage)
- Certificates (TLS certs for protocols)

**Optional**: Customer-defined mounts for file-based inputs
- CSV/JSON/XML data files from network shares
- Log files from other systems
- Production reports from local disks

**Why**: Bridges need to read data files and persist configuration across container restarts.

**Access pattern**: All processes run as UID 1000 (umhuser), read/write access to mounted paths.

---

### Network Access

**Required outbound**: HTTPS to `management.umh.app` (port 443)
- Configuration sync from Management Console
- Status reporting and heartbeat
- Action retrieval (deploy/update bridges)

**Data source connections** (customer-defined):
- Industrial protocols: OPC UA, Modbus TCP, Siemens S7, MQTT
- Network services: HTTP APIs, database servers
- IP addresses and ports: Fully configurable per bridge

**Why**: umh-core is a protocol converter connecting factory devices to cloud. This is the product's purpose.

**Inbound**: No services designed for internet exposure (edge-only deployment).

---

### Process Model

**All components run as single non-root user (UID 1000, umhuser)**:
- Main umh-core agent
- All bridges (benthos-umh instances)
- All data flows
- Redpanda broker
- Internal services

**Why non-root**: Container cannot escalate privileges, compatible with restricted Kubernetes environments, standard Docker security model.

**Container isolation**: Separate process namespace (can't see host processes), isolated network namespace (unless `--network=host`), limited filesystem access (only mounted paths).

---

## Known Limitations

### AUTH_TOKEN in Environment Variable

**Category**: Known Limitation (cannot fix in single-container architecture)

**Issue**: AUTH_TOKEN shared secret is stored in:
- Environment variable (`AUTH_TOKEN=xxx`)
- Configuration file (`/data/config.yaml`)

Both are readable by all processes running as umhuser (UID 1000).

**Why this cannot be fixed**:
- Per-process secrets require process isolation (see below)
- External secrets managers add complexity unsuitable for edge devices
- File-based secrets still readable by all processes in non-root container

**Risk**: Malicious bridge configuration could exfiltrate AUTH_TOKEN.

**Mitigation** (customer responsibility):
- Only deploy trusted bridge configurations
- Monitor network connections for unexpected destinations
- Rotate AUTH_TOKEN periodically via Management Console
- Use network segmentation to limit bridge internet access

---

### No Per-Bridge Isolation

**Category**: Accepted Risk (design trade-off)

**Technical constraint**: Per-bridge user isolation is not possible in non-root containers. Process-level user switching requires CAP_SETUID and CAP_SETGID capabilities, which are only available to root processes.

**Design decision**: Non-root container security prioritized over per-bridge isolation.

**Security trade-off**:
- ✅ Container cannot escalate to root privileges
- ✅ Standard Docker security model
- ✅ Compatible with restricted Kubernetes environments
- ❌ No isolation between bridges within container

**Shared access within container**:
- All bridges can read `/data/config.yaml` (contains AUTH_TOKEN)
- All bridges share access to mounted directories
- All bridges can see environment variables

**Container boundary still enforced**:
- Bridges cannot access host filesystem (except mounted paths)
- Bridges cannot see host processes
- Network isolation applies (unless using --network=host)

**Mitigation** (customer responsibility):
- Only deploy trusted bridge configurations
- Use network segmentation at infrastructure level
- Monitor bridge activity through logs and metrics
- Treat all bridges in an instance as same trust level

---

### TLS Certificate Validation Can Be Disabled

**Category**: Accepted Risk (corporate firewall compatibility)

**Issue**: `ALLOW_INSECURE_TLS=true` option disables certificate validation.

**Why this option exists**: Corporate firewalls often perform TLS inspection (MITM), and adding corporate CA certificates is complex.

**Risk**: MITM attacks possible if misused in production.

**Usage guidance**: Only use behind trusted corporate firewall. See [Network Configuration](./network-configuration.md) for details.

---

## Security Best Practices

### What to Mount

**Required**: `/data` for persistent storage (configs, logs, certificates)

**Common additional mounts**:
- Data files: Network shares with CSV/JSON/XML production data
- Log files: From PLCs or other systems
- Use read-only mounts when bridges only need to read

### What NOT to Mount

- Entire host filesystem (`/`)
- Docker socket (`/var/run/docker.sock`)
- Host `/etc` or `/var` directories

### Network Configuration

**Firewall requirements**: Allow outbound HTTPS to `management.umh.app`, connections to your data sources (PLCs, OPC UA servers, MQTT brokers).

**Network segmentation**: Configure your firewall to only give the container access to necessary IP addresses.

See [Network Configuration](./network-configuration.md) for proxy settings and TLS inspection handling.

### Bridge Configuration Security

**Critical**: All bridges run as same user (UID 1000) and can access AUTH_TOKEN.

**Therefore**:
- Only deploy trusted bridge configurations
- Review configurations before deployment
- Monitor bridge network connections
- Use network-level restrictions (firewall rules, network policies)

### Monitoring

**What to monitor**:
- Authentication failures to Management Console
- Unexpected network connections from bridges
- Resource constraint events (bridge creation blocked)
- FSM state transition failures

**Logs location**: `/data/logs/` (umh-core, benthos-*, redpanda)

---

## Customer Deployment Responsibilities

The following security configurations are **customer's responsibility** during deployment:

| OWASP Standard | Configuration | How to Implement |
|----------------|---------------|------------------|
| Container capabilities | Drop unnecessary capabilities | Docker: `--cap-drop=ALL`, Kubernetes: SecurityContext |
| AppArmor/SELinux | Security profiles | Apply appropriate profiles for your environment |
| Resource limits | CPU/memory constraints | Docker: `--memory`, `--cpus`, Kubernetes: resources |
| Network policies | Restrict pod-to-pod communication | Kubernetes NetworkPolicies or firewall rules |
| Secrets management | Protect AUTH_TOKEN at orchestrator level | Kubernetes Secrets with RBAC, Docker Secrets |
| Volume encryption | Encrypt `/data` volume | Enable at host/storage layer |

**Why these are excluded**: These require orchestration-level configuration (Docker runtime flags, Kubernetes manifests, host OS settings). umh-core provides secure **software**, but cannot enforce **deployment** security.

**Deployment security guidance**:
- [OWASP Docker Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Docker_Security_Cheat_Sheet.html)
- [CIS Docker Benchmark](https://www.cisecurity.org/benchmark/docker)
- [Kubernetes Security Best Practices](https://kubernetes.io/docs/concepts/security/)

---

## Related Documentation

- [Network Configuration](./network-configuration.md) - Proxy settings, TLS inspection, firewall requirements
- [Bridges Documentation](../../../usage/data-flows/bridges.md) - How bridges work
- [Benthos-UMH Inputs](https://docs.umh.app/benthos-umh/input) - 50+ supported protocols
- [Security Status](https://trust.umh.app) - ISO 27001 audit status, penetration testing, compliance

---

## Security Reporting

**For security issues**: security@umh.app (responsible disclosure)

**Do NOT**: Create public GitHub issues for security vulnerabilities

**Timeline**: Acknowledgment within 48 hours, fix timeline within 1 week
