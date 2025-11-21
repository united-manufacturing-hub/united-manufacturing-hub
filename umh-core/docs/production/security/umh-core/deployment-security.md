# umh-core Security

## Security Capabilities

This section documents umh-core's security features across container security, access control, network architecture, cryptography, supply chain integrity, and industrial protocol handling. Each capability is mapped to applicable industry standards (NIST, IEC 62443, OWASP) with implementation details and known limitations. The final section defines our shared responsibility model - what we secure in the software versus what you must secure in your deployment environment.

## Container Security

**Applicable Standards**: OWASP Docker Security Cheat Sheet requirement 2 (non-root user), NIST SP 800-190 (least privilege principle for containers), IEC 62443-4-2 CR 2.1 (authorization enforcement and least privilege at Security Level 2)

**Implementation**: All umh-core processes run as non-root user umhuser with UID 1000, preventing privilege escalation attacks. Each component operates in a separate process namespace isolated from the host, with limited filesystem access restricted to explicitly mounted paths only. The minimal Alpine Linux base image reduces the attack surface by excluding unnecessary system utilities and libraries.

**Known Limitations**: All bridges and data flows run as the same Linux user within the container, with no per-component user isolation. This design prioritizes non-root security over internal process isolation, as user switching requires root capabilities unavailable in non-root containers.

---

## Logging and Audit Trail

**Applicable Standards**: NIST SP 800-53 AU-2 and AU-3 (comprehensive audit logging with timestamps, event types, and outcomes), NIST SP 800-92 (log generation, storage, and protection), IEC 62443-4-2 CR 2.8 (auditable events for security-relevant actions)

**Implementation**: All services write structured logs to /data/logs/ with S6 supervision, using TAI64N timestamps for precise event ordering. FSM state transitions are tracked for all components, with rolling log rotation to manage storage. Logs capture configuration changes, component lifecycle events, and connection status for all industrial protocols and data flows.

**Known Limitations**: Current logging tracks system events and component states but does not capture individual user actions performed through the Management Console interface. User-level audit trails for configuration changes are planned for future releases.

---

## Access Control and Authentication

**Applicable Standards**: NIST SP 800-53 AC-6 (least privilege access control), IEC 62443-4-2 CR 1.2 (software process and device identification and authentication), NIST SP 800-171 IA-2 (identification and authentication of organizational users)

**Implementation**: Each umh-core instance authenticates to the Management Console using a unique AUTH_TOKEN shared secret combined with a per-instance InstanceUUID. The AUTH_TOKEN is user-configured with no default values, and Management Console provides the user authentication layer for operator access. TLS encryption protects authentication tokens during transmission to management.umh.app.

**Known Limitations**: The current AUTH_TOKEN is instance-level and shared across all components within the container. Future releases will implement per-message authentication to enable fine-grained access control and authorization for individual bridges and data flows.

---

## Network Security

**Applicable Standards**: NIST SP 800-82 (defense in depth for OT/ICS environments and network segmentation), IEC 62443-3-3 SR 5.1 (zone and conduit architecture for edge gateway deployment), IEC 62443-4-2 CR 5.1 (network segmentation support)

**Implementation**: umh-core follows an edge-only architecture designed for deployment at the OT/IT boundary (Purdue Level 3.5), with outbound HTTPS connections only to management.umh.app for configuration synchronization. No services are designed for inbound internet connections, and the local GraphQL API is restricted to localhost access only. Network isolation is enforced through separate network namespaces unless explicitly configured otherwise.

**Known Limitations**: The edge-only design requires outbound internet access to function and is not suitable for air-gapped or fully disconnected environments. Network segmentation and firewall rules must be configured properly by the customer to achieve defense in depth.

---

## Cryptography and TLS

**Applicable Standards**: NIST SP 800-52 (TLS 1.2+ configuration with modern cipher suites), NIST SP 800-53 SC-8 (transmission confidentiality and integrity), IEC 62443-4-2 CR 4.3 (use of cryptography conforming to applicable standards and regulations)

