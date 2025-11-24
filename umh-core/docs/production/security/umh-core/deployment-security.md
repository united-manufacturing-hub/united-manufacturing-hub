# umh-core Security

## Security Capabilities

This section documents umh-core's security features across container security, access control, network architecture, cryptography, supply chain integrity, and industrial protocol handling. Each capability is mapped to applicable industry standards (NIST, IEC 62443, OWASP) with implementation details and known limitations. The final section defines our shared responsibility model - what we secure in the software versus what you must secure in your deployment environment.

## Container Security

**Applicable Standards**: OWASP Docker Security Cheat Sheet requirement 2 (non-root user), NIST SP 800-190 (least privilege principle for containers), IEC 62443-4-2 CR 2.1 (authorization enforcement and least privilege at Security Level 2)

**Implementation**: All umh-core processes run as non-root user umhuser with UID 1000, preventing privilege escalation attacks. Each component operates in a separate process namespace isolated from the host, with limited filesystem access restricted to explicitly mounted paths only. The minimal Alpine Linux base image reduces the attack surface by excluding unnecessary system utilities and libraries.

**Deployment Considerations**: All bridges and [data flows](../../../usage/data-flows/) run as the same Linux user within the container, with no per-component user isolation. This design prioritizes non-root security over internal process isolation, as user switching requires root capabilities unavailable in non-root containers.

