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

## Details

### FC-001 — Trust only configured reverse proxies

- **Why:** Gin trusts every proxy by default, so `ClientIP()` returned whatever the client put in `X-Forwarded-For`. The per-IP rate limits on login, registration, password reset, MCP and intake could be bypassed by sending a different header value on each request.
- **What changed:** `httpapi.New` calls `router.SetTrustedProxies` with the list from the new `TRUSTED_PROXIES` env var (comma-separated IPs/CIDRs). When it's unset, no proxy is trusted and the connecting peer is the client. An invalid entry stops startup with a clear error. Added to the README config table.
- **Compatibility:** deployments behind a reverse proxy should set `TRUSTED_PROXIES` to the proxy's address. Otherwise all clients share the proxy's rate-limit bucket. That's safe, just coarser.
- **Upstreamable:** yes.
- **Tests:** `ratelimit_test.go`: rotating `X-Forwarded-For` doesn't bypass the limiter; a trusted proxy gives per-client buckets and ignores prepended fake hops; env parsing.
