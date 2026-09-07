# TLS and Certificates

This page covers the TLS connections to the Management Console: which hosts your browser and your umh-core instances connect to, who issues the certificates, and what happens when a corporate firewall inspects the traffic. The certificates that umh-core manages on the instance, and the user certificates that a [passphrase](../management-console/authentication/passwords-and-passphrases.md) decrypts, are a different topic and are not covered here.

## Hosts and certificates

| Host | Who connects | Certificate |
| --- | --- | --- |
| `management.umh.app` | Your browser for the console, umh-core for its API under `/api`, and Docker for image pulls under `/oci` | Issued by a publicly trusted certificate authority and renewed automatically |
| `auth.management.umh.app` | Your browser during sign-in | Issued by a publicly trusted certificate authority and renewed automatically |

Both certificates chain to public roots that ship with every current browser and operating system. There is nothing to install and nothing to pin on your side. The intermediate certificate and the leaf certificate change at every renewal, so allowlist the two hostnames in your firewall, not certificate fingerprints.

## Supported TLS versions

| Version | `management.umh.app` | `auth.management.umh.app` |
| --- | --- | --- |
| TLS 1.3 | Accepted | Accepted |
| TLS 1.2 | Accepted | Accepted |
| TLS 1.1 and older | Refused | Refused |

Both hosts send HTTP Strict Transport Security headers, so a browser that has visited them once refuses plain HTTP afterwards. `management.umh.app` also offers HTTP/2 and HTTP/3 to browsers.

umh-core connects with TLS 1.2 or higher and validates the certificate against the trust store of its container. See [Cryptography and TLS](../production/security/umh-core/deployment-security.md#cryptography-and-tls).

## Certificate authorities

The Management Console does not run a certificate authority for its own endpoints. Trust comes from the public roots in the browser or operating system, and in the umh-core container from the `ca-certificates` package of its base image.

Certificate authorities inside umh-core, such as the one that signs certificates for your bridges and users, are unrelated to the TLS connection to the console.

## Corporate TLS inspection

A firewall that inspects TLS terminates the connection itself and presents a certificate signed by your corporate certificate authority instead of the public one.

- **Browser.** Sign-in and the console keep working as long as the corporate certificate authority is in the operating system's trust store, which your IT usually manages. This applies to `auth.management.umh.app` as well.
- **umh-core.** The container trusts only the public roots by default, so it refuses the corporate certificate. Add your corporate certificate authority to the container's trust store. If that is not possible, `ALLOW_INSECURE_TLS` is the fallback described below. See [Network Configuration](../production/security/umh-core/network-configuration.md#tls-inspection-mitm) for both options and for proxy settings, which usually go together with inspection.

## ALLOW_INSECURE_TLS

`ALLOW_INSECURE_TLS=true` makes umh-core accept any certificate for its connection to `management.umh.app`, and lowers the minimum TLS version for that connection to TLS 1.0. It has no effect on your browser.

| Where to set it | Value |
| --- | --- |
| Environment variable | `ALLOW_INSECURE_TLS=true` |
| `config.yaml` | `agent.communicator.allowInsecureTLS: true` |

With validation disabled, anyone between the instance and the console can read and change the traffic, including the `AUTH_TOKEN`. Use it only behind a firewall that you trust to be the only party inspecting the connection, and prefer adding the corporate certificate authority. See [TLS Certificate Validation Can Be Disabled](../production/security/umh-core/deployment-security.md#tls-certificate-validation-can-be-disabled) for the full list of what the flag affects, and the [Configuration Reference](configuration-reference.md) for the setting itself.
