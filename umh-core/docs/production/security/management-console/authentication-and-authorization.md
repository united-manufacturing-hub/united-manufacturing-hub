# ManagementConsole Authentication and Authorization

For authorization and authentication, we use a 2-layer solution:

- **Layer 1**: Identity and company access (session token) - who you are and which company's instances you can access
- **Layer 2**: Permissions (permission grants) - what you can do within that company

This separation provides clear boundaries in our distributed, multi-tenant system. Layer 1 ensures users can only communicate with instances within their company, while Layer 2 provides fine-grained authorization within that company.

---

## How It Works

### Layer 1: Identity and Company Access

**Applicable Standards**: NIST SP 800-63B (Digital Identity Guidelines), IEC 62443-4-2 CR 1.1 (Human User Identification and Authentication), OWASP Authentication Cheat Sheet

Layer 1 handles authentication - proving who you are and which company you belong to. Both users and UMH instances authenticate at this layer.

#### User Authentication

Users authenticate to verify their identity and their access rights to a company. Both authentication methods result in a session token that identifies you and your company:

**Auth0**: Modern authentication that simplifies login and enables integration with enterprise systems such as SAML:

- Each email can be assigned to multiple companies (configured in Auth0)
- Default login uses a one-time password sent via email
- Enterprise customers can customize the login experience:
  - Integration with company SAML or SSO
  - Additional multi-factor authentication methods
- The user is redirected to Auth0 to complete the authentication process

**Legacy**: Email and password authentication (deprecated):

- Each email can only be assigned to one company
- Email addresses are not validated for existence
- Multi-factor authentication is not available
- Password complexity requirements apply (minimum 12 characters, at least one uppercase letter, one digit, and one symbol)

Once logged in, the user remains authenticated until the session token expires.

#### Security Controls

Authentication security is provided by Auth0, which implements:

- **Brute force protection**: Automatic rate limiting and account lockout after failed attempts
- **Credential stuffing detection**: Anomaly detection for automated attacks
- **Session management**: Configurable idle and absolute timeout policies
- **Adaptive MFA**: Risk-based authentication challenges

