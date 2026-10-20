# Fork changes

This is a personal fork of [Dvorinka/cal](https://github.com/Dvorinka/cal), maintained at [mihaipriboi/cal](https://github.com/mihaipriboi/cal) for a self-hosted, single-user deployment.

This file lists every change the fork makes to upstream code: what changed, why, and how it affects compatibility. Thanks to the upstream author for Cal. Changes marked *upstreamable* are happily offered back as PRs.

- **Forked from:** upstream `3875d41` (v0.1.10)
- **Last upstream sync:** none yet

## How to read this

- Each change has an ID (`FC-NNN`). Commits that implement it mention the ID, so `git log --grep FC-003` finds them.
- **Compatibility** notes API, data or default-behavior changes, and whether the change could go upstream as-is.
- Fork database migrations are numbered from `10001` so they never collide with upstream's `000NN` files. Goose runs with `WithAllowMissing` so later upstream migrations still apply.

## Change log

| ID | Area | Summary | Upstreamable |
| --- | --- | --- | --- |
| [FC-001](#fc-001--trust-only-configured-reverse-proxies) | API | Client IP honors `X-Forwarded-For` only from `TRUSTED_PROXIES` | yes |
| [FC-002](#fc-002--sign-ups-closed-by-default-after-the-first-account) | API, web | Sign-ups closed by default once an account exists; first sign-up race-free | yes |
| [FC-003](#fc-003--clear-error-for-passwords-over-72-bytes) | API, web | Passwords over 72 bytes get a 400 with a reason instead of a 500 | yes |

## Details

### FC-001 — Trust only configured reverse proxies

- **Why:** Gin trusts every proxy by default, so `ClientIP()` returned whatever the client put in `X-Forwarded-For`. The per-IP rate limits on login, registration, password reset, MCP and intake could be bypassed by sending a different header value on each request.
- **What changed:** `httpapi.New` calls `router.SetTrustedProxies` with the list from the new `TRUSTED_PROXIES` env var (comma-separated IPs/CIDRs). When it's unset, no proxy is trusted and the connecting peer is the client. An invalid entry stops startup with a clear error. Added to the README config table.
- **Compatibility:** deployments behind a reverse proxy should set `TRUSTED_PROXIES` to the proxy's address. Otherwise all clients share the proxy's rate-limit bucket. That's safe, just coarser.
- **Upstreamable:** yes.
- **Tests:** `ratelimit_test.go`: rotating `X-Forwarded-For` doesn't bypass the limiter; a trusted proxy gives per-client buckets and ignores prepended fake hops; env parsing.

### FC-002 — Sign-ups closed by default after the first account

- **Why:** registration stayed open after the first (admin) account unless an admin closed it, and a failed user count or config read let sign-ups through. An instance exposed to the internet would accept strangers until someone noticed the toggle. Two simultaneous first sign-ups could also both become admin.
- **What changed:**
  - `AllowRegistration` is true only when an admin has explicitly set it. A missing key or a DB error reads as closed.
  - `registrationClosed` fails closed on errors.
  - `CreateUser` runs in a transaction under a Postgres advisory lock. It re-checks the policy there (the first account is always admitted and becomes admin) and returns `ErrRegistrationClosed`, which the handler maps to 403.
  - The Settings → Users & access note explains the default.
- **Compatibility:** behavior change for existing instances whose admin never touched the toggle: sign-ups are now closed until the toggle is turned on. Fresh instances still accept their first account.
- **Upstreamable:** yes.
- **Tests:** the admin test checks default-closed (403), then opening; new `TestConcurrentFirstRegistration` (6 parallel first sign-ups → exactly one admin, the rest 403). The test helper gained `newEmptyTestServer` and `openRegistration`; the shopping test opens registration before adding its second user.

### FC-003 — Clear error for passwords over 72 bytes

- **Why:** bcrypt rejects input longer than 72 bytes. Registration and password change allowed up to 128 characters, so a long passphrase failed inside the hasher and surfaced as a generic 500.
- **What changed:** new `auth.ValidatePassword` (at least 8 characters, at most 72 bytes, with user-facing messages) used by register, change password and reset password. Login is unchanged, since an over-long password simply doesn't match. The new-password inputs in the web app get `maxLength={72}`.
- **Compatibility:** passwords of 73–128 bytes, which never worked, are now refused with a 400 and a reason. No existing password is affected.
- **Upstreamable:** yes.
- **Tests:** `TestValidatePassword` (bounds, multi-byte characters, hashing at the limit); `TestRegisterRejectsOverlongPassword` (73 bytes → 400 mentioning the limit, 72 bytes → 200).
