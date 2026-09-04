# Username and Password

You enter your email address and a password to sign in. Two kinds of account use this method, and they differ in where the password is stored and what else it is used for.

## Who this applies to

**Enterprise accounts** sign in with username and password by default. The password is stored by our identity provider, and the sign-in form is the [default connection](README.md#which-company-you-sign-in-to) of your company's organization. Enterprise customers with the [single sign-on add-on](enterprise-sso.md) sign in through their own identity provider instead.

**Community accounts created with a password** keep signing in with that password. The same password is also your [passphrase](passwords-and-passphrases.md). New Community accounts use [Magic Link](magic-link.md). <!-- Commented out on purpose, because this feature is not fully rolled out yet. Will be removed once live. --> <!-- and an existing account can be moved across from the Management Console, see [Moving a Community account to Magic Link](#moving-a-community-account-to-magic-link). -->

## Signing in

1. Open the Management Console and enter your email address.
2. The console recognises which method your account uses. Enterprise accounts are sent to the sign-in form of their company's organization at our identity provider. Community accounts with a password see the password field directly.
3. Enter your password.
4. Enterprise accounts then enter their passphrase, which decrypts the certificate. For Community accounts with a password, the password already did this. See [Why every sign-in still asks for a Passphrase](passwords-and-passphrases.md#why-every-sign-in-still-asks-for-it).

Signing in on another device signs you out everywhere else. See [Sessions](sessions.md).

## Resetting a password

Enterprise passwords are stored by our identity provider and can be reset there from the sign-in form.

For old Community accounts created with a password, the password is also the passphrase that decrypts your certificate, so it is handled like a passphrase: only you hold it. Keep it in your password manager. There is no way to reset your password, if you forgot it. See [Passwords and Passphrases](passwords-and-passphrases.md#the-passphrase).

<!-- 
Commented out on purpose: The Community upgrade feature is currently being rolled out.
This is OK to exist as repository context, but shouldn't be mentioned in . This section will be made visible once the feature is fully rolled out

## Moving a Community account to Magic Link

{% hint style="info" %}
Community accounts created with a password use an older sign-in method. When you are signed in, a banner on the home page opens a short wizard that moves your account to the current method. Have your current password ready, it becomes your passphrase.
{% endhint %}

After the move you sign in with [Magic Link](magic-link.md), Google or LinkedIn, and the console asks for your former password as the passphrase. If you can no longer remember it, contact `support@umh.app`.
-->