For enterprise customers with custom Auth0 tenants, additional controls can be configured through the Auth0 dashboard. See [Auth0 Security Documentation](https://auth0.com/docs/secure) for details.

#### Instance Authentication

A UMH instance authenticates using an AUTH_TOKEN that is generated during the initial setup process by the user who creates the instance.

The AUTH_TOKEN is a cryptographically secure random token displayed once during instance creation. The user must copy this token and configure it in the UMH instance.

**Security Note**: The user who creates an instance has one-time visibility of the AUTH_TOKEN. For secure AUTH_TOKEN storage and rotation procedures at the instance level, see [umh-core deployment security](../umh-core/deployment-security.md#auth_token-in-environment-variable).

The AUTH_TOKEN serves two purposes:

- **Layer 1 authentication**: A double-hash of the AUTH_TOKEN is stored in ManagementConsole for authentication. The instance sends the double-hash to prove its identity and receives a session token for subsequent communication.
- **Layer 2 credential encryption**: A single-hash of the AUTH_TOKEN is used as the credential encryption key. This allows the instance to decrypt its own credentials, while ManagementConsole (which only has the double-hash) cannot.

### Layer 2: Permissions Within Your Company

**Applicable Standards**: NIST SP 800-53 AC-3 (Access Enforcement), NIST SP 800-53 AC-6 (Least Privilege), IEC 62443-4-2 CR 2.1 (Authorization Enforcement), OWASP Authorization Cheat Sheet

Layer 2 uses a hierarchical permission system where each user and instance has a defined role at specific locations within a company.

#### Account Owner

The first user who creates a company becomes the **Account Owner**. This role has special significance:

- The Account Owner cannot be changed or transferred
- Has Admin access to all locations within the company
- Can perform all administrative actions including user invitation and instance creation

> **Design Trade-off: Account Owner Cannot Be Removed**
>
> The Account Owner has permanent Admin access and cannot be transferred or demoted. This ensures a guaranteed recovery path if other admins lose access.
>
> **Best practice:** Treat Account Owner as a break-glass account - use only for initial setup and emergency access. Enable MFA via Auth0, store credentials in a secure password manager, and create separate admin accounts for daily work.

**Why this design**: The Account Owner provides a guaranteed recovery path if other admins lose access or permissions become misconfigured. Since it cannot be removed or demoted, it ensures at least one account always has full control.

#### Locations

Locations represent positions in an organizational tree structure. This flexible path format allows unlimited depth to match your actual organization. You can use ISA-95, KKS, or any organizational naming standard - level 0 (enterprise) is the only required level.

Location paths use the same dot-separated format as [topic paths](../../usage/unified-namespace/topic-convention.md):

- `ACME` (enterprise only)
- `ACME.Munich` (enterprise.site)
- `ACME.Munich.Assembly` (enterprise.site.area)
- `ACME.Munich.Assembly.Line1` (enterprise.site.area.line)
- `ACME.Munich.Assembly.Line1.Cell5` (enterprise.site.area.line.workcell)
- can be extended up to more location levels than 5

#### Roles

Three roles control what actions users and instances can perform at their assigned locations:

| Role   | Capabilities                                                                |
| ------ | --------------------------------------------------------------------------- |
| Admin  | Full control including the ability to invite other users at their locations |
| Editor | Can create and modify resources but cannot manage users                     |
| Viewer | Read-only access                                                            |

Users can have different roles at different locations. For example, a user can be an Admin at `ACME.Munich.Assembly.Line1` but only a Viewer at `ACME.Munich.Assembly.Line2`.

**Security context**: These roles control ManagementConsole actions. At the instance level, anyone with write access (Editor or Admin) can deploy bridge configurations that have full access to all instance resources - see [umh-core deployment security](../umh-core/deployment-security.md) for details. The meaningful security boundary is between read-only (Viewer) and write access (Editor/Admin).

#### Permission Inheritance

Permissions are inherited downward: a user with access at `ACME.Munich` automatically has access to everything within Munich, such as `ACME.Munich.Assembly.Line1.Cell5`.

You can also define exceptions to override inherited permissions. For example, a user could have Viewer access at `ACME.Munich.Assembly` but Admin access specifically at `ACME.Munich.Assembly.Line1.Cell5`.

#### User and Instance Management

Only admins and the Account Owner can invite new users and add instances. When inviting users, admins can only grant permissions for locations where they themselves have admin access. This prevents privilege escalation and ensures that permissions flow naturally through the organization. Currently, every admin can create instances in every location. If they would create an instance that is outside of their location permission scope, they would not be able to modify it via the Management Console.

### Access Revocation

**Applicable Standards**: NIST SP 800-53 AC-2 (Account Management), OWASP Session Management Cheat Sheet

Access revocation happens at Layer 1 (session invalidation), not through permission grant expiration:

- **User removal**: When a user is removed from ManagementConsole, their session token is invalidated immediately - they can no longer authenticate
- **Permission updates**: Admins can update user permissions at any time. ManagementConsole validates the user's current permissions from the database on each request, so changes take effect quickly (within the cache window of up to 10 minutes)
- **Instance removal**: Removing an instance from ManagementConsole denies all further communication as authentication in layer 1 fails

> **Design Trade-off: Permission Updates Require Active Session**
>
> When an admin updates a user's permissions, the new permission certificate is automatically applied in the background by the user's frontend worker. However, this only happens while the user is logged in. If the user is offline, they retain their original privileges until their next login.
>
> **Best practice:** To immediately revoke access or demote a user, **remove them entirely** - this invalidates their session without requiring them to be online. You can then re-invite them with the correct permissions.

**What happens when users leave**: When you remove a user, they immediately lose access (session invalidated). Resources they created and users they invited remain - permission grants represent organizational decisions, not personal relationships.

### Session Management

**Applicable Standards**: OWASP Session Management Cheat Sheet, NIST SP 800-63B (Session Binding)

ManagementConsole manages user sessions independently from Auth0 using JWT cookies signed with `JWT_SECRET_KEY`.

**Session Lifecycle**:

- **Creation**: Session token issued after successful Auth0 authentication
- **Storage**: HTTPOnly cookie with Secure and SameSite=Strict attributes
- **Token Validity**: Each JWT token is valid for **14 days**
- **Sliding Window Refresh**: When a token is within **7 days of expiring** and the user is active (e.g., page load), a fresh 14-day token is issued automatically
- **Absolute Session Limit**: Regardless of activity, users must re-authenticate after **30 days** from their original login
- **Termination**: Explicit logout, absolute timeout, or user removal from company

**Session Policies**:

- **Concurrent sessions**: Multiple sessions from different devices are permitted but may lead to unreliable connections. For devices requiring simultaneous access, create separate user accounts
- **Token refresh**: Session extends automatically on API activity when within the last 7 days of the 14-day token validity
- **Cross-device**: Each device maintains its own independent session

> **Current Limitation: No Idle Timeout**
>
> There is no automatic logout after periods of inactivity. A session remains valid as long as:
>
> 1. The token hasn't expired (14 days without activity), AND
> 2. The 30-day absolute limit hasn't been reached
>
> **Best practice:** Log out when leaving your workstation. For environments requiring shorter session lifetimes, consider enterprise SSO with your own timeout policies.

**Forced Logout**: Currently, individual session termination requires user removal and re-invitation. Bulk session revocation for specific users is planned for a future release.

---

## Threat Model

ManagementConsole's authentication system is designed to protect against specific threat actors while accepting certain risks as design trade-offs.

### Trust Boundaries

The authentication system operates across three trust boundaries:

1. **User Browser ↔ ManagementConsole**: TLS-encrypted connections with session tokens stored as HTTPOnly cookies (stateless authentication)
2. **ManagementConsole ↔ Auth0**: OAuth 2.0 flow for user authentication, with Auth0 handling credential storage and MFA
3. **ManagementConsole ↔ umh-core Instances**: AUTH_TOKEN-based authentication for instance identity, with permission certificates for authorization

### Threat Actors and Protection

| Threat Actor                 | Capabilities                                            | Protection Level                                                                                            |
| ---------------------------- | ------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| **External Attacker**        | Credential stuffing, phishing, brute force              | **Protected** - Auth0 provides rate limiting, anomaly detection, MFA                                        |
| **Compromised User**         | Access to company data at their permission level        | **Protected** - Session timeout, per-request permission validation, removal invalidates session immediately |
| **Malicious Insider**        | Permission escalation attempts, invite key interception | **Partially Protected** - Backend enforces admin-location rules; two-channel invite delivery                |
| **Account Owner Compromise** | Complete company takeover                               | **Design trade-off** - See Account Owner section above                                                      |

### Attack Scenarios and Mitigations

**Credential Attacks**: Auth0 protects against credential stuffing and brute force through automatic rate limiting and anomaly detection. For enterprise customers, MFA can be enforced for all users.

**Session Hijacking**: Session tokens are stored in HTTPOnly cookies (not accessible via JavaScript), preventing XSS-based token theft. All connections use TLS encryption.

**Cross-Company Access**: Each company is isolated through Auth0 organizations. Users authenticate to specific companies and cannot access data from other organizations.

**Privilege Escalation**: Admins can only grant permissions for locations where they have admin access. The backend validates all permission operations, preventing UI-based manipulation.

**Phishing Attacks**: The two-channel invite system (email link + separate invite key) prevents email-only attacks. Users must have both pieces to join a company.

### Out of Scope

ManagementConsole authentication does NOT protect against:

- **Compromised umh-core instances**: Instance-level security is covered in [umh-core deployment security](../umh-core/deployment-security.md#threat-model-simplified)
- **Physical access to user devices**: Users are responsible for device security
- **Auth0 infrastructure compromise**: Auth0 is responsible for their platform security (see Shared Responsibility Model)

---

## Shared Responsibility Model

Security is a shared responsibility between UMH and our customers. This section clarifies who is responsible for what. For instance-level security responsibilities, see [umh-core deployment security - Shared Responsibility Model](../umh-core/deployment-security.md#shared-responsibility-model).

### We (UMH) Are Responsible For

| Area                          | Our Responsibility                                                           |
| ----------------------------- | ---------------------------------------------------------------------------- |
| Authentication Infrastructure | Auth0 integration, session token generation, secure credential hashing       |
| Authorization Framework       | RBAC system, permission inheritance, location-based access control           |
| Secure Defaults               | Password complexity requirements, invite key separation, double-hash storage |
| Platform Security             | ManagementConsole application security, API security, session management     |
| Audit Logging                 | Recording authentication events and permission changes (see note below)      |

> **Known Limitation: Audit Logging**
>
> Comprehensive audit logging with user-accessible logs, configurable retention, and SIEM integration is planned but not yet fully implemented. Currently, authentication events are logged internally but not exposed through a user interface. Enterprise customers requiring detailed audit trails should contact UMH support to discuss available options.

### You (Customer) Are Responsible For

| Area                    | Your Responsibility                                           |
| ----------------------- | ------------------------------------------------------------- |
| Account Owner Security  | Protecting the Account Owner credentials (recovery account)   |
| AUTH_TOKEN Management   | Secure storage and transmission of instance AUTH_TOKENs       |
| Invite Key Distribution | Sharing invite keys through secure out-of-band channels       |
| User Lifecycle          | Promptly removing users who leave your organization           |
| Access Reviews          | Periodic review of user permissions and access levels         |
| Enterprise SSO          | Configuration and security of your SAML/SSO identity provider |

### Shared Responsibilities

| Area              | Details                                                                             |
| ----------------- | ----------------------------------------------------------------------------------- |
| Permission Design | UMH provides the RBAC framework; you define appropriate roles per location          |
| Incident Response | UMH monitors platform; you monitor for compromised credentials                      |
| Compliance        | UMH provides security controls; you ensure usage meets your compliance requirements |

---

## Compliance Alignment

This section maps ManagementConsole security controls to industry standards.

### Authentication Standards

| Standard              | Requirement                                            | Implementation                             |
| --------------------- | ------------------------------------------------------ | ------------------------------------------ |
| OWASP Authentication  | MFA, brute force protection, secure session management | Auth0 provides all authentication controls |
| NIST SP 800-63B AAL2  | Multi-factor authentication, session timeout ≤24hr     | Auth0 MFA + configurable session lifetime  |
| IEC 62443 SR 1.1      | Human user identification and authentication           | Auth0 unique user IDs + authentication     |
| IEC 62443 SR 1.1 RE 2 | MFA for untrusted networks (SL2+)                      | Auth0 MFA enforced for all external access |

### Authorization Standards

| Standard            | Requirement                             | Implementation                                     |
| ------------------- | --------------------------------------- | -------------------------------------------------- |
| OWASP Authorization | Least privilege, deny by default, RBAC  | Platform implements role-based access control      |
| NIST SP 800-53 AC-3 | Access enforcement at all access points | ManagementConsole enforces via database lookups    |
| NIST SP 800-53 AC-6 | Least privilege                         | Viewer/Editor/Admin roles with minimal permissions |
| IEC 62443 SR 2.1    | Authorization enforcement for all users | Platform validates permissions on each request     |

### Session Management

| Standard        | Requirement                            | Implementation                                                               |
| --------------- | -------------------------------------- | ---------------------------------------------------------------------------- |
| OWASP Session   | Token rotation, secure cookies, logout | Backend issues HTTPOnly JWT cookies (14-day sliding window, 30-day absolute) |
| NIST SP 800-63B | Idle timeout ≤1hr (AAL2)               | Not yet implemented (30-day absolute timeout only)                           |

**Note**: Auth0 handles authentication only. ManagementConsole backend manages sessions independently using its own JWT cookies signed with `JWT_SECRET_KEY`.

### Accepted Limitations

| Standard             | Requirement                     | Current Status                                 |
| -------------------- | ------------------------------- | ---------------------------------------------- |
| NIST SP 800-63B AAL2 | Idle timeout ≤1hr               | Not implemented (30-day absolute timeout only) |
| NIST AC-2(3)         | Disable dormant accounts        | Not implemented (manual process)               |
| IEC 62443 SL3        | MFA for all interfaces          | MFA for external only (SL2 compliant)          |
| OWASP                | Immediate permission revocation | Permission updates require user acceptance     |

### Target Security Level

ManagementConsole targets **IEC 62443 Security Level 2 (SL2)**, appropriate for:

- Protection against intentional violation using simple means
- Cybercrime-level threat actors with generic skills
- Standard manufacturing and industrial operations

For critical infrastructure requiring SL3+, contact UMH for enterprise security options.

---

### References

- [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)
- [OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)
- [NIST SP 800-63B Digital Identity Guidelines](https://pages.nist.gov/800-63-4/sp800-63b.html)
- [NIST SP 800-53 Access Control](https://csrc.nist.gov/pubs/sp/800/53/r5/upd1/final)
- [ISA/IEC 62443 Series of Standards](https://www.isa.org/standards-and-publications/isa-standards/isa-iec-62443-series-of-standards)

---

## Reference

### User Invitation Process

When an admin invites a new user:

1. **Admin specifies**: Email address, role, and location permissions
2. **System generates**: Invite link + separate invite key (shown only to the admin)
3. **Auth0 sends**: Automatic invitation email to the user
4. **Admin shares**: The invite key through a separate secure channel
5. **User accepts**: Clicks link, authenticates with Auth0, enters invite key

The invite key can only be used once and enables secure key exchange without the backend ever seeing the user's private credentials.

**Why two pieces?** The invite link proves email ownership (via Auth0). The invite key, shared separately, ensures the inviting admin intended this specific person to receive access. This prevents email forwarding attacks.

### Deployment Considerations

The permission infrastructure exists and controls what users see in ManagementConsole, but UMH Core does not yet validate individual user permissions when executing commands. Similarly, users accept messages from all instances within their company without per-instance verification. This means that within a company, all authenticated users can execute all actions on UMH Core instances, regardless of their assigned role or location permissions.

The permission system provides fine-grained authorization that is ready for enforcement once validation is implemented.

### Auth0 Organization Linking

Each company can be linked to an Auth0 organization, enabling:

- Single sign-on through your corporate identity provider
- Centralized user management in Auth0
- Multi-company access with one email (each company links to its own Auth0 org)

The link is established during company setup and validated during every user onboarding - users can only join companies that match their Auth0 organization.
