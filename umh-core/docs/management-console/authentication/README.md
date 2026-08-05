# Authentication

Access to the Management Console works in two layers:

- **Identity and company access.** Who you are, and whose instances you are allowed to reach. This page covers that layer.
- **Permissions.** What you may do once you are inside a company. See [Users and Permissions](../users-and-permissions/README.md).

## How you sign in

| Method | Who it applies to |
| --- | --- |
| A one-time code sent to your email address | The default where your company has configured nothing else |
| Username and password | Accounts that were set up with a password |
| Single sign-on through your company's own identity provider | Enterprise, as an add-on that is paid for separately |

The sign-in screen also offers **Continue with Google** and **Continue with LinkedIn**, which sign you in with the address held by that provider.

Which of these you are offered depends on the connections enabled for your company, see below. Once your company has a default connection, everyone on your team signs in through it.

## Which company you sign in to

Companies are separated at sign-in, and one email address can belong to more than one company.

A company can be linked to its own organization in Auth0, the identity platform we use. Where that link exists, you can only join the company through that organization, which is checked when your account is created and every time you accept an invitation. A company that is not linked yet, which happens for a while after an upgrade from Community, is not separated this way.

An organization accepts sign-ins through one or more **connections**. A connection is an identity source: your corporate SAML or OIDC provider, a social login, or an emailed one-time code.

On an Enterprise plan you pick a **default connection** for your team. Everyone signs in through it, and invitations go through it too. Point it at your own identity provider and your usual single sign-on, multi-factor and offboarding rules decide who gets in. Pointing a default connection at your own identity provider is the single sign-on add-on, which is paid for on top of the Enterprise plan. Contact your account executive to enable it.

UMH staff are the one exception. You do not create accounts for them in your identity provider, their invitations use UMH's own single sign-on instead. See [Fallback Access](../users-and-permissions/fallback-access.md).

## What Auth0 protects against

Sign-in itself is handled by Auth0, which provides:

- Rate limiting and account lockout after repeated failed attempts
- Detection of automated credential stuffing
- Risk-based multi-factor challenges. To require multi-factor from every user, configure it in your own identity provider and connect it with single sign-on
- Storage of passwords, for the accounts that use one

Enterprise customers with their own Auth0 tenant can configure further controls in the Auth0 dashboard. See the [Auth0 security documentation](https://auth0.com/docs/secure).

## How instances sign in

A UMH Core instance authenticates with an `AUTH_TOKEN`, not with a user account. The console generates the token while you create the instance and shows it once. Copy it then and configure it on the instance.

The token does two things:

- It proves the instance's identity, so the console accepts its status and hands it the configuration meant for it.
- It encrypts the credentials the instance stores, such as PLC passwords. The key is derived from the token, so the instance can decrypt its own credentials and the console cannot.

Keep the token somewhere you can find it again, such as your password manager. For storage and rotation on the instance, see [AUTH_TOKEN in Environment Variable](../../production/security/umh-core/deployment-security.md#auth_token-in-environment-variable).

## Sessions

Your session is managed by the Management Console itself, independently of Auth0.

| Session rule | Value |
| --- | --- |
| Session length | 14 days |
| Renewal | While you are active and within 7 days of expiry, the session is extended by another 14 days |
| Hard limit | 30 days after you first signed in, you sign in again regardless of activity |
| Ends on | Sign-out, the 30 day limit, or removal from the company |

Each device keeps its own session. Signing in from several devices at once is allowed, but the connection to your instances can become unreliable when you do. Devices that need to be connected at the same time should get their own accounts.

{% hint style="warning" %}
There is no automatic sign-out after a period of inactivity. A session stays valid until the 14 day token expires or the 30 day limit is reached, whichever comes first. Sign out when you leave your workstation. If you need shorter sessions than that, use single sign-on and set the policy in your own identity provider.
{% endhint %}

## Removing access

| What you do | What happens |
| --- | --- |
| Remove a user | Their session ends immediately and they can no longer sign in |
| Change a user's permissions | Applied while they are signed in, within a few minutes. A user who is offline keeps their previous permissions until they sign in again |
| Remove an instance | The instance can no longer communicate with the console |

To take access away straight away, remove the user rather than lowering their role, then invite them again with the permissions you want. What they built stays: their bridges, data models and the users they invited belong to the company, not to them.

Sessions cannot be ended individually yet, so removing the user is the only way to end a session you do not control.
