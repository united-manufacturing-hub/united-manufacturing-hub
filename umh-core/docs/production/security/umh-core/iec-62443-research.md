# IEC 62443 Standards Research for umh-core

## Executive Summary

This document provides research findings on IEC 62443 standards relevant to umh-core's security model as an industrial edge gateway software component. The IEC 62443 series provides comprehensive cybersecurity requirements for Industrial Automation and Control Systems (IACS), including standards applicable to software components, secure development lifecycles, and system security requirements.

**umh-core context**: Single-container edge gateway deployed at factory floor level, processing industrial protocol data (Modbus, S7, OPC UA, MQTT), with outbound-only connectivity to Management Console (management.umh.app) and customer-defined data sources. All processes run as non-root user (UID 1000).

## IEC 62443 Series Overview

The IEC 62443 series addresses industrial automation and control systems (IACS) security throughout their lifecycle, organized into four main categories:

1. **Part 1**: General concepts, models, and terminology
2. **Part 2**: Policies and procedures (asset owners and service providers)
3. **Part 3**: System-level requirements (zones, conduits, security levels)
4. **Part 4**: Component-level requirements (secure development, technical requirements)

The standards define four Security Levels (SL) that correlate required countermeasures with adversary sophistication:

- **SL 0**: No special requirement or protection required
- **SL 1**: Protection against unintentional or accidental misuse
- **SL 2**: Protection against intentional misuse by simple means with few resources, general skills, and low motivation
- **SL 3**: Protection against intentional misuse by sophisticated means with moderate resources, automation-specific knowledge, and moderate motivation
- **SL 4**: Protection against intentional misuse using sophisticated means with extensive resources, automation-specific knowledge, and high motivation

**Recommendation for umh-core**: Target **SL 2** as baseline, with selected SL 3 requirements where feasible (aligns with typical edge gateway deployments in manufacturing environments behind corporate firewalls).

---

## IEC 62443-4-1: Secure Product Development Lifecycle Requirements

### Standard Overview

**Full Title**: Security for industrial automation and control systems - Part 4-1: Secure product development lifecycle requirements

**Published**: 2018-01 (IEC 62443-4-1:2018)

**Scope**: Process requirements for the secure development of products (hardware, software, firmware) used in industrial automation and control systems.

### Relevance to umh-core

IEC 62443-4-1 specifies the secure development lifecycle (SDL) requirements that apply directly to umh-core as a software product. This standard is foundational for demonstrating security-by-design principles.

### Key Requirements Applicable to umh-core

#### 1. Security Requirements Definition (SR-1)

**Requirement**: Define security requirements based on risk assessment and threat modeling.

**umh-core Implementation**:
- Documented threat model in deployment-security.md identifies protected vs. non-protected scenarios
- Clear separation of edge-only architecture (outbound connections only)
- Risk acceptance documented for known limitations (AUTH_TOKEN access, no per-bridge isolation)

**Alignment**: ✅ Compliant - Threat model documented and security requirements defined based on deployment context

#### 2. Secure Design (SR-2)

**Requirement**: Design products using security design principles (defense in depth, least privilege, fail secure).

**umh-core Implementation**:
- Non-root container execution (UID 1000, umhuser) - least privilege principle
- Process isolation via container namespaces
- Secure defaults: TLS enabled by default, authentication required (no default passwords)
- Minimal base image reduces attack surface

**Alignment**: ✅ Compliant - Demonstrates defense in depth and least privilege

#### 3. Secure Implementation (SR-3)

**Requirement**: Implement secure coding practices, including coding guidelines and code reviews.

**umh-core Implementation**:
- Go language with memory safety
- CI/CD pipeline includes: golangci-lint, go vet, nilaway static analysis
- Lefthook pre-commit hooks enforce code quality (gofmt, go vet, license checks)
- Pre-push hooks run nilaway and golangci-lint

**Alignment**: ✅ Compliant - Automated secure coding enforcement via CI/CD

#### 4. Verification and Validation (SR-4)

**Requirement**: Test security features and verify security requirements are met.

**umh-core Implementation**:
- Ginkgo v2 test framework with unit and integration tests
- Testcontainers for Docker-based integration testing
- CI fails on focused tests (ensures comprehensive test execution)

**Gap**: No formal security testing (penetration testing, fuzzing, vulnerability scanning of compiled binaries)

**Alignment**: ⚠️ Partial - Functional testing present, security-specific testing to be added

#### 5. Defect Management (SR-5)

**Requirement**: Track, triage, and remediate security defects.

**umh-core Implementation**:
- Linear issue tracking for all defects
- Sentry integration for error tracking and analysis
- FOSSA for OSS license compliance and vulnerability tracking
- Aikido for container image vulnerability scanning

**Alignment**: ✅ Compliant - Comprehensive defect tracking and remediation process

