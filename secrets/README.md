# Secrets backup

Petra's Fly app secrets — the VAPID keypair, `OPENAI_API_KEY`, and the Tigris/S3
credentials — are **write-only on Fly**: `fly secrets list` shows only names and
digests, never values, and there is no read-back. The Litestream backup restores
*data only*, and these secrets are not in git either. So if the Fly app is lost,
they are gone unless we keep a copy elsewhere.

This directory keeps an [age](https://github.com/FiloSottile/age)-encrypted copy
of each app's secrets. The ciphertext (`<app>.env.age`) is safe to commit; the
age **private key** lives outside the repo and in your password manager.

## What is committed vs. ignored

| File | Committed? | Contents |
|---|---|---|
| `age-recipients.txt` | yes | age **public** keys (not sensitive) |
| `petra.env.example` | yes | template listing the required secret names |
| `<app>.env.age` | yes | **encrypted** secrets — safe |
| `<app>.env` | **no** (gitignored) | decrypted plaintext — never commit |
| the age private key | **no** (lives at `~/.config/petrapp/secrets-age.key`) | back this up in your password manager |

## One-time setup

```sh
make secrets-keygen                       # creates ~/.config/petrapp/secrets-age.key, prints the public key
#   → add the printed public key to secrets/age-recipients.txt
#   → store the PRIVATE key (~/.config/petrapp/secrets-age.key) in your password manager

cp secrets/petra.env.example secrets/petra.env
#   → fill in real values (read them from your password manager / Tigris+OpenAI dashboards;
#     you cannot read them back from Fly)

make secrets-encrypt FLY_APP=petra        # → secrets/petra.env.age
rm secrets/petra.env                       # delete the plaintext
git add secrets/petra.env.age secrets/age-recipients.txt
```

Repeat with `FLY_APP=petra-staging` for staging.

## Rotating a secret

```sh
make secrets-decrypt FLY_APP=petra        # → secrets/petra.env (gitignored)
$EDITOR secrets/petra.env                  # change the value
make secrets-encrypt FLY_APP=petra        # re-encrypt
rm secrets/petra.env
fly secrets set KEY=newvalue --app petra   # apply to the live app
git commit -am "chore(secrets): rotate KEY"
```

## Recovery (rebuild from nothing)

After recreating the app and restoring the age key from your password manager:

```sh
make fly-secrets-push FLY_APP=petra        # decrypts and pipes into `fly secrets import`
```

The plaintext is streamed over a pipe and never written to disk. This triggers a
Fly release. See `docs/disaster-recovery.md` for the full rebuild procedure.
