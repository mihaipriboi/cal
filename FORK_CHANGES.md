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
| [FC-004](#fc-004--allow-out-of-order-migrations-for-fork-migrations) | API | Goose applies migrations with `WithAllowMissing` (fork migrations use 10001+) | no |
| [FC-005](#fc-005--caldav-uids-scoped-per-account) | API, DB | CalDAV UIDs unique per account; sync never updates another user's row | yes |
| [FC-006](#fc-006--html-mail-in-a-sandboxed-frame-csp-for-the-app-shell) | web, Android | HTML mail in a sandboxed iframe; CSP meta for the app shell; mail-password export needs a native confirmation | yes |
| [FC-007](#fc-007--android-no-backups-https-only-no-default-keystore-password) | Android | App data excluded from backups; HTTPS only; no built-in keystore password | partly |

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

### FC-004 — Allow out-of-order migrations for fork migrations

- **Why:** the fork adds its own database migrations. Numbering them after upstream (`00023`, …) would collide with upstream's next files, so fork migrations start at `10001`. Goose refuses by default to apply a migration whose version is lower than one already applied, so a later upstream `00023` would fail on a database that already has `10001`.
- **What changed:** `app.migrate` calls `goose.Up(db, ".", goose.WithAllowMissing())`.
- **Compatibility:** no effect on databases with only upstream migrations. Fork-only.
- **Upstreamable:** no (only needed because of the fork's numbering).
- **Tests:** `app/app_test.go`: the real migration set applies and is idempotent; a synthetic `00002` added after `10001` is refused without the option and applied with it. These tests need `CAL_TEST_DATABASE_URL` and skip without it.

### FC-005 — CalDAV UIDs scoped per account

- **Why:** `entries.external_uid` had a global unique index, and the CalDAV pull upsert keyed on it alone. The same event in two calendars (two accounts, same UID) collided. With more than one user, one account's sync could overwrite another user's entry that happened to share a UID.
- **What changed:**
  - Migration `10001_caldav_uid_per_account.sql` replaces `entries_external_uid_key` with a unique index on `(account_id, external_uid)`.
  - `UpsertSyncedEntry` targets that index and only updates when the existing row belongs to the same user.
- **Compatibility:** no data change; the new index is strictly looser than the old one. The down migration restores the old index and fails if duplicate UIDs now exist across accounts.
- **Upstreamable:** yes. The migration would be renumbered into upstream's sequence.
- **Tests:** `TestCaldavUIDScopedPerAccount`: same UID across users and across one user's accounts gives separate rows; re-sync updates in place; a write naming another user's account leaves their row untouched. The test helper gained `newTestStore`.

### FC-006 — HTML mail in a sandboxed frame, CSP for the app shell

- **Why:** HTML mail was injected into the app's own DOM after a regex clean-up that well-known markup gets past, e.g. `<img src="x"/onerror=…>` or entity-encoded `javascript:`. On the web the server's CSP blocked inline script. The Android/iOS WebView has no CSP, though, so a crafted message could run script with access to the native mail plugin, including its export of stored mail passwords. Remote images in mail also loaded on open, which lets senders track reads.
- **What changed:**
  - **Mail rendering** (`MailPage.tsx`, new `lib/mailframe.ts`):
    - HTML mail renders in `<iframe sandbox="allow-popups allow-popups-to-escape-sandbox" srcdoc=…>`, with no `allow-scripts` and no `allow-same-origin`, so it runs with an opaque origin.
    - The frame document carries its own CSP (`default-src 'none'; img-src data: cid:`), plus `<base target="_blank">` and `no-referrer`.
    - Remote images stay blocked until the reader clicks **Load remote images**, which is reset per message.
    - The regex sanitizer is removed.
  - **App-shell CSP** (`vite.config.ts`): the built `index.html` gets a `Content-Security-Policy` meta tag matching the server's policy, except `connect-src 'self' https:`, which native builds need to reach a user-chosen server. When the server header is present too, the browser enforces both. Build only, so the Vite dev server is unaffected.
  - **Native export** (`CalMailPlugin.exportAccounts`): it now shows a native "Copy N mail accounts, including passwords, to HOST?" dialog and rejects with `cancelled` unless confirmed. The TS side passes the destination host and shows "Import cancelled".
- **Compatibility:** the plugin method takes `{ destination }`; it's only called by the app's own import flow. Very long HTML mail now scrolls inside a fixed-height frame.
- **Upstreamable:** yes.
- **Tests:** `mailframe.test.ts` (CSP contents, no script permissions, remote-content detection). Also checked in headless Chromium with the real module and policy:
  - Six injection payloads (`onerror` after a slash, nested `<script>`, plain `<script>`, SVG `onbegin`, an entity-encoded `javascript:` link, meta refresh) are all inert.
  - No remote request is made until images are allowed, and then only the image loads.
  - The Android project compiles and its unit tests pass.

### FC-007 — Android: no backups, HTTPS only, no default keystore password

- **Why:** the Android shell allowed backups (the WebView storage holds the session token, and SharedPreferences hold the widget token and mail accounts), allowed cleartext HTTP everywhere, and `build.gradle` fell back to a hard-coded keystore password when the env vars were missing.
- **What changed:**
  - `AndroidManifest.xml`: `allowBackup="false"` and `fullBackupContent="false"`; new `res/xml/data_extraction_rules.xml` excludes all app data from cloud backup and device-to-device transfer on Android 12+; `usesCleartextTraffic` is replaced by `res/xml/network_security_config.xml` (`cleartextTrafficPermitted="false"`).
  - `capacitor.config.ts`: `androidScheme` `"http"` → `"https"`. The http scheme existed to reach plain-HTTP servers without mixed-content errors.
  - `MainActivity`: share deep links use `bridge.getLocalUrl()` instead of a hard-coded `http://localhost/`.
  - `build.gradle`: release signing is configured only when `keystore/cal-release.jks` exists **and** `CAL_STORE_PASSWORD` and `CAL_KEY_PASSWORD` are set. Otherwise the release APK is built unsigned, with a warning if the keystore is present.
- **Compatibility:**
  - The Android app can no longer connect to a plain-HTTP server; HTTPS is required.
  - Changing the scheme changes the WebView origin, so an existing install starts with empty local storage and needs to sign in again.
  - CI signing is unchanged (it already passes both env vars).
- **Upstreamable:** partly. Backups and the keystore change, yes. HTTPS-only is a policy choice upstream may not want, since it drops support for plain-HTTP LAN servers.
- **Tests:** `assembleDebug`, `assembleRelease` (unsigned without secrets) and `testDebugUnitTest` pass. `aapt2 dump` of the APK confirms the manifest flags.
