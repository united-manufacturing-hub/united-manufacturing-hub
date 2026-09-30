# Compliance

How the Management Console's access controls map to the standards a security review will ask about, which choices we made on purpose, and what is on the roadmap.

UMH is a product supplier, so the Management Console is assessed as a component under IEC 62443-4-2 (software application). IEC 62443-3-3 applies to the system you build with it, and is yours or your integrator's to assess. Component Requirements (CR) in 4-2 mirror the System Requirements (SR) in 3-3 by number, so CR 1.1 corresponds to SR 1.1.

## Authentication

Relevant standards: NIST SP 800-63B, IEC 62443-4-2 CR 1.1, OWASP Authentication Cheat Sheet.

| Standard | Requirement | How it is met |
| -- | -- | -- |
| OWASP Authentication | Multi-factor, brute force protection, secure session management | Our identity provider provides brute force protection, and multi-factor as described below. Session management is ours, see [Sessions](<#sessions>) |
| NIST SP 800-63B AAL2 | Two authentication factors at every sign-in | Met for Enterprise companies that connect their own identity provider with multi-factor enforced, or that ask UMH to enforce multi-factor for them. Community accounts use one factor by design, see [Decisions and roadmap](<#decisions-and-roadmap>) |
| NIST SP 800-63B AAL2 | Reauthentication at most every 24 hours | We chose longer sessions, see [Decisions and roadmap](<#decisions-and-roadmap>) |
| IEC 62443-4-2 CR 1.1 | Identify and authenticate human users on all interfaces | Our identity provider issues a unique identity per user, and every request to the console carries it |
| IEC 62443-4-2 CR 1.1 RE 1 (SL2) | Unique identification and authentication | Met. No shared accounts; each user is a separate identity |
| IEC 62443-4-2 CR 1.1 RE 2 (SL3) | Multi-factor authentication on all interfaces | An Enterprise capability. Enterprise companies enforce it through their own identity provider or ask UMH to enforce it. Community accounts use a one-time email code with risk-based challenges from our identity provider |

## Authorization

Relevant standards: NIST SP 800-53 AC-3 and AC-6, IEC 62443-4-2 CR 2.1, OWASP Authorization Cheat Sheet.

| Standard | Requirement | How it is met |
| -- | -- | -- |
| OWASP Authorization | Least privilege, deny by default, validate permissions on every request | Access is granted per location and inherits downward from there. Nothing is granted outside a location you were given. Every request is checked server-side |
| NIST SP 800-53 AC-3 | Enforce approved authorizations for logical access | The console enforces permissions server-side on every request. Permission data is cached for up to 10 minutes by design, see [Decisions and roadmap](<#decisions-and-roadmap>) |
| NIST SP 800-53 AC-6 | Least privilege | Viewer, Editor and Admin, granted at the narrowest location that works |
| IEC 62443-4-2 CR 2.1 RE 1 (SL2) | Authorization enforcement for all users | Met. The console enforces per user on every request |

## Sessions

Relevant standards: OWASP Session Management Cheat Sheet, NIST SP 800-63B reauthentication.

| Standard | Requirement | How it is met |  |
| -- | -- | -- | -- |
| OWASP Session | Secure cookies, server-side sign-out, absolute and idle timeouts | Sessions are held in cookies JavaScript cannot read, sent only over HTTPS, and invalidated server-side on sign-out. Session length is a deliberate choice, see [Decisions and roadmap](<#decisions-and-roadmap>) |  |
| NIST SP 800-63B AAL2 | Idle timeout of 1 hour or less | We chose no idle timeout, see [Decisions and roadmap](<#decisions-and-roadmap>) |  |

Our identity provider authenticates. Everything after that, including the session rules above, is managed by the Management Console itself. See [Sessions](<../authentication/sessions.md>).

## Audit

Relevant standards: NIST SP 800-53 AU-2 and AU-12, IEC 62443-4-2 CR 2.8, OWASP Authorization Cheat Sheet (logging).

| Standard | Requirement | How it is met |
| -- | -- | -- |
| IEC 62443-4-2 CR 2.8 | Auditable events for access control and account management | The console records sign-in, sign-up, session invalidation, invitations created, deleted and redeemed, users removed, instances registered, updated and deleted, and permission grant changes |
| NIST SP 800-53 AU-12 | Audit record generation, readable by authorized users | Company owners and Admins read the log under User Management and export it as CSV. Records are kept; nothing deletes them |

## Decisions and roadmap

Some requirements in the tables above are met differently from the letter of the standard. Each is either a choice we made on purpose, or work that is planned. Both are listed here so a reviewer sees them in one place.

### By design

| Standard | Requirement | What we do, and why |
| -- | -- | -- |
| NIST SP 800-63B AAL2 | Two factors at every sign-in | Community accounts sign in with a one-time email code, and our identity provider adds risk-based challenges when a sign-in looks unusual. Enforced multi-factor is part of the Enterprise plan, where it follows your own identity provider's policy or is switched on by UMH for you |
| NIST SP 800-63B AAL2, OWASP Session | Reauthentication within 24 hours, idle timeout of 1 hour or less | A session lasts 14 days from its last renewal and 30 days at most. In return, each account holds exactly one session: signing in elsewhere ends the previous one. Sign out when you leave your workstation. See [Sessions](<../authentication/sessions.md>) |
| NIST SP 800-53 AC-2(3) | Disable inactive accounts automatically | Last sign-in is recorded and visible to you. Whether an account is dormant is your call, not a timer's: removing people who leave is your side of the [Shared Responsibility](<shared-responsibility.md>) split |
| NIST SP 800-53 AC-3 | Authorization changes take effect at once | The browser checks for role changes every 10 seconds. A change applies at the next check, or at the next sign-in if the user is signed out. Instances follow within one minute. A removed user is signed out at their next CRUD action. This approach ensures the secret that defines the user's access never leaves their device. |
| IEC 62443-4-2 CR 1.1 RE 2 (SL3) | Multi-factor on all interfaces | Available to Enterprise companies, see [Authentication](<#authentication>). Our target is SL2, see [Target security level](<#target-security-level>) |

### On the roadmap

| Standard | Requirement | Where it stands |
| -- | -- | -- |
| Audit | Retention you control, forwarding to a SIEM | Records are kept indefinitely today and exported as CSV. Retention settings and SIEM forwarding are planned, see [Shared Responsibility](<shared-responsibility.md>) |

## Target security level

The Management Console targets **IEC 62443-4-2 Security Level 2** as a software application component. SL2 covers intentional attacks using simple means, by attackers with generic skills, low motivation and few resources, which is the profile that fits standard manufacturing operations. The console meets CR 1.1 RE 1 and CR 2.1 RE 1, the SL2 enhancements in the two areas above.

For critical infrastructure that has to meet SL3 or above, talk to UMH about what is possible.

## References

* [OWASP Authentication Cheat Sheet](<https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html>)
* [OWASP Authorization Cheat Sheet](<https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html>)
* [OWASP Session Management Cheat Sheet](<https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html>)
* [NIST SP 800-63B Digital Identity Guidelines, Revision 4](<https://pages.nist.gov/800-63-4/sp800-63b.html>)
* [NIST SP 800-53 Revision 5](<https://csrc.nist.gov/pubs/sp/800/53/r5/upd1/final>)
* [ISA/IEC 62443 Series of Standards](<https://www.isa.org/standards-and-publications/isa-standards/isa-iec-62443-series-of-standards>)
* [IEC 62443-4-2:2019](<https://webstore.iec.ch/en/publication/34421>)
