# TLS and Certificates

This page covers the TLS connections to the Management Console: which hosts your browser and your umh-core instances connect to, who issues the certificates, and what happens when a corporate firewall inspects the traffic. The certificates that umh-core manages on the instance, and the access to your instances that a [passphrase](../management-console/authentication/passwords-and-passphrases.md) unlocks, are a different topic and are not covered here.

## Hosts and certificates

| Host | Who connects | Certificate |
| --- | --- | --- |
| `management.umh.app` | Your browser for the console, umh-core for its API, and Docker for image pulls | Issued by a publicly trusted certificate authority and renewed automatically |
| `auth.management.umh.app` | Your browser during sign-in | Issued by a publicly trusted certificate authority and renewed automatically |

Both certificates chain to public roots that ship with every current browser and operating system. There is nothing to install and nothing to pin on your side. The leaf certificate changes at every renewal and the intermediate certificate can change with it, so allowlist the two hostnames in your firewall, not certificate fingerprints.

## Supported TLS versions

| Version | `management.umh.app` | `auth.management.umh.app` |
| --- | --- | --- |
| TLS 1.3 | Accepted | Accepted |
| TLS 1.2 | Accepted | Accepted |
| TLS 1.1 and older | Refused | Refused |

Both hosts send HTTP Strict Transport Security headers, so a browser that has visited them once refuses plain HTTP afterwards. Both hosts also offer HTTP/2 and HTTP/3 to browsers.

umh-core connects with TLS 1.2 or higher and validates the certificate against the trust store of its container. See [Cryptography and TLS](../production/security/umh-core/deployment-security.md#cryptography-and-tls).

## Certificate authorities

The Management Console does not run a certificate authority for its own endpoints. Trust comes from the public roots in the browser or operating system, and in the umh-core container from the `ca-certificates` package installed in the container image.

Certificate authorities inside umh-core, such as the one that signs certificates for your bridges and users, are unrelated to the TLS connection to the console.

## Corporate TLS inspection

A firewall that inspects TLS terminates the connection itself and presents a certificate signed by your corporate certificate authority instead of the public one.

- **Browser.** Sign-in and the console keep working as long as the corporate certificate authority is in the operating system's trust store, which your IT usually manages. This applies to `auth.management.umh.app` as well.
- **umh-core.** The container trusts only the public roots by default, so it refuses the corporate certificate. Add your corporate certificate authority to the container's trust store. See [Instance Stays Offline Behind a Corporate Firewall](../usage/instances/communicator/instance-offline-behind-corporate-firewall.md) for the steps. Proxy settings usually go together with inspection and are in [Network Configuration](../production/security/umh-core/network-configuration.md#proxy-configuration).

## ALLOW_INSECURE_TLS

`ALLOW_INSECURE_TLS` and `agent.communicator.allowInsecureTLS` no longer turn off certificate validation. umh-core always validates the certificate of `management.umh.app` when it logs in to the Management Console, and requires TLS 1.2 or higher. A value that is still set does no harm. To trust a firewall that inspects TLS, add your corporate certificate authority as described in [Instance Stays Offline Behind a Corporate Firewall](../usage/instances/communicator/instance-offline-behind-corporate-firewall.md#add-your-corporate-ca-to-the-container).
