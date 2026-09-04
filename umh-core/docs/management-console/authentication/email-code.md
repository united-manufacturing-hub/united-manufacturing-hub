# Email Code

An email code lets you sign in without a password. You enter your email address. Our identity provider emails you a code. You type that code into the sign-in screen.

## Who this applies to

The email code is the default for Community accounts.

Enterprise accounts use a different method. They sign in through the [default connection](README.md#which-company-you-sign-in-to) their company picked. Without the [single sign-on add-on](enterprise-sso.md), that connection is username and password.

Some older Community accounts still use a username and password. When you enter such an email address, the console sends you to that older login instead. The email code becomes available once we have moved your account across. See [Username and Password](username-and-password.md).

## Signing in

1. Open the Management Console and enter your email address.
2. Choose to receive a code by email. Our identity provider sends an email containing your verification code.
3. Enter the code in the sign-in screen that is still open in your browser.
4. Enter your passphrase. Signing in proves who you are; the passphrase decrypts your certificate, and the email code does not replace it. See [Passwords and Passphrases](passwords-and-passphrases.md#why-every-sign-in-still-asks-for-it).

The first time you sign in with an email address that has no account yet, the console asks you to complete your profile before it asks for a passphrase.

The sign-in screen also offers **Continue with Google** and **Continue with LinkedIn**. Those sign you in with the address held by that provider and do not send a code.

## Why we use email codes as our default

- **Only the latest code counts.** Requesting a new code invalidates every earlier one, and a code stops working the moment it has been used. A code copied from an old email is worthless.
- **Codes are short-lived.** A code is valid for a few minutes. If yours has expired, request a new one.
- **Guessing is cut off.** After three wrong entries the code is rejected and a fresh one has to be requested, so a code cannot be brute-forced.
- **The inbox is the credential.** The code goes to the address you entered and nowhere else, so access to the Management Console follows access to your mailbox. If you cannot read that inbox, use another sign-in method.
- **No password to protect.** No password is stored for this method, so there is none to reset and none to leak. If you receive a code you did not request, reply to that email so we can look into it.
- **Your passphrase stays yours.** The email code proves who you are; the passphrase separately decrypts your certificate and never leaves your device. See [Passwords and Passphrases](passwords-and-passphrases.md).
- **One session per account.** Signing in on another device signs you out everywhere else, so a stale session cannot linger on a machine you left. See [Sessions](sessions.md).
