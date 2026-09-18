# Sessions

Regardless of which [authentication method](README.md) your account uses, session timeouts apply the same way.

Your session is managed by the Management Console itself, independently of the identity provider.

| Session rule | Value |
| --- | --- |
| Session length | 14 days |
| Renewal | While you are active and within 7 days of expiry, the session is extended by another 14 days |
| Hard limit | 30 days after you first signed in, you sign in again regardless of activity |
| Ends on | Sign-out, the 30 day limit, or removal from the company |

Each device keeps its own session. Signing in from several devices at once is allowed, but the connection to your instances can become unreliable when you do. Devices that need to be connected at the same time should get their own accounts.

{% hint style="warning" %}
There is no automatic sign-out after a period of inactivity. A session stays valid until the 14 day token expires or the 30 day limit is reached, whichever comes first. Sign out when you leave your workstation. If you need shorter sessions than that, use single sign-on and set the policy in your own identity provider.
{% endhint %}