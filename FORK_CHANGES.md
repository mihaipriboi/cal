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
| [FC-008](#fc-008--compose-builds-the-image-locally) | deploy | `docker-compose.yml` always builds the app image from the checkout | no |
| [FC-009](#fc-009--ci-manual-image-publishing-no-automatic-releases) | CI | Manual, pinned workflow publishes `ghcr.io/mihaipriboi/cal`; upstream release workflows are manual-only | no |
| [FC-010](#fc-010--deploy-compose-for-the-published-image) | deploy | `deploy/compose.yml` runs the published image at a pinned tag | no |
| [FC-011](#fc-011--uploads-served-by-sniffed-type-unsafe-types-download) | API | Uploaded files are served by sniffed type; anything that could run script downloads instead of rendering | yes |
| [FC-012](#fc-012--cors-exact-origin-matching) | API | CORS matches origins by parsed host instead of string prefix | yes |
| [FC-013](#fc-013--android-mail-requires-tls-and-checks-the-host) | Android | Native mail requires STARTTLS on plain ports and verifies the certificate hostname | yes |
| [FC-014](#fc-014--optional-network-restriction-for-the-mail-api) | API | Optional `MAIL_ALLOWED_NETS` limits the Mail API to listed networks | yes |
| [FC-015](#fc-015--address-check-at-connect-time-for-user-supplied-urls) | API | Link previews, feeds and webhooks check the address actually dialed, on every redirect | yes |
| [FC-016](#fc-016--deploy-compose-follows-latest) | deploy | `deploy/compose.yml` follows `latest`; updating is `docker compose pull && docker compose up -d` | no |
| [FC-017](#fc-017--image-build-fetches-and-runs-less) | build | No embedded-Postgres download and no npm install scripts during the image build | yes |
| [FC-018](#fc-018--mcp-card-tools-check-the-columns-board) | API | MCP `create_card`/`move_card` only accept a column of the card's board | yes |

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

### FC-008 — Compose builds the image locally

- **Why:** this fork is deployed from its own source. Upstream's compose file names `ghcr.io/dvorinka/cal:latest` with `pull_policy: missing`, so a plain `docker compose up -d` would run upstream's published image instead of the fork's code.
- **What changed:** the `cal` service uses `image: cal-local:${CAL_TAG:-dev}` with `pull_policy: build`, so Compose always builds from the checkout. The header explains tagging per commit (`CAL_TAG=$(git rev-parse --short HEAD)`) to keep older builds for rollback. A commented `TRUSTED_PROXIES` example was added (see FC-001).
- **Compatibility:** fork-only. `install.sh` is unchanged and still targets upstream (it downloads upstream's compose file and pulls its image). For the fork, use `docker compose up -d --build` from a checkout instead.
- **Upstreamable:** no.
- **Tests:** none possible without Docker on the dev machine; verify with `docker compose config` and `docker compose up -d --build` on the deploy host.

### FC-009 — CI: manual image publishing, no automatic releases

- **Why:** the fork publishes its own container image and releases nothing automatically. Upstream's `auto-release.yml` bumps the version, commits, tags and dispatches the release pipelines on every push to `main`, and `release.yml`/`android.yml` publish on any `v*` tag. Enabling Actions on the fork would publish releases under the fork on the next push, including tags fetched from upstream.
- **What changed:**
  - `auto-release.yml`, `release.yml` and `android.yml` are triggered only by `workflow_dispatch` (a comment says why).
  - New `fork-image.yml` (manual, `main` only) builds the Dockerfile and pushes `ghcr.io/<owner>/cal` tagged `sha-<commit>`, plus optionally `latest` and a custom tag.
    - It runs with `contents: read` and `packages: write` permissions, and the checkout keeps no persisted credentials.
    - Every third-party action is pinned to a commit SHA.
    - It records max-mode provenance and an SBOM, and prints the tag and digest in the job summary.
  - `ci.yml` (tests and the image smoke build) is unchanged.
- **Compatibility:** fork-only.
- **Upstreamable:** no.
- **Tests:** `actionlint` v1.7.7 passes on all changed workflows.

### FC-010 — Deploy compose for the published image

- **Why:** a host should be able to run the fork from its published image without a source checkout or a local build, and never pick up a new version implicitly.
- **What changed:** new `deploy/compose.yml` and `deploy/.env.example`.
  - The `cal` image is `ghcr.io/mihaipriboi/cal:${CAL_TAG}`, and `CAL_TAG` is required, so there's no floating `latest`.
  - It uses the external Postgres service.
  - `SESSION_SECURE=true`, and `WEB_ORIGIN`, `TRUSTED_PROXIES` and `TZ` come from `.env`.
  - The root `docker-compose.yml` (build from source) is unchanged.
- **Compatibility:** fork-only, additive.
- **Upstreamable:** no.
- **Tests:** checked on the deploy host with `docker compose config` and `up -d` (no Docker on the dev machine).

### FC-011 — Uploads served by sniffed type; unsafe types download

- **Why:** `/api/files/<name>` and shared file links were served with an extension-derived type and `Content-Disposition: inline` on the app's own origin. An uploaded HTML or SVG file could therefore run script with the user's session when opened.
- **What changed:** `httpapi/files.go`, new `serveUpload` used by both `serveFile` and `serveSharedFile`.
  - The type comes from the file's first 512 bytes (`http.DetectContentType`), not the name or the stored MIME.
  - Images, audio/video, PDF and plain text are served inline. Anything else (HTML, SVG, XML, scripts, unknown) is `application/octet-stream` with `Content-Disposition: attachment`.
  - Non-PDF responses carry `Content-Security-Policy: sandbox; default-src 'none'; …`. PDFs skip `sandbox` because browsers won't run their PDF viewer in a sandboxed document.
  - The filename in the header has quotes, backslashes and CR/LF stripped.
- **Compatibility:** opening an uploaded HTML/SVG file now downloads it instead of rendering it. Image previews and PDFs behave as before.
- **Upstreamable:** yes.
- **Tests:** `files_serve_test.go` covers HTML, SVG, HTML bytes with a `.png` name, PNG, text and PDF, plus header sanitising.

### FC-012 — CORS exact origin matching

- **Why:** the CORS check accepted any origin *starting with* `http://localhost`, `http://127.0.0.1` and so on, so `http://localhost.example.com` passed. It also accepted any `capacitor://` or `ionic://` host.
- **What changed:** `httpapi/server.go`, new `allowedOrigin`.
  - `WEB_ORIGIN` must match exactly.
  - Otherwise the origin is parsed. `capacitor://localhost` and `ionic://localhost` are allowed, and so are `http(s)` origins whose hostname is exactly `localhost`, `127.0.0.1` or `::1` (any port).
  - Origins with userinfo, a path, a query or a fragment are rejected.
- **Compatibility:** no change for the web app, the Capacitor apps or local dev servers.
- **Upstreamable:** yes.
- **Tests:** `TestAllowedOrigin` (table of allowed and rejected origins).

### FC-013 — Android mail requires TLS and checks the host

- **Why:** the native mail client (JavaMail 1.6) enabled STARTTLS but didn't require it, so a network attacker could strip the upgrade and see credentials in cleartext. JavaMail 1.6 also doesn't verify that the server certificate matches the hostname unless told to.
- **What changed:** `MailUtil.tlsProps`, used for both IMAP and SMTP in `CalMailPlugin`.
  - Plain ports set `starttls.enable` and `starttls.required`.
  - `ssl.checkserveridentity=true` unless the account's existing "insecure" flag is set. That flag keeps its previous meaning: trust-all plus no hostname check, for self-signed servers.
- **Compatibility:** servers that don't offer STARTTLS on a plain port now fail to connect instead of falling back to cleartext.
- **Upstreamable:** yes.
- **Tests:** `MailUtilTest` (three `tlsProps` cases).

### FC-014 — Optional network restriction for the Mail API

- **Why:** the Mail module holds IMAP/SMTP credentials that can read, delete and send mail. Some deployments want it usable only from the local network or a VPN, even when the rest of Cal is reachable from the internet.
- **What changed:**
  - New env `MAIL_ALLOWED_NETS`: comma-separated IPs/CIDRs, parsed at startup, where invalid values stop the server.
  - When set, every `/api/mail/*` route returns 403 ("Mail is only available from your local network or VPN") unless `ClientIP()` is inside one of the networks. `ClientIP()` is the same value rate limiting uses, so it honors `TRUSTED_PROXIES`.
  - Unset means no restriction (previous behavior).
  - Added to `deploy/compose.yml`, `deploy/.env.example` and the README config table.
- **Compatibility:** opt-in, so there's no change unless the variable is set. The native Android mail client talks to the mail server directly and isn't affected.
- **Upstreamable:** yes.
- **Tests:** `TestRequireClientNet` covers LAN direct, VPN via the proxy, internet via the proxy, a spoofed left-most `X-Forwarded-For`, an untrusted peer, IPv6, and the unset case.

### FC-015 — Address check at connect time for user-supplied URLs

- **Why:** link previews, feed subscriptions and webhooks fetch URLs supplied by the user. The private-address guard (`checkURL`) resolved the hostname and checked the answer, but the HTTP client then resolved it again to connect. A DNS server that answers differently the second time (DNS rebinding) could steer the request to an internal address. Webhook delivery also used a client with Go's default redirect policy: it followed up to 10 redirects without re-checking them, and kept the POST body on 307/308. The checked ranges also missed shared address space (`100.64.0.0/10`), `0.0.0.0/8` and IPv6 translation prefixes such as NAT64.
- **What changed:** new `httpapi/outbound.go`, used by `unfurl.go`, `feeds.go` and `webhooks.go`.
  - `newOutboundClient` builds a client whose dialer refuses blocked addresses in `net.Dialer.Control`. The check runs on the IP actually being connected to, on every connection, so it covers each redirect hop and changed DNS answers.
  - The blocked list (`blockedNets`) covers private, loopback, link-local, shared (CGNAT), multicast and reserved IPv4 space, and the IPv6 equivalents plus NAT64, Teredo and 6to4. IPv4-mapped IPv6 addresses are checked as IPv4.
  - Redirects: at most 3, and only to `http`/`https`. This applies to link previews, feeds and webhooks alike.
  - Guarded clients ignore `HTTP_PROXY`/`HTTPS_PROXY`. Through a proxy, the dialed address is the proxy's, so the check would mean nothing.
  - `checkURL` is kept as an early check with a readable error (for example when a webhook is added). It uses the same list.
  - `CAL_ALLOW_PRIVATE_FEEDS` and `CAL_ALLOW_PRIVATE_WEBHOOKS` keep their meaning. When set, feeds or webhooks use a client with the same timeouts and redirect limit but no address check. Link previews have no opt-out, as before.
- **Compatibility:**
  - Webhooks now follow at most 3 redirects (was 10), never to blocked addresses.
  - Link previews follow up to 3 redirects (was 2).
  - URLs in the newly listed ranges, for example a Tailscale `100.x` address, now need the matching opt-out.
  - With `CAL_ALLOW_PRIVATE_FEEDS` set, a feed that redirects within the private network now works. Before, the redirect check refused it.
  - Operator-configured servers (CalDAV, CardDAV, Invidious) aren't affected.
- **Upstreamable:** yes.
- **Tests:** `outbound_test.go`:
  - `TestIsBlockedIP`: a table of blocked and allowed addresses, including mapped and NAT64 forms.
  - `TestOutboundClientRefusesBlockedDial`: the dial itself is refused, with no `checkURL` involved.
  - `TestOutboundClientRefusesBlockedRedirect`: a POST redirected (307) from an allowed host to a blocked one never reaches the second server.
  - `TestOutboundClientRedirectLimits`: the hop count and non-http schemes.
  - `TestDeliverWebhookPrivateOptOut` and `TestFetchICSPrivateOptOutFollowsRedirect`: both opt-outs.

### FC-016 — Deploy compose follows `latest`

- **Why:** with `CAL_TAG` required (FC-010), every update meant editing `.env`. A moving tag alone wouldn't have helped: with `pull_policy: missing`, `docker compose pull` skips images already present ("Image is already present locally").
- **What changed:**
  - `deploy/compose.yml` uses `ghcr.io/mihaipriboi/cal:${CAL_TAG:-latest}` and no longer sets `pull_policy`, so `docker compose pull && docker compose up -d` updates both Cal and `postgres:16-alpine` (patch releases).
  - `latest` still moves only when the manual "Fork image" workflow runs on `main` (FC-009), so a host changes only when someone pulls.
  - `CAL_TAG` is optional. Set it to a `sha-<commit>` tag to hold a version or roll back.
  - The compose header documents the update, rollback and prune steps and a pre-update `pg_dump`. `.env.example` and the workflow summary text match.
- **Compatibility:** hosts that already set `CAL_TAG` keep that pin until they clear it.
- **Upstreamable:** no.
- **Tests:**
  - `docker compose config` resolves the image to `:latest` with `CAL_TAG` unset or empty, and to the pinned tag when set.
  - Docker Compose 5.6.0 confirms `pull` re-pulls a present tag without `pull_policy`, and skips it with `pull_policy: missing`.
  - `actionlint` v1.7.7 passes on `fork-image.yml`.

### FC-017 — Image build fetches and runs less

- **Why:**
  - The Dockerfile booted the server once during the build to "pre-warm" the embedded Postgres. That downloads about 80 MB of executables from Maven Central, whose checksum check is skipped if the checksum can't be fetched, and bakes an initialised cluster into a layer. Deployments with an external database (`DATABASE_URL`, as in both compose files) never use it.
  - `npm ci` also ran package install scripts, which no dependency needs on Linux. The only one is `fsevents`, which is macOS-only and not installed.
- **What changed:** `Dockerfile`.
  - The pre-warm runs only with `--build-arg PREWARM_EMBEDDED_PG=true`. The default is `false`.
  - The web stage uses `npm ci --ignore-scripts`.
  - The root `docker-compose.yml` comment notes that a single-container setup downloads the binaries on first start unless the image was pre-warmed.
- **Compatibility:**
  - Images with an external database behave the same and are smaller.
  - A container started *without* `DATABASE_URL` now downloads the embedded Postgres on first boot (needs network), unless built with the argument.
- **Upstreamable:** yes.
- **Tests:** `docker build` of the image, with the build log checked for no Maven traffic, and a smoke run against an external Postgres.

### FC-018 — MCP card tools check the column's board

- **Why:** the MCP `create_card` and `move_card` tools accepted any `columnId`, unlike the REST handlers (`createEntry`, `moveCard`) and the shared-board move. An agent could place a card on one board in a column of another.
- **What changed:** `httpapi/mcp.go`. Both tools call `ColumnInBoard` and return a tool error ("column not on this board") otherwise. `create_card` checks only when a column is given.
- **Compatibility:** no change for valid calls.
- **Upstreamable:** yes.
- **Tests:** `TestMCPCardColumnMustBelongToBoard` (`mcp_cards_test.go`): cross-board create and move are refused, same-board ones work.
