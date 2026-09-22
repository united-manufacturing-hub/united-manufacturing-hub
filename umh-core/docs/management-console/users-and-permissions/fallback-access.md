# Emergency Access

Admins lose access. People leave, addresses get disabled, permissions get set wrong. Fallback access is what you arrange in advance so that a company is never locked out of its own instances.

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

## Reviewing it

Check your fallback access when you review permissions. Two things drift: the address on the Account Owner stops being monitored. Decide each time whether the access should stay.
