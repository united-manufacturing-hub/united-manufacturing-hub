# Authentication

Access to the Management Console works in two layers:

- **Identity and company access.** Who you are, and whose instances you are allowed to reach. This page covers that layer.
- **Permissions.** What you may do once you are inside a company. See [Users and Permissions](../users-and-permissions/README.md).

## How you sign in

Sign-in runs through our IdP rather than the Management Console itself. The provider stores any passwords, verifies each method below, and adds the protections in [What the identity provider protects against](#what-the-identity-provider-protects-against).

| Method | Who it applies to |
| --- | --- |
| A one-time code sent to your email address | The default for all Community users |
| Community username and password | Accounts that were set up with a password in the past |
| Enterprise Username and password | The default for Enterprise accounts without the SSO add-on|
| Single sign-on through your company's own identity provider | Enterprise, as an add-on that is paid for separately |

The sign-in screen also offers **Continue with Google** and **Continue with LinkedIn**, which sign you in with the address held by that provider.

As an Enterprise Plan customer, your account executive will help you get with started with the SSO package. By default, your account's login method will be username and password.

Depending on your account, the Management Console can ask you for two different secrets: a password and a passphrase. They do different jobs, and only the password can be reset. See [Passwords and Passphrases](passwords-and-passphrases.md).

## Which company you sign in to

Companies are separated at sign-in, and one email address can belong to more than one company.

A company can be linked to its own organization with our identity provider. Where that link exists, you can only join the company through that organization, which is checked when your account is created and every time you accept an invitation. A company that is not linked yet, which happens for a while after an upgrade from Community, is not separated this way.

An organization accepts sign-ins through one or more **connections**. A connection is an identity source: your corporate SAML or OIDC provider, a social login, or an emailed one-time code.

On an Enterprise plan you pick a **default connection** for your team. Everyone signs in through it, and invitations go through it too. Point it at your own identity provider and your usual single sign-on, multi-factor and offboarding rules decide who gets in. Pointing a default connection at your own identity provider is the single sign-on add-on, which is paid for on top of the Enterprise plan. Contact your account executive to enable it.

UMH staff are the one exception. You do not create accounts for them in your identity provider, their invitations use UMH's own single sign-on instead. See [Emergency Access](../users-and-permissions/fallback-access.md).

## What the identity provider protects against

Sign-in itself is handled by our identity provider, which provides:

- Rate limiting and account lockout after repeated failed attempts
- Detection of automated credential stuffing
- Risk-based multi-factor challenges. To require multi-factor from every user, configure it in your own identity provider and connect it with single sign-on
- Storage of passwords, for the accounts that use one

Enterprise customers who connect their own identity provider can configure further controls there, through their usual single sign-on, multi-factor and offboarding policies.
