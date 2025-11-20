# umh-core Security

## Relevant OWASP Standards

For umh-core as software, the following OWASP standards are relevant:

| Standard | Status | Implementation |
|----------|--------|----------------|
| **OWASP Docker Security #2** | ✅ Compliant (2025-02) | Non-root container (UID 1000, umhuser) |
| **OWASP Docker Security #0** | ✅ Compliant (2025-02) | Regular updates via CI/CD pipeline |
| **OWASP IoT Top 10 I1** | ✅ Compliant (2025-02) | No default passwords (AUTH_TOKEN user-configured) |
| **OWASP IoT Top 10 I9** | ✅ Compliant (2025-02) | Secure defaults (TLS enabled, auth required) |
| **OWASP OT Top 10 #9** | 📋 Documented (2025-02) | Protocol security limitations (Modbus, S7 lack encryption) |
| **OWASP OT Top 10 #10** | ✅ Compliant (2025-02) | Non-root containers, minimal base image |

**Supply chain security**: Container images scanned via Aikido, OSS licenses via FOSSA. ISO 27001 audit in progress. Status: https://trust.umh.app

---

## Threat Model (Simplified)

umh-core **primarily protects against**:
- **Unintentional compromise of external industrial systems** due to vulnerabilities in our software (we don't run as root, minimal network attack surface, TLS by default)
- **Supply chain risks** (signed images, vulnerability scanning, SBOM)
- **Misconfiguration leading to internet exposure** by default (we design for edge-only deployment)

umh-core **does not protect against**:
- **A malicious operator with configuration access** (Management Console UI or filesystem access to config.yaml) who deploys bridge configurations that:
  - Connect to and compromise external industrial systems
  - Exfiltrate AUTH_TOKEN via outbound network requests
  - Read sensitive data from mounted volumes
- **Compromise of the container runtime, host OS, or Kubernetes control plane**

This model aligns with industry-standard edge gateway security - we secure our software, you secure your infrastructure.

---

## Deployment Model: Edge-Only Architecture

umh-core is designed for **edge-only deployment**, which means:

**Network architecture**:
- ✅ **Outbound HTTPS** to `management.umh.app` (configuration sync, required)
- ✅ **Outbound connections** to data sources (MQTT brokers, OPC UA servers, Modbus devices, APIs)
- ❌ **No inbound internet connections** - no services designed for internet exposure
- ❌ **No public-facing APIs** - GraphQL API is for local access only (localhost:8090)

**Typical deployment location**: Factory floor, behind corporate firewall, on-premises

**Why this matters for security**:
- Attack surface reduced (no services listening for inbound internet connections)
- Management Console cannot push commands; umh-core pulls configuration changes
- Network segmentation best practice: umh-core sits between OT networks and IT infrastructure

**Not "air-gapped"**: umh-core requires outbound internet access to function. It is not designed for fully air-gapped/disconnected environments.

---

## What umh-core Accesses and Why

### Filesystem Access

**Required**: `/data` directory (persistent storage)
- Configuration files (config.yaml with AUTH_TOKEN)
- Logs (rolling logs for all services)
- Redpanda data (message broker storage)

**Optional**: Customer-defined mounts for file-based inputs
- CSV/JSON/XML data files from network shares
- Log files from other systems
- Production reports from local disks

**Why**: Bridges need to read data files and persist configuration across container restarts.

**Access pattern**: Access depends on mount configuration (e.g., `:ro` for read-only, `:rw` for read-write).

---

### Network Access

**Required outbound**: HTTPS to `management.umh.app` for configuration sync and status reporting.

See [Network Configuration](./network-configuration.md) for details on proxy settings and TLS inspection.

**Data source connections**: Customer-defined connections to industrial devices and data sources.

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

**Persistence behavior**: Setting `AUTH_TOKEN` via environment variable (`docker run -e AUTH_TOKEN=xxx`) writes it to `/data/config.yaml` **permanently**. On subsequent container restarts, the value from config.yaml is used even if the environment variable is not set.

**Why this design**: Ensures configuration persists across container restarts without requiring environment variables every time. Once set via environment variable or Management Console, AUTH_TOKEN is stored in `/data/config.yaml` on the persistent volume.

**Security implication**: Both storage locations are readable by all processes running as umhuser (UID 1000). This includes:
- All bridges (protocol converters, data flows, stream processors)
- Any process started within the container
- Any code executed via bridge configurations

**Risk**: Malicious bridge configuration could exfiltrate AUTH_TOKEN via outbound network requests.

**What you should do**:
1. **Accept the risk** if you control all bridge configurations (recommended for most deployments)
2. **Monitor** network connections for unexpected outbound traffic (exfiltration attempts)
3. **Rotate if compromised**:
   - Create new instance in Management Console
   - Copy new AUTH_TOKEN to deployment configuration
   - Update container environment variable or config.yaml
   - Remove old instance from Management Console

---

### No User/Process Isolation Between Bridges

**Category**: Accepted Risk (design trade-off)

**What this means**: All bridges (protocol converters, data flows, stream processors) run as the same Linux user (UID 1000, umhuser). There is **no user-level or process-level isolation** between different bridges within the container.

**What this does NOT mean**:
- ❌ Bridges do NOT interfere with each other's data processing (Redpanda isolates message flows by topic)
- ❌ Bridges are NOT resource-limited together (each bridge can have separate CPU/memory limits via s6-softlimit)
- ❌ Data is NOT shared between bridges (each benthos instance has separate configuration and state)

**Technical constraint**: Per-bridge user isolation is not possible in non-root containers. Process-level user switching requires CAP_SETUID and CAP_SETGID capabilities, which are only available to root processes.

**Design decision**: Non-root container security prioritized over per-bridge user isolation.

**Security implications**:

**Shared access within container** (because all run as same user):
- All bridges can read `/data/config.yaml` (contains AUTH_TOKEN)
- All bridges share access to mounted directories
- All bridges can see each other's environment variables
- All bridges can read each other's configuration files

**Container boundary still enforced**:
- Bridges cannot access host filesystem (except mounted paths)
- Bridges cannot see host processes
- Network isolation applies (unless using --network=host)

**Why non-root is worth the trade-off**:
- ✅ Container cannot escalate to root privileges (even if bridge is compromised)
- ✅ Standard Docker security model (defense in depth)
- ✅ Compatible with restricted Kubernetes environments (no special permissions needed)

---

### TLS Certificate Validation Can Be Disabled

**Category**: Accepted Risk (corporate firewall compatibility)

**Issue**: `ALLOW_INSECURE_TLS=true` option disables certificate validation for:
- **Connection to management.umh.app** (configuration sync and status reporting)
- **Bridge connections** to data sources (HTTPS APIs, MQTTS brokers, etc.)

**Why this option exists**: Corporate firewalls often perform TLS inspection (MITM), and adding corporate CA certificates is complex.

**Risk**: MITM attacks possible if misused:
- **Management connection**: Attacker could intercept AUTH_TOKEN during transmission
- **Bridge connections**: Attacker could intercept or modify industrial data in transit

**Usage guidance**: Only use behind trusted corporate firewall where TLS inspection is performed. See [Network Configuration](./network-configuration.md) for details on adding corporate CA certificates (preferred) vs using `ALLOW_INSECURE_TLS=true`.

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

---

## Shared Responsibility Model

### We are responsible for:
- **Software supply chain** (container images, SBOM, vulnerability scanning via Aikido/FOSSA)
- **Secure defaults** (non-root execution, TLS enabled by default, no default passwords)
- **Clear documentation** of protocol limitations and security considerations
- **Regular security updates** via our Docker registry and documented release process

### You are responsible for:
- **Infrastructure and runtime** (Docker/Kubernetes configuration, host OS security, network architecture)
- **Secrets lifecycle** (AUTH_TOKEN storage, rotation, access controls)
- **Monitoring and incident response** (log aggregation, security monitoring, forensics)
- **Deployment security** (capabilities, AppArmor/SELinux, resource limits, network policies)
- **Reading this documentation** - we provide secure software, you must deploy it securely

**This aligns with cloud vendor models** - we secure the software, you secure the deployment environment.

**For detailed OWASP/CIS compliance guidance**, see:
- [OWASP Docker Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Docker_Security_Cheat_Sheet.html)
- [CIS Docker Benchmark](https://www.cisecurity.org/benchmark/docker)
- [Kubernetes Security Best Practices](https://kubernetes.io/docs/concepts/security/)
