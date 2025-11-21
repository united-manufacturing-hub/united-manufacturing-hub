# IEC 62443 Compliance Evaluation for umh-core

## Executive Summary

**Evaluation Date**: 2025-11-20

**Security Level Assessment**: umh-core demonstrates baseline **Security Level 1 (SL 1)** compliance with strong foundations for **SL 2** certification. However, critical gaps exist that must be addressed before formal certification.

**Overall Compliance Breakdown**:
- ✅ **Fully Compliant**: 42 requirements (58%)
- 📋 **Known Limitations (Documented)**: 8 requirements (11%)
- 👤 **Customer Responsibility (Documented)**: 12 requirements (17%)
- ❌ **Critical Gaps (Not Addressed)**: 10 requirements (14%)

**Key Finding**: umh-core's architecture and secure development practices align well with IEC 62443 principles, but **operational technology (OT) specific concerns** are underrepresented in current documentation. The focus has been on IT security patterns (containers, TLS, non-root) without adequate coverage of industrial-specific threats.

---

## Critical Gaps Requiring Immediate Attention

### 1. OT/ICS-Specific Security Concerns

**Gap**: IEC 62443 emphasizes operational technology security, but deployment-security.md reads like a cloud-native security document.

**Missing OT/SCADA Security Considerations**:

❌ **Industrial Protocol Security Policy**
- **IEC 62443 Reference**: FR 3 (System Integrity), CR 3.1
- **What's Missing**: No documented policy for handling unencrypted industrial protocols (Modbus, S7) in high-risk environments
- **Why It Matters**: OT networks often have equipment that cannot be upgraded or encrypted. IEC 62443 requires explicit risk acceptance and compensating controls.
- **Recommendation**: Add section "Industrial Protocol Security Considerations" to deployment-security.md:
  - Document which protocols lack encryption (Modbus TCP, S7, raw MQTT)
  - Recommend network segmentation as primary control (Zone 1 isolated from Zone 3)
  - Specify firewall requirements for protocol-specific ports
  - Require physical security for OT networks (IEC 62443-3-3 SR 5.1)

❌ **Process Safety Impact Analysis**
- **IEC 62443 Reference**: IEC 62443-3-3 System Security Requirements
- **What's Missing**: No discussion of safety-critical systems or SIL (Safety Integrity Level) considerations
- **Why It Matters**: Manufacturing environments may include safety systems (emergency stops, interlocks, pressure relief). Compromising these systems has physical safety implications beyond data confidentiality.
- **Current Risk**: Users might deploy umh-core to monitor safety-critical systems without understanding blast radius
- **Recommendation**: Add "Safety System Integration Policy" to deployment-security.md:
  - **NEVER connect umh-core directly to safety-instrumented systems (SIS)**
  - Safety data should only be accessed via read-only monitoring interfaces
  - Document that umh-core is NOT a safety system (no SIL rating)
  - Require separate network segments for safety systems (IEC 61511 compliance)

❌ **Availability Requirements for Manufacturing**
- **IEC 62443 Reference**: FR 7 (Resource Availability), CR 7.1, CR 7.4
- **What's Missing**: No documentation of availability SLA, recovery time objectives (RTO), or data loss tolerance (RPO)
- **Why It Matters**: Manufacturing downtime costs thousands per minute. IEC 62443 SL 2+ requires demonstrating DoS protection and recovery capabilities.
- **Current Documentation**: Generic "container restarts restore clean state" - no metrics or guarantees
- **Recommendation**: Add "Availability and Recovery Requirements" section:
  - Document typical restart time (RTO: ~30 seconds for cold restart, ~5 seconds for process restart)
  - Specify data loss window (RPO: depends on Redpanda retention, typically minutes)
  - Document DoS protection mechanisms (resource limiting, channel overflow behavior)
  - Add monitoring requirements for production deployments (Watchdog, FSM state tracking)
  - Specify when HA/redundancy is required (critical production lines)

❌ **Legacy Equipment Integration Guidance**
- **IEC 62443 Reference**: IEC 62443-2-4 (Service Provider Requirements)
- **What's Missing**: No guidance for integrating with equipment that cannot be updated (PLCs from 1990s, proprietary SCADA systems)
- **Why It Matters**: OT environments have 20-30 year equipment lifecycles. Modern security controls (TLS, authentication) may not exist on legacy devices.
- **Recommendation**: Add "Legacy Equipment Security" section:
  - Document pattern: umh-core as security boundary (gateway model, not endpoint model)
  - Recommend network-level controls for legacy devices (firewall rules, VLANs)
  - Accept that some industrial protocols lack modern security (document risk)
  - Specify monitoring requirements (detect unauthorized access to legacy devices)

---

### 2. Zone and Conduit Model - Missing Boundary Protections

**Gap**: Deployment-security.md mentions "edge-only architecture" but doesn't map to IEC 62443-3-3 zone/conduit model in sufficient detail.

❌ **Zone Placement Policy**
- **IEC 62443 Reference**: IEC 62443-3-3 SR 5.1 (Network Segmentation)
- **What's Current**: Generic statement "umh-core sits between OT networks and IT infrastructure"
- **What's Missing**:
  - Explicit zone classification (Level 0-5 Purdue model)
  - Conduit requirements for each zone boundary
  - Firewall rule templates for typical deployments
- **Why It Matters**: IEC 62443 requires explicit zone/conduit documentation for system certification
- **Recommendation**: Add detailed "Network Architecture" section with:
  ```
  Purdue Model Level 3.5 (DMZ/Edge Gateway Zone)
  ├── Conduit A: OT → Edge (Levels 0-2 → Level 3.5)
  │   ├── Protocols: Modbus TCP (502), S7 (102), OPC UA (4840), MQTT (1883/8883)
  │   ├── Direction: Inbound to umh-core only (no reverse control)
  │   ├── Security: Firewall rules restrict source IPs, unencrypted protocols
  │   └── Compensating Controls: Physical security, network isolation, monitoring
  │
  └── Conduit B: Edge → IT (Level 3.5 → Level 4/5)
      ├── Protocol: HTTPS (443) to management.umh.app
      ├── Direction: Outbound only from umh-core (pull model)
      ├── Security: TLS 1.2+, mutual authentication via AUTH_TOKEN
      └── Firewall Requirements: Allow outbound 443, block all inbound
  ```

❌ **Conduit Security Requirements**
- **IEC 62443 Reference**: IEC 62443-3-3 SR 5.2 (Zone Boundary Protection)
- **What's Missing**: Specific firewall rules, protocol filtering requirements, intrusion detection guidance
- **Current State**: "Use firewall rules" - no specifics
- **Recommendation**: Add "Firewall Configuration Templates" with:
  - Sample iptables/firewalld rules for typical deployments
  - Minimum required ports (by protocol)
  - Recommended deny-by-default policy
  - Intrusion detection system (IDS) recommendations for conduits

❌ **Inter-Zone Communication Policy**
- **IEC 62443 Reference**: IEC 62443-3-3 SR 5.1
- **What's Missing**: Policy for when umh-core bridges multiple OT zones (e.g., Assembly line + Packaging line)
- **Current Risk**: Users might create single umh-core instance that bridges security zones without understanding implications
- **Recommendation**: Add guidance:
  - Deploy separate umh-core instances per security zone (preferred)
  - If bridging zones, document risk and require network segmentation
  - Use separate bridges with different connections for different zones
  - Monitor cross-zone data flows for anomalies

---

### 3. Access Control and Authorization - Critical Gaps

