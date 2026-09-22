# Passwords and Passphrases

The Management Console asks you for up to two secrets: a password and a passphrase. Every account has a passphrase. Whether you also have a password depends on how you sign in. The two sound alike but do different jobs, and only the password can be reset.

| Secret | What it is | Can it be reset? |
| --- | --- | --- |
| **Password** | Proves who you are at sign-in. | Yes, except for Community accounts created with a password. See [The password](#the-password). |
| **Passphrase** | Unlocks your access to your instances. Only you can do that. | No |

## The password

The password answers "are you allowed to sign in". It works like any other login password. Our identity provider stores it. If you forget it, you can reset it from the sign-in form. See [Resetting a password](username-and-password.md#resetting-a-password).

Google, LinkedIn, single sign-on and email codes replace the password with another way to prove who you are. With those methods, you have no password to manage.

Community accounts created with a password use that password as their passphrase too. That password cannot be reset. See [Resetting a password](username-and-password.md#resetting-a-password).

## The passphrase

The passphrase unlocks your access to your instances. Behind that access is a permission grant, which the Management Console issues to your account. It is stored locked on our servers, and your passphrase is the only thing that unlocks it.

You create the passphrase. It never leaves your device. We never see it, so we cannot unlock your access for you, and no one can reset it.

Keep the passphrase somewhere you can find it again, such as your password manager. If you lose it, no one can unlock your access again, including us.

## Why every sign-in asks for the passphrase

Every sign-in method asks for the passphrase, whichever way you proved your identity. Proving who you are and unlocking your access to your instances are two separate steps. Signing in covers the first. The passphrase covers the second.

An email code involves no password at all. It still prompts for the passphrase to unlock your access to your instances.
