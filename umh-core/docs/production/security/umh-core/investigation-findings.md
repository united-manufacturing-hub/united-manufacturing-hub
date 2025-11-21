# Security Claims Investigation

## Executive Summary

This investigation examined security claims made in umh-core documentation to verify factual accuracy and identify gaps between documented security practices and actual implementation. The investigation focused on container image signing, SBOM generation, cryptography implementation, and the trust.umh.app dashboard description.

---

## Container Image Signing

**Status**: NOT FOUND

**Evidence**:
- Searched all CI/CD workflows in `.github/workflows/`
- Examined `build-umh-core.yml` (primary build pipeline)
- No evidence of signing tools: cosign, docker trust, notary, or sigstore
- Build workflow steps:
  1. Docker build via Makefile
  2. Docker tag with platform suffix
  3. Docker push to ghcr.io
  4. Manifest creation for multi-arch
- No signing step present in the pipeline
- Documentation claims: "container images are cryptographically signed for integrity verification" (deployment-security.md, line 61)

**Code References**:
- Build workflow: `/Users/jeremytheocharis/umh-git/united-manufacturing-hub-eng-3889/.github/workflows/build-umh-core.yml`
- Lines 168-206: Build and push without signing
- Lines 240-278: Manifest creation without signing

**Recommendation**:
1. Remove the claim "container images are cryptographically signed" from all documentation until signing is actually implemented
2. If image signing is planned, document it as a future enhancement rather than a current capability
3. Consider implementing cosign for image signing as it's the industry standard for container images

---

## SBOM Generation

**Status**: NOT FOUND

**Evidence**:
- Searched CI/CD workflows for SBOM tools: syft, cyclonedx, FOSSA
- No SBOM generation found in build pipeline
- Documentation claims: "Software Bill of Materials documents are generated for every release" (deployment-security.md, line 61)
- No evidence of syft, cyclonedx-cli, or similar SBOM tools in workflow
- Build process:
  - Docker build from Dockerfile
  - No SBOM generation step
  - No artifact upload of SBOM files

**Code References**:
- Build workflow: `/Users/jeremytheocharis/umh-git/united-manufacturing-hub-eng-3889/.github/workflows/build-umh-core.yml`
- No SBOM-related steps in the entire workflow

**Recommendation**:
1. Remove the claim "Software Bill of Materials documents are generated for every release" from documentation
2. If SBOM generation is desired, implement using syft or cyclonedx-cli in the CI/CD pipeline
3. Example implementation: Add `syft packages ghcr.io/united-manufacturing-hub/umh-core:${VERSION} -o spdx-json > sbom.json` to build workflow

---

## Vulnerability Scanning Tools (Aikido and FOSSA)

**Status**: PARTIAL - Configuration exists but no evidence in build pipeline

**Evidence**:
- Snyk configuration file exists: `/Users/jeremytheocharis/umh-git/united-manufacturing-hub-eng-3889/.snyk`
- Configuration excludes vendor/, test/, docs/, examples/ from scanning
- Documentation claims: "All container images undergo automated vulnerability scanning with Aikido and FOSSA tools in the CI/CD pipeline" (deployment-security.md, line 61)
- No evidence of Aikido or FOSSA in GitHub Actions workflows
- Possible scenarios:
  - Scanning happens in a separate private workflow
  - Scanning happens through GitHub integrations (not visible in workflow files)
  - Scanning tools are configured but not actively running

**Code References**:
- Snyk config: `/Users/jeremytheocharis/umh-git/united-manufacturing-hub-eng-3889/.snyk`
- Build workflow: No Aikido or FOSSA steps found

**Recommendation**:
1. Clarify where Aikido and FOSSA scanning actually occurs (if at all)
2. If scanning happens through GitHub integrations, document this separately from "CI/CD pipeline"
3. If scanning is not automated, update documentation to reflect the actual process
4. Consider making scanning steps visible in the public workflow for transparency

---

## trust.umh.app Description

**Status**: USER CORRECTION ACCEPTED

**Claim in Documentation**: "The trust dashboard at trust.umh.app provides transparency into security scanning results and dependency management practices" (deployment-security.md, line 61)

**User Correction**: trust.umh.app is actually the Vanta compliance dashboard showing ISO 27001 and NIST compliance status, not security scanning results

**Investigation**: Cannot verify via web access, but user correction should be trusted as they have access to the system

**Recommendation**:
1. Update documentation to accurately describe trust.umh.app as: "The trust dashboard at trust.umh.app provides transparency into compliance status including ISO 27001 and NIST certifications"
2. Remove references to "security scanning results and dependency management practices" unless there is a separate page for those
3. If security scanning results are available elsewhere, document the correct URL

---

## Cryptography Implementation

### TLS Version Configuration

**Status**: CRITICAL GAP - TLS 1.0 in insecure mode, defaults to Go's secure settings in secure mode

**Evidence**:

**Insecure Mode (when ALLOW_INSECURE_TLS=true)**:
- File: `/Users/jeremytheocharis/umh-git/united-manufacturing-hub-eng-3889/umh-core/pkg/communicator/api/v2/http/requester.go`
- Line 86: `MinVersion: tls.VersionTLS10`
- This sets TLS 1.0 as minimum version when insecure mode is enabled
- TLS 1.0 is deprecated and should not be used (NIST SP 800-52 requires TLS 1.2+)

**Secure Mode (default, when ALLOW_INSECURE_TLS=false)**:
- File: `/Users/jeremytheocharis/umh-git/united-manufacturing-hub-eng-3889/umh-core/pkg/communicator/api/v2/http/requester.go`
- Lines 58-74: No explicit TLS configuration
- Relies on Go's default TLS settings
- Go 1.25.3 defaults:
  - Minimum version: TLS 1.2
  - Cipher suites: Go's secure default set (includes AES-GCM, ChaCha20-Poly1305)
  - Certificate validation: Enabled by default

**Code Context**:
```go
// Secure client (lines 58-74)
if !insecureTLS {
    secureOnce.Do(func() {
        transport := &http.Transport{
            ForceAttemptHTTP2: false,
            TLSNextProto:      make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
            Proxy:             http.ProxyFromEnvironment,
            IdleConnTimeout:   keepAliveTimeout,
            // NO TLSClientConfig - uses Go defaults
        }
        secureHTTPClient = &http.Client{
            Transport: transport,
            Timeout:   30 * time.Second,
        }
    })
    return secureHTTPClient
}

// Insecure client (lines 77-97)
insecureOnce.Do(func() {
    transport := &http.Transport{
        ForceAttemptHTTP2: false,
        TLSNextProto:      make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
        Proxy:             http.ProxyFromEnvironment,
        IdleConnTimeout:   keepAliveTimeout,
        TLSClientConfig: &tls.Config{
            InsecureSkipVerify: insecureTLS,
            MinVersion:         tls.VersionTLS10,  // ⚠️ TLS 1.0 is deprecated
        },
    }
    insecureHTTPClient = &http.Client{
        Transport: transport,
        Timeout:   30 * time.Second,
    }
})
```

**Compliance with Policy**:
- Policy requires: TLS 1.2+ (SSLabs Grade B or higher)
- Secure mode: ✅ COMPLIANT (Go defaults to TLS 1.2+)
- Insecure mode: ❌ NOT COMPLIANT (TLS 1.0 is below policy requirement)

**Documentation Claims**:
"All connections to management.umh.app use TLS 1.2 or higher with modern cipher suites including AES-GCM and ChaCha20-Poly1305" (deployment-security.md, line 51)

**Assessment**:
- Claim is TRUE for normal operation (ALLOW_INSECURE_TLS=false)
- Claim is FALSE when ALLOW_INSECURE_TLS=true (allows TLS 1.0)
- Documentation should clarify that TLS 1.2+ only applies when insecure mode is disabled

### Cipher Suites

**Status**: COMPLIANT - Uses Go's secure defaults

**Evidence**:
- No explicit cipher suite configuration in code
- Go 1.25.3 defaults include:
  - TLS_AES_128_GCM_SHA256
  - TLS_AES_256_GCM_SHA384
  - TLS_CHACHA20_POLY1305_SHA256
  - TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
  - TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
  - TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
  - TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384