#### 6. Patch Management (SR-6)

**Requirement**: Provide security updates through efficient patch management process.

**umh-core Implementation**:
- Regular container image updates via CI/CD pipeline
- Docker registry for versioned image distribution
- Management Console can trigger updates (pull model)
- Release notes document security fixes (as seen in benthos-umh bump PRs)

**Gap**: No formal SLA for security patch delivery timeframe

**Alignment**: ✅ Compliant - Update mechanism exists, formalize SLA for critical vulnerabilities

#### 7. Product End-of-Life (SR-7)

**Requirement**: Define end-of-life process including security support sunset.

**Gap**: No documented end-of-life policy for umh-core versions

**Alignment**: ❌ Non-compliant - Requires documentation of version support lifecycle and security update timelines

### Security Development Lifecycle Assurance (SDLA) Certification

IEC 62443-4-1 supports SDLA certification at four levels (SL 1-4), demonstrating development process maturity. Consider pursuing **SDLA Level 2** certification to validate secure development practices for manufacturing customers.

---

## IEC 62443-4-2: Technical Security Requirements for IACS Components

### Standard Overview

**Full Title**: Security for industrial automation and control systems - Part 4-2: Technical security requirements for IACS components

**Published**: 2019-02 (IEC 62443-4-2:2019, Second Edition)

**Scope**: Technical security requirements for IACS components, including embedded devices, network devices, host devices, and **software applications**.

### Relevance to umh-core

IEC 62443-4-2 defines Component Requirements (CR) that apply directly to umh-core as a **software application** component. Requirements are organized by the seven Foundational Requirements (FR) with Component Requirements (CR) and Requirement Enhancements (RE) for each security level.

**umh-core classification**: Software Application (SAR - Software Application Requirements apply)

### Seven Foundational Requirements (FR) for Components

#### FR 1: Identification and Authentication Control (IAC)

**Purpose**: Identify and authenticate all users (humans, software processes, devices) before allowing access.

##### CR 1.1: Human User Identification and Authentication

**Requirement**: Unique identification and authentication for all human users.

**umh-core Implementation**:
- AUTH_TOKEN shared secret for instance authentication to Management Console
- No multiple user accounts (single-instance model)
- Management Console provides user authentication (separate system)

**Gap**: AUTH_TOKEN is shared secret (not per-user authentication). All users with AUTH_TOKEN have full access.

**Alignment**: ⚠️ Partial at SL 1 - Single-instance authentication model appropriate for edge deployment, but lacks per-user granularity

##### CR 1.2: Software Process and Device Identification and Authentication

**Requirement**: Unique identification and authentication for software processes and devices.

**umh-core Implementation**:
- Each umh-core instance identified by unique InstanceUUID
- AUTH_TOKEN + InstanceUUID pair authenticates instance to Management Console
- Benthos bridges inherit parent instance authentication

**Alignment**: ✅ Compliant at SL 1-2 - Instance-level authentication with unique identifiers

##### CR 1.3: Account Management

**Requirement**: Support adding, modifying, disabling, and removing accounts.

**umh-core Implementation**:
- Instance lifecycle managed via Management Console (create, remove)
- AUTH_TOKEN rotation via Management Console (create new instance, migrate, delete old)

**Gap**: No granular account management within single instance (single authentication credential)

**Alignment**: ⚠️ Partial at SL 1 - Instance-level management, not per-user

##### CR 1.7: Strength of Password-Based Authentication

**Requirement**: Enforce password strength requirements.

**umh-core Implementation**:
- AUTH_TOKEN generated by Management Console (not user-chosen password)
- Appears to be UUID or similar strong random value

**Alignment**: ✅ Compliant at SL 1-2 - Strong random token generation

##### CR 1.9: Strength of Public Key-Based Authentication

**Requirement**: Support public key authentication where applicable.

**Gap**: umh-core uses shared secret (AUTH_TOKEN), not public key infrastructure (PKI)

**Alignment**: ❌ Not applicable at SL 1-2 (SL 3+ typically requires PKI for industrial components)

#### FR 2: Use Control (UC)

**Purpose**: Enforce authorization for authenticated users (least privilege principle).

##### CR 2.1: Authorization Enforcement

**Requirement**: Enforce approved authorizations for access to resources.

**umh-core Implementation**:
- Process-level isolation: All processes run as umhuser (UID 1000), non-root
- Container namespaces provide filesystem and network isolation from host
- No privilege escalation possible (no CAP_SETUID/CAP_SETGID)

**Gap**: No authorization granularity within container (all bridges access same resources)

**Alignment**: ✅ Compliant at SL 1 - Container-level authorization, process isolation from host

##### CR 2.2: Wireless Access Management

**Requirement**: Authorize and restrict wireless access where applicable.

**Alignment**: ✅ N/A - umh-core does not provide wireless access management (wired network connections only)

