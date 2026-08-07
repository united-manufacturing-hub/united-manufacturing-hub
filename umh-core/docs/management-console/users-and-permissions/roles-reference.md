# Roles Reference

Roles are not the same in every account. Work out which setup you have in [Users and Permissions](README.md) first, then read the matching section below. There is deliberately no combined table.

## Roles per location

Three roles apply at the locations a user or instance is assigned to.

| Role | What it allows |
| --- | --- |
| Admin | Everything Editor allows, plus inviting users at the locations where they are an admin. Any admin can add an instance at any location, see [Managing users](managing-access.md#managing-users) |
| Editor | Create and modify resources, such as bridges, data models and instance configuration. Cannot manage users |
| Viewer | Read-only. See resources and their state, change nothing |

A role applies to the location it was granted at and everything below it. Exceptions set at a lower location override what is inherited, see [Permission inheritance](location-based-permissions.md#permission-inheritance).

These capabilities apply in the Management Console. They do not apply on an instance, which does not check the role behind a request, see [What roles do not cover](README.md#what-roles-do-not-cover).

## One role for the whole company

Here your role is not tied to a location. **Current Role** on the **Permissions** tab names it, and the permission matrix underneath shows which groups of actions it covers:

| Permission group | Covers |
| --- | --- |
| Get actions | Read-only operations for viewing resources and states |
| Edit actions | Operations that modify existing resources |
| Delete actions | Operations that remove resources |
| Deploy actions | Operations for deploying new resources |
| Test actions | Operations for testing configurations |
| System actions | System-level operations |
| Company actions | Company-level operations |

Expand a group in the matrix to see the individual actions in it. The matrix is generated from the role you hold, so it is the list for your own account rather than a general one. Like the roles above, it covers the console only, see [What roles do not cover](README.md#what-roles-do-not-cover).
