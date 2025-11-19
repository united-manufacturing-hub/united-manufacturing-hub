# Security Documentation Plan

## Key Principles

**Keep umh-core and ManagementConsole documentation SEPARATE:**

| Component | Deployment | Security Focus |
|-----------|------------|----------------|
| **umh-core** | Edge/on-premise | Container isolation, volume mounts, network access, bridge permissions |
| **ManagementConsole** | Cloud platform | User authentication, authorization, API security, cloud infrastructure |

## Current Documentation

### umh-core (Edge Security)

| Document | Description | Status |
|----------|-------------|--------|
| `deployment-security.md` | Container isolation, volume mounts, environment variables | Complete |
| `bridge-access-model.md` | Bridge permissions, host network mode, protocol security | Complete |
| `network-configuration.md` | Outbound connections, corporate firewalls, proxy settings | Complete |

## Future Topics

### umh-core

- **secrets-management.md** - Handling AUTH_TOKEN and sensitive configuration
- **audit-logging.md** - Logging security-relevant events
- **hardening-guide.md** - Production hardening recommendations

### ManagementConsole

- **authentication.md** - User authentication mechanisms
- **authorization.md** - Role-based access control
- **cloud-infrastructure.md** - Cloud security architecture
- **api-security.md** - API authentication and rate limiting

## Writing Guidelines

1. **Be concise** - Practical guidance, not comprehensive theory
2. **Use tables** - Especially for comparisons, risk assessments, configurations
3. **Include examples** - Real docker commands, config snippets
4. **Separate concerns** - umh-core docs should not reference ManagementConsole internals
5. **Keep updated** - Documentation must match current code behavior
