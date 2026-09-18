# Threat Model

The Management Console's authentication and authorization is built to keep specific attackers out. Some risks are accepted deliberately, and this page says which.

## Trust boundaries

| Boundary | How it is protected |
| --- | --- |
| Your browser and the Management Console | TLS, with the session held in a cookie that JavaScript cannot read |
| The Management Console and our identity provider | OAuth 2.0. The identity provider stores credentials and runs multi-factor challenges |
| The Management Console and your instances | The instance authenticates with its `AUTH_TOKEN`. What a user may do through the console is decided by the permissions the console holds for them |

## Threat actors

| Attacker | What they can attempt | Where you stand |
| --- | --- | --- |
| Outside attacker | Credential stuffing, phishing, brute force | **Protected.** Our identity provider rate-limits, detects anomalies and can enforce multi-factor |
| A user whose account is taken over | Everything that user could do in the console, and everything any member can do on an instance | **Partly protected.** Permissions are checked per request in the console and removing the user ends the session immediately. Instances do not enforce per-user permissions, see [What roles do not cover](../users-and-permissions/README.md#what-roles-do-not-cover) |
| A user acting in bad faith from inside | Escalating their own permissions, intercepting an invite | **Partly protected.** An admin can only grant permissions where they are already an admin, and an invitation needs both the link and the separately delivered key |
| Whoever holds the Account Owner, where a company has one | Taking over the company | **Accepted risk.** The account cannot be demoted, which is what makes it a recovery path. See [Emergency Access](../users-and-permissions/fallback-access.md) |

## Attack scenarios

| Attack | What stops it |
| --- | --- |
| Guessing or replaying credentials | Our identity provider rate-limits sign-in and detects anomalies. Enterprise customers can require multi-factor for everyone |
| Stealing a session | The session cookie is not readable by JavaScript, so a cross-site scripting bug does not hand over a session. All traffic runs over TLS |
| Reaching another company's data | A sign-in belongs to one company at a time, and a company linked to an organization with our identity provider can only be joined through that organization |
| Escalating privileges in the console | Permission changes are validated in the backend, so editing the request in the browser changes nothing. An admin can only grant what they hold at that location |
| Phishing an invitation | An invitation needs the link and a key sent through a different channel, so a forwarded email is not enough |

## Not covered here

- A compromised instance, because the instance runs on your hardware. See [Threat Model (Simplified)](../../production/security/umh-core/deployment-security.md#threat-model-simplified).
- A compromised user device. Device security is yours.
- The identity provider's own infrastructure, which the provider is responsible for. See [Shared Responsibility](shared-responsibility.md).
