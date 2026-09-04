# Shared Responsibility

Securing the account layer is split between UMH and you. The instance has its own split, see [Shared Responsibility Model](../../production/security/umh-core/deployment-security.md#shared-responsibility-model).

## Ours

| Area | What we do |
| --- | --- |
| Authentication | Run the identity-provider integration, issue sessions, store instance credentials so that we cannot read them |
| Authorization | Provide the role system, permission inheritance and location-based access |
| Secure defaults | The password policy we configure with our identity provider, invitations that need two separate pieces, credential storage the console itself cannot decrypt |
| Platform | Security of the Management Console application and its API |
| Audit logging | Record sign-ins and permission changes |
| UMH staff accounts | Identity, authentication and offboarding of the `@umh.app` accounts you invite. UMH staff reach your company only through an invitation you issue |

{% hint style="info" %}
Audit logging is recorded but not yet available for self-service. Logs with retention settings you control, and forwarding into a SIEM, are planned and not built. Get in touch with your Account Executive if you need an audit trail.
{% endhint %}

## Yours

| Area | What you do |
| --- | --- |
| The Account Owner | Protect the account that cannot be demoted, see [Emergency Access](../users-and-permissions/fallback-access.md) |
| Instance tokens | Store and transmit each instance's `AUTH_TOKEN` safely |
| Invitations | Send invite keys through a channel other than the invitation email |
| Joiners and leavers | Remove people when they leave your organization |
| Access reviews | Check periodically who holds what, and where |
| Your identity provider | Configure and secure the single sign-on you connect |
| UMH staff | Decide which UMH people to invite, and remove them when the work is done |

## Both

| Area | How it splits |
| --- | --- |
| Permission design | We provide the role system, you decide who holds which role at which location |
| Incident response | We watch the platform, you watch for misuse of your own credentials |
| Compliance | We provide the controls, you decide whether your use of them satisfies your obligations |
