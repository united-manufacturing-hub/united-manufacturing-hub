# Users and Permissions

What you may do inside a company depends on the role you hold and where in the company you hold it. Users and instances both have roles.

## Which setup do you have

Permission models differ between accounts, so the roles you have are not necessarily the roles a colleague at another company has. Open the account menu, then **Settings**, then the **Permissions** tab, and compare what you see:

- **One role for the whole company.** **Current Role** names a single role (`Account Owner`), and a matrix below it ticks off the permission groups that role covers.
- **Roles per location.** Your roles are listed against locations, so you can hold one role at one part of the company and a different role at another.

Role capabilities for both setups are in the [Roles Reference](roles-reference.md). Locations, roles and how permissions inherit are covered in [Location-Based Permissions](location-based-permissions.md), and inviting, changing and removing people in [Managing Access](managing-access.md).

Not every company is on the same setup, and moving between them is something we arrange rather than something you switch on. Ask your account executive which setup you are on and what moving would involve. Ask them too if your screen matches neither description.

### What roles do not cover
<!-- @claude: update this based on my comment input -->
These roles decide what you can do in the Management Console. They stop there. An instance does not check the role of the user behind a request, so any member of your company can act on any instance in it, whatever role they hold and wherever they hold it. See [No Per-User Access Control Within Instance](../../production/security/umh-core/deployment-security.md#no-per-user-access-control-within-instance).

Two consequences worth planning around. Inside the console, the boundary that holds is read-only against write access, so Viewer is the role to give someone who should not change anything. At the instance level there is no boundary, so treat membership of a company as access to the machines that company runs.
