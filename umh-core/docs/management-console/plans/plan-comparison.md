# Editions Reference

The Management Console has two editions. Which one your company is on depends on whether an Enterprise licence is attached to it, see [Licenses & Plans](README.md). The edition applies to the whole company: every user and every instance registered under it.

## Editions at a glance

Without an active licence, your company is on the **Community Edition** and can register one instance. Registering a second one fails with "Company has reached the maximum number of instances". Everything else about that instance works as documented under [Usage](../../usage/README.md).

An active licence puts your company on the **Enterprise Edition** and unlocks:

- Registering more than one instance.
- Inviting users and assigning roles and location permissions. See [Users and Permissions](../users-and-permissions/README.md).
- The **Permissions** tab in Settings, which shows your own role.
- The **Enterprise** release channel when adding an instance. Stable and Nightly are available on every plan.
- A review step in the deploy dialog for bridges, stand-alone flows, stream processors and data models that shows the changes before they are applied.
- Rollback to an earlier version from the version history, including a diff against the deployed version.
- Instance audit logs. This also requires the **Audit Logs** early access feature under **Settings → Settings**.

Enterprise Single Sign-On is a separate paid add-on, see [Authentication](../authentication/README.md).

## Feature table

| Feature | Community | Enterprise |
|---|---|---|
| Registered instances per company | 1 | No limit enforced by the console |
| Instance release channels | Stable, Nightly | Stable, Nightly, Enterprise |
| Users per company | Account Owner only; inviting is disabled | Invite users, see [Users and Permissions](../users-and-permissions/README.md) |
| Roles and location permissions | Not available | Account Owner, Admin, Editor, Viewer, scoped by location. See [Roles Reference](../users-and-permissions/roles-reference.md) |
| **Permissions** tab in Settings | Hidden | Shows your own role and location access |
| Review of changes before a deploy (bridges, stand-alone flows, stream processors, data models) | Deploys directly | Shows a diff against the running configuration first |
| Rollback to an earlier version from the version history | Not available | Available, with a diff against the deployed version |
| Instance audit logs | Not available | Available; also requires the **Audit Logs** early access feature under **Settings → Settings** |
| Single sign-on with your own identity provider | Not available | Paid add-on, see [Authentication](../authentication/README.md) |

Where a feature is not available in the Community Edition, the console shows the control disabled with a tooltip such as "You need an active license to invite users", or hides it.
