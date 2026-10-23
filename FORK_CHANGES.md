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
| [FC-019](#fc-019--github-sync-loop-reuses-one-ticker) | API | GitHub sync uses one ticker instead of one per iteration | yes |
| [FC-020](#fc-020--push-subscriptions-scoped-to-their-owner) | API | Push subscribe/unsubscribe can't take over or delete another user's endpoint | yes |
| [FC-021](#fc-021--no-property-injection-through-icalendar-urlimage) | ICS | CR/LF can't inject properties into `feed.ics` or CalDAV pushes | yes |
| [FC-022](#fc-022--note-links-and-images-limited-to-safe-urls) | web | Markdown links only for http(s)/mailto/same-origin; no protocol-relative images | yes |
| [FC-023](#fc-023--web-session-stays-in-the-httponly-cookie) | auth | Web client keeps no session token; `?session=` only authorizes GET/HEAD | yes |
| [FC-024](#fc-024--logout-clears-cached-personal-data) | web | Logout drops the entries snapshot and API cache; tokens never cached in localStorage | yes |
| [FC-025](#fc-025--all-workflow-actions-pinned-by-commit) | CI | Every GitHub Action is pinned to a commit SHA | yes |
| [FC-026](#fc-026--credential-key-outside-the-database-github-token-encrypted-and-write-only) | security | Encryption key moves to `DATA_DIR/secret.key`; GitHub token encrypted, write-only; tokens left out of exports/backups | yes |
| [FC-027](#fc-027--closing-linked-github-issues-is-opt-in) | API | Completing a card closes its GitHub issue only if `githubCloseIssues` is on | yes |
| [FC-028](#fc-028--one-token-per-capability) | auth | Separate tokens for MCP, email intake, ICS feed and widget | yes |
| [FC-029](#fc-029--sessions-stored-as-hashes) | auth | Session tokens stored as SHA-256; session list shows hashes, not tokens | yes |
| [FC-030](#fc-030--logout-ends-bearer-sessions-too) | auth | Native (Bearer) logout now invalidates the session server-side | yes |
| [FC-031](#fc-031--rename-and-recolour-subscribed-calendars) | feeds | `PATCH /api/feeds/:id` (name, colour); distinct default colours; ⋯ menu in the sidebar | yes |
| [FC-032](#fc-032--saving-an-existing-entry-from-the-editor) | API | Entry PATCH accepts `remind` and treats empty times as unset, so editor saves work | yes |
| [FC-033](#fc-033--tasks-without-a-date) | API, DB, web | Tasks may have no date: a "No date" group in Tasks, quick-add default, Schedule action | yes |

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

### FC-019 — GitHub sync loop reuses one ticker

- **Why:** `GitHubSyncLoop` created a new `time.Ticker` on every loop iteration. With Go 1.23+ (this module targets 1.26), unreferenced tickers are garbage-collected, so the practical cost was an allocation per wait. Under older toolchains, each one leaked.
- **What changed:** `httpapi/github.go` creates one ticker before the loop and stops it on exit.
- **Compatibility:** none.
- **Upstreamable:** yes.
- **Tests:** none specific. `go vet` and the existing suite pass.

### FC-020 — Push subscriptions scoped to their owner

- **Why:** subscribing reassigned an existing push endpoint to whoever posted it (`ON CONFLICT (endpoint) DO UPDATE SET user_id = …`), and `POST /api/push/unsubscribe` deleted an endpoint regardless of owner. With several accounts, a user who learned another's endpoint could silence that device or delete its subscription.
- **What changed:**
  - `store.UpsertPushSub` updates an existing endpoint only if it belongs to the same user, **or** the posted `p256dh`/`auth` keys equal the stored ones. That is the same browser subscription switching accounts, which keeps working as before.
  - Unsubscribe uses the new `DeleteUserPushSub(user, endpoint)`.
  - `DeletePushSub(endpoint)` stays for the reminder loop, which prunes endpoints the push service reports gone.
- **Compatibility:** unchanged for single-user setups and for account switching in one browser. A foreign subscribe or unsubscribe of someone else's endpoint is now a silent no-op (still 204).
- **Upstreamable:** yes.
- **Tests:** `TestPushSubscriptionOwnership`: a foreign subscribe with different keys and a foreign unsubscribe leave the owner's row alone, the same keys move it, and the owner can unsubscribe.

### FC-021 — No property injection through iCalendar URL/IMAGE

- **Why:** the iCalendar encoder (used by `/api/feed.ics`, the shared people calendar and CalDAV push) wrote `URL:` and `IMAGE:` values verbatim. A CR/LF in an entry's link URL therefore started a new property line, so arbitrary properties could be injected into the feed or the pushed object. TEXT values escaped `\n` but passed a bare `\r` through.
- **What changed:** `internal/ical/ical.go`.
  - New `uriValue` drops ASCII control characters from URI values. `URL` and `IMAGE` use it, are folded like other long lines, and are omitted when empty.
  - `escapeText` also turns `\r\n` and a bare `\r` into the escaped newline.
- **Compatibility:** normal URLs are unchanged. Long URLs are now folded per RFC 5545; parsers unfold them.
- **Upstreamable:** yes.
- **Tests:** `TestEncodeNoPropertyInjection`: CR, LF and CRLF in summary, description, URL and a long image URL produce no extra property lines and round-trip through `Parse`.

### FC-022 — Note links and images limited to safe URLs

- **Why:** the note renderer (`lib/markdown.tsx`) put any Markdown link target into `href`. `javascript:` is blocked by React 19 and the web CSP, but other schemes (`data:`, `intent:`, `vbscript:`) weren't filtered, and the Capacitor shell has no CSP. Images allowed any `src` starting with `/`, which includes protocol-relative `//host/…` and `/\host/…`, so they loaded from other hosts.
- **What changed:** `lib/markdown.tsx`.
  - New exported `safeHref`: only `http(s)://`, `mailto:` and same-origin paths become links. Anything else renders as plain text.
  - "Same-origin" means a single leading `/`, not `//` or `/\`. Image sources use the same rule.
- **Compatibility:** links with other schemes in notes now show as text.
- **Upstreamable:** yes.
- **Tests:** `lib/markdown.test.tsx` covers the allowed and rejected hrefs (including mixed case, leading space, `//` and `/\`), unsafe links rendered as text, and protocol-relative images not rendered.

### FC-023 — Web session stays in the HttpOnly cookie

- **Why:**
  - The web app is authenticated by the `cal_session` HttpOnly cookie, but the API client also stored the session token from the login response in `localStorage` and appended it as `?session=` to file and image URLs. Script running in the page could read the token, and it reached access logs and browser history through those URLs.
  - On the server, `?session=` authorized any request method, although it only exists for `<img src>` and file links in the native apps, which can't send headers.
- **What changed:**
  - `packages/api-client`: on a page served by the Cal server (not an app shell), the client keeps no token. `setSession` doesn't store it, a stale `cal:session` is removed on start, and `assetUrl` returns plain paths. App shells (Capacitor, Wails) keep the Bearer token and signed asset URLs as before.
  - `lib/markdown.tsx`: `/api/…` image paths get the server and session only when a native server URL is set.
  - `httpapi/server.go` `bearerOrQuery`: `?session=` is honoured only on GET/HEAD.
- **Compatibility:** web sessions continue through the cookie, so there's no re-login. Native apps are unchanged: they use Bearer headers for API calls and `?session=` only for GETs.
- **Upstreamable:** yes.
- **Tests:**
  - `lib/remote.test.ts`: on a plain web page no token is stored and asset URLs are unsigned; in the Capacitor shell the token is stored and URLs are signed.
  - `TestQuerySessionOnlyForReads`: GET with `?session=` → 200, PUT/POST with it → 401, Bearer POST works.

### FC-024 — Logout clears cached personal data

- **Why:** logging out removed only the cached user. Three things stayed in the browser:
  - the offline entries snapshot (`cal:entries`, all entries);
  - the service worker's `cal-api` cache of `/api/entries` and `/api/holidays` responses;
  - `cal:settings`, which also held the API, widget and people-share tokens.
- **What changed:** `lib/offline.ts`, `stores/planner.ts`.
  - Logout calls `clearCachedData()`, which removes the cached user and the entries snapshot and deletes the `cal-api` Cache Storage bucket.
  - `cacheSettings` blanks `widgetToken`, `apiToken` and `peopleShareToken` before writing. The server returns them with the settings on every load. The rest of the settings cache, such as the theme for the login screen, is kept.
- **Compatibility:** after logout, an offline reload no longer shows the previous user's entries. The pending offline write queue (`cal:opq`) is left as is, so unsynced edits aren't discarded.
- **Upstreamable:** yes.
- **Tests:** `lib/offline.test.ts`: tokens never reach `cal:settings`; logout clears the user and entries, keeps the theme, and deletes the `cal-api` cache.

### FC-025 — All workflow actions pinned by commit

- **Why:** `ci.yml`, which runs on every push to `main` and on pull requests, and the manual release workflows referenced third-party actions by movable major tags (`@v7`). Whoever controls an action's repository can repoint such a tag. `fork-image.yml` was already pinned (FC-009).
- **What changed:** every `uses:` in `ci.yml`, `android.yml`, `release.yml` and `auto-release.yml` is pinned to the commit its major tag pointed at on 2026-10-21, with the exact version in a comment. The commits match those already in `fork-image.yml` where the actions overlap.
- **Compatibility:** none. Updating an action now means changing the SHA deliberately.
- **Upstreamable:** yes.
- **Tests:** `actionlint` v1.7.7 passes on all workflows. Each SHA was resolved with the GitHub API (`commits/<tag>`) and matched to its `vX.Y.Z` tag.
- **Not changed:** `release.yml` still fetches AppImage tooling from moving `continuous` builds. That workflow is manual-only in the fork (FC-009) and isn't used to publish anything.

### FC-026 — Credential key outside the database; GitHub token encrypted and write-only

- **Why:**
  - The AES-256-GCM key for stored credentials (CalDAV, CardDAV and mail passwords, Google tokens) was kept in `server_config`, in the same database as the ciphertexts, so any database dump or backup decrypted them. It was also generated lazily, so two concurrent first uses could each create a key.
  - The GitHub token was stored in plaintext, returned to the browser in every settings response, and included in `/api/export` and the nightly `DATA_DIR/backups/*.json`. Those also contained the API, widget and people-share tokens.
- **What changed:**
  - **Key file** (`store/crypto.go`, `app/app.go`). The key lives in `CAL_SECRET_KEY_FILE`, default `<DATA_DIR>/secret.key` (mode 0600, base64). `store.InitSecrets` runs once at startup, under an advisory lock:
    - A missing file is created. If the database still has the legacy `crypto_key`, that key is adopted as is, with no re-encryption. Otherwise a new random key is generated.
    - A provided key file that differs from the legacy key gets every encrypted column re-encrypted to it, in one transaction.
    - The legacy key is then deleted from the database, and a key fingerprint (`secret_key_fingerprint`, a truncated SHA-256) is stored.
    - With a missing or different key file once credentials exist, startup fails with a clear error instead of generating a key that can't decrypt them.
    - `Encrypt`/`Decrypt` fail with `ErrNoSecretKey` until `InitSecrets` has run.
  - **GitHub token.**
    - Migration `10002` adds `settings.github_token_enc`. `InitSecrets` encrypts existing plaintext tokens into it and clears `github_token`.
    - The token is write-only: `PUT /api/settings/github-token {token}` and `DELETE /api/settings/github-token`. Settings responses carry `githubConnected` instead of the token.
    - `PUT /api/settings` no longer writes it, and accepts and ignores a `githubToken` field from older clients.
    - Server-side readers use `store.GitHubToken`: GitHub inbox, MCP `github_inbox`, the sync loop and card auto-close.
  - **Exports and backups.** `/api/export` and the nightly backup files blank the API, widget and people-share tokens (`exportSettings`). Restore never read settings back, so nothing is lost.
  - **Web.** The Settings → Integrations GitHub field is a password input with Save and Remove buttons and shows "saved" when a token is stored. The GitHub page checks `githubConnected`.
  - **Docs.** `CAL_SECRET_KEY_FILE` is in the README config table. The `deploy/compose.yml` header notes that `cal-data` now holds `secret.key`. `openapi.yaml` documents the new endpoint.
- **Compatibility:**
  - Existing installs migrate automatically on first start. The key moves to `<DATA_DIR>/secret.key`, so back up the data volume together with the database: a database restored without that file can't decrypt stored credentials.
  - Rolling back to an older build after this migration needs the old key back in `server_config`.
  - Migration `10002` Down drops the encrypted GitHub token, which then has to be entered again.
  - API: settings carry `githubConnected` instead of `githubToken`.
- **Upstreamable:** yes.
- **Tests:** `secrets_test.go`:
  - Legacy database (key in `server_config`, an encrypted CalDAV password, a plaintext GitHub token) with no key file: the key is adopted into a 0600 file, the password still decrypts, the token is encrypted, and the legacy key is gone.
  - The same database with a different provided key: values are re-encrypted.
  - A restart with the same key works, while a different or missing key file returns `ErrSecretKeyMismatch` and creates no new file.
  - `TestGitHubTokenWriteOnly`: set, the `githubConnected` flag, a full settings save with `githubToken` that doesn't change it, no token in `/api/settings` or `/api/export`, no API or widget token in the export, invalid token → 400, and remove.

### FC-027 — Closing linked GitHub issues is opt-in

- **Why:** completing a board card linked to a GitHub issue always closed that issue on GitHub with the user's token. It's a write to an external service that users may not expect, made whenever a GitHub token was set for reading the inbox.
- **What changed:**
  - Migration `10003` adds `settings.github_close_issues BOOLEAN NOT NULL DEFAULT false`, exposed as `githubCloseIssues`.
  - `updateEntry` closes the linked issue only when it's on.
  - The web Settings → Integrations section gets the switch "Close the GitHub issue when its card is completed".
  - `ghAPI` is a variable so tests can point it at a stub.
- **Compatibility:** behaviour change. Existing users who relied on auto-close must turn it on. Reading GitHub (inbox, card sync from GitHub to Cal) is unchanged.
- **Upstreamable:** yes.
- **Tests:** `TestGitHubCloseIssueIsOptIn`, against a stub GitHub API: completing a linked card sends no PATCH with the setting off and exactly one with it on.

### FC-028 — One token per capability

- **Why:** two tokens covered four capabilities.
  - The API token was the only credential for both `/api/mcp` (full read/write over entries, people, files and the GitHub inbox) and `/api/intake`, which is meant to be handed to a mail-forwarding service. Leaking the intake URL therefore leaked the whole account.
  - The widget token, described as read-only for the Today widget, also unlocked `/api/feed.ics`, the export of all tasks and events with descriptions.
- **What changed:**
  - Migration `10004` adds `settings.intake_token` and `settings.feed_token`, seeded from `api_token` and `widget_token` respectively.
  - `store.UserByToken(kind, token)` and `store.RegenerateToken(kind)` replace the per-token functions. Kinds are `TokenAPI` (MCP), `TokenIntake` (`/api/intake`, create-only), `TokenFeed` (`/api/feed.ics`) and `TokenWidget` (`/api/widget/today`). An empty token never matches.
  - New rotate endpoints `POST /api/settings/intake-token` and `/api/settings/feed-token`. One `rotateToken` handler serves all four.
  - `/api/intake` also accepts the token as `Authorization: Bearer`.
  - Exports and backups blank the new tokens too, and the web settings cache never stores them.
  - Web Settings: the intake URL is shown with its own copy and rotate buttons, and the ICS URL uses the feed token with its own rotate button.
  - `openapi.yaml` is updated.
- **Compatibility:**
  - Existing intake and ICS URLs keep working, because the new tokens start as copies.
  - Rotating the API token no longer changes the intake URL, and rotating the widget token no longer changes the ICS URL.
  - API: settings gain `intakeToken` and `feedToken`.
- **Upstreamable:** yes.
- **Tests:**
  - `TestTokensAreScoped`: the four tokens are distinct, and each one opens only its own endpoint (intake, feed, widget, MCP). Intake also works with a Bearer header, and rotation retires the old intake token.
  - `TestScopedTokenMigrationKeepsURLs`: on a database migrated to `10003` with an existing user, `10004` seeds `intake_token = api_token` and `feed_token = widget_token`.

### FC-029 — Sessions stored as hashes

- **Why:** sessions were stored as the raw token: the `sessions.id` UUID that clients send as cookie or Bearer. Anyone with read access to the database or a dump could sign in as any user. `GET /api/sessions` also returned those IDs, so the session list handed every one of the user's tokens to any script running in the page.
- **What changed:**
  - Migration `10005` changes `sessions.id` to `TEXT` and replaces each existing value with `encode(sha256(token), 'hex')` in place, so current logins stay valid.
  - The store hashes the presented token everywhere (`sessionKey`): `CreateSession` returns the token and stores only its hash, and `UserBySession`, `TouchSession`, `DeleteSession` and `RevokeOtherSessions` hash their argument.
  - `ListSessions` returns the hash as the session's public ID, and `RevokeSession` takes that ID. The hash can't be used to sign in.
- **Compatibility:**
  - Nobody is signed out by the upgrade.
  - Session IDs in `/api/sessions` are now 64-character hex strings. The web UI only uses them as keys for revoking.
  - Down migration: hashes can't be reversed, so it deletes all sessions and everyone signs in again.
- **Upstreamable:** yes.
- **Tests:**
  - `TestSessionsListedByHash`: the list doesn't contain the token, the current entry's ID is SHA-256(token), and revoking by that ID ends the session.
  - `TestSessionHashMigrationKeepsLogins`: on a database at `10004` with an old-style session row, migrating stores the hash and the pre-upgrade token still signs in.

### FC-030 — Logout ends Bearer sessions too

- **Why:** `POST /api/auth/logout` deleted the server-side session only when it arrived as the `cal_session` cookie. The native apps authenticate with a Bearer token, so their logout only cleared local state and the token stayed valid for up to 30 days.
- **What changed:** `httpapi/server.go` `logout` ends the session presented either as the cookie or as `Authorization: Bearer`.
- **Compatibility:** none for the web app. Native logouts now also end the session on the server.
- **Upstreamable:** yes.
- **Tests:** `TestBearerLogoutEndsSession`: after a Bearer logout, the token gets 401. Before the fix it still got 200.

### FC-031 — Rename and recolour subscribed calendars

- **Why:** subscribed calendars (feeds) got a colour only when created, and the web app never sent one, so every feed was `sky` and they couldn't be told apart. There was no way to rename or recolour a feed afterwards.
- **What changed:**
  - **API.**
    - New `PATCH /api/feeds/:id` with `{name?, color?}` (`store.UpdateFeed`). The name is trimmed and 1–80 characters. The colour must be one of the palette tokens. Any other field, including `url`, is rejected with 400, so a feed's source can only change by deleting and re-adding it, which keeps every URL on `createFeed`'s checks. Another user's feed gives 404.
    - A feed's events take its colour when they're read, so recolouring applies to every past and future event of that feed.
    - `POST /api/feeds` validates a given colour. Without one, it picks the first palette colour no other feed of the user has (`nextFeedColor`), instead of always `sky`.
    - The palette list (`paletteColors`, `httpapi/colors.go`) is now one list, shared with the MCP `create_entry` schema.
  - **Web.**
    - New `FeedMenu` popover with a name field and colour dots. A colour applies on click, and the name saves on Enter or when the popover closes. Escape discards a typed name.
    - It opens from a ⋯ button on each sidebar "Calendars" row (shown on hover, always shown on touch screens), from right-clicking the row, or from the feed's colour square in Settings → Feeds, which wasn't styled and so was invisible before.
    - Updates are optimistic (`applyFeedPatch` in `lib/feeds.ts`), so the sidebar and the calendar's feed events change at once, and they're rolled back if the server refuses.
    - The popover is portalled to `<body>`, because the transformed mobile sidebar drawer would otherwise anchor `position: fixed`.
  - **Docs.** `openapi.yaml` documents the PATCH and the colour enum. There's also a missing space fixed in the Settings feeds note.
- **Compatibility:** additive API. Existing feeds keep their colour. An unknown colour on create is now a 400 (it used to be stored and rendered unstyled).
- **Upstreamable:** yes.
- **Tests:**
  - `TestFeedRenameAndRecolour`: two new feeds get different colours, an unknown colour on create gives 400, and a PATCH trims the name and sets the colour, which the feed's events then carry. An empty name, unknown colour or `url` field gives 400, and an unknown or foreign feed gives 404.
  - `TestNextFeedColor`: no repeats until the palette is used up.
  - `lib/feeds.test.ts`: the optimistic patch only touches the right feed and its events.
  - Checked by hand in headless Chromium at desktop and phone sizes: ⋯ on hover and touch, recolour visible on the event immediately, rename on Enter, Escape discards, the Settings square opens the menu, and the menu stays inside a 390 px screen.

### FC-032 — Saving an existing entry from the editor

- **Why:** every save of an existing entry from the editor failed with 400 "invalid entry", for two reasons:
  - The editor sends `remind` with every save (a number, or `null` for no reminder), and `openapi.yaml` documents it on `EntryPatch`, but the PATCH handler rejected any unknown key and had no `remind` case.
  - The editor sends `startTime` and `endTime` as `""` for an untimed entry. `""` clears a time, but `UpdateEntry`'s end-after-start check treated it as set, and `"" <= ""` failed the check.
- **What changed:**
  - `parsePatch` takes `remind`: an integer from 0 to 10080 (the range the `entries_remind_check` constraint allows) sets it, and `null` removes it (`EntryPatch.ClearRemind`). Anything else gives 400.
  - Changing or removing the reminder resets `reminded_at`, the same as changing the start time, so a new reminder can fire.
  - `UpdateEntry` treats an empty start or end time as unset when it checks the merged row, the same way the database stores it (NULL).
  - `openapi.yaml` gives the `remind` range and what `null` means.
- **Compatibility:** PATCHes that used to fail now succeed. No data change.
- **Upstreamable:** yes.
- **Tests:**
  - `TestPatchRemind`: setting 15 and then `null` round-trips, and -1, 10081 and a string give 400.
  - `TestPatchEmptyTimes`: an editor-style save of an untimed entry succeeds, and so do setting and clearing both times. An end without a start, or an end before the start, still gives 400.

### FC-033 — Tasks without a date

- **Why:** every entry needed a date, so a to-do with no particular day had to be given a fake one (usually today), where it then showed as overdue. Task apps commonly keep such tasks in an undated "inbox" list instead.
- **What changed:**
  - **Database.** Migration `10006`:
    - `entries.date` and `entry_revisions.date` become nullable.
    - A new check, `entries_undated_check`, allows a NULL date only on a task with no start time, end time, repeat or reminder, since each of those needs a day to anchor to.
    - Two settings columns are added, `quick_add_today` and `undated_first`. Both default to false, so a client that doesn't send them keeps the defaults.
    - Rolling back gives undated tasks their creation day and drops undated revisions.
  - **API.**
    - `Entry.date` is omitted for undated tasks. `POST /api/entries` accepts a task without `date` under the same rules as the check. Events, notes and links still need one. A CalDAV-synced entry can't be undated.
    - `PATCH` with `"date": null` (or `""`) makes a task undated. If the merged entry would break the rule, it gives 400.
    - `GET /api/entries` with a range leaves undated tasks out. `undated=1` adds them, and a query without a range always includes them. Search, export/restore, revisions, MCP and webhooks carry them.
    - Undated tasks are skipped by `feed.ics`, the reminders, the morning digest, habit streaks and every date-range view. The agenda markdown lists open undated tasks under "No date".
    - MCP: `create_entry` no longer requires `date` for tasks. `update_entry` takes `date: null` or `""`. `list_entries` gains `undated`, which adds undated tasks to a ranged query.
    - Email intake: a missing or invalid `date` still means today, and `"date": null` creates an undated task.
    - Settings: `quickAddToday` (quick-added tasks whose text names no day are dated today instead of left undated) and `undatedFirst` (the No date group is listed first instead of last).
  - **Web.**
    - **Tasks page.** Open undated tasks get a "No date" group, last by default. Each of those rows has a Schedule button with Today, Tomorrow, Next week (the start of next week) and a date picker.
    - **Quick-add.** Text that names no day creates an undated task, unless `quickAddToday` is on. A time alone ("standup 9:30") still means today.
    - **Editor.** Tasks have a "No date" checkbox, which hides the time, repeat and reminder fields. If any of those are set, it asks before removing them. Clearing a task's date field does the same. Switching an undated task to another type fills in a date.
    - **Today page.** A collapsed "No date · N" list of open undated tasks.
    - **Settings → General.** The two settings above.
    - **Other pages.** Calendar views, the sidebar counts, boards, tags, trash and the command palette handle entries without a date. `formatDayShort` and `formatDayTitle` say "No date".
  - **Docs.** `openapi.yaml` (the rules, `undated`, the nullable PATCH `date`, the intake `null`, the settings) and the README feature list.
- **Compatibility:**
  - API contract change: `date` may be absent on task entries, and clients that assume it's always there need to handle that.
  - Quick-add behaviour change: text without a day used to mean today. The old behaviour is the `quickAddToday` setting.
  - No existing data changes.
- **Upstreamable:** yes.
- **Tests:**
  - `TestUndatedTaskRules`: an undated task is created. An undated event or note, or an undated task with a time, repeat or reminder, gives 400. A ranged list leaves undated tasks out, `undated=1` and an unranged list include them, and search finds them. The two settings round-trip.
  - `TestUndatedTaskPatch`: schedule, clear (refused while a time is set, allowed together with clearing the time), `""` also clears, an event can't lose its date and an undated task can't become an event. Revisions keep the NULL date, and restoring dated and undated revisions both work.
  - `TestUndatedTaskExports`: `feed.ics` leaves undated tasks out and the agenda has "No date". Export, restore preview and restore into a fresh database keep the task undated.
  - `TestUndatedTaskMCPAndIntake`: MCP create, schedule, unschedule, list with and without a range or `undated`. An undated MCP event fails. Intake with a missing, `null` or given date.
  - `TestUndatedMigration`: the check rejects undated events and undated tasks with a time, repeat or reminder. Down to `10005` dates undated tasks by creation day, and Up again works.
  - Web unit tests: `groupTasks` No date bucket and its order, `compareDates`, quick-add without a day and with a time only, `formatDayShort`/`formatDayTitle` without a date.
  - Checked in headless Brave at desktop and phone sizes:
    - quick-add into No date, Upcoming and Today;
    - Schedule → Tomorrow;
    - the editor's No date checkbox on a plain task and on a timed task (with the confirmation);
    - Pick a date, and a switch to Event filling in a date;
    - the Today page's No date list;
    - both settings, and the calendar with undated tasks loaded;
    - no horizontal scroll at 390 px.