##### CR 2.4: Mobile Code

**Requirement**: Control execution of mobile code (scripts, plugins).

**umh-core Implementation**:
- Benthos configuration files define data processing logic (YAML-based)
- Bloblang scripting language for data transformation (sandboxed within benthos)
- No arbitrary code execution from external sources

**Gap**: Bridge configurations can include arbitrary Bloblang code and HTTP requests (risk documented in threat model)

**Alignment**: ⚠️ Partial at SL 1 - Scripting sandboxed within benthos, but bridge configs are trusted input

##### CR 2.8: Auditable Events

**Requirement**: Generate audit records for security-relevant events.

**umh-core Implementation**:
- Comprehensive logging via S6-supervised processes (tai64n timestamps)
- FSM state transitions logged with component, state, and timestamp
- All network requests logged (management API, data source connections)
- Logs stored in `/data/logs/` with rotation (`.s` files)

**Gap**: No structured audit log format (logs are unstructured text)

**Alignment**: ✅ Compliant at SL 1 - Comprehensive logging, needs structured format for SL 2

##### CR 2.9: Audit Storage Capacity

**Requirement**: Allocate sufficient audit storage and warn when capacity is low.

**umh-core Implementation**:
- Logs stored on persistent volume (`/data/logs/`)
- S6 log rotation prevents unbounded growth
- No proactive warning when disk space low

**Gap**: No monitoring or alerting for disk space exhaustion

**Alignment**: ⚠️ Partial at SL 1 - Log rotation present, needs capacity monitoring

##### CR 2.12: Provisioning Product Supplier Roots of Trust

**Requirement**: Securely provision root certificates for verifying component authenticity.

**umh-core Implementation**:
- TLS connections to management.umh.app use system root CA certificates
- Container image includes standard CA bundle
- Corporate CA certificates can be added via volume mount

**Gap**: No verification of container image signatures (Docker Content Trust not enforced)

**Alignment**: ⚠️ Partial at SL 1 - TLS certificate validation present, container image signing to be added

#### FR 3: System Integrity (SI)

**Purpose**: Ensure integrity of the IACS to prevent unauthorized manipulation.

##### CR 3.1: Communication Integrity

**Requirement**: Protect integrity of transmitted information.

**umh-core Implementation**:
- HTTPS/TLS for management.umh.app communication (TLS 1.2+)
- Protocol-specific security: MQTTS (TLS), OPC UA (certificate-based security)
- Known limitation: Modbus TCP and S7 lack encryption (protocol limitation, documented)

**Alignment**: ✅ Compliant at SL 1-2 for management traffic; ⚠️ Protocol limitations documented for industrial protocols

##### CR 3.3: Security Functionality Verification

**Requirement**: Verify security functions are operating correctly.

**umh-core Implementation**:
- FSM state machines monitor component health
- Watchdog monitors heartbeat to Management Console
- Process supervision via S6 (automatic restart on failure)

**Gap**: No formal security function self-tests (e.g., cryptographic module validation)

**Alignment**: ⚠️ Partial at SL 1 - Health monitoring present, formal security self-tests to be added

##### CR 3.4: Software and Information Integrity

**Requirement**: Verify integrity of software, firmware, and information.

**umh-core Implementation**:
- Container images pulled from trusted registry (umh Docker registry)
- SBOM generation via FOSSA
- Vulnerability scanning via Aikido

**Gap**: No verification of image signatures at deployment time (Docker Content Trust not enforced)

**Alignment**: ⚠️ Partial at SL 1-2 - Supply chain security present, runtime verification to be added

##### CR 3.8: Session Integrity

**Requirement**: Protect integrity of sessions.

**umh-core Implementation**:
- HTTPS sessions to management.umh.app protected by TLS
- AUTH_TOKEN included in message payload (not in URL parameters)

**Alignment**: ✅ Compliant at SL 1-2 - TLS protects session integrity

##### CR 3.9: Protection of Audit Information

**Requirement**: Protect audit logs from unauthorized access and modification.

**umh-core Implementation**:
- Logs stored in `/data/logs/` owned by umhuser (UID 1000)
- All processes run as umhuser (can read/write logs)
- No write protection after log creation

**Gap**: Logs not protected from modification by compromised bridge (all processes run as same user)

**Alignment**: ⚠️ Partial at SL 1 - Basic file permissions, needs immutable log storage for SL 2+

#### FR 4: Data Confidentiality (DC)

**Purpose**: Ensure confidentiality of information on communication channels and in storage.

##### CR 4.1: Information Confidentiality

**Requirement**: Protect confidentiality of information requiring protection.

**umh-core Implementation**:
- TLS encryption for management.umh.app communication
- AUTH_TOKEN stored in `/data/config.yaml` with filesystem permissions (readable by umhuser)
- Protocol-specific encryption: MQTTS, OPC UA with certificates

