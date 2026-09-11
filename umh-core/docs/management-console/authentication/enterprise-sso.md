# Enterprise Single-Sign On (SSO) with your own Identity Provider (IdP)

{% hint style="warning" %}
Enterprise Single Sign-On is a paid add-on and is not included in the Community & Enterprise plan. Get in touch with your account executive to learn more.
{% endhint %}

## Who this applies to

This functionality is available for any Enterprise plan customer. The Management Console features an identity provider (IdP) that allows enterprise customers to federate access through their own SSO provider. 

## Connecting your identity provider

Enterprise SSO extends your existing identity and access management strategy to the Management Console, so the same governance, provisioning, and offboarding controls your organization already trusts apply here as well. Centralizing authentication in your identity provider reduces the operational overhead of managing separate credentials, strengthens your security posture, and helps satisfy audit and compliance requirements around access control.

Onboarding is a managed, white-glove process. Our team partners with your stakeholders and IT organization to plan the integration, align it with your corporate security policies, and validate it end to end before go-live, minimizing disruption to your users and ensuring a smooth rollout across your enterprise.

## Supported Methods

| Method | Type |
| --- | --- |
| SAML 2.0 | Custom |
| OpenID Connect (OIDC) | Custom |
| Okta | Native |
| Google Workspace | Native |
| Microsoft Azure AD (Microsoft Entra ID) | Native |
| ADFS | Native |
| LDAP / Active Directory | Native |
| Ping Federate | Native |

## Multi-factor authentication

When you federate access through your own identity provider, multi-factor authentication (MFA) remains fully under your control, governed by your corporate security policies. We do not layer additional MFA on top of your custom IdP integration, so your organization retains a single, authoritative point of enforcement and a consistent audit trail across every system your identity provider protects.

MFA should not be confused with passphrases, which serve a distinct purpose: decrypting your authorization certificate. For how sign-in and certificate decryption work together, see [Why every sign-in asks for a passphrase](passwords-and-passphrases.md#why-every-sign-in-still-asks-for-it).