❌ **No Role-Based Access Control (RBAC) Within Instance**
- **IEC 62443 Reference**: FR 1 (Identification and Authentication Control), CR 1.1, CR 1.3
- **What's Current**: AUTH_TOKEN shared secret provides binary access (all or nothing)
- **What's Missing**: No ability to limit actions by user role (read-only vs. admin)
- **Why It Matters**: IEC 62443 SL 2 requires distinguishing between operator, engineer, and administrator roles
- **Current Risk**: Any user with AUTH_TOKEN can:
  - Deploy/delete bridges
  - Modify configuration
  - Access logs containing sensitive data
  - Restart services
- **Categorization**: This is a **Gap requiring implementation**, not a Known Limitation
- **Recommendation**:
  - **Short-term**: Document as Known Limitation with workaround (use separate instances for different privilege levels)
  - **Long-term**: Implement action-level authorization (Management Console enforces RBAC before queuing actions)
  - Add to deployment-security.md "Access Control Model":
    - Current: Instance-level authentication (all users equal)
    - Workaround: Deploy separate instances for production vs. development
    - Management Console RBAC prevents unauthorized actions (enforced before reaching umh-core)

❌ **No Audit Trail for Configuration Changes**
- **IEC 62443 Reference**: FR 2 (Use Control), CR 2.8 (Auditable Events)
- **What's Current**: Logs show "Successfully wrote config" but not WHO made the change or WHY
- **What's Missing**:
  - User identity in log messages (action logs show instanceUUID, not user email)
  - Change justification (reason for config modification)
  - Audit log immutability (logs are mutable by compromised bridge)
- **Why It Matters**: Regulatory compliance (FDA 21 CFR Part 11, GAMP 5) requires audit trails for pharmaceutical/medical manufacturing
- **Recommendation**: Add "Audit Logging Requirements" section:
  - umh-core logs SHOULD include user identity from action payload (if available)
  - Management Console MUST log all actions with user identity before queuing
  - For regulated environments, enable immutable audit logging (write-once storage)
  - Document that umh-core alone is insufficient for audit compliance (requires Management Console logs + umh-core logs)

❌ **Privilege Escalation Risk**
- **IEC 62443 Reference**: FR 2 (Use Control), CR 2.1
- **What's Current**: Non-root container prevents escalation to host root
- **What's Missing**: No discussion of privilege escalation within container (all processes as umhuser)
- **Scenario**: Compromised bridge with malicious Bloblang code could:
  - Read AUTH_TOKEN from config.yaml
  - Modify other bridges' configurations
  - Kill other bridges' processes (same UID)
  - Exfiltrate data from mounted volumes
- **Why It Matters**: IEC 62443 requires limiting lateral movement after compromise
- **Current Documentation**: Threat model mentions "malicious operator" but not "compromised bridge"
- **Recommendation**: Add "Lateral Movement Prevention" section:
  - Document risk: Compromised bridge can access all instance resources
  - Compensating controls:
    - Monitor network connections for unexpected destinations (exfiltration detection)
    - Use read-only volume mounts where possible
    - Deploy separate instances for untrusted bridges (isolation via separate containers)
    - Enable resource limiting (prevents DoS of other bridges)
  - **Accept risk** for typical deployments (trusted bridge configurations)
  - **Require additional controls** for high-security environments (separate instances, network monitoring)

---

### 4. Cryptography and Certificate Management

❌ **No Documented Cryptographic Inventory**
- **IEC 62443 Reference**: FR 4 (Data Confidentiality), CR 4.3
- **What's Current**: "Uses TLS 1.2+ and Go standard library crypto"
- **What's Missing**:
  - Approved cryptographic algorithms (AES-256-GCM, ChaCha20-Poly1305, etc.)
  - Key lengths and strengths (RSA 2048+ bits, ECDSA P-256+)
  - Certificate validation policy (chain of trust, revocation checking)
  - Cryptographic module validation (FIPS 140-2 compliance for regulated environments)
- **Why It Matters**: IEC 62443 SL 3+ requires documenting approved cryptography. Some customers (DoD, finance) require FIPS compliance.
- **Recommendation**: Add "Cryptographic Controls" section:
  - List supported TLS cipher suites (prioritize modern AEAD ciphers)
  - Document certificate requirements (X.509, 2048-bit RSA or 256-bit ECDSA minimum)
  - Specify certificate validation (full chain verification, no self-signed in production)
  - Document FIPS 140-2 status (Go standard library is not FIPS-validated; use BoringCrypto build for FIPS)
  - Add guidance for FIPS environments: `GOEXPERIMENT=boringcrypto go build`

❌ **OPC UA Certificate Management**
- **IEC 62443 Reference**: FR 4 (Data Confidentiality), CR 4.3
- **What's Current**: deployment-security.md mentions "OPC UA with certificates" but no details
- **What's Missing**:
  - Certificate generation procedure (umh-core generates? user provides?)
  - Certificate trust model (trusted vs. rejected servers)
  - Certificate renewal policy (expiration handling)
  - Revocation checking (CRL/OCSP support)
