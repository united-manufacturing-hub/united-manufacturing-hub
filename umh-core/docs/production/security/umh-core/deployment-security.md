# umh-core Security

## Relevant Standards

### Non-Root Container Execution
- **OWASP Docker Security #2**: Non-root user (UID 1000)
- **NIST SP 800-190**: Least privilege principle for containers
- **IEC 62443-4-2 CR 2.1**: Authorization enforcement, least privilege (SL 2)

**Status**: ✅ Compliant (2025-02)
**Implementation**: All processes run as umhuser (UID 1000), no privilege escalation possible

---

### Supply Chain Security
- **OWASP Docker Security #0**: Regular image updates, vulnerability scanning
- **NIST SP 800-161**: SBOM generation, vulnerability management, supply chain risk controls
- **IEC 62443-4-1 SR-5**: Defect management and security vulnerability tracking

**Status**: ✅ Compliant (2025-02)
**Implementation**: Aikido/FOSSA scanning, SBOM generation, signed images, automated CI/CD pipeline. Trust dashboard: https://trust.umh.app

---

### Secure Defaults and Configuration
- **OWASP IoT Top 10 I1**: No default passwords
- **OWASP IoT Top 10 I9**: Secure defaults
- **NIST SP 800-53 CM-7**: Least functionality principle
- **IEC 62443-4-2 CR 2.1**: Secure by default configuration

**Status**: ✅ Compliant (2025-02)
**Implementation**: AUTH_TOKEN user-configured (no defaults), TLS enabled by default, authentication required, minimal base image

---

### Industrial Protocol Security
- **OWASP OT Top 10 #9**: Legacy protocol security limitations
- **IEC 62443-3-3 SR 5.1**: Network segmentation for OT protocols
- **IEC 62443-4-2 CR 3.1**: Communication integrity requirements

**Status**: 📋 Known Limitation (2025-02)
**Implementation**: Modbus TCP and S7 protocols lack native encryption (protocol design limitation). Compensating controls: network segmentation (OT zone isolated from IT/internet), physical security, firewall rules restrict protocol access by IP. See Known Limitations section for details.

---

### Logging and Audit Trail
- **NIST SP 800-53 AU-2/AU-3**: Comprehensive audit logging with timestamps, event types, outcomes
- **NIST SP 800-92**: Log generation, storage, and protection
- **IEC 62443-4-2 CR 2.8**: Auditable events for security-relevant actions

**Status**: ✅ Compliant (2025-02)
**Implementation**: All services log to /data/logs/ with S6 supervision, TAI64N timestamps, FSM state transitions tracked, rolling logs with rotation

---

### Network Security and Segmentation
- **NIST SP 800-82**: Defense in depth for OT/ICS environments, network segmentation
- **IEC 62443-3-3 SR 5.1**: Zone and conduit architecture (edge gateway deployment)
- **IEC 62443-4-2 CR 5.1**: Network segmentation support

**Status**: ✅ Compliant (2025-02)
**Implementation**: Edge-only architecture (outbound HTTPS only), no inbound internet services, designed for deployment at OT/IT boundary (Purdue Level 3.5)

---

### TLS and Cryptography
- **NIST SP 800-52**: TLS 1.2+ configuration with modern cipher suites
- **NIST SP 800-53 SC-8**: Transmission confidentiality and integrity
- **IEC 62443-4-2 CR 4.3**: Use of industry-standard cryptography

**Status**: ✅ Compliant (2025-02)
**Implementation**: TLS 1.2+ for management.umh.app connections, AES-GCM and ChaCha20-Poly1305 cipher suites, Go standard library crypto. Certificate validation enabled by default (ALLOW_INSECURE_TLS option documented with warnings for corporate TLS inspection scenarios).

---

### Container Isolation and Integrity
- **OWASP Docker Security #10**: Minimal attack surface
- **NIST SP 800-190**: Container isolation (separate namespaces)
- **IEC 62443-4-2 CR 3.4**: Software and information integrity

**Status**: ✅ Compliant (2025-02)
**Implementation**: Separate process namespace (cannot see host processes), isolated network namespace, limited filesystem access (only mounted paths), minimal base image reduces attack surface

---

### Access Control and Authentication
- **NIST SP 800-53 AC-6**: Least privilege access
- **IEC 62443-4-2 CR 1.2**: Software process and device identification/authentication
- **NIST SP 800-171 IA-2**: Identification and authentication

**Status**: ✅ Compliant (2025-02)
**Implementation**: AUTH_TOKEN shared secret for instance authentication, InstanceUUID unique per instance, Management Console provides user authentication layer

---

### Vulnerability Management and Patching
- **NIST SP 800-53 SI-2**: Flaw remediation and security updates
- **IEC 62443-4-1 SR-6**: Patch management process
- **IEC 62443-2-3**: Patch management in IACS environments

**Status**: ✅ Compliant (2025-02)
**Implementation**: Aikido vulnerability scanning (container images), regular container image updates via CI/CD, documented update process, version-controlled releases

---

### Availability and Recovery
- **NIST CSF RC**: Recovery function (restore capabilities after incidents)
- **IEC 62443-4-2 CR 7.3/CR 7.4**: Control system backup and recovery
- **NIST SP 800-53 CP-10**: System recovery and reconstitution

**Status**: ✅ Compliant (2025-02)
**Implementation**: FSM automatic retry and recovery, S6 supervision with automatic process restart, persistent storage (/data) for configuration, immutable container images enable reliable recovery

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
- **Network segmentation and zone placement** (IEC 62443-3-3 compliant architecture, firewall rules for OT/IT boundaries)
- **Physical security** (secure location for umh-core hardware, restricted physical access)
- **Backup and disaster recovery** (configuration backups, persistent volume snapshots, tested restore procedures)
- **High availability** (deploying multiple instances if required for critical production lines)
- **Security event monitoring** (SIEM integration if required, intrusion detection systems)
- **Corporate CA certificate management** (adding certificates for TLS inspection scenarios)
- **Reading this documentation** - we provide secure software, you must deploy it securely

**This aligns with cloud vendor models** - we secure the software, you secure the deployment environment.

**For detailed OWASP/CIS compliance guidance**, see:
- [OWASP Docker Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Docker_Security_Cheat_Sheet.html)
- [CIS Docker Benchmark](https://www.cisecurity.org/benchmark/docker)
- [Kubernetes Security Best Practices](https://kubernetes.io/docs/concepts/security/)