**Implementation**: All connections to management.umh.app use TLS 1.2 or higher with modern cipher suites including AES-GCM and ChaCha20-Poly1305. Certificate validation is enabled by default using Go's standard library crypto packages, and all cryptographic operations follow current industry standards for secure communications.

**Known Limitations**: An ALLOW_INSECURE_TLS configuration option is available for corporate environments with TLS inspection, which disables certificate validation when enabled. This option should only be used behind trusted corporate firewalls where inspection is performed, as it makes the system vulnerable to man-in-the-middle attacks.

---

## Supply Chain Security

**Applicable Standards**: OWASP Docker Security Cheat Sheet requirement 0 (regular image updates and vulnerability scanning), NIST SP 800-161 (SBOM generation, vulnerability management, and supply chain risk controls), IEC 62443-4-1 SR-5 (product defect management and security vulnerability tracking)

**Implementation**: All container images undergo automated vulnerability scanning with Aikido and FOSSA tools in the CI/CD pipeline. Software Bill of Materials documents are generated for every release, and container images are cryptographically signed for integrity verification. The trust dashboard at trust.umh.app provides transparency into security scanning results and dependency management practices.

**Known Limitations**: Supply chain security depends on timely updates from upstream dependencies and proper image verification during deployment.

---

## Industrial Protocol Security

**Applicable Standards**: OWASP OT Top 10 number 9 (legacy protocol security limitations and risks), IEC 62443-3-3 SR 5.1 (network segmentation for operational technology protocols), IEC 62443-4-2 CR 3.1 (communication integrity requirements)

**Implementation**: umh-core supports industrial protocols including OPC UA (with certificate-based security), MQTT (with TLS support), Modbus TCP, and S7 communication. Connections are managed through protocol-specific bridges with configurable security parameters where the underlying protocol provides security features.

**Known Limitations**: Modbus TCP and S7 protocols lack native encryption capabilities due to inherent protocol design limitations. Compensating controls include network segmentation to isolate OT zones from IT and internet networks, physical security requirements for deployment locations, and firewall rules to restrict protocol access by IP address. These compensating controls must be implemented by the customer as part of their overall IEC 62443-3-3 zone architecture.

---

## Threat Model (Simplified)

umh-core primarily protects against unintentional compromise of external industrial systems due to vulnerabilities in our software. The non-root execution model prevents privilege escalation, the minimal network attack surface reduces exposure, and TLS is enabled by default for all external communications. Supply chain risks are mitigated through signed images, vulnerability scanning, and SBOM generation. The edge-only architecture design prevents misconfiguration that could lead to internet exposure of industrial protocols.

umh-core does not protect against malicious operators with configuration access. An operator with access to the Management Console UI or direct filesystem access to config.yaml can deploy bridge configurations that connect to external industrial systems, exfiltrate the AUTH_TOKEN via outbound network requests, or read sensitive data from mounted volumes. Similarly, the system cannot protect against compromise of the underlying container runtime, host operating system, or Kubernetes control plane.

This model aligns with industry-standard edge gateway security - we secure our software, you secure your infrastructure.

---

## Deployment Model: Edge-Only Architecture

umh-core is designed for edge-only deployment with a specific network architecture. The system requires outbound HTTPS connections to management.umh.app for configuration synchronization and supports outbound connections to data sources including MQTT brokers, OPC UA servers, Modbus devices, and APIs. No services are designed for inbound internet connections, and the GraphQL API is restricted to local access only via localhost:8090.

The typical deployment location is on the factory floor, behind corporate firewall, on-premises at customer sites. This architecture reduces the attack surface by eliminating services that listen for inbound internet connections. The Management Console cannot push commands to instances; instead, umh-core pulls configuration changes on its own schedule. This aligns with network segmentation best practices where umh-core sits between OT networks and IT infrastructure at the boundary layer.

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

---

### Network Access

