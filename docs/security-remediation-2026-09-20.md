# Security remediation record — 20 September 2026

Implemented in both repositories on `security/audit-remediation-2026-09-20`. The application uses one private library and a single admin-provisioned Cognito owner account. The owner subsequently authorized production deployment; see the [rollout record](security-rollout-2026-09-20.md) for deployed controls and live verification. Owner contact details and account-specific recovery inventory are maintained privately.

## Findings and fixes

| Finding | Implemented correction | Rollout requirement (status in rollout record) |
|---|---|---|
| F1: browser key grants full access | Cognito authorization code + PKCE, admin-only account creation, owner-group authorization, verified issuer/audience/RS256/expiry/ID-token claims in Go and Cognito at API Gateway; no API-key fallback | Provision owner; deploy both repos; retire old keys and frontend assets |
| F2: client URLs choose deletion target | Server-generated immutable UUID-bound storage keys; no image URL/key inputs; typed parent validation; deletion constrained to one bucket and `images/*` | Migrate any legacy image records before deleting them |
| F3: unenforced upload constraints | Authenticated raw uploads capped at 4 MiB; decode/re-encode real image bytes; dimension/pixel/output caps; atomic daily count/byte budget; immutable S3 writes | Deploy API binary media configuration and new upload client |
| F4: unauthenticated development listener | Mandatory JWT authentication, loopback-only binding, bounded timeouts/headers and exact CORS | Local changes take effect on next server restart |
| Public images / tracking | Private S3, five-minute signed reads, exact frontend image origin, four-minute image-query refresh; legacy public URLs never rendered | Apply bucket policy/public blocks and check any legacy source buckets |
| Unbounded data operations | Strict bounded JSON and fields, typed strongly consistent Gets, bounded queries, explicit limit errors and Lambda envelope size checks | Deploy backend; large libraries may need pagination beyond current limits |
| Recovery / excessive runtime permissions | DynamoDB PITR, retained resources, S3 versioning/encryption, scoped database/object permissions | Back up before rollout; recovery settings protect future history and incur storage charges |
| Frontend hardening | CSP and other hosting headers, session-scoped tokens, cache clearing, refresh-token rotation/revocation, no foreign image origins, environment-file ignores | Verify actual Amplify headers and environment configuration |
| Vulnerable dependencies | Go minimum 1.26.8, patched frontend dependency lockfile and compatible OIDC/image libraries | Rebuild/redeploy; previous artifacts keep their previous dependencies |

Existing category/deck/card IDs, content and FSRS history are retained. Media migration is dry-run first, allowlists source buckets/prefixes, preserves originals, bounds reads, conditionally attaches keys, and verifies destination bytes when resuming an interrupted copy. No production migration was applied.

## Verification

- `go test ./...` and `go vet ./...`: passed.
- `go test -race ./...`: passed; the subsequent response-envelope fix also passed the full ordinary suite and a real Lambda adapter regression.
- `sam validate --lint`: passed.
- Frontend: 18 test files / **183 tests passed**, TypeScript/Vite production build passed. Standard `npm ci` succeeded during the initial remediation; this follow-up changed no dependencies.
- `npm audit`: **zero reported vulnerabilities**.
- `govulncheck v1.8.0`: **no vulnerabilities found** in the full backend and Linux/ARM64 Lambda-rooted source scans using Go 1.26.8. The target scan ran the native scanner with `go run -exec 'env GOOS=linux GOARCH=arm64' ... ./cmd/lambda`.
- Whitespace checks passed in both repos. New tests use local fakes, signed test JWTs and image fixtures; they do not attack production.
- Independent read-only review identified logout revocation, read-after-create consistency and escaped proxy-response sizing issues. All were corrected and rechecked; no remaining critical or important source finding was reported.
- The usability/cost follow-up repeated the full frontend suite and build, backend tests/vet, SAM lint and header YAML validation. Regression tests cover real OIDC popup state/PKCE handling, draft/file recovery, locked reconnects, interrupted uploads, signed-image reuse and private caching. A separate source review found no remaining blocking issue; live Cognito/browser popup behavior and actual Amplify headers still require deployment smoke checks.

Live checks were read-only: the media migration dry run found zero typed image records in one scan page; a separate scan found neither of the two table records missing `entity_type`; the configured media bucket had zero current objects. No private card text, credentials or media were printed. Existing deployment settings confirmed the previously public bucket, missing PITR/versioning and automatic Amplify deployment.

The historical [audit report](security-audit-2026-09-20.md) and exploit probes remain as baseline evidence. The probe runner now refuses the remediated checkout; passing the old probes would mean the original insecure behavior was present. The current regression suite is the normal Go/frontend test suite.

## Remaining rollout and practical limits

The initial local fixes were followed by an authorized rollout using the [production runbook](security-deployment-2026-09-20.md). Fresh builds, the reviewed change set, backup, maintenance, owner provisioning, coordinated release and key retirement are recorded in the [rollout record](security-rollout-2026-09-20.md), together with completed and remaining deployed smoke checks.

Drain old Lambda invocations and let old 15-minute signed upload grants expire, or explicitly block their writes, before the final backup. The default wait procedure needs at least 16 minutes before backup plus deployment time. Keep the service unavailable and storage private on failure; never roll back to shared-key/public-image access.

The new browser session refreshes automatically for at most one day. Temporary renewal failures get bounded retries while the token is valid. Expiry locks the UI and API access while keeping drafts and cached data in tab memory; same-owner popup reauthentication restores them. Explicit logout or an unauthorized/different account discards them. Reloading or closing the tab loses unsaved drafts. Logout immediately clears local data and makes a bounded best-effort revocation call; network failure can leave the refresh token usable until expiry or administrative revocation. Copied JWTs and signed image URLs can remain valid for five minutes. Images allow 60 seconds of private browser caching, so already downloaded copies are not instantly erased by logout. Optional MFA requires explicit enrollment; it is not automatically active.

Uploads lose animation and metadata, have a 4 MiB input limit and a 100-upload/100-MiB normalized-data daily quota. Lists reject results over 1000 items, 50 pages or 4 MiB rather than silently dropping records; escape-heavy responses can reach the Lambda envelope bound earlier. Private orphan objects may require reconciliation after uncertain writes, and deleted S3 versions remain recoverable for up to 30 days.

No security audit guarantees absence of all vulnerabilities or past misuse. Historical access/log investigation and live owner-login validation remain deployment follow-up; previously downloaded public data cannot be recalled.

## Usability and cost follow-up

The owner requested these refinements after reviewing the original remediation. The upload architecture remains authenticated backend uploads. No additional service or public media access is introduced.

- Recover temporary renewal failures and retain drafts in memory during a locked session; use same-owner popup reauthentication to avoid navigating away.
- Suspend background queries and hidden-dialog Escape handlers while locked. Resume only stale image metadata automatically, so reauthentication does not overwrite text drafts with a fresh record fetch.
- Retain a newly saved card's pending image batch across an authorization rejection. Explicit continuation uses the saved card ID and confirmed progress; an uncertain server result requires inspecting the saved card instead of blindly replaying an upload. Discard/logout releases local preview URLs.
- Disable paid API Gateway method metrics while retaining ordinary stage metrics.
- Cache content-hashed frontend assets for a year; revalidate the app shell and known routes, and keep private API responses uncached.
- Briefly cache image responses privately and retain a successfully displayed immutable image across signed-URL refreshes. Metadata renewal continues, but signature changes alone no longer force repeated image downloads.
- Keep recovery settings, five-minute signed URLs, image quotas and the image-decoding memory allocation. This reduces avoidable monitoring/bandwidth usage; it is not a fixed billing cap.
