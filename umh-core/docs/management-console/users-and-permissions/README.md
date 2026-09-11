# Users and Permissions

{% hint style="warning" %}
Management Console continues to evolve on a regular basis. The same applies to our permission system. Depending on when you created your account, you'll see different options.
{% endhint %}

What you may do inside a company depends on the role you hold and where in the company you hold it. Users and instances both have roles.

## How to determine your permission setup

{% hint style="warning" %}
User and Permission Management is part of the **Enterprise Plan**. If you're using the Community Plan, your account can only have one user, which is automatically the Account Owner with full CRUD permissions.
{% endhint %}

Permission models differ between accounts, so the roles you have are not necessarily the roles a colleague at another company has. Open the account menu, click **Settings**, select the **Permissions** tab, and compare what you see:

- **One role for the whole company.** **Current Role** names a single role (`Account Owner`), and a matrix below it ticks off the permission groups that role covers.
- **Roles per location.** Your roles are listed against locations, so you can hold one role at one part of the company and a different role at another.

Role capabilities for both setups are in the [Roles Reference](roles-reference.md). Locations, roles and how permissions inherit are covered in [Location-Based Permissions](location-based-permissions.md), and inviting, changing and removing people in [Managing Access](managing-access.md).

Not every company is on the same setup, and moving between them is something we arrange rather than something you switch on. Ask your account executive which setup you are on and what moving would involve.

### Where these roles apply

Your role governs what you can do in the Management Console, where permissions are managed centrally for your account and for the instances your company connects.

`umh-core` connects to the Management Console through its `AUTH_TOKEN`, and can also run locally. For how instances connect and authenticate, see [Access Control and Authentication](../../production/security/umh-core/deployment-security.md#access-control-and-authentication).
