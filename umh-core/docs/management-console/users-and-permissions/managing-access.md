# Managing Access

## Managing users

Admins can invite users and add instances. Where a company has an Account Owner, that account can do both everywhere. An admin can grant permissions only for locations where they are an admin themselves, which stops anyone from inviting their way to more access than they have.

Instance creation is not restricted that way: any admin can create an instance at any location. Create instances inside a location where you hold admin access, because you cannot change one you created outside it. If that happens, an admin whose locations cover it, or the Account Owner, can change it for you.

For who should hold the Account Owner, and who to nominate for emergency access, see [Emergency Access](fallback-access.md).

### Inviting a user

1. The admin enters the email address, the role, and the locations it applies to.
2. The console produces an invite link and, separately, an invite key that only the admin sees.
3. Our identity provider emails the invitation to the address.
4. The admin sends the invite key to the person through another channel.
5. The person opens the link, signs in, and enters the invite key.

An invite key works once.

Both halves are needed on purpose. The link proves the person controls the email address. The key, sent another way, proves the admin meant to invite this particular person, so a forwarded invitation email is not enough to join your company.

## Removing access

| What you do | What happens |
| --- | --- |
| Remove a user | Their session ends immediately and they can no longer sign in |
| Change a user's permissions | Applied while they are signed in, within a few minutes. A user who is offline keeps their previous permissions until they sign in again |
| Remove an instance | The instance can no longer communicate with the console |

To take access away straight away, remove the user rather than lowering their role, then invite them again with the permissions you want. What they built stays: their bridges, data models and the users they invited belong to the company, not to them.

Sessions cannot be ended individually yet, so removing the user is the only way to end a session you do not control.
