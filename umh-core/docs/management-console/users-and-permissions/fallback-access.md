# Emergency Access

Admins lose access. People leave, addresses get disabled, permissions get set wrong. Fallback access is what you arrange in advance so that a company is never locked out of its own instances.

Two things to arrange: set up your break-glass account deliberately, and nominate someone who can get you back in.

## The break-glass account

### What is a break-glass account?

A break-glass account is an account with full permissions that nobody uses for daily work. It exists for one purpose: when every regular admin is locked out, someone signs in with it, repairs the permissions or invites new admins, and signs out again. The name comes from the glass cover on a fire alarm: you break it only in an emergency. Because the account is used rarely, it is kept out of the normal flow of people joining and leaving, its credentials are stored in one controlled place, and its use is a deliberate, visible act rather than routine.

If you already run break-glass accounts in other systems, the Management Console works the same way and you can skip to the practices below.

### The Account Owner is your break-glass account

In the Management Console, the **Account Owner** is the account that created the company. It has Admin access everywhere in the company, and it cannot be transferred or demoted. That is deliberate: it guarantees one account always has full control, whatever happens to the others. Not every company has one. Ask your account executive if you are not sure whether yours does.

The Account Owner cannot be recovered either, so decide before you register which account it will be, and treat it as a break-glass account from day one:

- Use an address your organization owns and can keep, such as `ot-team@example.com`. Not the personal account of an employee who may leave.
- Keep its credentials in your team's password manager rather than passing them around.
- Switch on multi-factor authentication for it.
- Do not work with it day to day. Invite ordinary admin accounts from it and use those.

## Nominating UMH staff as emergency access

We recommend keeping at least one UMH team member in your company, so there is someone who can restore your access if your own admins cannot. The same access lets us do longer pieces of setup work for you, such as building bridges or data models, which a call cannot cover.

To nominate someone:

1. Ask your account executive which UMH team member to invite.
2. Invite them through the normal invitation flow, using their `@umh.app` address. Grant Admin at your enterprise location, because permissions inherit downward and access granted lower down will not help if the problem is lower down.
3. Remove them when you no longer want the access in place.

You don't need to add UMH team members to your single-sign on solution. When adding UMH team members to your account, we use our own identity provider.

Control stays on both sides:

- UMH staff reach your company only through an invitation you issue, and only for as long as you leave it in place.
- UMH manages `@umh.app` identities centrally, so access is tied to employment. When someone leaves UMH, their identity is disabled and their access to your company goes with it. If UMH is ever compromised, the same central control lets us cut all of those accounts at once.

## Reviewing it

Check your fallback access when you review permissions. Two things drift: the address on the Account Owner stops being monitored, and UMH staff invited for one piece of work stay invited afterwards. Decide each time whether the access should stay.