**Note**: Mounted volumes must be writable by UID 1000. See [Filesystem Access](#filesystem-access).

⚠️ **Safety System Warning**: umh-core is NOT SIL-rated and must not be used in safety-instrumented systems. Refer to IEC 61508/61511 for safety requirements.

---

## Logging and Audit Trail

**Applicable Standards**: NIST SP 800-53 AU-2 and AU-3 (comprehensive audit logging with timestamps, event types, and outcomes), NIST SP 800-92 (log generation, storage, and protection), IEC 62443-4-2 CR 2.8 (auditable events for security-relevant actions)

**Implementation**: All services write structured logs to /data/logs/ with S6 supervision, using TAI64N timestamps for precise event ordering. FSM state transitions are tracked for all components, with rolling log rotation to manage storage. Logs capture configuration changes, component lifecycle events, and connection status for all industrial protocols and data flows.

**Deployment Considerations**: Current logging tracks system events and component states but does not capture individual user actions performed through the ManagementConsole interface. User-level audit trails for configuration changes are planned for future releases.

---

## Access Control and Authentication

**Applicable Standards**: NIST SP 800-53 AC-6 (least privilege access control), IEC 62443-4-2 CR 1.2 (software process and device identification and authentication), NIST SP 800-171 IA-2 (identification and authentication of organizational users)

**Implementation**: Each umh-core instance authenticates to the ManagementConsole using a unique AUTH_TOKEN shared secret combined with a per-instance InstanceUUID. The AUTH_TOKEN is user-configured with no default values, and ManagementConsole provides the user authentication layer for operator access. TLS encryption protects authentication tokens during transmission to management.umh.app.

**Deployment Considerations**: The current AUTH_TOKEN is instance-level and shared across all components within the container. Future releases will implement per-message authentication to enable fine-grained access control and authorization.

---

## Network Security

**Applicable Standards**: NIST SP 800-82 (defense in depth for OT/ICS environments and network segmentation), IEC 62443-3-3 SR 5.1 (zone and conduit architecture for edge gateway deployment), IEC 62443-4-2 CR 5.1 (network segmentation support)

**Implementation**: umh-core follows an edge-only architecture designed for deployment at the OT/IT boundary (Purdue Level 3 (DMZ between OT and IT networks)), with outbound HTTPS connections only to management.umh.app for configuration synchronization. Typical deployment: umh-core in DMZ, outbound connections to cloud, inbound connections from OT networks only. No direct internet exposure. By default, no services listen for inbound internet connections. However, customers can configure data flows with HTTP server inputs that expose ports within the container. Network isolation is enforced through separate network namespaces unless explicitly configured otherwise.

Deploy umh-core in a DMZ with firewalls on both OT and IT boundaries. See IEC 62443-3-3 for zone architecture.

**Customer-Configured HTTP Servers**: Customer-configured HTTP servers require coordination between application teams (who configure bridges via ManagementConsole) and security/infrastructure teams (who must explicitly expose container ports). This two-team requirement ensures managed attack surface. umh-core does not provide built-in authentication for HTTP inputs. Deploy behind your standard reverse proxy (nginx, HAProxy) for authentication and TLS.

**Deployment Considerations**: The edge-only design requires outbound internet access to function and is not suitable for air-gapped or fully disconnected environments. Network segmentation and firewall rules must be configured properly by the customer to achieve defense in depth.

---

## Cryptography and TLS

**Applicable Standards**: NIST SP 800-52 (TLS 1.2+ configuration with modern cipher suites), NIST SP 800-53 SC-8 (transmission confidentiality and integrity), IEC 62443-4-2 CR 4.3 (use of cryptography conforming to applicable standards and regulations)

**Implementation**: All connections to management.umh.app use TLS 1.2 or higher with modern cipher suites including AES-GCM and ChaCha20-Poly1305. Certificate validation is enabled by default using Go's standard library crypto packages, and all cryptographic operations follow current industry standards for secure communications. This is the secure default configuration.

**Deployment Considerations**: An ALLOW_INSECURE_TLS configuration option is available for corporate environments with TLS inspection, which disables certificate validation when enabled. This option should only be used behind trusted corporate firewalls where inspection is performed, as it makes the system vulnerable to man-in-the-middle attacks. When ALLOW_INSECURE_TLS is enabled, the minimum TLS version is reduced to TLS 1.0 to maximize compatibility with corporate TLS inspection proxies (note: TLS 1.0 is only available with this flag and does not meet the SSLabs Grade B standard). This setting should only be used behind trusted corporate firewalls.

---

## Supply Chain Security

**Applicable Standards**: OWASP Docker Security Cheat Sheet requirement 0 (regular image updates and vulnerability scanning), NIST SP 800-161 (SBOM generation, vulnerability management, and supply chain risk controls), IEC 62443-4-1 SR-5 (product defect management and security vulnerability tracking)

**Implementation**: All container images undergo automated vulnerability scanning with Aikido. SBOM and OSS License Compliance is tracked through FOSSA. The compliance dashboard at trust.umh.app tracks standards alignment (such as ISO27001 or NIST) through Vanta.

**Deployment Considerations**: Supply chain security depends on timely updates from upstream dependencies and proper image verification during deployment.

---

## Industrial Protocol Security

**Applicable Standards**: OWASP OT Top 10 number 9 (legacy protocol security limitations and risks), IEC 62443-3-3 SR 5.1 (network segmentation for operational technology protocols), IEC 62443-4-2 CR 3.1 (communication integrity requirements)

**Implementation**: umh-core supports industrial protocols including OPC UA (with certificate-based security), MQTT (with TLS support), Modbus TCP, and S7 communication. Connections are managed through protocol-specific bridges with configurable security parameters where the underlying protocol provides security features.

**Deployment Considerations**: Modbus TCP and S7 protocols lack native encryption capabilities due to inherent protocol design limitations. Compensating controls include network segmentation to isolate OT zones from IT and internet networks, physical security requirements for deployment locations, and firewall rules to restrict protocol access by IP address. These compensating controls must be implemented by the customer as part of their overall IEC 62443-3-3 zone architecture.

---

## PLC Integration Limits

**Warning**: Aggressive polling can overwhelm PLCs and cause CPU overload or crashes. Each PLC model has different limits for concurrent connections, polling rates, and tag counts. Consult your device manual for specifications and test polling rates in non-production before deployment. S7 protocol is particularly sensitive to connection overload.

---

## Threat Model (Simplified)

umh-core primarily protects against unintentional compromise of external industrial systems due to vulnerabilities in our software. The non-root execution model prevents privilege escalation, the minimal network attack surface reduces exposure, and TLS is enabled by default for all external communications. Supply chain risks are mitigated through vulnerability scanning and dependency tracking. The pull-based deployment model (see Deployment Model below) prevents misconfiguration that could expose industrial protocols to the internet.

umh-core does not protect against malicious operators with configuration access. An operator with access to the ManagementConsole UI or direct filesystem access to config.yaml can deploy bridge configurations that connect to external industrial systems, exfiltrate the AUTH_TOKEN via outbound network requests, or read sensitive data from mounted volumes. Similarly, the system cannot protect against compromise of the underlying container runtime, host operating system, or Kubernetes control plane.

This model aligns with industry-standard edge gateway security - we secure our software, you secure your infrastructure.

---

## Deployment Model: Edge-Only Architecture

umh-core is designed for edge-only deployment with a specific network architecture. The system requires outbound HTTPS connections to management.umh.app for configuration synchronization and supports outbound connections to data sources including MQTT brokers, OPC UA servers, Modbus devices, and APIs. No services are designed for inbound internet connections.

The typical deployment location is on the factory floor, behind corporate firewall, on-premises at customer sites. This architecture reduces the attack surface by eliminating services that listen for inbound internet connections. The ManagementConsole queues configuration changes that umh-core instances pull on their own schedule for execution. This aligns with network segmentation best practices where umh-core sits between OT networks and IT infrastructure at the boundary layer.

The system is not designed for air-gapped environments. umh-core requires outbound internet access to management.umh.app to function and cannot operate in fully disconnected deployments.

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

#### Volume Permission Requirements

The `/data` directory must be writable by UID 1000 (umhuser). Set ownership before starting:
```bash
sudo chown -R 1000:1000 /path/to/data
```

---

### Network Access

**Required outbound**: HTTPS to `management.umh.app` for configuration sync and status reporting.

See [Network Configuration](./network-configuration.md) for details on proxy settings and TLS inspection.

**Data source connections**: Customer-defined connections to industrial devices and data sources.

**Inbound**: No services designed for internet exposure (edge-only deployment).

---

### Process Model

All components run as single non-root user with UID 1000 named umhuser. This includes the main umh-core agent, all bridges implemented as benthos-umh instances, all data flows, the Redpanda broker, and internal services.

See [Container Security](#container-security) above for details on non-root execution model, process isolation, and filesystem access restrictions.

---

## Deployment Considerations

### AUTH_TOKEN in Environment Variable

**Category**: Deployment Consideration (cannot fix in single-container architecture)

**Issue**: AUTH_TOKEN shared secret flow:
- **Input**: Environment variable (`AUTH_TOKEN=xxx`) during first container start
- **Persistent storage**: Written to `/data/config.yaml` automatically
- **Subsequent starts**: Read from config.yaml (environment variable no longer required)

**Why this design**: Ensures configuration persists across container restarts without requiring environment variables every time. Once set via environment variable or ManagementConsole, AUTH_TOKEN is stored in `/data/config.yaml` on the persistent volume.

**Security implication**: Both storage locations are readable by all processes running as umhuser with UID 1000. This includes all bridges handling protocol converters, data flows, and stream processors, any process started within the container, and any code executed via bridge configurations.

**Risk**: Malicious bridge configuration could exfiltrate AUTH_TOKEN via outbound network requests.

**What you should do**:
1. **Understand the security model**: This follows standard container secret management patterns used by Docker and Kubernetes. All processes within the container share the secret, which is industry-standard for single-container architectures. For enhanced secret isolation, enterprise deployments can integrate with HashiCorp Vault or similar secret management systems.
2. **Monitor** network connections for unexpected outbound traffic (exfiltration attempts)
3. **Rotate if compromised**:
   - Create new instance in ManagementConsole
   - Copy new AUTH_TOKEN to deployment configuration
   - Update container environment variable or config.yaml
   - Remove old instance from ManagementConsole

---

### No User/Process Isolation Between Bridges

**Category**: Accepted Risk (design trade-off)

**What this means**: All bridges (protocol converters, data flows, stream processors) run as the same Linux user (UID 1000, umhuser). There is no user-level or process-level isolation between different bridges within the container.

### No Per-User Access Control Within Instance

**Category**: Deployment Consideration (planned for future releases)

**Issue**: AUTH_TOKEN authorizes the umh-core instance to communicate with ManagementConsole. Once authorized, any user within that organization can perform all actions on this instance through the ManagementConsole UI. Per-user and per-action access restrictions at the instance level are not currently implemented but are planned for future releases.

### Logs Accessible to All Processes

**Category**: Deployment Consideration (non-root container design)

**Issue**: All processes running as umhuser (UID 1000) can read and potentially modify log files in /data/logs/. This is a consequence of the non-root container design. Customers requiring tamper-proof logs should implement log forwarding to external SIEM systems with append-only storage.

**What this does not mean**: Bridges do not interfere with each other's data processing, as Redpanda isolates message flows by topic. Bridges are not resource-limited together; each bridge can have separate CPU and memory limits configured via s6-softlimit. Data is not shared between bridges, as each benthos instance maintains separate configuration and state.

**Technical constraint**: Per-bridge user isolation is not possible in non-root containers. Process-level user switching requires CAP_SETUID and CAP_SETGID capabilities, which are only available to root processes.

**Design decision**: Non-root container security prioritized over per-bridge user isolation.

**Security implications**:

Because all bridges run as the same user, they share access within the container. All bridges can read the /data/config.yaml file containing AUTH_TOKEN, access the same mounted directories, view each other's environment variables, and read each other's configuration files.

The container boundary remains enforced despite shared user access. Bridges cannot access the host filesystem except through explicitly mounted paths, cannot see host processes, and network isolation applies unless explicitly disabled with host networking mode.

Non-root execution provides security benefits that justify this trade-off. The container cannot escalate to root privileges even if a bridge is compromised. This follows the standard Docker security model with defense in depth. The design is compatible with restricted Kubernetes environments that do not permit privileged containers or special permissions.

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
- **Volume permissions** (ensuring `/data` is writable by UID 1000)
- **Monitoring and incident response** (log aggregation, security monitoring, forensics)
- **Deployment security** (capabilities, AppArmor/SELinux, resource limits, network policies)
- **Network segmentation and zone placement** (deploy at Purdue Level 3 per IEC 62443-3-3 zone architecture, firewall rules for OT/IT boundaries)
- **PLC polling rate configuration** (configure polling intervals per device specifications to prevent overload)
- **Physical security** (secure deployment locations and restrict physical access per IEC 62443-3-3 SR 5.1)
- **OT safety systems** (umh-core must not be integrated into safety-instrumented systems; see IEC 61508/61511)
- **Backup and disaster recovery** (configuration backups, persistent volume snapshots, tested restore procedures per business continuity requirements)
- **High availability** (deploying multiple instances if required for critical production lines)
- **Security event monitoring** (SIEM integration if required, intrusion detection systems)
- **Corporate CA certificate management** (adding certificates for TLS inspection scenarios)
- **Reading this documentation** - we provide secure software, you must deploy it securely

**This aligns with cloud vendor models** - we secure the software, you secure the deployment environment.

**For detailed OWASP/CIS compliance guidance**, see:
- [OWASP Docker Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Docker_Security_Cheat_Sheet.html)
- [CIS Docker Benchmark](https://www.cisecurity.org/benchmark/docker)
- [Kubernetes Security Best Practices](https://kubernetes.io/docs/concepts/security/)

---

## Enterprise Security Features

The following security capabilities and documentation require an enterprise license:

- Security testing reports and penetration test results
- Software Bill of Materials (SBOM) access and vulnerability disclosure timelines
- Incident response playbooks and operational runbooks
- End-of-life policy, extended support, and long-term security update commitments
- Service Level Agreements (SLAs) for security patch response times
- Compliance attestations and audit support documentation

Enterprise license provides compliance documentation and SLA-backed support, not additional security features.

For enterprise licensing information, contact the United Manufacturing Hub sales team.
