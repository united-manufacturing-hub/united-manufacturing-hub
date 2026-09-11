# Management Console Security

Access to the Management Console is decided in two layers: identity and company access first, then permissions within that company. [Authentication](../authentication/README.md) and [Users and Permissions](../users-and-permissions/README.md) cover how you set both up. This section covers the design behind them, for readers who have to sign off on it.

| Page | Answers |
| --- | --- |
| [Threat Model](threat-model.md) | Who this is designed to keep out, what it accepts as a risk, and what it does not cover |
| [Shared Responsibility](shared-responsibility.md) | Which parts of security are ours and which are yours |
| [Compliance](compliance.md) | How the controls map to OWASP, NIST and IEC 62443, including where they fall short |

Security on the instance itself is separate, because the instance runs on your hardware and inside your network. See [umh-core Security](../../production/security/umh-core/deployment-security.md).
