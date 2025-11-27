# Security Documentation Overview

This directory contains security documentation for the United Manufacturing Hub platform.

## Component Scope

The UMH platform consists of two main security domains:

### umh-core (Edge Gateway Container)
**Documentation**: `umh-core/deployment-security.md`

Security scope:
- Instance-level authentication (AUTH_TOKEN)
- Container security (non-root execution, process isolation)
- Edge gateway security architecture
- Protocol converter and data flow security
- Network security for edge deployment
- Supply chain security (vulnerability scanning, dependencies)
- Industrial protocol handling (OPC UA, Modbus, S7, MQTT)

### ManagementConsole (Cloud Platform)
**Documentation**: `management-console/` (separate repository)

Security scope:
- User authentication and multi-factor authentication (MFA)
- Role-based access control (RBAC) for users
- User-level audit trails and action logging
- Cloud security and API protection
- Session management and user permissions
- Organization and team access controls

## Security Responsibility Boundary

**umh-core** handles edge security - authenticating the instance, securing the container, and protecting data flows at the factory edge.

**ManagementConsole** handles user security - authenticating users, controlling access, and securing the cloud platform.

Together they provide defense-in-depth: instance authentication (umh-core) + user authentication (ManagementConsole) + customer infrastructure security.
