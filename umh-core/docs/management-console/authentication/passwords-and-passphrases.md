# Passwords and Passphrases

Depending on your account, the Management Console can ask you for two different secrets. They sound alike but do different jobs, and they behave differently when something goes wrong.

| Secret | What it is | Can it be reset? |
| --- | --- | --- |
| **Password** | Proves who you are at sign-in, the same as any username-and-password login. Our identity provider stores it. Google, LinkedIn, single sign-on and email codes take its place with another way to prove who you are. | Yes, when our identity provider stores it. Community accounts created with a password use it as their passphrase, see [Username and Password](username-and-password.md#resetting-a-password). |
| **Passphrase** | A decryption key you create. Your account keeps a certificate on our servers, and the passphrase decrypts it. Only you hold the passphrase, so we never see it and cannot decrypt the certificate for you. | No |

## The password

The password answers "are you allowed to sign in". It works like any other login password. When our identity provider stores it, you can reset it if you forget it. If you sign in with Google, LinkedIn, single sign-on, or an email code, that method proves who you are instead and you have no password to manage.

## The passphrase

The passphrase answers a separate question: it unlocks the certificate your account needs before the console can act on your behalf. You create the passphrase, and it never leaves your device. Because we never receive it, we cannot decrypt the certificate for you, and there is no way to reset or recover it.

Keep it somewhere you can find it again, such as your password manager. If it is lost, the certificate cannot be recovered.

## Why every sign-in still asks for it

Proving who you are and decrypting the certificate are two separate steps. Signing in covers the first; the passphrase covers the second. So every sign-in method still needs the passphrase, whichever way you proved your identity. Even an email code, which involves no password at all, prompts for the passphrase so the console can decrypt the certificate.
