# Users and Permissions

What you may do inside a company depends on the role you hold and where in the company you hold it. Users and instances both have roles.

## Which setup do you have

Permission models differ between accounts, so the roles you have are not necessarily the roles a colleague at another company has. Open the account menu, then **Settings**, then the **Permissions** tab, and compare what you see:

- **One role for the whole company.** **Current Role** names a single role, for example Super-Admin, and a matrix below it ticks off the permission groups that role covers.
- **Roles per location.** Your roles are listed against locations, so you can hold one role at one part of the company and a different role at another.

The rest of this page describes the second of those. Role capabilities for both are in the [Roles Reference](roles-reference.md).

Not every company is on the same setup, and moving between them is something we arrange rather than something you switch on. Ask your account executive which setup you are on and what moving would involve. Ask them too if your screen matches neither description.

## Locations

A location is a position in your company's tree. Level 0, the enterprise, is the only level you have to use. Everything below it is yours to choose, whether you follow ISA-95, KKS, or your own naming.

Location paths use the same dot-separated format as [topic paths](../../usage/unified-namespace/topic-convention.md):

- `ACME` (enterprise)
- `ACME.Munich` (enterprise, site)
- `ACME.Munich.Assembly` (enterprise, site, area)
- `ACME.Munich.Assembly.Line1` (enterprise, site, area, line)
- `ACME.Munich.Assembly.Line1.Cell5` (enterprise, site, area, line, work cell)

Add more levels if your organization needs them.

## Roles

Three roles decide what a user or instance may do at the locations they are assigned to. Admin has full control including inviting others, Editor can create and change resources but not manage users, and Viewer can only read. The full capability list is in the [Roles Reference](roles-reference.md).

A user can hold different roles at different locations, for example Admin at `ACME.Munich.Assembly.Line1` and Viewer at `ACME.Munich.Assembly.Line2`.

### Permission inheritance

Permissions inherit downward. Access at `ACME.Munich` also grants access to everything under Munich, including `ACME.Munich.Assembly.Line1.Cell5`.

You can override an inherited permission for a specific location. A user can be a Viewer at `ACME.Munich.Assembly` and an Admin at `ACME.Munich.Assembly.Line1.Cell5` only.

### What roles do not cover

These roles decide what you can do in the Management Console. They stop there. An instance does not check the role of the user behind a request, so any member of your company can act on any instance in it, whatever role they hold and wherever they hold it. See [No Per-User Access Control Within Instance](../../production/security/umh-core/deployment-security.md#no-per-user-access-control-within-instance).

Two consequences worth planning around. Inside the console, the boundary that holds is read-only against write access, so Viewer is the role to give someone who should not change anything. At the instance level there is no boundary, so treat membership of a company as access to the machines that company runs.

## Managing users

Admins can invite users and add instances. Where a company has an Account Owner, that account can do both everywhere. An admin can grant permissions only for locations where they are an admin themselves, which stops anyone from inviting their way to more access than they have.

Instance creation is not restricted that way: any admin can create an instance at any location. Create instances inside a location where you hold admin access, because you cannot change one you created outside it. If that happens, an admin whose locations cover it, or the Account Owner, can change it for you.

For who should hold the Account Owner, and who to nominate for emergency access, see [Fallback Access](fallback-access.md).

### Inviting a user

1. The admin enters the email address, the role, and the locations it applies to.
2. The console produces an invite link and, separately, an invite key that only the admin sees.
3. Auth0 emails the invitation to the address.
4. The admin sends the invite key to the person through another channel.
5. The person opens the link, signs in, and enters the invite key.

An invite key works once.

Both halves are needed on purpose. The link proves the person controls the email address. The key, sent another way, proves the admin meant to invite this particular person, so a forwarded invitation email is not enough to join your company.
