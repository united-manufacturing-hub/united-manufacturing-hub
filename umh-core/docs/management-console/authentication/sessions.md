# Sessions

Regardless of which [authentication method](README.md) your account uses, session timeouts apply the same way.

## Session Durations & Renewal

Your session is managed by the Management Console itself, independently of the identity provider.

| Session rule | Value |
| --- | --- |
| Session length | 14 days |
| Renewal | While you are active and within 7 days of expiry, the session is extended by another 14 days |
| Hard limit | 30 days after you first signed in, you sign in again regardless of activity |
| Ends on | Sign-out, the 30 day limit, or removal from the company |

## One session per account

Each account has one active session at a time. When you sign in from another device, the existing session is invalidated and you are signed out of the Management Console everywhere else.

This is intentional. Most work in the Management Console is a one-off change, often to a single umh-core instance. Allowing several sessions at once would let the same account push concurrent, conflicting changes to your umh-core instances.


{% hint style="warning" %}
There is no automatic sign-out after a period of inactivity. A session stays valid until the 14 day token expires or the 30 day limit is reached, whichever comes first. Sign out when you leave your workstation.
{% endhint %}