# Threat Model

The Management Console defends against attackers who try to guess credentials, steal a session, reach another company's data, or grant themselves more permissions. Some risks are accepted, and some threats are outside what the Management Console can prevent.

Some measures are ours to run and some are yours. Each table below names which side a measure is on. [Shared Responsibility](shared-responsibility.md) has the full split. [Compliance](compliance.md) maps the measures to the standards they follow.

- [How each connection is secured](#how-each-connection-is-secured) covers encryption and authentication between the browser, the identity provider and your instances
- [Threat actors](#who-could-attack-the-management-console) lists who might attack and what they could attempt
- [Attack scenarios](#how-to-stop-or-prevent-common-attack-patterns) lists attack patterns and what stops them
- [Threats outside the Management Console](#threats-outside-the-management-console) lists what we cannot prevent

## How each connection is secured

The Management Console has three network connections: to your browser, to our identity provider, and to your instances. The table below shows how our network connections are secured.

| Connection | How it is secured |
| --- | --- |
| Your browser and the Management Console | Traffic is encrypted with TLS. After sign-in, the browser holds the session in a cookie that JavaScript cannot read |
| The Management Console and our identity provider | Sign-in uses OAuth 2.0. The identity provider stores your credentials and runs multi-factor challenges |
| The Management Console and your instances | The instance proves its identity with the `AUTH_TOKEN` environment variable set when its container starts, see [Access Control and Authentication](../../production/security/umh-core/deployment-security.md#access-control-and-authentication). |

## Who could attack the Management Console

Attackers can be various actors, from outside your company or from inside it. The table below shows what each of them could attempt, how far the Management Console protects you, and what you have to do yourself.

| Attacker | What they can attempt | Where you stand | What you do |
| --- | --- | --- | --- |
| Outside attacker | Credential stuffing, phishing, brute force | **Protected.** Sign-in runs through our identity provider, which rate-limits attempts and checks them for anomalies | Use a unique, strong password. With the [single sign-on](../authentication/enterprise-sso.md) add-on, your own identity provider can require multi-factor and password rotation for every user |
| A user whose account is taken over | Everything that user could do in the Management Console, including on the instances their company connects | **Partly protected.** The backend checks the user's permissions on every request, and removing the user ends their session at once. The instance itself does not check per-user permissions and trusts the Management Console's decision, see [Where these roles apply](../users-and-permissions/README.md#where-these-roles-apply) | Remove the user as soon as you suspect a takeover, see [Removing access](../users-and-permissions/managing-access.md#removing-access). Invite them again once the account is secured. Contact your account executive if you need an audit trail |
| A user acting in bad faith from inside | Granting themselves more permissions, intercepting an invitation | **Partly protected.** An admin can only grant permissions where they are already an admin. An invitation needs an invite key that the admin sends separately, see [Inviting a user](../users-and-permissions/managing-access.md#inviting-a-user) | Send invite keys through a channel other than the invitation email. Review roles and location access periodically, and remove people who leave your company |
| The Account Owner | Taking over the company | **Accepted risk.** The Account Owner cannot be demoted, so that one account keeps full control when every other admin is locked out. Not every company has one | Register the Account Owner on an address your company owns and monitors, not on an employee's personal account. Use it only when no other admin can act, see [Emergency Access](../users-and-permissions/fallback-access.md) |

## How to stop or prevent common attack patterns

Attackers can try different ways to get into an account or a company. The table below lists common attack patterns and what stops or prevents them.

| Attack | What stops it |
| --- | --- |
| Guessing or replaying credentials | Our identity provider rate-limits sign-in attempts and checks them for anomalies. Enterprise customers can require multi-factor for every user through their own identity provider, see [single sign-on](../authentication/enterprise-sso.md) |
| Stealing a session | JavaScript cannot read the session cookie, so a cross-site scripting bug cannot pass the session to an attacker. All traffic is encrypted with TLS. Optionally, see [One session per account](../authentication/sessions.md#one-session-per-account) for context on how we handle sessions. |
| Reaching another company's data | A sign-in belongs to one company at a time. A company linked to its own organization with our identity provider can only be joined through that organization, see [Which company you sign in to](../authentication/README.md#which-company-you-sign-in-to) |
| Granting yourself more permissions | Editing a request in the browser grants nothing, because the backend checks every grant against what the granting admin holds at that location |
| Phishing an invitation | An invitation is issued for one email address, and joining also needs the invite key that the admin sends through another channel. A forwarded invitation email alone is not enough |

## Threats outside the Management Console

The Management Console cannot prevent the following threats, because they affect hardware or infrastructure that we do not control.

### Compromised devices and instances

The Management Console cannot tell a compromised device from its real user. Whoever controls the device holds that user's access to your company.

The same applies to instances. An instance runs on hardware outside our control, so we cannot tell whether it has been compromised. See [Threat Model (Simplified)](../../production/security/umh-core/deployment-security.md#threat-model-simplified).

### Identity provider infrastructure

By default, you sign in through our identity provider using OAuth 2.0. We do not operate the identity provider's infrastructure, so a security incident there is outside what we can prevent.

Such an incident does not by itself grant access to your instances. Signing in also asks for your passphrase, where your account has one. The passphrase unlocks your access to your instances, see [Why every sign-in asks for the passphrase](../authentication/passwords-and-passphrases.md#why-every-sign-in-asks-for-the-passphrase).

With the [single sign-on add-on](../authentication/enterprise-sso.md), your users sign in through your own identity provider instead. Sign-in security then depends on your setup rather than ours, so monitor your identity provider for suspicious sign-ins. See [Shared Responsibility](shared-responsibility.md) for the split.