- **Why OPC UA is Special**: Unlike HTTPS, OPC UA uses application-level certificates (not tied to DNS names). Certificate rejection is common source of connection failures.
- **Recommendation**: Add "OPC UA Certificate Security" section:
  - Document certificate generation (benthos-umh auto-generates if not provided)
  - Explain trust model (servers may reject umh-core's certificate initially)
  - Add troubleshooting steps (get server's certificate, add to trust list)
  - Document certificate storage location (`/data/certificates/opcua/`)
  - Recommend certificate rotation schedule (12-24 months for long-lived deployments)

❌ **TLS Certificate Pinning for Management Console**
- **IEC 62443 Reference**: FR 4 (Data Confidentiality), CR 4.3 RE 2 (SL 3)
- **What's Current**: umh-core trusts system CA bundle for management.umh.app
- **Gap**: No certificate pinning or public key pinning (HPKP)
- **Risk**: Compromised CA could issue fraudulent certificate for management.umh.app
- **Why It Matters**: IEC 62443 SL 3 requires strong authentication for management connections
- **Recommendation**: Add certificate pinning for management.umh.app:
  - Pin public key hash (not full certificate, allows rotation)
  - Document backup pins (allow for certificate rotation without release)
  - Add to deployment-security.md as SL 3 enhancement
  - Implement as opt-in feature flag (breaking change for some TLS inspection scenarios)

---

### 5. Operational Security and Monitoring

❌ **No SIEM Integration Guidance**
- **IEC 62443 Reference**: FR 6 (Timely Response to Events), CR 6.1, CR 6.2
- **What's Current**: "Logs accessible via filesystem" and "get-logs action"
- **What's Missing**:
  - Real-time log streaming (syslog, fluentd, Splunk forwarder)
  - Security event correlation (detect attack patterns across instances)
  - Alerting thresholds (when to notify security team)
- **Why It Matters**: IEC 62443 SL 2+ requires continuous monitoring and timely response
- **Current Gap**: Logs are local-only until manually retrieved (incident response delayed)
- **Recommendation**: Add "Security Monitoring and SIEM Integration" section:
  - Document log format (S6 tai64n timestamps, unstructured text)
  - Recommend structured logging for production (JSON output, optional feature flag)
  - Provide syslog forwarding pattern (S6 log pipeline to rsyslog)
  - List critical log patterns for alerting:
    - "Outbound message channel is full" (communicator overflow)
    - "connection reset by peer" (network issues or attack)
    - "AUTH_TOKEN" in unexpected contexts (potential exfiltration)
    - Multiple rapid "FSM transition to starting_failed" (DoS or misconfiguration)
  - Recommend log retention policy (90 days minimum for compliance)

❌ **No Intrusion Detection Guidance**
- **IEC 62443 Reference**: FR 6 (Timely Response to Events), CR 6.2 RE 2 (SL 3)
- **What's Current**: No mention of intrusion detection systems (IDS)
- **What's Missing**:
  - Network-based IDS recommendations (Suricata, Snort, Zeek)
  - Host-based IDS recommendations (OSSEC, Wazuh, Falco)
  - Anomaly detection patterns specific to umh-core behavior
- **Why It Matters**: IEC 62443 SL 3 requires continuous security monitoring
- **Recommendation**: Add "Intrusion Detection for OT Environments" section:
  - Recommend network IDS on conduits (monitor industrial protocols for anomalies)
  - Document normal umh-core network behavior:
    - Periodic HTTPS to management.umh.app (every 10ms for pull, 1s for push)
    - Protocol-specific connections to data sources (Modbus 502, S7 102, OPC UA 4840)
    - Internal Redpanda traffic (localhost:9092)
  - Define suspicious patterns:
    - Unexpected outbound connections (exfiltration)
    - Industrial protocol traffic to unauthorized destinations
    - Port scanning from umh-core container
    - Unusual process execution (compromised bridge)
  - Recommend Falco for container security monitoring (detects anomalous container behavior)

❌ **No Incident Response Playbook**
- **IEC 62443 Reference**: IEC 62443-2-4 SP-3 (System Security Management)
- **What's Current**: "Rotate AUTH_TOKEN if compromised" (one bullet point)
- **What's Missing**: Comprehensive incident response procedures
- **Why It Matters**: IEC 62443 requires documented incident response for service providers
- **Recommendation**: Add "Incident Response Procedures" section with playbooks for:
  1. **Suspected AUTH_TOKEN Compromise**:
     - Immediately create new instance in Management Console
     - Deploy new instance with new AUTH_TOKEN
     - Verify data flow on new instance
     - Delete compromised instance from Management Console
     - Review logs for exfiltration (search for AUTH_TOKEN in outbound requests)
     - Rotate any credentials accessible to compromised instance
  2. **Compromised Bridge Configuration**:
     - Stop affected bridge via Management Console
     - Review bridge configuration for malicious code (Bloblang scripts, HTTP processors)
     - Check network logs for unauthorized connections
     - Redeploy bridge from known-good template
     - Monitor for repeat compromise (persistent attacker)
  3. **Container Runtime Compromise**:
     - Isolate affected host (firewall rules block all traffic)
     - Preserve forensic evidence (do not restart container)
     - Capture memory dump if possible (Docker container commit)
     - Review host logs for privilege escalation attempts
     - Rebuild host from known-good image
     - Restore umh-core from configuration backup
  4. **Network Attack (DoS, Port Scanning)**:
     - Enable rate limiting at firewall (protect umh-core from DoS)
     - Review firewall logs for attack source
     - Block attacker IPs at network edge
     - Verify umh-core resource limits prevented impact
     - Document attack pattern for future detection

❌ **No Backup and Restore Procedures**
- **IEC 62443 Reference**: FR 7 (Resource Availability), CR 7.3, CR 7.4
- **What's Current**: "Configuration stored in /data/config.yaml" and "Management Console stores configuration"
- **What's Missing**:
  - Backup frequency recommendations (RPO: Recovery Point Objective)
  - Backup verification procedures (test restores regularly)
  - Disaster recovery time objectives (RTO: Recovery Time Objective)
  - Data loss scenarios and mitigation
- **Why It Matters**: IEC 62443 requires demonstrating recovery capability
- **Recommendation**: Add "Backup and Disaster Recovery" section:
  - **Configuration Backup**:
    - Primary: Management Console stores configuration (can restore to new instance)
    - Secondary: Backup `/data/config.yaml` via volume snapshot (daily recommended)
    - Test restore procedure quarterly (verify configuration import works)
  - **Data Backup**:
    - Redpanda data in `/data/redpanda/` (message broker storage)
    - Retention policy depends on use case (historical analysis vs. real-time only)
    - For critical data, enable Redpanda replication or Kafka mirroring to central cluster
  - **Recovery Time Objectives**:
    - Cold start (new instance): ~5 minutes (provision + configuration + bridge startup)
    - Hot restore (existing instance, new container): ~30 seconds
    - Partial failure (single bridge): ~5 seconds (S6 automatic restart)
  - **Recovery Point Objectives**:
    - Configuration: Zero data loss (stored in Management Console)
    - In-flight data: Up to buffer size (Redpanda retention, typically minutes)
    - Historical data: Depends on backup frequency (daily backup = up to 24h loss)

---

### 6. Secure Development Lifecycle Gaps

❌ **No Formal Security Testing Program**
- **IEC 62443 Reference**: IEC 62443-4-1 SR-4 (Verification and Validation)
- **What's Current**: "Ginkgo v2 test framework with unit and integration tests"
- **What's Missing**:
  - Penetration testing (manual security assessment)
  - Fuzzing (automated input validation)
  - Static application security testing (SAST beyond linting)
  - Dynamic application security testing (DAST)
  - Security regression testing (verify fixes stay fixed)
- **Why It Matters**: IEC 62443-4-1 requires security-specific testing, not just functional testing
- **Current State**: Aikido scans container images (supply chain), but no application-level security testing
- **Recommendation**: Add "Security Testing Program" to development process:
  - **Static Analysis**:
    - Add gosec to CI/CD pipeline (Go security scanner)
    - Add Semgrep with custom rules for umh-core patterns (AUTH_TOKEN handling, FSM state races)
    - Run SonarQube or CodeQL for vulnerability detection
  - **Dynamic Testing**:
    - Fuzzing with go-fuzz on benthos config parser (crash/hang detection)
    - Fuzzing on GraphQL API (malformed queries)
    - Docker socket scanning with Trivy (runtime vulnerabilities)
  - **Penetration Testing**:
    - Annual third-party security assessment (pre-certification requirement)
    - Internal security review before major releases (0.x.0 versions)
    - Scope: Network attack surface, authentication/authorization, input validation
  - **Security Regression Testing**:
    - Create test cases for each security fix (prevent re-introduction)
    - Add to CI/CD pipeline (fail build if security tests fail)

❌ **No Product End-of-Life (EOL) Policy**
- **IEC 62443 Reference**: IEC 62443-4-1 SR-7 (Product End-of-Life)
- **What's Current**: No documentation
- **What's Missing**:
  - Version support lifecycle (how long is a version supported?)
  - Security update SLA (critical vulnerabilities patched within X days)
  - EOL notification process (how much notice before support ends?)
  - Migration path from EOL versions
- **Why It Matters**: IEC 62443 requires defined EOL process for product certification
- **Recommendation**: Add "Version Support and End-of-Life Policy" section:
  - **Support Lifecycle**:
    - Major version (1.x): 24 months after next major version release
    - Minor version (1.x): 12 months or until next minor version
    - Patch version (1.x.y): No guaranteed support (upgrade to latest patch)
  - **Security Updates**:
    - Critical vulnerabilities (CVSS 9.0+): Patch within 7 days
    - High vulnerabilities (CVSS 7.0-8.9): Patch within 30 days
    - Medium vulnerabilities (CVSS 4.0-6.9): Patch in next minor release
    - Low vulnerabilities (CVSS 0.1-3.9): Fix opportunistically
  - **EOL Notification**:
    - 6 months notice before EOL (via release notes, security advisories)
    - Final security patch released at EOL date
    - Migration guide to current version
  - **Exceptions**:
    - Critical security vulnerabilities may be backported to EOL versions (case-by-case)
    - Long-term support (LTS) versions may be offered for enterprise customers

❌ **No Formal Patch Management SLA**
- **IEC 62443 Reference**: IEC 62443-2-3 (Patch Management)
- **What's Current**: iec-62443-research.md identifies this as gap with "Medium" priority
- **What's Missing**: Documented SLA for security patch delivery
- **Why It Matters**: IEC 62443 requires timely security updates. Customers need to know response time for vulnerabilities.
- **Recommendation**: Document in deployment-security.md "Security Update SLA":
  - **Critical (CVSS 9.0+)**: Patch within 7 days of disclosure
  - **High (CVSS 7.0-8.9)**: Patch within 30 days
  - **Medium (CVSS 4.0-6.9)**: Patch within 90 days
  - **Low (CVSS 0.1-3.9)**: No SLA (opportunistic fix)
  - **Exceptions**:
    - Zero-day exploits: Emergency patch within 24-48 hours
    - No known exploit: Timeline may extend if mitigation available
    - Requires breaking change: Coordinate with customers, may delay
  - **Notification**:
    - Security advisories via GitHub Security Advisories
    - Email notification to registered customers (if list exists)
    - Update to Management Console banner (notify users to update)

---

### 7. Documentation and Transparency Gaps

❌ **No Known Vulnerabilities Disclosure**
- **IEC 62443 Reference**: IEC 62443-4-1 SR-5 (Defect Management)
- **What's Current**: Aikido/FOSSA scan for vulnerabilities, but no public disclosure
- **What's Missing**: Security advisories for known vulnerabilities
- **Why It Matters**: IEC 62443 requires transparency about security defects
- **Recommendation**: Add "Security Advisories" section to deployment-security.md:
  - Link to GitHub Security Advisories (if enabled)
  - Document process for CVE assignment (when applicable)
  - List currently known vulnerabilities with workarounds (if any)
  - Add statement: "See https://github.com/united-manufacturing-hub/umh-core/security/advisories for current security advisories"

❌ **No Threat Intelligence Integration**
- **IEC 62443 Reference**: FR 6 (Timely Response to Events)
- **What's Current**: No mention of threat intelligence
- **What's Missing**: How umh-core stays informed about emerging OT threats
- **Why It Matters**: OT environments face evolving threats (Stuxnet, TRITON, Industroyer). IEC 62443 requires staying current.
- **Recommendation**: Add "Threat Intelligence and Vulnerability Management" section:
  - Document sources: CISA ICS-CERT advisories, ICS-CERT@US-CERT.gov mailing list
  - Subscribe to OT-specific threat feeds (ICS-CERT, SANS ICS, Dragos)
  - Review for applicability to umh-core (industrial protocols, edge gateway threats)
  - Update documentation/code if mitigations needed

---

## Detailed Requirement Evaluation

### IEC 62443-4-1: Secure Product Development Lifecycle

| Requirement | Status | Evidence | Recommendation |
|-------------|--------|----------|----------------|
| **SR-1: Security Requirements Definition** | ✅ Compliant | Threat model documented in deployment-security.md | Add OT-specific threat scenarios |
| **SR-2: Secure Design** | ✅ Compliant | Non-root container, least privilege, secure defaults | Document defense-in-depth layers |
| **SR-3: Secure Implementation** | ✅ Compliant | Go memory safety, CI/CD linting, pre-commit hooks | Add gosec and Semgrep to CI |
| **SR-4: Verification and Validation** | ❌ Gap | Functional tests exist, no security testing | Add penetration testing, fuzzing, SAST/DAST |
| **SR-5: Defect Management** | ✅ Compliant | Linear, Sentry, FOSSA, Aikido tracking | Add public security advisories |
| **SR-6: Patch Management** | 📋 Partial | Update mechanism exists, no formal SLA | Document patch delivery timeline (7/30/90 days) |
| **SR-7: Product End-of-Life** | ❌ Gap | No documented EOL policy | Add version support lifecycle and EOL process |

---

### IEC 62443-4-2: Technical Security Requirements for IACS Components

#### FR 1: Identification and Authentication Control (IAC)

| CR | Requirement | Status | Evidence | Recommendation |
|----|-------------|--------|----------|----------------|
| 1.1 | Human User Identification and Authentication | ⚠️ Partial SL1 | AUTH_TOKEN shared secret (not per-user) | Known Limitation: Document instance-level authentication model |
| 1.2 | Software Process and Device Identification | ✅ SL1-2 | InstanceUUID + AUTH_TOKEN authenticates instance | Compliant |
| 1.3 | Account Management | ⚠️ Partial SL1 | Instance lifecycle via Management Console | Known Limitation: No per-user accounts within instance |
| 1.7 | Strength of Password-Based Authentication | ✅ SL1-2 | AUTH_TOKEN is strong random value (UUID-like) | Compliant |
| 1.9 | Strength of Public Key Authentication | ❌ N/A SL3+ | Uses shared secret, not PKI | SL3 requirement: Consider PKI for high-security customers |

**Analysis**: umh-core's instance-level authentication is appropriate for edge gateway deployment but lacks granular RBAC. This should be documented as "Known Limitation" rather than ignored.

#### FR 2: Use Control (UC)

| CR | Requirement | Status | Evidence | Recommendation |
|----|-------------|--------|----------|----------------|
| 2.1 | Authorization Enforcement | ✅ SL1 | Non-root container, process isolation from host | Add section on within-container authorization limitations |
| 2.2 | Wireless Access Management | ✅ N/A | umh-core does not manage wireless access | Compliant |
| 2.4 | Mobile Code | ⚠️ Partial SL1 | Bloblang sandboxed, but bridge configs are trusted | Document risk: Malicious bridge configs can execute code |
| 2.8 | Auditable Events | ⚠️ SL1 (needs SL2) | Comprehensive logs, but unstructured | Add structured logging (JSON) for SIEM integration |
| 2.9 | Audit Storage Capacity | ⚠️ Partial SL1 | Log rotation prevents unbounded growth, no capacity monitoring | Add disk space monitoring and alerting |
| 2.12 | Provisioning Product Supplier Roots of Trust | ⚠️ Partial SL1 | TLS certificate validation, no image signature verification | Implement Docker Content Trust or cosign |

**Critical Gap - CR 2.4 (Mobile Code)**: Deployment-security.md mentions "malicious bridge configuration" in threat model but doesn't explain what "mobile code" means in umh-core context. Bloblang scripts ARE mobile code (customer-provided data transformation logic executed by benthos). This should be documented more clearly with examples of risk (HTTP processor exfiltrating AUTH_TOKEN, file processor reading sensitive volumes).

#### FR 3: System Integrity (SI)

| CR | Requirement | Status | Evidence | Recommendation |
|----|-------------|--------|----------|----------------|
| 3.1 | Communication Integrity | ✅ SL1-2 | TLS for management, protocol limitations documented | Compliant (Modbus/S7 limitations known) |
| 3.3 | Security Functionality Verification | ⚠️ Partial SL1 | FSM health monitoring, no security self-tests | Add cryptographic module self-tests (FIPS) |
| 3.4 | Software and Information Integrity | ⚠️ Partial SL1-2 | SBOM, vulnerability scanning, no image signature verification | ❌ Gap: Implement image signing (high priority) |
| 3.8 | Session Integrity | ✅ SL1-2 | TLS protects management sessions | Compliant |
| 3.9 | Protection of Audit Information | ⚠️ Partial SL1 | Logs not protected from modification | Consider write-once log storage for SL2+ |

**Critical Gap - CR 3.4**: iec-62443-research.md identifies "No container image signature verification" as **High priority** gap, but deployment-security.md doesn't mention it at all. This is a significant security control for supply chain integrity.

#### FR 4: Data Confidentiality (DC)

| CR | Requirement | Status | Evidence | Recommendation |
|----|-------------|--------|----------|----------------|
| 4.1 | Information Confidentiality | ⚠️ Partial SL1 | TLS in transit, AUTH_TOKEN readable by all bridges | 📋 Known Limitation: Document and accept risk |
| 4.2 | Information Persistence | ✅ SL1 | Controlled persistence on `/data/` volume | Compliant |
| 4.3 | Use of Cryptography | ✅ SL1-2 | TLS 1.2+, Go standard crypto | ❌ Gap: Document approved algorithms and key strengths |

**Critical Gap - CR 4.3**: deployment-security.md says "TLS enabled by default" but doesn't specify which TLS versions, cipher suites, or key lengths. IEC 62443 requires explicit cryptographic inventory. FIPS 140-2 compliance also not documented (required for some customers).

#### FR 5: Restricted Data Flow (RDF)

| CR | Requirement | Status | Evidence | Recommendation |
|----|-------------|--------|----------|----------------|
| 5.1 | Network Segmentation | ✅ SL1-2 | Edge gateway architecture implements zone boundary | ❌ Gap: Add detailed zone/conduit diagrams with Purdue model |
| 5.2 | Zone Boundary Protection | ✅ SL1-2 | Outbound-only, external firewall required | ❌ Gap: Add firewall rule templates and IDS guidance |
| 5.3 | General Purpose Person-to-Person Communication | ✅ N/A | No person-to-person communication features | Compliant |
| 5.4 | Application Partitioning | ⚠️ Partial SL1 | Process separation, no user-level isolation | 📋 Known Limitation: Document and accept trade-off |

**Critical Gap - FR 5**: iec-62443-research.md has excellent zone/conduit analysis, but deployment-security.md doesn't reference it. Users won't know how to deploy securely without explicit zone placement guidance.

#### FR 6: Timely Response to Events (TRE)

| CR | Requirement | Status | Evidence | Recommendation |
|----|-------------|--------|----------|----------------|
| 6.1 | Audit Log Accessibility | ✅ SL1 | Logs via filesystem and get-logs action | ❌ Gap: Add real-time log streaming for SL2+ |
| 6.2 | Continuous Monitoring | ✅ SL1 | Watchdog, FSM monitoring | ❌ Gap: Add SIEM integration guidance and IDS recommendations |

**Critical Gap - FR 6**: No guidance on security monitoring for OT environments. IEC 62443 SL 2+ requires continuous monitoring and timely incident response. Deployment-security.md should include SIEM integration patterns and alerting thresholds.

#### FR 7: Resource Availability (RA)

| CR | Requirement | Status | Evidence | Recommendation |
|----|-------------|--------|----------|----------------|
| 7.1 | Denial of Service Protection | ✅ SL1 | Resource limiting, S6 restart, pull model | ❌ Gap: Document availability SLA and RTO/RPO |
| 7.2 | Resource Management | ✅ SL1-2 | Per-bridge CPU/memory limits, resource blocking | Compliant |
| 7.3 | Control System Backup | ✅ SL1 | Configuration backup via Management Console | ❌ Gap: Add backup/restore procedures and testing |
| 7.4 | Control System Recovery | ✅ SL1-2 | Container restart, immutable images | ❌ Gap: Document recovery time objectives |

**Critical Gap - FR 7**: Manufacturing environments need availability guarantees. Deployment-security.md should document typical restart times (RTO), data loss windows (RPO), and when HA/redundancy is required.

---

### IEC 62443-3-3: System Security Requirements (Deployment Guidance)

These are **Customer Responsibility** but require guidance:

| SR | Requirement | Status | Guidance Needed |
|----|-------------|--------|-----------------|
| 1.1 | Human User Identification | 👤 Customer | Document that Management Console provides user authentication |
| 1.13 | Access via Untrusted Networks | 👤 Customer | Add corporate firewall configuration guidance (TLS inspection) |
| 2.1 | Authorization Enforcement | 👤 Customer | Add filesystem mount best practices (read-only where possible) |
| 3.1 | Communication Integrity | 👤 Customer | Emphasize network segmentation for unencrypted protocols |
| 5.1 | Network Segmentation | 👤 Customer | ❌ Gap: Add Purdue model zone placement examples |
| 7.6 | Network and Security Configuration Settings | 👤 Customer | Add guidance on protecting /data volume access |

**Critical Gap**: deployment-security.md has "Shared Responsibility Model" but doesn't provide actionable guidance for customer responsibilities. Users need concrete examples:
- Sample firewall rules for typical deployment
- Kubernetes NetworkPolicy templates
- Volume mount security best practices
- Corporate CA certificate installation procedure

---

### IEC 62443-2-4: Security Program Requirements (Service Providers)

These apply to **partners/integrators**, not umh-core software:

| SP | Requirement | Status | Guidance Needed |
|----|-------------|--------|-----------------|
| SP-1 | Security Program Management | 👤 Partner | Create partner enablement guide with security checklist |
| SP-2 | Risk Assessment | 👤 Partner | Provide risk assessment template for customer environments |
| SP-3 | System Security Management | 👤 Partner | Document secure deployment best practices |
| SP-4 | Security Awareness and Training | 👤 Partner | Create training materials on umh-core security model |

**Recommendation**: Create separate "Partner Security Guide" document with IEC 62443-2-4 compliance checklist for integrators.

---

### IEC 62443-2-3: Patch Management (Customer/Provider)

| Phase | Requirement | Status | Evidence | Recommendation |
|-------|-------------|--------|----------|----------------|
| Identification | Monitor for vulnerabilities | ✅ Compliant | Aikido, FOSSA, Dependabot, Sentry | Add public security advisories |
| Prioritization | Assess patches by risk | ⚠️ Partial | Informal prioritization | ❌ Gap: Adopt CVSS scoring and formal SLA |
| Testing | Test before deployment | ✅ Compliant | CI/CD pipeline, Testcontainers | Compliant |
| Deployment | Deploy in controlled manner | ✅ Compliant | Immutable images, easy rollback | Add customer deployment guidance |
| Verification | Verify patch success | ✅ Compliant | FSM health monitoring, status messages | Compliant |

---

## OT-Specific Security Concerns Analysis

### Industrial Protocol Vulnerabilities

**What We Missed**: Deployment-security.md mentions "Modbus TCP and S7 lack encryption" but doesn't explain the full risk model.

**Missing Context**:
1. **Modbus TCP (Port 502)**:
   - No authentication (any client can send commands if firewall allows)
   - No integrity checking (man-in-the-middle can modify data)
   - No confidentiality (plaintext data transmission)
   - **Risk**: Attacker on OT network can read/write PLC registers
   - **IEC 62443 Control**: Network segmentation (Zone 1 isolated), read-only monitoring preferred

2. **S7 (Port 102)**:
   - Proprietary protocol with minimal security
   - Authentication via "password" but transmitted in plaintext
   - No encryption of data or commands
   - **Risk**: Industrial espionage (competitor reads production data)
   - **IEC 62443 Control**: Physical security, network isolation, monitoring for unauthorized access

3. **OPC UA (Port 4840)**:
   - Modern security (certificates, encryption) BUT configuration complexity
   - Certificate rejection is common (users struggle with trust model)
   - **Risk**: Users disable security mode to "get it working"
   - **IEC 62443 Control**: Document certificate setup procedure, provide troubleshooting guide

**Recommendation**: Add "Industrial Protocol Security Analysis" section with table:

| Protocol | Port | Authentication | Encryption | Integrity | IEC 62443 Control |
|----------|------|----------------|------------|-----------|-------------------|
| Modbus TCP | 502 | None | None | None | Network segmentation (Zone 1 isolation), read-only preferred |
| S7 | 102 | Plaintext password | None | None | Physical security, monitoring, VLANs |
| OPC UA | 4840 | Certificates | TLS (optional) | Yes (if TLS enabled) | Always enable SecurityMode=SignAndEncrypt |
| MQTT | 1883/8883 | Username/password | TLS (8883 only) | Yes (if TLS) | Use MQTTS (8883) for external brokers |

---

### Safety System Integration

**What We Completely Missed**: No discussion of Safety Instrumented Systems (SIS) or Safety Integrity Levels (SIL).

**Why This Matters**:
- IEC 62443 is often deployed alongside IEC 61508/61511 (functional safety standards)
- Manufacturing sites have safety systems (emergency stops, pressure relief, interlocks)
- umh-core is NOT a safety system and should never be in safety loop
- Customers might try to monitor safety signals without understanding risk

**Missing Guidance**:
1. **umh-core is NOT SIL-rated** (no safety certification)
2. **NEVER use umh-core for safety-critical control** (read-only monitoring only)
3. **Safety data should be isolated** (separate network segment, read-only access)
4. **Latency is not guaranteed** (umh-core is best-effort, not real-time)

**Recommendation**: Add prominent warning in deployment-security.md:

```
## Safety System Integration Warning

⚠️ **CRITICAL**: umh-core is NOT a safety system and has NO Safety Integrity Level (SIL) rating.

**DO NOT**:
- Use umh-core for safety-critical control logic
- Rely on umh-core for emergency shutdown systems
- Connect umh-core to safety PLCs without read-only interface
- Assume real-time guarantees (best-effort delivery only)

**Acceptable Use**:
- Monitor safety system status via read-only connection
- Log safety events for compliance/analysis (not real-time response)
- Connect to separate monitoring interface (NOT production safety controller)

For safety-critical applications, consult IEC 61508/61511 certified systems.
```

---

### Physical Security Requirements

**What We Missed**: IEC 62443 assumes physical security controls for OT environments, but we don't document expectations.

**Missing Guidance**:
- Where should umh-core hardware be physically located? (locked server room? factory floor?)
- Who should have physical access? (OT engineers? IT staff? contractors?)
- What happens if device is stolen? (AUTH_TOKEN persists in /data volume)

**Recommendation**: Add "Physical Security Requirements" section:

```
## Physical Security

umh-core assumes deployment in **physically secure location**:

### Location Requirements
- **Preferred**: Locked server room or network closet (restricted access)
- **Acceptable**: Factory floor in locked cabinet (limit physical access)
- **NOT ACCEPTABLE**: Open factory floor accessible to all personnel

### Access Control
- Limit physical access to authorized OT/IT personnel
- Log physical access to device location (security camera, badge reader)
- Require two-person rule for sensitive operations (production environments)

### Theft/Loss Scenario
If umh-core device is stolen:
1. **Immediately revoke instance** in Management Console (invalidates AUTH_TOKEN)
2. **Rotate AUTH_TOKEN** for any instances using shared configuration
3. **Review logs** for unauthorized access before theft
4. **Change any credentials** accessible to stolen device (mounted volumes, network shares)

Physical security is part of IEC 62443 zone protection - umh-core relies on it.
```

---

### Availability and Resilience

**What We Missed**: No discussion of high availability (HA), redundancy, or failover for critical production lines.

**Missing Guidance**:
- When is single umh-core instance acceptable? (non-critical data collection)
- When is HA required? (critical production lines, regulatory compliance)
- What are failure modes? (single point of failure analysis)
- How to achieve redundancy? (multiple instances, load balancing)

**Recommendation**: Add "High Availability and Redundancy" section:

```
## High Availability (HA) Considerations

### Single Instance Deployment (Acceptable for):
- Development/testing environments
- Non-critical data collection (historical analysis only)
- Environments where temporary data loss is acceptable
- **RTO**: ~30 seconds (container restart)
- **RPO**: ~5 minutes (Redpanda buffer size)

### High Availability Required for:
- Critical production lines (downtime = lost revenue)
- Regulatory compliance (FDA 21 CFR Part 11, GAMP 5)
- Real-time monitoring (immediate detection of issues)
- Safety system monitoring (read-only, but no data loss acceptable)

### HA Deployment Patterns:

**Pattern 1: Active-Standby**
- Deploy two umh-core instances (separate AUTH_TOKENs)
- Both collect data from same sources (duplicate connections)
- Primary pushes to production Kafka, standby to backup
- Failover: Manually switch consumers to standby Kafka

**Pattern 2: Active-Active (Preferred)**
- Deploy two umh-core instances with different bridges
- Partition data sources by criticality (Instance A = critical, Instance B = non-critical)
- Both push to shared Kafka cluster (no single point of failure)
- Failover: Redeploy critical bridges to surviving instance

**Pattern 3: Kubernetes HA**
- Deploy umh-core as StatefulSet with replicas
- Use shared persistent volume (NFS, Ceph)
- Anti-affinity rules (pods on different nodes)
- Automatic failover via Kubernetes

### Limitations:
- umh-core is NOT designed for hot failover (sub-second recovery)
- Bridge state is local (cannot be shared between instances)
- Active-active requires manual bridge partitioning
```

---

## Recommendations by Category

### 1. Immediate Actions (Blocking for Certification)

**These must be addressed before pursuing IEC 62443-4-1 (SDLA) or 4-2 (CSA) certification:**

1. **Implement Container Image Signing** (CR 3.4)
   - Priority: **Critical**
   - Effort: Medium (2-3 days)
   - Implement: Docker Content Trust or cosign image signing
   - Document: Signature verification procedure in CI/CD
   - Impact: Prevents supply chain attacks

2. **Document Cryptographic Inventory** (CR 4.3)
   - Priority: **Critical**
   - Effort: Low (1 day)
   - Document: Approved algorithms, key lengths, TLS cipher suites
   - Add: FIPS 140-2 build instructions (BoringCrypto)
   - Impact: Required for SL 2+ compliance

3. **Add Security Testing to CI/CD** (SR-4)
   - Priority: **Critical**
   - Effort: Medium (3-5 days)
   - Implement: gosec, Semgrep, fuzzing (go-fuzz)
   - Add: Security regression tests for known CVEs
   - Impact: Detects vulnerabilities before release

4. **Create Product End-of-Life Policy** (SR-7)
   - Priority: **High**
   - Effort: Low (2-3 hours)
   - Document: Version support lifecycle, EOL notification process
   - Publish: Security update SLA (7/30/90 day response times)
   - Impact: Required for IEC 62443-4-1 certification

5. **Add Zone/Conduit Deployment Guidance** (FR 5)
   - Priority: **High**
   - Effort: Medium (1-2 days)
   - Create: Purdue model diagrams with umh-core placement
   - Add: Firewall rule templates for typical deployments
   - Document: Network segmentation best practices
   - Impact: Customers cannot deploy securely without this

---

### 2. Short-Term Enhancements (0-3 months)

**Improve compliance posture and documentation:**

1. **Add OT-Specific Threat Analysis**
   - Document industrial protocol vulnerabilities (Modbus, S7)
   - Add safety system integration warnings (SIL ratings)
   - Create physical security requirements section
   - Document availability requirements (RTO/RPO)

2. **Implement Structured Audit Logging**
   - Add JSON logging option (feature flag)
   - Include user identity in action logs (from Management Console)
   - Document syslog forwarding patterns
   - Create SIEM integration guide

3. **Create Security Monitoring Guide**
   - Document critical log patterns for alerting
   - Add IDS recommendations (Suricata, Falco)
   - Define normal network behavior baseline
   - Create incident response playbooks

4. **Develop Backup and Disaster Recovery Procedures**
   - Document backup frequency recommendations
   - Add restore testing procedures
   - Define RTO/RPO for different deployment types
   - Create HA deployment patterns

5. **Add Certificate Management Documentation**
   - Document OPC UA certificate generation
   - Explain certificate trust model
   - Add troubleshooting guide for rejections
   - Recommend rotation schedules

---

### 3. Medium-Term Improvements (3-6 months)

**Address architectural gaps:**

1. **Implement Role-Based Access Control (RBAC)**
   - Design: Action-level authorization in Management Console
   - Implement: Role definitions (operator, engineer, admin)
   - Document: Permission matrix (which roles can execute which actions)
   - Impact: Meets IEC 62443 CR 1.1 for SL 2

2. **Add TLS Certificate Pinning**
   - Implement: Public key pinning for management.umh.app
   - Document: Backup pins for rotation
   - Add: Feature flag (opt-in, breaks TLS inspection)
   - Impact: SL 3 requirement for management connections

3. **Create Partner Security Enablement Program**
   - Develop: IEC 62443-2-4 compliance checklist
   - Create: Training materials on umh-core security model
   - Provide: Risk assessment templates for customer environments
   - Impact: Enables partners to deliver secure integrations

4. **Establish Formal Security Testing Program**
   - Schedule: Annual third-party penetration testing
   - Implement: Internal security review for major releases
   - Create: Security bug bounty program (optional)
   - Document: Vulnerability disclosure policy
   - Impact: Industry-standard security assurance

---

### 4. Long-Term Strategic (6-12 months)

**Pursue formal certification:**

1. **IEC 62443-4-1 SDLA Level 2 Certification**
   - Engage: Third-party assessment body (TUV, UL, ISASecure)
   - Document: Secure development lifecycle evidence
   - Audit: Development processes, tooling, training
   - Certificate: 3-year validity, annual surveillance audits
   - Cost: $30k-50k (assessment + remediation)
   - Impact: Market differentiation, customer confidence

2. **IEC 62443-4-2 CSA Level 2 Certification**
   - Pre-requisite: SDLA Level 2 completed
   - Engage: Same assessment body (continuity)
   - Document: Component security requirements evidence
   - Test: Security functionality, vulnerability assessment
   - Certificate: 3-year validity, requires SDLA maintenance
   - Cost: $40k-60k (assessment + testing)
   - Impact: Competitive advantage in OT market

3. **Develop Customer Deployment Certification Program**
   - Create: "IEC 62443 Compliant Deployment" checklist
   - Provide: Verification scripts (firewall rules, configuration)
   - Offer: Optional deployment review service
   - Issue: Compliance certificate for customer deployments
   - Impact: Revenue opportunity, customer success

---

### 5. Specific Documentation Additions

**Sections to add to deployment-security.md:**

1. **Industrial Protocol Security Analysis** (immediately)
   - Table of protocols with security characteristics
   - Risk analysis for each protocol
   - IEC 62443 control mapping (zone segmentation, monitoring)
   - When to use vs. avoid each protocol

2. **Safety System Integration Warning** (immediately)
   - Prominent warning: umh-core is NOT SIL-rated
   - Acceptable use: Read-only monitoring only
   - Prohibited use: Safety-critical control logic
   - Reference: IEC 61508/61511 for safety systems

3. **Zone and Conduit Deployment Guidance** (high priority)
   - Purdue model diagrams (Levels 0-5)
   - umh-core placement (Level 3.5 DMZ)
   - Conduit security requirements (firewall rules)
   - Network segmentation best practices

4. **Physical Security Requirements** (high priority)
   - Device location requirements (locked room)
   - Access control expectations
   - Theft/loss incident response

5. **Availability and High Availability** (high priority)
   - Single instance RTO/RPO (30s/5min)
   - When HA is required (critical production)
   - HA deployment patterns (active-standby, active-active)
   - Limitations (not hot failover)

6. **Cryptographic Controls** (critical)
   - Approved algorithms (AES-256, ChaCha20, RSA-2048+)
   - TLS configuration (1.2+ only, AEAD ciphers)
   - Certificate requirements (X.509, chain validation)
   - FIPS 140-2 build instructions

7. **Security Monitoring and SIEM Integration** (medium priority)
   - Structured logging (JSON output option)
   - Syslog forwarding patterns
   - Critical log patterns for alerting
   - SIEM integration examples (Splunk, ELK)

8. **Intrusion Detection for OT Environments** (medium priority)
   - Network IDS recommendations (Suricata, Zeek)
   - Host IDS recommendations (Falco)
   - Normal behavior baseline (umh-core traffic patterns)
   - Suspicious activity detection

9. **Incident Response Playbooks** (medium priority)
   - AUTH_TOKEN compromise response
   - Compromised bridge configuration
   - Container runtime compromise
   - Network attack (DoS, port scanning)

10. **Backup and Disaster Recovery** (medium priority)
    - Backup frequency recommendations
    - Restore testing procedures
    - RTO/RPO by deployment type
    - Data loss scenarios and mitigation

11. **OPC UA Certificate Security** (low priority)
    - Certificate generation procedure
    - Trust model explanation
    - Troubleshooting certificate rejections
    - Rotation schedule recommendations

12. **Patch Management and EOL Policy** (critical)
    - Version support lifecycle
    - Security update SLA (7/30/90 days by CVSS)
    - EOL notification process (6 months)
    - Migration path from EOL versions

---

## Risk Acceptance Documentation

**For Known Limitations that should be explicitly accepted:**

### 1. AUTH_TOKEN Accessible to All Bridges

**Status**: 📋 Known Limitation (documented in deployment-security.md)

**Risk**: Compromised bridge can exfiltrate AUTH_TOKEN via outbound network request.

**IEC 62443 Requirement**: FR 4 (Data Confidentiality), CR 4.1

**Why Cannot Fix**: Single-container architecture requires all processes to run as same user (UID 1000). Secrets isolation requires per-process users (not possible in non-root container).

**Compensating Controls**:
- Monitor network connections for unexpected destinations (exfiltration detection)
- Use network policies to block unauthorized outbound connections
- Deploy separate instances for untrusted bridges (isolation via separate containers)
- Rotate AUTH_TOKEN if compromise suspected

**Risk Level**: Medium (requires compromised bridge configuration AND network access)

**Acceptance Criteria**: Appropriate for trusted bridge configurations. High-security environments should deploy separate instances for untrusted bridges.

---

### 2. No Per-User Authentication Within Instance

**Status**: 📋 Known Limitation (should be documented)

**Risk**: All users with AUTH_TOKEN have full control (cannot limit by role).

**IEC 62443 Requirement**: FR 1 (Identification and Authentication), CR 1.1, CR 1.3

**Why Trade-Off Made**: Edge gateway model prioritizes simplicity over granular RBAC. Management Console provides user authentication; umh-core authenticates instances.

**Compensating Controls**:
- Management Console enforces RBAC before queuing actions (umh-core receives only authorized actions)
- Deploy separate instances for production vs. development (instance-level isolation)
- Audit logs in Management Console track user actions (before reaching umh-core)

**Risk Level**: Low (Management Console provides RBAC layer)

**Acceptance Criteria**: Appropriate for edge deployment model. Management Console RBAC is sufficient for most customers.

---

### 3. No User-Level Isolation Between Bridges

**Status**: 📋 Known Limitation (documented in deployment-security.md)

**Risk**: Compromised bridge can interfere with other bridges (same UID).

**IEC 62443 Requirement**: FR 2 (Use Control), CR 2.1; FR 5 (Restricted Data Flow), CR 5.4

**Why Trade-Off Made**: Non-root container security prioritized over per-bridge user isolation. Process-level user switching requires root capabilities.

**Compensating Controls**:
- Redpanda isolates message flows by topic (bridges don't share data)
- S6 softlimit provides per-bridge resource limits (CPU, memory)
- Container boundary prevents host access (defense in depth)
- Non-root execution prevents privilege escalation

**Risk Level**: Medium (shared filesystem access within container)

**Acceptance Criteria**: Trade-off worth it for non-root security. Alternative would be multi-container architecture (significant redesign).

---

### 4. Industrial Protocol Encryption Limitations

**Status**: 📋 Known Limitation (documented in deployment-security.md under OWASP OT Top 10 #9)

**Risk**: Modbus TCP, S7 protocols lack encryption (plaintext data transmission).

**IEC 62443 Requirement**: FR 4 (Data Confidentiality), CR 4.1; FR 3 (System Integrity), CR 3.1

**Why Cannot Fix**: Protocol design limitation (cannot be changed at umh-core level).

**Compensating Controls**:
- Network segmentation (OT zone isolated from IT/internet)
- Physical security (OT networks in restricted areas)
- Firewall rules (restrict protocol access by IP)
- Monitoring (detect unauthorized protocol connections)

**Risk Level**: High (but inherent to OT environments)

**Acceptance Criteria**: Standard OT risk. IEC 62443 acknowledges legacy protocol limitations. Network segmentation is primary control.

---

### 5. TLS Certificate Validation Can Be Disabled

**Status**: 📋 Known Limitation (documented in deployment-security.md)

**Risk**: `ALLOW_INSECURE_TLS=true` enables MITM attacks.

**IEC 62443 Requirement**: FR 4 (Data Confidentiality), CR 4.3

**Why Option Exists**: Corporate firewalls perform TLS inspection (MITM by design). Adding corporate CA certificates is complex.

**Compensating Controls**:
- Only use behind trusted corporate firewall (where MITM is authorized)
- Prefer adding corporate CA certificates (documented alternative)
- Document risk in deployment guide

**Risk Level**: Medium (only if misused outside corporate firewall)

**Acceptance Criteria**: Acceptable for corporate environments with TLS inspection. Should not be used in untrusted networks.

---

## Summary of Categorization Recommendations

**Update deployment-security.md with these categorizations:**

### Move to "Known Limitations" Section:
1. AUTH_TOKEN accessible to all bridges (already there, keep)
2. No user-level isolation between bridges (already there, keep)
3. TLS certificate validation can be disabled (already there, keep)
4. **ADD**: No per-user authentication within instance
5. **ADD**: No RBAC within instance (Management Console enforces)
6. **ADD**: Logs modifiable by compromised bridge
7. **ADD**: No real-time log streaming (pull-based only)
8. **ADD**: No container image signature verification (until implemented)

### Add to "Customer Responsibility" Section:
1. Infrastructure and runtime security (already there)
2. Secrets lifecycle (already there)
3. Monitoring and incident response (already there)
4. Deployment security (already there)
5. **ADD**: Network segmentation and zone placement (IEC 62443-3-3)
6. **ADD**: Firewall rules for conduits
7. **ADD**: Physical security of umh-core device
8. **ADD**: Backup and disaster recovery procedures
9. **ADD**: SIEM integration and security monitoring
10. **ADD**: Corporate CA certificate management
11. **ADD**: High availability (if required for critical production)

### Add New "Protocol Security Limitations" Section:
1. Modbus TCP lacks encryption (protocol limitation)
2. S7 lacks encryption (protocol limitation)
3. Raw MQTT (1883) lacks encryption (user choice, use 8883)
4. OPC UA encryption depends on SecurityMode (user configuration)
5. Compensating controls: Network segmentation, physical security

### Must Be Implemented (Gaps):
1. Container image signature verification (CR 3.4) - HIGH PRIORITY
2. Formal security testing program (SR-4) - HIGH PRIORITY
3. Product end-of-life policy (SR-7) - HIGH PRIORITY
4. Security update SLA (IEC 62443-2-3) - HIGH PRIORITY
5. Cryptographic inventory documentation (CR 4.3) - HIGH PRIORITY
6. Zone/conduit deployment guidance (FR 5) - HIGH PRIORITY
7. OT-specific threat analysis - MEDIUM PRIORITY
8. Structured audit logging - MEDIUM PRIORITY
9. SIEM integration guidance - MEDIUM PRIORITY
10. Incident response playbooks - MEDIUM PRIORITY

---

## Certification Readiness Assessment

### Current State: Pre-Certification

**IEC 62443-4-1 SDLA Level 2 Readiness**: **65%**
- ✅ SR-1 (Security Requirements): Yes
- ✅ SR-2 (Secure Design): Yes
- ✅ SR-3 (Secure Implementation): Yes
- ❌ SR-4 (Verification and Validation): No (missing security testing)
- ✅ SR-5 (Defect Management): Yes
- ⚠️ SR-6 (Patch Management): Partial (missing SLA)
- ❌ SR-7 (Product End-of-Life): No (missing policy)

**IEC 62443-4-2 CSA Level 2 Readiness**: **70%**
- FR 1 (IAC): 80% (missing RBAC documentation)
- FR 2 (UC): 70% (missing structured logging, audit protection)
- FR 3 (SI): 65% (missing image signing, security self-tests)
- FR 4 (DC): 75% (missing cryptographic inventory)
- FR 5 (RDF): 60% (missing zone/conduit guidance)
- FR 6 (TRE): 60% (missing SIEM integration, IDS guidance)
- FR 7 (RA): 85% (missing HA documentation)

**Estimated Time to Certification Ready**: **3-4 months** with focused effort

**Critical Path**:
1. Implement image signing (2-3 days)
2. Add security testing to CI/CD (3-5 days)
3. Document cryptographic inventory (1 day)
4. Create EOL policy and patch SLA (2-3 hours)
5. Add zone/conduit deployment guide (1-2 days)
6. Formalize secure development lifecycle documentation (1 week)
7. Engage third-party assessor (2-3 months for assessment)

**Total Effort Estimate**: ~2 weeks development + 2-3 months assessment

---

## Conclusion

umh-core demonstrates **strong alignment with IEC 62443 principles** for an industrial edge gateway, particularly in:
- Secure development practices (automated testing, vulnerability scanning, supply chain security)
- Component security (non-root execution, secure defaults, TLS by default)
- Edge-only architecture (reduced attack surface, zone boundary behavior)

However, **critical gaps exist in operational technology (OT) specific security documentation**:

1. **Industrial protocol security** is mentioned but not analyzed in depth
2. **Safety system integration** is completely absent (major risk for manufacturing)
3. **Zone and conduit placement** lacks actionable guidance (Purdue model diagrams missing)
4. **Availability requirements** are not documented (RTO/RPO undefined)
5. **Physical security expectations** are not stated (customers may deploy insecurely)
6. **Security monitoring** guidance is minimal (no SIEM integration or IDS recommendations)
7. **Incident response** is limited to one bullet point (needs full playbooks)

**Key Insight**: deployment-security.md currently reads like a **cloud-native security document** (containers, TLS, non-root) but lacks the **operational technology context** required for IEC 62443 compliance in manufacturing environments.

**Recommended Next Steps**:

1. **Immediate** (blocking for certification):
   - Implement container image signing
   - Document cryptographic inventory
   - Create EOL policy and patch SLA
   - Add zone/conduit deployment guidance

2. **Short-term** (0-3 months):
   - Add OT-specific threat analysis
   - Create security monitoring guide
   - Document backup and disaster recovery
   - Add safety system integration warnings

3. **Medium-term** (3-6 months):
   - Implement RBAC for actions
   - Add structured audit logging
   - Establish formal security testing program
   - Create partner enablement program

4. **Long-term** (6-12 months):
   - Pursue IEC 62443-4-1 SDLA Level 2 certification
   - Pursue IEC 62443-4-2 CSA Level 2 certification
   - Develop customer deployment certification program

**Final Assessment**: umh-core is **well-positioned for IEC 62443 certification** with 3-4 months of focused effort. The architecture is sound; the gaps are primarily in documentation and operational security guidance.