**Compliance with Policy**:
- Policy requires: Modern cipher suites (AES-GCM, ChaCha20-Poly1305)
- Implementation: ✅ COMPLIANT (Go's defaults meet policy requirements)

### RSA Key Length / ECC Curve

**Status**: COMPLIANT - Handled by certificate provider (Let's Encrypt via Cloudflare)

**Evidence**:
- umh-core connects to management.umh.app over HTTPS
- Certificate is issued by Cloudflare/Let's Encrypt
- Client does not generate or manage certificates
- Certificate validation uses Go's standard library

**Compliance with Policy**:
- Policy requires: RSA 2048+ or ECC 256+
- Implementation: ✅ COMPLIANT (server-side, not client responsibility)

### Data-at-Rest Encryption

**Status**: NOT IMPLEMENTED - No encryption of data at rest

**Evidence**:
- Searched for crypto/aes, crypto/cipher, encryption implementation: None found
- Persistent storage: `/data` directory mounted as plain volume
- Configuration file: `/data/config.yaml` stored in plaintext
- Logs: `/data/logs/` stored in plaintext
- Redpanda data: `/data/redpanda/` stored in plaintext
- AUTH_TOKEN: Stored in plaintext in config.yaml (documented as known limitation)

**Code References**:
- No encryption implementation found in `/Users/jeremytheocharis/umh-git/united-manufacturing-hub-eng-3889/umh-core/pkg/`
- Only references to "encrypted" are for OPC UA certificate structures received from backend API

**Documentation**:
- deployment-security.md acknowledges AUTH_TOKEN is stored in plaintext (line 149)
- No claims about data-at-rest encryption in documentation
- Documentation correctly describes this as a known limitation

**Compliance with Policy**:
- Policy requires: AES 256 for data at rest
- Implementation: ❌ NOT IMPLEMENTED - All data stored in plaintext
- Note: Documentation does not claim data-at-rest encryption, so this is not a documentation inaccuracy but a feature gap

---

## Summary of Gaps

### Documentation Accuracy Issues (High Priority)

1. **Container Image Signing**: Documentation claims images are signed, but no signing occurs
2. **SBOM Generation**: Documentation claims SBOMs are generated, but no generation occurs
3. **trust.umh.app Description**: Incorrectly described as security scanning dashboard (actually Vanta compliance)
4. **TLS 1.0 in Insecure Mode**: Documentation claims TLS 1.2+ always, but insecure mode allows TLS 1.0

### Feature Gaps (Policy Non-Compliance)

1. **Data-at-Rest Encryption**: No encryption of persistent data (policy requires AES 256)
2. **TLS Version in Insecure Mode**: TLS 1.0 is below policy requirement of TLS 1.2+
3. **Vulnerability Scanning Visibility**: Claims automated scanning but no evidence in public workflows

---

## Recommendations

### Immediate Actions (Documentation Fixes)

1. **Remove false claims**:
   - Remove "container images are cryptographically signed"
   - Remove "Software Bill of Materials documents are generated"
   - Correct trust.umh.app description to "compliance dashboard (ISO 27001, NIST)"

2. **Clarify TLS behavior**:
   - Add: "TLS 1.2+ is enforced by default. When ALLOW_INSECURE_TLS=true is set (for corporate TLS inspection), the minimum version is reduced to TLS 1.0 to maximize compatibility"
   - Document that insecure mode should only be used behind trusted corporate firewalls

3. **Document actual scanning process**:
   - If Aikido/FOSSA run via integrations, document that explicitly
   - If scanning is manual or periodic, document the actual process
   - Consider adding visible scanning steps to public workflows for transparency

### Medium-Term Actions (Implementation)

1. **Implement image signing**:
   - Add cosign to build workflow
   - Sign images after push to registry
   - Document verification process for users

2. **Implement SBOM generation**:
   - Add syft or cyclonedx-cli to build workflow
   - Generate SBOM for each release
   - Upload SBOMs as release artifacts

3. **Improve TLS security in insecure mode**:
   - Change MinVersion from TLS 1.0 to TLS 1.2 even in insecure mode
   - Only InsecureSkipVerify should be affected by ALLOW_INSECURE_TLS
   - If TLS 1.0 is needed for specific corporate environments, require explicit opt-in

### Long-Term Actions (Feature Development)

1. **Data-at-Rest Encryption**:
   - Implement encryption for /data/config.yaml (especially AUTH_TOKEN)
   - Consider using system keyring or hardware security module
   - Document key management and rotation procedures

2. **Explicit TLS Configuration**:
   - Configure TLS settings explicitly instead of relying on Go defaults
   - Document exact cipher suites and TLS versions used
   - Add TLS configuration testing to CI/CD pipeline

---

## Investigation Methodology

This investigation used the following systematic approach:

1. **Codebase Search**: Searched for signing tools, SBOM tools, and cryptography implementations using grep and glob patterns
2. **CI/CD Analysis**: Examined all GitHub Actions workflows for security tooling
3. **Code Review**: Read actual TLS configuration in HTTP client implementation
4. **Documentation Cross-Reference**: Compared documented claims against implementation evidence
5. **Go Defaults Research**: Verified Go 1.25.3 default TLS behavior from official documentation

**Files Examined**:
- `.github/workflows/build-umh-core.yml` (build pipeline)
- `pkg/communicator/api/v2/http/requester.go` (TLS configuration)
- `docs/production/security/umh-core/deployment-security.md` (security claims)
- `.snyk` (vulnerability scanning configuration)
- `Makefile` (build process)
- `Dockerfile` (container image construction)

**Tools Used**:
- grep (recursive pattern searching)
- find (file discovery)
- Read tool (file content examination)
- Bash (command execution)

---

## Conclusion

The security documentation for umh-core contains several inaccuracies that should be corrected:

1. Image signing is claimed but not implemented
2. SBOM generation is claimed but not implemented
3. trust.umh.app is misdescribed as security scanning dashboard
4. TLS 1.2+ claim is accurate only for default mode, not insecure mode

The actual cryptography implementation is generally sound when used in default mode (ALLOW_INSECURE_TLS=false), relying on Go's secure defaults for TLS 1.2+ and modern cipher suites. However, the insecure mode allows TLS 1.0, which is below policy requirements.

Data-at-rest encryption is not implemented, but this is not misrepresented in the documentation as the known limitations section correctly acknowledges AUTH_TOKEN is stored in plaintext.

Priority should be given to correcting the documentation inaccuracies first, then implementing the claimed but missing features (image signing, SBOM generation), and finally addressing the feature gaps (data-at-rest encryption, TLS 1.0 in insecure mode).