**Gap**: AUTH_TOKEN readable by all processes running as umhuser (documented risk)

**Alignment**: ⚠️ Partial at SL 1 - Encryption in transit, secrets in filesystem accessible to all bridges

##### CR 4.2: Information Persistence

**Requirement**: Prevent unauthorized information persistence on devices.

**umh-core Implementation**:
- Persistent data limited to `/data/` volume
- Container filesystem ephemeral (reset on container restart)
- No sensitive data in environment variables after startup (written to config.yaml)

**Gap**: AUTH_TOKEN persists in config.yaml permanently (documented behavior)

**Alignment**: ✅ Compliant at SL 1 - Controlled persistence on dedicated volume

##### CR 4.3: Use of Cryptography

**Requirement**: Use industry-standard cryptography for confidentiality and integrity.

**umh-core Implementation**:
- TLS 1.2+ for HTTPS (management.umh.app)
- Go standard library crypto (FIPS-validated implementations available)
- Protocol libraries use standard cryptography (MQTT TLS, OPC UA certificates)

**Gap**: No documentation of approved cryptographic algorithms or key strengths

**Alignment**: ✅ Compliant at SL 1-2 - Standard cryptography used, needs formal documentation for SL 3

#### FR 5: Restricted Data Flow (RDF)

**Purpose**: Segment network via zones and conduits to limit data flow.

##### CR 5.1: Network Segmentation

**Requirement**: Provide or support network segmentation.

**umh-core Deployment Model**:
- Edge-only architecture: umh-core sits at boundary between OT network (data sources) and IT network (Management Console)
- Typical zone model:
  - **Zone 1 (OT/Field)**: PLCs, Modbus devices, S7 devices, OPC UA servers
  - **Zone 2 (Edge)**: umh-core instance
  - **Zone 3 (IT/Enterprise)**: Management Console (management.umh.app)
- Conduits:
  - **OT → Edge**: Protocol-specific connections (Modbus, S7, OPC UA, MQTT)
  - **Edge → IT**: HTTPS outbound to management.umh.app

**umh-core Implementation**:
- No inbound internet connections (edge-only, outbound-only)
- Network isolation via container networking
- Supports deployment behind corporate firewall (recommended)

**Alignment**: ✅ Compliant at SL 1-2 - Edge gateway architecture naturally implements zone boundary

##### CR 5.2: Zone Boundary Protection

**Requirement**: Control communications at zone boundaries.

**umh-core Implementation**:
- Outbound-only connectivity model (no inbound services exposed to internet)
- Customer controls firewall rules for data source access (OT zone)
- TLS required for management traffic (IT zone)

**Gap**: umh-core itself does not implement firewall (relies on deployment environment)

**Alignment**: ✅ Compliant at SL 1-2 - Secure boundary behavior, external firewall required for SL 3

##### CR 5.3: General Purpose Person-to-Person Communication Restrictions

**Requirement**: Prohibit or restrict person-to-person communication features.

**Alignment**: ✅ N/A - umh-core does not provide person-to-person communication features

##### CR 5.4: Application Partitioning

**Requirement**: Separate application components based on criticality and access levels.

**umh-core Implementation**:
- Process-level separation: umh-core agent, benthos bridges, Redpanda broker run as separate processes
- S6 supervision provides process isolation and independent restart
- Container namespaces provide isolation from host

**Gap**: All processes run as same Linux user (no user-level isolation between bridges)

**Alignment**: ⚠️ Partial at SL 1 - Process separation present, user-level isolation not feasible in non-root container

#### FR 6: Timely Response to Events (TRE)

**Purpose**: Respond to security violations in a timely manner.

##### CR 6.1: Audit Log Accessibility

**Requirement**: Provide access to audit logs for analysis.

**umh-core Implementation**:
- Logs accessible via filesystem (`/data/logs/`)
- Logs can be retrieved via Management Console (`get-logs` action)
- S6 logs use tai64n timestamps (convertible to human-readable)

**Gap**: No real-time log streaming or SIEM integration

**Alignment**: ✅ Compliant at SL 1 - Logs accessible on-demand, real-time streaming for SL 2+

##### CR 6.2: Continuous Monitoring

**Requirement**: Continuously monitor the IACS for security events.

**umh-core Implementation**:
- Watchdog monitors heartbeat to Management Console (detects connectivity loss)
- FSM state machines monitor component health
- Status messages pushed to Management Console every polling interval

**Gap**: No intrusion detection or security event correlation

**Alignment**: ✅ Compliant at SL 1 - Health monitoring present, security monitoring for SL 2+

#### FR 7: Resource Availability (RA)

**Purpose**: Ensure availability of the IACS against denial-of-service conditions.

##### CR 7.1: Denial of Service Protection

**Requirement**: Limit effects of denial-of-service attacks.

