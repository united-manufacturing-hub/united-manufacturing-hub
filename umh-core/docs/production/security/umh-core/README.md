# umh-core Security

umh-core is a single-container edge gateway designed for deployment at the OT/IT boundary. Security is built on non-root execution, outbound-only connections, and clear responsibility boundaries.

**For complete security documentation**, see [deployment-security.md](./deployment-security.md)

## Quick Checklist

Things to keep in mind when deploying umh-core:

**Container Setup:**
- Mount `/data` for persistent storage - mount extra paths only when you need them
- Configure your firewall to only give the container access to the IP addresses it needs
- Set `AUTH_TOKEN` for ManagementConsole connection
- Only use `ALLOW_INSECURE_TLS=true` if you're behind a corporate firewall with TLS inspection

**Network:**
- Allow outbound HTTPS to `management.umh.app`
- Deploy in DMZ (Purdue Level 3) with firewalls on both OT and IT boundaries
- Configure proxy if needed (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`)

**Industrial Protocols:**
- Start with conservative PLC polling rates and test before production
- Consult device manuals for connection and polling limits

---

## Documentation Guide

- **This README**: Quick security checklist for deployment
- **[deployment-security.md](./deployment-security.md)**: Complete security architecture, standards compliance, and deployment considerations
- **ManagementConsole security**: See ManagementConsole documentation (user auth, RBAC, MFA)
