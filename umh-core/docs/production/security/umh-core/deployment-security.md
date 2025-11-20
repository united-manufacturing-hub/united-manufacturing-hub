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

Both are readable by all processes running as umhuser (UID 1000).

**Risk**: Malicious bridge configuration could exfiltrate AUTH_TOKEN.

**What you should do**:
1. **Accept the risk** if you control all bridge configurations (recommended for most deployments)
2. **Monitor** network connections for unexpected outbound traffic (exfiltration attempts)
3. **If compromised**: Create new instance in Management Console, copy new AUTH_TOKEN, update your deployment, remove old instance

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
- **Regular security updates** via CI/CD pipeline and documented release process

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