**umh-core Implementation**:
- Resource limiting via `agent.enableResourceLimitBlocking` (CPU thresholds)
- S6 process supervision with automatic restart
- Communicator channel overflow protection (warns but doesn't block agent)

**Gap**: No rate limiting on incoming management API requests (pull model limits exposure)

**Alignment**: ✅ Compliant at SL 1 - Basic DoS protection, pull model architecture reduces attack surface

##### CR 7.2: Resource Management

**Requirement**: Provide ability to allocate resources by priority.

**umh-core Implementation**:
- Per-bridge CPU/memory limits via S6 softlimit
- Resource blocking prevents bridge creation when resources constrained
- Redpanda reserved 1 CPU core (~5 bridges per remaining CPU core)

**Alignment**: ✅ Compliant at SL 1-2 - Resource allocation and limiting implemented

##### CR 7.3: Control System Backup

**Requirement**: Provide ability to back up critical configuration and data.

**umh-core Implementation**:
- Configuration stored in `/data/config.yaml` on persistent volume
- Customers responsible for volume backup (shared responsibility model)
- Management Console stores configuration (can restore to new instance)

**Alignment**: ✅ Compliant at SL 1 - Configuration backup via Management Console, data backup customer responsibility

##### CR 7.4: Control System Recovery and Reconstitution

**Requirement**: Provide ability to restore system to known secure state.

**umh-core Implementation**:
- Container restart restores to clean state (ephemeral container filesystem)
- Configuration persists across restarts (`/data/config.yaml`)
- Immutable container images ensure consistent deployment

**Alignment**: ✅ Compliant at SL 1-2 - Container model provides reliable recovery

### Component Security Assurance (CSA) Certification

IEC 62443-4-2 supports Component Security Assurance (CSA) certification at four levels (SL 1-4). Consider pursuing **CSA Level 2** certification for umh-core as a software application component.

**Certification requires**:
- IEC 62443-4-1 (SDLA) compliance (secure development lifecycle)
- IEC 62443-4-2 component requirements met for target security level
- Independent third-party assessment

---

## IEC 62443-3-3: System Security Requirements and Security Levels

### Standard Overview

**Full Title**: Industrial communication networks - Network and system security - Part 3-3: System security requirements and security levels

**Published**: 2013-08 (IEC 62443-3-3:2013)

**Scope**: System-level security requirements for IACS, including zones, conduits, and security level definitions.

### Relevance to umh-core

IEC 62443-3-3 defines system-level requirements that apply to umh-core **deployments** (how umh-core is integrated into customer IACS environments). This standard guides customers on secure integration practices.

### Key Concepts for umh-core Deployments

#### Zones and Conduits

**Definition**:
- **Zone**: Grouping of logical or physical assets that share common security requirements
- **Conduit**: Logical grouping of communication channels connecting two or more zones

**umh-core Typical Deployment Zones**:

```
┌─────────────────────────────────────────────────────────────┐
│ Zone 3: Enterprise IT Network                                │
│ - Management Console (management.umh.app)                    │
│ - Cloud services, databases, analytics platforms            │
└─────────────────────────────────────────────────────────────┘
                            ▲
                            │ Conduit 2: HTTPS/TLS
                            │ (Outbound only from umh-core)
                            │
┌─────────────────────────────────────────────────────────────┐
│ Zone 2: DMZ / Edge Gateway Zone                             │
│ - umh-core instance                                          │
│ - Edge processing, protocol conversion                      │
│ - Message broker (Redpanda)                                 │
└─────────────────────────────────────────────────────────────┘
                            ▲
                            │ Conduit 1: Industrial Protocols
                            │ (Modbus, S7, OPC UA, MQTT)
                            │
┌─────────────────────────────────────────────────────────────┐
│ Zone 1: OT / Field Network                                  │
│ - PLCs, RTUs, SCADA systems                                 │
│ - Sensors, actuators, controllers                           │
│ - Industrial devices                                         │
└─────────────────────────────────────────────────────────────┘
```

**Security Conduit Requirements**:
- **Conduit 1 (OT → Edge)**: Protocol-specific security, may lack encryption (Modbus, S7), firewall rules restrict access
- **Conduit 2 (Edge → IT)**: TLS 1.2+ required, outbound-only connections, corporate firewall inspection acceptable

#### System Security Requirements (SR)

IEC 62443-3-3 defines System Requirements (SR) mapped to the seven Foundational Requirements. These apply to **umh-core deployments**, not the software itself.

**Key SRs relevant to umh-core deployment guidance**:

##### SR 1.1: Human User Identification and Authentication

**Requirement**: System must identify and authenticate human users.

**Deployment Guidance**: Management Console provides user authentication (separate from umh-core instance authentication). Customers should implement strong authentication for Management Console access.

##### SR 1.13: Access via Untrusted Networks

**Requirement**: Require additional authentication for access via untrusted networks.

**Deployment Guidance**: umh-core requires HTTPS/TLS for all management traffic. Customers deploying behind TLS-inspecting firewalls should add corporate CA certificates (preferred) or use `ALLOW_INSECURE_TLS=true` only in trusted environments.

##### SR 2.1: Authorization Enforcement

**Requirement**: Enforce authorized access to resources based on access control policies.

**Deployment Guidance**:
- Filesystem access: Mount volumes with appropriate permissions (`:ro` for read-only data sources)
- Network access: Use firewall rules to restrict data source connections to required protocols/ports
- Container capabilities: Run without additional Linux capabilities (no `--privileged`, no `--cap-add`)

##### SR 3.1: Communication Integrity

**Requirement**: Protect integrity of transmitted information.

**Deployment Guidance**:
- Always use TLS for management connections (default behavior)
- For industrial protocols lacking encryption (Modbus, S7), rely on network segmentation and physical security (Zone 1 should be isolated OT network)

##### SR 5.1: Network Segmentation

**Requirement**: Logically or physically segment the network into multiple zones.

**Deployment Guidance**:
- Deploy umh-core in separate zone (DMZ or edge gateway zone)
- Use firewall rules to restrict OT zone access (only umh-core can connect to industrial devices)
- Restrict IT zone access (only umh-core can connect to Management Console)
- Do not expose umh-core GraphQL API (localhost:8090) to internet

##### SR 7.6: Network and Security Configuration Settings

**Requirement**: Control and limit security configuration changes.

**Deployment Guidance**:
- Protect `/data` volume access (contains config.yaml with AUTH_TOKEN)
- Use read-only filesystem mounts where possible
- Monitor configuration changes via audit logs

### Security Level Target (SL-T)

**Recommendation for umh-core deployments**: Target **SL 2** for typical manufacturing environments, with **SL 3** for high-value or safety-critical deployments.

**Rationale**:
- **SL 2**: Protection against intentional misuse by simple means (typical corporate security posture)
- **SL 3**: Protection against sophisticated adversaries with automation-specific knowledge (defense, critical infrastructure)

---

## IEC 62443-2-4: Security Program Requirements for IACS Service Providers

### Standard Overview

**Full Title**: Security for industrial automation and control systems - Part 2-4: Security program requirements for IACS service providers

**Published**: 2015-06, updated 2023-12 (Second Edition)

**Scope**: Security capabilities that service providers (integrators, maintenance providers) should offer to asset owners.

### Relevance to umh-core

IEC 62443-2-4 applies to organizations that **provide umh-core integration and maintenance services** (partners, consultants, internal IT/OT teams). This standard is less relevant to umh-core software itself, but important for go-to-market and partner enablement.

### Key Requirements for umh-core Service Providers

#### SP-1: Security Program Management

**Requirement**: Establish and maintain an IACS security program.

**Application**: Partners providing umh-core integration should document their security practices (secure configuration, deployment checklists, incident response).

#### SP-2: Risk Assessment

**Requirement**: Perform risk assessments for customer IACS environments.

**Application**: Integration partners should assess customer environments for appropriate umh-core deployment location (zone placement, firewall rules, network segmentation).

#### SP-3: System Security Management

**Requirement**: Implement and maintain security controls for customer systems.

**Application**: Partners should follow umh-core deployment security best practices (non-root execution, TLS configuration, volume mount security, resource limits).

#### SP-4: Security Awareness and Training

**Requirement**: Provide security awareness training to service personnel.

**Application**: Partners should train personnel on umh-core security model, threat model, known limitations, and secure configuration practices.

---

## IEC 62443-2-3: Patch Management in the IACS Environment

### Standard Overview

**Full Title**: Security for industrial automation and control systems - Part 2-3: Patch management in the IACS environment

**Published**: 2015-06 (Technical Report)

**Scope**: Guidelines for developing a patch management program for IACS environments.

### Relevance to umh-core

IEC 62443-2-3 provides guidance on **managing umh-core updates** in customer environments. This applies to both umh-core maintainers (providing patches) and customers (deploying patches).

### Patch Management Lifecycle

#### Phase 1: Patch Identification

**Requirement**: Monitor for security vulnerabilities and available patches.

**umh-core Implementation**:
- Vulnerability scanning via Aikido (container images)
- FOSSA for OSS dependency tracking
- GitHub Dependabot for dependency updates
- Sentry for runtime error detection

**Alignment**: ✅ Compliant - Automated vulnerability detection

#### Phase 2: Patch Prioritization

**Requirement**: Assess patches based on risk and criticality.

**umh-core Implementation**:
- Critical security fixes released as hotfix releases
- Non-critical fixes bundled in regular releases
- Release notes document security vs. feature changes

**Gap**: No formal CVSS scoring or patch urgency classification

**Alignment**: ⚠️ Partial - Prioritization happens informally, needs formal process

#### Phase 3: Patch Testing

**Requirement**: Test patches in non-production environment before deployment.

**umh-core Implementation**:
- CI/CD pipeline runs tests on all changes
- Integration tests using Testcontainers
- Staging branch for pre-release testing

**Deployment Guidance for Customers**: Test umh-core updates in non-production environment before deploying to production instances.

**Alignment**: ✅ Compliant - Comprehensive testing before release

#### Phase 4: Patch Deployment

**Requirement**: Deploy patches in controlled manner with rollback capability.

**umh-core Implementation**:
- Container image updates via Docker registry
- Immutable container images (easy rollback to previous version)
- Management Console can trigger restart with new image (pull model)

**Deployment Guidance for Customers**:
- Use versioned image tags (not `latest`)
- Test rollback procedure before emergency need
- Document current running version for rollback

**Alignment**: ✅ Compliant - Container model supports controlled deployment and rollback

#### Phase 5: Patch Verification

**Requirement**: Verify patches applied successfully and didn't introduce issues.

**umh-core Implementation**:
- FSM state machines monitor component health post-update
- Watchdog monitors connectivity to Management Console
- Status messages indicate successful startup

**Deployment Guidance for Customers**: Monitor umh-core logs and status after updates, verify data flow resumes normally.

**Alignment**: ✅ Compliant - Health monitoring validates successful updates

### Patch Management Best Practices for umh-core

1. **Subscribe to security advisories**: Monitor umh-core release notes and security announcements
2. **Establish update schedule**: Regular update cycle (e.g., monthly for non-critical, immediate for critical security)
3. **Test before production**: Use staging environment to validate updates
4. **Maintain version inventory**: Track which umh-core versions are deployed across instances
5. **Document rollback procedure**: Test rollback before needing it in emergency

---

## Gap Analysis and Recommendations

### Current Security Posture Summary

**Strengths** (IEC 62443 Aligned):
- Non-root container execution (least privilege)
- Secure defaults (TLS enabled, no default passwords)
- Comprehensive logging and monitoring
- Edge-only architecture (reduced attack surface)
- Supply chain security (Aikido, FOSSA, SBOM)
- Resource management and DoS protection
- Container-based isolation and recovery
- Zone boundary behavior (edge gateway model)

**Gaps** (Require Attention):

| Gap | IEC 62443 Requirement | Priority | Recommendation |
|-----|----------------------|----------|----------------|
| No container image signature verification | CR 3.4 (SI) | High | Implement Docker Content Trust or cosign image signing |
| AUTH_TOKEN accessible to all bridges | CR 4.1 (DC) | Medium | Accepted risk, document mitigation (monitor network traffic) |
| No structured audit log format | CR 2.8 (UC) | Medium | Implement structured logging (JSON) for SIEM integration |
| No security testing (penetration testing) | SR-4 (Verification) | High | Add security testing to CI/CD (fuzzing, static analysis) |
| No end-of-life policy documentation | SR-7 (Product EOL) | Low | Document version support lifecycle and security update SLA |
| No formal patch urgency classification | IEC 62443-2-3 | Medium | Adopt CVSS scoring and patch SLA (e.g., critical within 7 days) |
| Logs modifiable by compromised bridge | CR 3.9 (SI) | Low | Consider immutable log storage (write-once) for SL 2+ |
| No per-user authentication within instance | CR 1.1 (IAC) | Low | Accepted trade-off for single-instance edge model |
| No real-time log streaming | CR 6.1 (TRE) | Low | Add SIEM integration capability for SL 2+ environments |

### Recommended Security Level Targets

**Baseline Target: Security Level 2 (SL 2)**

Rationale: SL 2 provides protection against intentional misuse by simple means, appropriate for typical manufacturing environments behind corporate firewalls. umh-core currently meets most SL 2 requirements with identified gaps addressable in near-term roadmap.

**Stretch Target: Security Level 3 (SL 3) for Select Requirements**

Consider implementing select SL 3 requirements for competitive differentiation and high-security customers:
- Image signature verification (CR 3.4 RE 2)
- Structured audit logging with SIEM integration (CR 2.8 RE 2)
- Formal security testing program (SR-4 RE 2)
- Public key authentication option (CR 1.9) for management connections

### Certification Roadmap

**Phase 1: Foundation (Current State)**
- Document existing security controls mapping to IEC 62443-4-1 and 4-2
- Close critical gaps (image signing, security testing)
- Formalize secure development lifecycle documentation

**Phase 2: Compliance (6-12 months)**
- Pursue **IEC 62443-4-1 SDLA Level 2** certification (secure development lifecycle)
- Address medium-priority gaps (structured logging, patch SLA)
- Develop deployment security guidance for customers (IEC 62443-3-3 zone/conduit models)

**Phase 3: Market Differentiation (12-18 months)**
- Pursue **IEC 62443-4-2 CSA Level 2** certification (component security assurance)
- Implement select SL 3 requirements (PKI authentication, SIEM integration)
- Develop partner enablement program for IEC 62443-2-4 service provider compliance

### Customer Deployment Guidance

**Create comprehensive deployment guide** covering:
1. **Zone and conduit placement** (IEC 62443-3-3)
   - Recommended network architecture diagrams
   - Firewall rule examples
   - Network segmentation best practices

2. **Security configuration checklist** (IEC 62443-4-2)
   - Volume mount security (read-only where possible)
   - Network access restrictions
   - TLS configuration options
   - Resource limits

3. **Operational security procedures** (IEC 62443-2-3)
   - Patch management workflow
   - Incident response playbooks
   - Log monitoring and alerting
   - AUTH_TOKEN rotation process

4. **Risk acceptance documentation** (IEC 62443-3-3)
   - Known limitations (AUTH_TOKEN access, protocol encryption)
   - Threat model alignment with deployment environment
   - Compensating controls for identified gaps

---

## Conclusion

umh-core demonstrates strong alignment with IEC 62443 security principles for industrial edge gateway software, particularly in secure development practices (IEC 62443-4-1) and component security requirements (IEC 62443-4-2). The edge-only architecture, non-root container execution, and secure defaults provide a solid security foundation appropriate for manufacturing environments.

**Key Takeaways**:

1. **Current posture**: Meets most **Security Level 2 (SL 2)** requirements with identified gaps addressable in near-term roadmap
2. **Recommended target**: Pursue **IEC 62443-4-1 SDLA Level 2** and **IEC 62443-4-2 CSA Level 2** certification for market credibility
3. **Customer value**: Develop comprehensive deployment guidance mapping umh-core to IEC 62443-3-3 zone/conduit models
4. **Competitive differentiation**: Implement select SL 3 requirements (image signing, structured logging, PKI) for high-security customers

**Next Steps**:

1. Close critical gaps: Image signature verification, security testing program, patch SLA
2. Formalize secure development lifecycle documentation for SDLA certification
3. Develop customer deployment security guide with zone/conduit examples
4. Engage third-party assessor for IEC 62443 certification roadmap

---

## References

### IEC 62443 Standards (Official Publications)

- **IEC 62443-1-1:2022** - Terminology, concepts and models
  https://webstore.iec.ch/publication/65067

- **IEC 62443-2-3:2015** - Patch management in the IACS environment (Technical Report)
  https://webstore.iec.ch/publication/23274

- **IEC 62443-2-4:2023** - Security program requirements for IACS service providers (Second Edition)
  https://webstore.iec.ch/publication/73534

- **IEC 62443-3-3:2013** - System security requirements and security levels
  https://webstore.iec.ch/publication/7033

- **IEC 62443-4-1:2018** - Secure product development lifecycle requirements
  https://webstore.iec.ch/publication/33615

- **IEC 62443-4-2:2019** - Technical security requirements for IACS components (Second Edition)
  https://webstore.iec.ch/publication/34421

### ISA Resources

- **ISA/IEC 62443 Standards Overview**
  https://www.isa.org/standards-and-publications/isa-standards/isa-iec-62443-series-of-standards

- **ISASecure Certification Programs**
  https://isasecure.org/certification/

- **ISA Global Cybersecurity Alliance**
  https://gca.isa.org/

### Additional Reading

- **Cisco: ISA/IEC 62443-3-3 Compliance White Paper**
  https://www.cisco.com/c/en/us/products/collateral/security/isaiec-62443-3-3-wp.html

- **Keyfactor: IEC 62443-4-2 Technical Requirements**
  https://www.keyfactor.com/blog/iec-62443-4-2-technical-security-requirements-for-iacs-components/

- **UpGuard: What is IEC/ISA 62443-3-3:2013?**
  https://www.upguard.com/blog/isa-62443-3-3-2013

- **Fortinet: IEC 62443 Standard Cyberglossary**
  https://www.fortinet.com/resources/cyberglossary/iec-62443

- **Industrial Cyber: Essential Guide to IEC 62443**
  https://industrialcyber.co/features/the-essential-guide-to-the-iec-62443-industrial-cybersecurity-standards/

- **Securing IIoT with IEC 62443: Technical Guide**
  https://securityboulevard.com/2025/05/securing-iiot-with-iec-62443-a-technical-guide-to-breach-proof-architectures/

### Related umh-core Documentation

- **Deployment Security** - `/docs/production/security/umh-core/deployment-security.md`
- **Network Configuration** - `/docs/production/security/umh-core/network-configuration.md` (referenced)
- **Architecture Documentation** - Core architecture and FSM patterns
