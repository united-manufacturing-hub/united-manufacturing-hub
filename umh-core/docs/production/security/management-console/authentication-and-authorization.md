# UMH Authentication and Permission System

For authorization and authentication, we use a 2-layer solution:
- **Layer 1**: Identity and company access (session token) - who you are and which company's instances you can access
- **Layer 2**: Permissions (permission grants) - what you can do within that company

This separation provides clear boundaries in our distributed, multi-tenant system. Layer 1 ensures users can only communicate with instances within their company, while Layer 2 provides fine-grained authorization within that company.

## How It Works

### Layer 1: Identity and Company Access

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

#### Instance Authentication

A UMH instance authenticates using an AUTH_TOKEN that is generated during the initial setup process by the user who creates the instance.

The AUTH_TOKEN is a cryptographically secure random token displayed once during instance creation. The user must copy this token and configure it in the UMH instance.

**Security Note**: The admin who creates an instance has one-time visibility of the AUTH_TOKEN. This follows standard shared-secret patterns - treat the token like a password and share it securely with whoever configures the instance.

The AUTH_TOKEN serves two purposes:

- **Layer 1 authentication**: A double-hash of the AUTH_TOKEN is stored in ManagementConsole for authentication. The instance sends the double-hash to prove its identity and receives a session token for subsequent communication.
- **Layer 2 credential encryption**: A single-hash of the AUTH_TOKEN is used as the credential encryption key. This allows the instance to decrypt its own credentials, while ManagementConsole (which only has the double-hash) cannot.

### Layer 2: Permissions Within Your Company

Layer 2 uses a hierarchical permission system where each user and instance has a defined role at specific locations within a company.

#### Account Owner

The first user who creates a company becomes the **Account Owner**. This role has special significance:

- The Account Owner cannot be changed or transferred
- Has Admin access to all locations within the company
- Can perform all administrative actions including user invitation and instance creation
- In production environments, treat the Account Owner as a "break-glass" account - use it only for initial setup and emergency access

**Why this design**: The Account Owner provides a guaranteed recovery path if other admins lose access or permissions become misconfigured. Since it cannot be removed or demoted, it ensures at least one account always has full control.

#### Locations

Locations represent positions in an organizational tree structure. This flexible path format allows unlimited depth to match your actual organization. You can use ISA-95, KKS, or any organizational naming standard - level 0 (enterprise) is the only required level.

Location paths use the same dot-separated format as [topic paths](../../usage/unified-namespace/topic-convention.md):
- `ACME` (enterprise only)
- `ACME.Munich` (enterprise.site)
- `ACME.Munich.Assembly` (enterprise.site.area)
- `ACME.Munich.Assembly.Line1` (enterprise.site.area.line)
- `ACME.Munich.Assembly.Line1.Cell5` (full path)

#### Roles

Three roles control what actions users and instances can perform at their assigned locations:

| Role | Capabilities |
|------|-------------|
| Admin | Full control including the ability to invite other users at their locations |
| Editor | Can create and modify resources but cannot manage users |
| Viewer | Read-only access |

Users can have different roles at different locations. For example, a user can be an Admin at `ACME.Munich.Assembly.Line1` but only a Viewer at `ACME.Munich.Assembly.Line2`.

#### Permission Inheritance

Permissions are inherited downward: a user with access at `ACME.Munich` automatically has access to everything within Munich, such as `ACME.Munich.Assembly.Line1.Cell5`.

You can also define exceptions to override inherited permissions. For example, a user could have Viewer access at `ACME.Munich.Assembly` but Admin access specifically at `ACME.Munich.Assembly.Line1.Cell5`.

#### User and Instance Management

Only admins and the Account Owner can invite new users and add instances. When inviting users, admins can only grant permissions for locations where they themselves have admin access. This prevents privilege escalation and ensures that permissions flow naturally through the organization.

### Permission Grant Lifecycle

Permission grants are created during invitation (for users) or instance registration (for instances).

**Deployment Consideration**: Permission grants are valid indefinitely and are NOT revoked through expiration mechanisms.

**Why this design**: This simplifies the permission model by separating identity from access control:
- Permission grants prove identity and the permissions that were granted at creation time
- Layer 1 (session token/backend) controls current access rights
- Access denial happens by removing the user or instance, not by expiring permission grants

**Compensating controls**:

1. **Layer 1 enforcement**: When a user is removed from ManagementConsole, their session token is invalidated and they can no longer authenticate
2. **Permission updates**: Admins can update user permissions, which creates a new permission grant
3. **Instance removal**: Removing an instance from ManagementConsole denies all further communication

**Security implication**: If credentials are compromised, access denial requires action in Layer 1 (user removal or permission update through ManagementConsole). The permission grant alone does not grant access - a valid session token is also required.

**What happens when users leave**: When a user is removed from ManagementConsole, their session token is invalidated and they lose access. However, resources they created and users they invited remain in the system. Invited users retain their permissions (which came from the admin who invited them, not the removed user). This is intentional - permission grants represent organizational decisions, not personal relationships.

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

The permission infrastructure exists and controls what users see in ManagementConsole, but UMH Core does not yet validate individual user permissions when executing commands. This means that within a company, all authenticated users can execute all actions on UMH Core instances, regardless of their assigned role or location permissions.

The permission system provides fine-grained authorization that is ready for enforcement once validation is implemented in UMH Core.

### Auth0 Organization Linking

Each company can be linked to an Auth0 organization, enabling:
- Single sign-on through your corporate identity provider
- Centralized user management in Auth0
- Multi-company access with one email (each company links to its own Auth0 org)

The link is established during company setup and validated during every user onboarding - users can only join companies that match their Auth0 organization.
