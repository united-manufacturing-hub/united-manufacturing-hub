# UMH Auth and Certificate System Overview

For authorization and authentication, we use a 2-layer solution. This separation-of-concerns might be untypical but in the context of our distributed, multi-tenant system it provides clear boundaries:

- **Layer 1**: Users and UMH Instances authenticate here either against the backend or a third party auth provider such as SAML, SSO or social login. Each user and each instance belongs to a company. This layer ensures that users can only send and receive messages to/from instances within their company.

- **Layer 2**: In addition to the cross-company protection of Layer 1, this layer provides fine-grained authorization. Each user and UMH Instance verifies incoming messages before interpreting them, ensuring that the message actually comes from the sender and that the sender has the needed permissions to execute the requested action. This verification happens locally in the UMH Instance. The ManagementConsole backend is restricted to Layer 1 and does not read or execute user/instance messages.

**Deployment Considerations**: Layer 2 permission validation is currently implemented in UMH Classic but not yet in UMH Core. Within a company, all users can execute all actions on UMH Core instances. The certificate infrastructure exists and controls what users see in ManagementConsole, but UMH Core does not yet validate individual user permissions when executing commands.

## User Authentication

A user needs to authenticate against a company to verify their identity and their access rights to that company.

There are two methods to authenticate a user:

### Legacy Authentication (Deprecated)

The legacy method uses email and password for login. This method is deprecated and will be replaced by Auth0.

- Each email can only be assigned to one company
- Email addresses are not validated for existence
- Multi-factor authentication is not available
- Password complexity requirements apply (minimum 12 characters, at least one uppercase letter, one digit, and one symbol)

### Auth0 Authentication

Auth0 simplifies authentication and enables integration with enterprise systems such as SAML.

- Each email can be assigned to more than one company (configured in Auth0)
- Default login uses a one-time password sent via email
- Enterprise customers can customize the login experience:
  - Integration with company SAML or SSO
  - Additional multi-factor authentication methods
- The user is redirected to Auth0 to complete the authentication process

Once logged in, the user remains authenticated until the JWT token expires.

## Instance Authentication

A UMH Instance authenticates using an AUTH_TOKEN that is generated during the initial setup process by the user who creates the instance.

### AUTH_TOKEN

The AUTH_TOKEN is a cryptographically secure random token displayed once during instance creation. The user must copy this token and configure it in the UMH Instance.

The AUTH_TOKEN serves two purposes:

- **Layer 1**: A double-hash of the AUTH_TOKEN is stored in ManagementConsole for authentication. The instance sends the double-hash to prove its identity.
- **Layer 2**: A single-hash of the AUTH_TOKEN is used to encrypt the instance's certificate private key. This allows the instance to decrypt its own private key, while ManagementConsole (which only has the double-hash) cannot.

When a UMH Instance starts, it uses the AUTH_TOKEN to authenticate against ManagementConsole and receives a JWT token for subsequent communication.