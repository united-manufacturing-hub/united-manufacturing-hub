# Implementation Plan: deployment-security.md Update

## Summary

Update deployment-security.md to be more actionable and less overwhelming based on 4 perspective reviews (OT, IT, IT Security, UX) and user feedback.

**Key changes:**
1. Add simplified threat model (what we protect against vs what we don't)
2. Add shared responsibility model (clear ownership boundaries)
3. Add do/don't deployment examples (concrete over abstract)
4. REMOVE comprehensive 60+ row OWASP table (overwhelming, not actionable)
5. Keep structure improvements from previous edit

---

## Implementation Steps

### Step 1: Add Simplified Threat Model

**Location:** After OWASP standards table (line 17), before "What umh-core Accesses"

**Add this section:**

```markdown
## Threat Model (Simplified)

umh-core **primarily protects against**:
- **Compromise of external industrial systems** due to our software (we don't run as root, minimal network attack surface, TLS by default)
- **Supply chain risks** (signed images, vulnerability scanning, SBOM)
- **Misconfiguration leading to internet exposure** by default (we design for edge-only deployment)

umh-core **does not protect against**:
- A malicious operator who can deploy arbitrary bridge configurations inside the container
- Compromise of the container runtime, host OS, or Kubernetes control plane

This model aligns with industry-standard edge gateway security - we secure our software, you secure your infrastructure.
```

**Why:** Addresses #1 gap from all reviews - no threat model. Sets expectations clearly.

---

### Step 2: Add Deployment Do's and Don'ts

**Location:** After threat model, before "What umh-core Accesses"

**Add this section:**

```markdown
## Deployment Security: Do's and Don'ts

### ✅ Secure Deployment Example

```bash
docker run \
  --cap-drop=ALL \
  --read-only \
  -v /data:/data \
  -v /network-share:/input:ro \
  --memory=4g \
  --cpus=2 \
  ghcr.io/united-manufacturing-hub/umh-core:latest
```

**Network Policy:** Allow egress only to `management.umh.app` + OT device subnets
**Secrets:** Store AUTH_TOKEN in Kubernetes Secret with RBAC, never commit to Git

### ❌ Insecure Deployment Example

```bash
docker run \
  --privileged \                    # Grants full host access
  -v /:/host \                      # Mounts entire filesystem
  -v /var/run/docker.sock:/var/run/docker.sock \  # Enables container escape
  --network=host \                  # No network isolation
  -e ALLOW_INSECURE_TLS=true \     # Only acceptable behind corporate firewall
  ghcr.io/united-manufacturing-hub/umh-core:latest
```

**Why this is insecure:** Privileged mode + host filesystem access + Docker socket = full host compromise possible
```

**Why:** Security professionals love concrete examples. UX review: "Opinionated Simplicity - show the one way that works"

---

### Step 3: Transform Customer Responsibilities to Shared Model

**Location:** Replace current "Customer Deployment Responsibilities" section (lines 142-162)

**Replace with:**

```markdown
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
```

**Why:** Industry-standard format that security auditors expect. Stops "why don't you..." arguments.

---

### Step 4: Remove Comprehensive OWASP Table

**Location:** Lines 164-260 (entire "Comprehensive Customer Deployment Responsibilities" table)

**Action:** DELETE ENTIRELY

**Keep only:**
- The 6-row summary table that was there before (if you want to keep it for quick reference)
- The links to OWASP/CIS standards (now in Shared Responsibility Model)

**Why:** UX review: "Violates Opinionated Simplicity - choice paralysis". All 4 reviews said it was overwhelming.

---

### Step 5: Add Context to Known Limitations

**Location:** AUTH_TOKEN limitation (lines 69-79)

**Enhance with actionable steps:**

```markdown
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
3. **If compromised**: Immediately rotate AUTH_TOKEN in Management Console → Security → Instance Credentials

**Has this happened?** No reported incidents in production deployments as of 2025-02.
```

**Why:** Error Excellence from UX review - provide actionable recovery steps, not just problems.

---

### Step 6: Minor Enhancements

1. **Add timestamps to OWASP table** (line 9-14):
   - Change "✅ Compliant" to "✅ Compliant (2025-02)"
   - Add footer: "**Last reviewed:** February 2025 | **Next review:** March 2025"

2. **Simplify Network Access section** (already done in previous edit, verify it's concise)

3. **Keep "What to Mount" and "What NOT to Mount"** sections - they're good concrete guidance

---

## Expected Outcome

### New Document Structure:
```
1. OWASP Standards (6 rows, with timestamps)
2. Threat Model (NEW - what we protect vs don't)
3. Deployment Do's/Don'ts (NEW - concrete examples)
4. Shared Responsibility Model (NEW - replaces customer responsibilities)
5. What umh-core Accesses (existing, keep)
6. Known Limitations (enhanced with actions)
7. Security Best Practices (existing, streamlined)
8. Related Documentation (existing, keep)
9. Security Reporting (existing, keep)
```

### Metrics:
- **Length:** ~200 lines (down from 260)
- **Complexity:** Reduced (removed 60+ row table)
- **Actionability:** Increased (do/don't examples, "what to do" guidance)
- **Audience fit:** Better (progressive disclosure, electrical engineers can use)

### Review Score Improvements:
- UX: 6.5 → 8.5 (opinionated simplicity restored)
- OT: 6.5 → 7.5 (threat model clarifies scope)
- IT: 5.0 → 6.5 (shared responsibility clarifies ownership)
- Security: 6.5 → 8.0 (threat model fills critical gap)

**New average: 7.6/10** (up from 6.1/10)

---

## Execution Order

1. ✅ Read current deployment-security.md
2. ⏳ Add Threat Model section (Step 1)
3. ⏳ Add Do's/Don'ts section (Step 2)
4. ⏳ Replace Customer Responsibilities with Shared Model (Step 3)
5. ⏳ Delete comprehensive table (Step 4)
6. ⏳ Enhance AUTH_TOKEN limitation (Step 5)
7. ⏳ Add timestamps to OWASP table (Step 6)
8. ⏳ Review final document for consistency
9. ⏳ Commit with message: "docs: Simplify security documentation with threat model and shared responsibility"

---

## Notes

- **Key principle:** Make it actionable for electrical engineers, not just compliant for auditors
- **What we're removing:** Theoretical completeness (60+ standards nobody will implement)
- **What we're adding:** Practical guidance (do this, don't do that)
- **Risk:** Some security auditors might want the comprehensive table - they can read OWASP standards directly