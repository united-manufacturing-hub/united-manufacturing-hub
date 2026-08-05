# Compliance

How the Management Console's access controls map to the standards a security review will ask about, including the places where they do not yet meet them.

## Authentication

Relevant standards: NIST SP 800-63B, IEC 62443-3-3 SR 1.1, OWASP Authentication Cheat Sheet.

| Standard | Requirement | How it is met |
| --- | --- | --- |
| OWASP Authentication | Multi-factor, brute force protection, secure session management | Auth0 provides brute force protection and multi-factor. Session management is ours, see [Sessions](#sessions) |
| NIST SP 800-63B AAL2 | Multi-factor authentication | Available through Auth0 risk-based challenges, and enforceable for every user through your own identity provider once single sign-on is configured |
| NIST SP 800-63B AAL2 | Session timeout of 24 hours or less | **Not met**, see [Where we fall short](#where-we-fall-short) |
| IEC 62443-3-3 SR 1.1 | Identify and authenticate human users | Auth0 issues a unique identity per user |
| IEC 62443-3-3 SR 1.1 RE 2 | Multi-factor on untrusted networks, SL2 and above | Available through Auth0, and enforced by your own identity provider when single sign-on is configured. The default email one-time code is a single factor |

## Authorization

Relevant standards: NIST SP 800-53 AC-3 and AC-6, IEC 62443-3-3 SR 2.1, OWASP Authorization Cheat Sheet.

| Standard | Requirement | How it is met |
| --- | --- | --- |
| OWASP Authorization | Least privilege, deny by default, role-based access | Access is granted per location and inherits downward from there. Nothing is granted outside a location you were given |
| NIST SP 800-53 AC-3 | Enforce access at every access point | The console enforces permissions server-side on every request. A change to a signed-in user's permissions takes effect within minutes, see [Where we fall short](#where-we-fall-short) |
| NIST SP 800-53 AC-6 | Least privilege | Viewer, Editor and Admin, granted at the narrowest location that works |
| IEC 62443-3-3 SR 2.1 | Enforce authorization for all users | The console validates permissions per request. An instance does not enforce per-user permissions, see [Where we fall short](#where-we-fall-short) |

## Sessions

Relevant standards: OWASP Session Management Cheat Sheet, NIST SP 800-63B session binding.

| Standard | Requirement | How it is met |
| --- | --- | --- |
| OWASP Session | Token rotation, secure cookies, sign-out | Sessions are held in cookies JavaScript cannot read, rotating on a 14 day sliding window with a 30 day hard limit |
| NIST SP 800-63B | Idle timeout of 1 hour or less at AAL2 | **Not met**, see [Where we fall short](#where-we-fall-short) |

Auth0 authenticates. Everything after that, including the session rules above, is managed by the Management Console itself. See [Sessions](../authentication/README.md#sessions).

## Where we fall short

| Standard | Requirement | Status |
| --- | --- | --- |
| NIST SP 800-63B AAL2 | Idle timeout of 1 hour or less | Not implemented. A session expires 14 days after it was last renewed |
| NIST SP 800-63B AAL2 | Session timeout of 24 hours or less | Not met. A session runs for up to 14 days, and up to 30 days from first sign-in |
| NIST SP 800-53 AC-2(3) | Disable dormant accounts automatically | Not implemented. Removing them is manual |
| IEC 62443-3-3 SR 2.1 | Authorization enforced for all users at every access point | The console enforces per user. An instance does not: within a company, any member can act on any instance whatever their role, see [No Per-User Access Control Within Instance](../../production/security/umh-core/deployment-security.md#no-per-user-access-control-within-instance) |
| IEC 62443 SL3 | Multi-factor on all interfaces | Not met. Multi-factor is available for external access, which meets SL2 |
| OWASP | Revoke permissions immediately | Not met. A permission change reaches a signed-in user within minutes, and an offline user at their next sign-in. Removing the user revokes immediately |
| Audit trail | Audit records you can read and retain | Not available yet, see [Shared Responsibility](shared-responsibility.md) |

## Target security level

The Management Console targets **IEC 62443 Security Level 2**. SL2 covers intentional attacks using simple means, by attackers with generic skills and cybercrime-level resources, which is the profile that fits standard manufacturing operations.

For critical infrastructure that has to meet SL3 or above, talk to UMH about what is possible.

## References

- [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)
- [OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)
- [NIST SP 800-63B Digital Identity Guidelines](https://pages.nist.gov/800-63-4/sp800-63b.html)
- [NIST SP 800-53 Access Control](https://csrc.nist.gov/pubs/sp/800/53/r5/upd1/final)
- [ISA/IEC 62443 Series of Standards](https://www.isa.org/standards-and-publications/isa-standards/isa-iec-62443-series-of-standards)
