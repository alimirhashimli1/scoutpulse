# Sign-in setup

Four things are configuration rather than code. Each one is **off by default
and degrades rather than failing**: no Google credentials means no Google
button, no captcha secret means no challenge, no SMTP host means the
verification link is written to the log instead of emailed.

That is deliberate — a developer should be able to clone this and register an
account without first registering with Google, Cloudflare and a mail provider.
It also means a half-configured production deployment looks like it works. The
checklist at the bottom is there for that reason.

Everything below goes in `.env` at the repository root, which is gitignored.

---

## 1. Google sign-in

**This is why the Google button does not appear.** The code and database schema
have been in place since round 3; `GET /auth/providers` returns `[]` because no
credentials are set, and the frontend deliberately renders no button for a
provider that would 404.

1. Open the [Google Cloud Console](https://console.cloud.google.com/), create a
   project (or pick one).
2. **APIs & Services → OAuth consent screen.** Choose *External*, fill in the
   app name and your email. While the app is in *Testing* only accounts you
   list as test users can sign in — add your own address there.
3. **APIs & Services → Credentials → Create credentials → OAuth client ID.**
   Application type: *Web application*.
4. Under **Authorised redirect URIs** add exactly:

   ```
   http://localhost:8000/api/identity/api/v1/auth/google/callback
   ```

   This must match character for character, including the scheme and port.
   Google rejects a mismatch with `redirect_uri_mismatch`, which is the single
   most common failure here. The path comes from `PUBLIC_BASE_URL` — if you
   change that, change this too.
5. Copy the client ID and secret into `.env`:

   ```
   GOOGLE_CLIENT_ID=…apps.googleusercontent.com
   GOOGLE_CLIENT_SECRET=…
   ```

## 2. Facebook sign-in

1. Open [Meta for Developers](https://developers.facebook.com/apps/), create an
   app of type *Consumer*, and add the **Facebook Login** product.
2. **Facebook Login → Settings → Valid OAuth Redirect URIs:**

   ```
   http://localhost:8000/api/identity/api/v1/auth/facebook/callback
   ```

3. **Settings → Basic** holds the App ID and App Secret:

   ```
   FACEBOOK_CLIENT_ID=…
   FACEBOOK_CLIENT_SECRET=…
   ```

**One behaviour worth knowing.** Facebook's Graph endpoint does not report
whether an address is verified, so this service treats every Facebook address
as unverified. That is not cosmetic: an account is only linked to an existing
one when the provider vouches for the address, so a Facebook sign-in whose
address matches an existing account is **refused** rather than linked. Sign in
with the password first and link from the account page.

## 3. Human challenge (captcha)

Two providers work; both are verified the same way.

**Cloudflare Turnstile** — free, and needs no cookie-consent banner, which
matters if the site serves the EU. [dash.cloudflare.com → Turnstile](https://dash.cloudflare.com/?to=/:account/turnstile).

```
CAPTCHA_PROVIDER=turnstile
CAPTCHA_SITE_KEY=0x4AAA…
CAPTCHA_SECRET=0x4AAA…
```

**Google reCAPTCHA** — [google.com/recaptcha/admin](https://www.google.com/recaptcha/admin).

```
CAPTCHA_PROVIDER=recaptcha
CAPTCHA_SITE_KEY=…
CAPTCHA_SECRET=…
CAPTCHA_MIN_SCORE=0.5   # v3 only; it scores 0..1 instead of pass/fail
```

The **site key is public** and is sent to the browser — that is what it is for.
The **secret never leaves identity-svc**.

Verification happens server-side, and that is the only part that matters. A
client that wants to skip the widget simply does not render it and posts to the
API directly; what stops automated sign-ups is the service refusing a request
whose token the provider will not vouch for.

## 4. Email delivery

Registration by email is gated: the account exists but cannot sign in until the
address is confirmed.

**With no `SMTP_HOST`, nothing is sent.** The link is written to the service log
instead, which is what makes the flow testable without a mail account:

```
docker compose logs identity-svc | grep -i "email not sent"
```

Copy the URL out of that line and open it. For real delivery:

```
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USERNAME=…
SMTP_PASSWORD=…
SMTP_FROM=no-reply@example.com
```

Port 587 with STARTTLS. The mailer refuses to send credentials to a server that
does not offer STARTTLS rather than falling back to plaintext. Implicit TLS on
port 465 is *not* supported — most providers offer 587.

---

## Applying changes

`.env` is read when a container starts, so:

```
docker compose up -d identity-svc
```

A plain `restart` does **not** re-read `.env`.

## Checking what is actually on

```
curl http://localhost:8000/api/identity/api/v1/auth/config
```

```json
{
  "providers": ["google"],
  "captcha": { "enabled": true, "provider": "turnstile", "site_key": "0x4AAA…" },
  "verification_required": true
}
```

The frontend renders both auth pages from exactly this response, so if a button
or a widget is missing, this tells you whether the server thinks it is
configured — which is faster than guessing at the browser.

## Before this is public

- [ ] Google and Facebook redirect URIs point at the real domain, not localhost
- [ ] `PUBLIC_BASE_URL` and `FRONTEND_URL` set to the real domain
- [ ] `CAPTCHA_SECRET` set — otherwise sign-up is open to automation
- [ ] `SMTP_HOST` set — otherwise **no one ever receives a link and every new
      account is permanently stuck unverified**
- [ ] Google consent screen moved out of *Testing*, or only listed test users
      can sign in
