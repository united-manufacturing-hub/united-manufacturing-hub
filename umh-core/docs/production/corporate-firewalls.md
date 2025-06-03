# Corporate Firewalls

If you're behind a corporate firewall that performs TLS inspection (MITM), you might see certificate errors. In this case, you can:

1. **Recommended:** Add your corporate CA certificate to the container's trusted certificates
2. **Last Resort:** Set `allowInsecureTLS: true` in your config:

```yaml
agent:
  communicator:
    allowInsecureTLS: true  # WARNING: Only use if corporate firewall blocks secure connections
```

⚠️ **Security Warning:** The `allowInsecureTLS` option disables certificate validation. Only use this if:

* You're behind a corporate firewall that you trust
* You cannot add your corporate CA certificate
* You understand the security implications