**Required outbound**: HTTPS to `management.umh.app` for configuration sync and status reporting.

See [Network Configuration](./network-configuration.md) for details on proxy settings and TLS inspection.

**Data source connections**: Customer-defined connections to industrial devices and data sources.

**Inbound**: No services designed for internet exposure (edge-only deployment).

---

### Process Model

All components run as single non-root user with UID 1000 named umhuser. This includes the main umh-core agent, all bridges implemented as benthos-umh instances, all data flows, the Redpanda broker, and internal services.

The non-root execution model ensures the container cannot escalate privileges even if compromised. This design is compatible with restricted Kubernetes environments that prohibit privileged containers and follows the standard Docker security model for production deployments.

Container isolation is enforced through several mechanisms. Each container operates in a separate process namespace that prevents visibility of host processes. Network isolation is maintained through isolated network namespaces unless explicitly configured with host networking mode. Filesystem access is limited to explicitly mounted paths only, with no access to the broader host filesystem.

---

## Known Limitations

### AUTH_TOKEN in Environment Variable

**Category**: Known Limitation (cannot fix in single-container architecture)

**Issue**: AUTH_TOKEN shared secret is stored in:
- Environment variable (`AUTH_TOKEN=xxx`)
- Configuration file (`/data/config.yaml`)

**Persistence behavior**: Setting `AUTH_TOKEN` via environment variable (`docker run -e AUTH_TOKEN=xxx`) writes it to `/data/config.yaml` permanently. On subsequent container restarts, the value from config.yaml is used even if the environment variable is not set.

**Why this design**: Ensures configuration persists across container restarts without requiring environment variables every time. Once set via environment variable or Management Console, AUTH_TOKEN is stored in `/data/config.yaml` on the persistent volume.

**Security implication**: Both storage locations are readable by all processes running as umhuser with UID 1000. This includes all bridges handling protocol converters, data flows, and stream processors, any process started within the container, and any code executed via bridge configurations.

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

**What this means**: All bridges (protocol converters, data flows, stream processors) run as the same Linux user (UID 1000, umhuser). There is no user-level or process-level isolation between different bridges within the container.

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
- **Monitoring and incident response** (log aggregation, security monitoring, forensics)
- **Deployment security** (capabilities, AppArmor/SELinux, resource limits, network policies)
- **Network segmentation and zone placement** (IEC 62443-3-3 compliant architecture, firewall rules for OT/IT boundaries)
- **Physical security** (secure location for umh-core hardware, restricted physical access)
- **Backup and disaster recovery** (configuration backups, persistent volume snapshots, tested restore procedures)
- **High availability** (deploying multiple instances if required for critical production lines)
- **Security event monitoring** (SIEM integration if required, intrusion detection systems)
- **Corporate CA certificate management** (adding certificates for TLS inspection scenarios)
- **Reading this documentation** - we provide secure software, you must deploy it securely

### Customer-Created Network Exposures

Customers can configure data flows with HTTP server inputs that expose ports within the container (for example, an HTTP input listening on 0.0.0.0:8080 to receive data from external systems). When creating such configurations, you are responsible for securing these endpoints as they create network attack surface outside our edge-only security model. This includes configuring firewall rules to restrict access to HTTP endpoints, implementing authentication and authorization for HTTP inputs, setting up rate limiting to prevent denial of service attacks, validating and sanitizing all data received via HTTP, ensuring network segmentation isolates HTTP endpoints from critical systems, and monitoring for attacks on exposed ports. umh-core does not provide built-in authentication, authorization, or rate limiting for customer-configured HTTP inputs.

**This aligns with cloud vendor models** - we secure the software, you secure the deployment environment.

**For detailed OWASP/CIS compliance guidance**, see:
- [OWASP Docker Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Docker_Security_Cheat_Sheet.html)
- [CIS Docker Benchmark](https://www.cisecurity.org/benchmark/docker)
- [Kubernetes Security Best Practices](https://kubernetes.io/docs/concepts/security/)
