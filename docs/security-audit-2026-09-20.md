# Security audit — both flashcard repositories

> Historical baseline report. The findings below describe backend commit `725eb35a3832fdc36987bd853686709ee932bc03` and frontend commit `df024f5b3c33f4f02220b22e42e85a71b28f068f`, before the uncommitted security fixes. Source line references may have moved. See the [remediation record](security-remediation-2026-09-20.md) for current code status and the [production cutover](security-deployment-2026-09-20.md) for outstanding deployment work. The original probes assert insecure behavior and are restricted to the baseline checkout.

Audited on 20 September 2026. The highest risk is the authentication design: the frontend distributes the same API key that grants unrestricted access to the backend. Two independent storage issues allow callers to select objects for deletion and obtain uploads without enforced content-type or size limits.

| ID | Severity | Finding | Scope |
|---|---|---|---|
| F1 | Critical, if deployed with the documented public hosting | Browser-distributed API key grants full data access | Both repositories |
| F2 | High | Client-controlled image URLs select S3 deletion targets | Backend |
| F3 | High | Upload grants enforce neither image content type nor size | Backend; frontend limit is bypassable |
| F4 | Medium | Development API exposes AWS-backed operations on every interface | Backend |

These are confirmed code/configuration findings. Public reachability, deployed credentials, and effective AWS policies were not tested. Severity assumes the single-user application's records should be restricted to its owner. An independently configured access gateway could reduce F1's exposure; no such control is declared in either repository.

Scope: `Flashcard-Lambda` at `725eb35a3832fdc36987bd853686709ee932bc03` and `flashcard-frontend` at `df024f5b3c33f4f02220b22e42e85a71b28f068f`. Both working trees were clean at the start. Reviewed the current API, persistence and cascade flows, recent FSRS changes, browser rendering and API calls, SAM/Amplify configuration, dependency versions, and locally available Git history. No application code, dependency versions, AWS resources, or deployment settings were changed.

**F1 — Browser-distributed API key grants full data access**

Evidence: [frontend config](/Users/zhaohanqing/Documents/GitHub/flashcard-frontend/src/api/config.ts:9), [request header](/Users/zhaohanqing/Documents/GitHub/flashcard-frontend/src/api/client.ts:30), [Amplify build](/Users/zhaohanqing/Documents/GitHub/flashcard-frontend/amplify.yml:12), [API Gateway auth](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/template.yaml:23), and [router](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/internal/httpapi/router.go:62).

The production bundle includes `VITE_API_KEY`, and all API calls use it as `X-Api-Key`. SAM requires that key but configures no identity authorizer. The application has no authenticated principal or owner checks. Anyone able to download the hosted JavaScript can recover the key, list categories, follow category/deck/card IDs, read all content and review history, change records, and invoke cascading deletes. UUIDs do not prevent enumeration because list endpoints supply the IDs.

Validation: built the real frontend with an intentionally fake key, `SECURITY_AUDIT_PUBLIC_SENTINEL`, and found it in the generated JavaScript. A local router probe also returned category data with no authentication headers and an arbitrary Origin. The router probe does **not** claim API Gateway accepts requests without its required key; the public bundle supplies that key. This behavior follows [Vite's documented environment handling](https://vite.dev/guide/env-and-mode). [AWS explicitly advises against API keys as authentication or authorization](https://docs.aws.amazon.com/apigateway/latest/developerguide/api-gateway-api-usage-plans.html).

Fix: require a verified identity at API Gateway or an authenticated backend proxy. For this single-user app, allowlist the owner's immutable identity; supporting multiple users would additionally require ownership on records and every child access. Remove the privileged key from browser code. After closing the access path, rotate the old key and invalidate old bundles. Restricting CORS or rotating a key that is still bundled does not establish authentication. Acceptance checks should cover anonymous access, another signed-in identity, and access to every CRUD, review, and presign route.

Heal suitability: requires an authentication design decision and coordinated frontend/backend rollout; unsuitable for automatic patching without that decision.

**F2 — Client-controlled image URLs select S3 deletion targets**

Evidence: [image request fields](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/internal/models/request.go:62), [record creation](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/internal/persistence/repository.go:59), [cascade deletion](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/internal/service/cascade.go:101), [S3 delete implementation](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/internal/storage/s3.go:54), and [legacy bucket permissions](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/template.yaml:78).

Image create/update requests require only a nonempty `imageURL`. The backend does not bind it to a server-issued upload or validate the parent on creation. Deleting that image record parses its supplied URL into a bucket and key, then executes `DeleteObject` using the Lambda role. A caller can create a throwaway record pointing at a known object and delete the record to delete that object. Changing an existing record's URL has the same effect. This reaches keys unrelated to the record, including legacy bucket objects covered by the role's broad delete permissions.

Validation: a local API probe accepted an image record referencing a nonexistent parent and an unrelated legacy-bucket URL; deleting the record forwarded exactly that URL to storage. A second probe used the real AWS SDK with an in-memory HTTP transport and observed `DELETE flash-card-app-answer-images.s3.us-east-1.amazonaws.com/unrelated/private-object.png`, even though the configured bucket was different. No AWS request was made.

Impact is limited by the effective IAM and bucket policies and requires knowing the target key. This is an S3 deletion confused-deputy issue, not arbitrary HTTP SSRF or permission to delete from every AWS bucket.

Fix: store a server-controlled upload/object identifier and immutable bucket/key binding, authorize its parent and owner, and derive deletion targets from that binding. Restrict the role and storage implementation to explicitly configured buckets and prefixes. Migrate legacy URLs through a validated mapping. A bucket allowlist alone does not prevent selecting someone else's object within that bucket. Acceptance checks should reject forged keys, unrelated upload IDs, nonexistent parents, and out-of-scope legacy objects.

Heal suitability: an explicit bucket/prefix allowlist is a bounded containment patch; complete upload ownership binding needs a small data/API design change.

**F3 — Upload grants enforce neither image content type nor size**

Evidence: [presign validation](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/internal/httpapi/presign.go:23), [S3 signing](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/internal/storage/s3.go:36), [frontend size limit](/Users/zhaohanqing/Documents/GitHub/flashcard-frontend/src/features/cards/CardCreateDialog.tsx:24), [direct S3 upload](/Users/zhaohanqing/Documents/GitHub/flashcard-frontend/src/api/resources.ts:110), and [public bucket policy](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/template.yaml:193).

The handler checks only an `image/` prefix supplied by the caller. It accepts SVG and invented MIME types and requires neither an upload size nor an existing parent. More significantly, the pinned S3 SDK removes Content-Type during PUT presigning: the actual generated URL has `X-Amz-SignedHeaders=host`. Consequently the client can change the PUT's Content-Type, including to a non-image type. The README's assertion that content type is signed is incorrect. The SDK also uses an unsigned payload, and this code supplies no length restriction.

Validation: generated a presigned URL locally using dummy credentials; signed headers were exactly `host` and expiry was 900 seconds. The pinned [AWS SDK source](https://github.com/aws/aws-sdk-go-v2/blob/service/s3/v1.101.0/service/s3/api_op_PutObject.go) calls `RemoveContentTypeHeader` during `PresignPutObject`. No real upload or oversized payload was sent.

A caller with API access can bypass the UI's 10 MiB limit, upload arbitrary content to a publicly readable bucket, and consume S3 storage/request/transfer resources outside the API Gateway/Lambda limits. Grants can also be reused while valid. [AWS documents presigned URL reuse](https://docs.aws.amazon.com/AmazonS3/latest/userguide/using-presigned-url.html) and that [usage-plan quotas are best-effort, not hard cost caps](https://docs.aws.amazon.com/apigateway/latest/developerguide/api-gateway-api-usage-plans.html). Active content would run on the S3 origin if opened as a document; this audit did not establish JavaScript execution on the SPA origin.

Fix: issue authenticated, parent-bound upload grants with an S3-enforced size policy, such as presigned POST with `content-length-range`, or an upload service that enforces length. Restrict MIME types and validate/decode the actual content before making it available. Keep pending uploads private, expire abandoned uploads, and enforce per-user byte/count quotas. If PUT remains, explicitly enforce and test its signed constraints; a client-supplied MIME declaration alone does not validate image bytes. Coordinate the frontend contract with the backend.

Heal suitability: requires a coordinated upload-contract change; a frontend-only patch would not mitigate the issue.

**F4 — Development API exposes AWS-backed operations on every interface**

Evidence: [server default](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/cmd/server/main.go:15), [Makefile override](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/Makefile:7), and [AWS wiring](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/internal/app/app.go:20).

Both the server's default and `make run` bind `:8080`. This development entry point uses actual configured DynamoDB/S3 resources but has none of API Gateway's key checks. While it is running, a network peer that can reach port 8080 can perform the same reads and destructive operations without any credential. The configured AWS identity and resource names determine the blast radius. The host firewall/network can reduce exposure; neither was assessed.

Validation: inspected both bind settings and confirmed unauthenticated application access using the local router probe. The audit did not start an AWS-connected server or probe the LAN.

Fix: default both paths and the documented command to `127.0.0.1:8080`; require explicit opt-in for remote binding and authenticated access when needed. Restrict development CORS to intended local origins and use isolated development resources. Loopback alone does not replace authentication against other local processes.

Heal suitability: yes, the bind-default/Makefile/documentation change is bounded and readily verifiable.

**Deployment and hardening observations**

- Image confidentiality — the template deliberately disables policy-level public blocking and grants anonymous `s3:GetObject` on all objects ([policy](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/template.yaml:179)). Anyone holding an image URL can read it independently of future API authentication. This is a medium privacy risk if screenshots/notes are private; public media may be an intentional product decision. UUID keys reduce guessing, not access by a holder of the URL. Use a private bucket with authorized short-lived downloads or appropriately protected CloudFront delivery. A distribution with unrestricted public downloads would not make the images private. Heal needs the intended media-access policy and a migration decision.
- Resource limits — generic CRUD bodies lack the review route's explicit byte limit, and list/history queries collect all pages into memory ([handlers](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/internal/httpapi/handlers.go:66), [query loop](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/internal/persistence/store.go:151)). Add body/field limits, bounded pagination, and request deadlines. AWS service limits provide some bounds; no memory-exhaustion exploit was demonstrated. The comments describing hard cost caps should also be corrected.
- Recovery — table retention protects stack deletion/replacement, but the template does not enable DynamoDB point-in-time recovery or S3 versioning. Retention does not undo application-level deletion. Consider recovery controls alongside F1/F2 remediation.
- Frontend defense in depth — no CSP/frame policy is declared in the hosting files, and `.gitignore` omits plain `.env` and some environment variants. Actual response headers can be configured outside Git and were not inspected. No committed secret was detected. Restrict permitted image origins as part of the storage fix; current `<img>` elements accept stored external URLs, allowing third-party tracking requests.

These observations are separate from the four demonstrated application/configuration findings and should not be counted as demonstrated exploits.

**Dependency assessment**

The available compiler is Go **1.26.2**. `govulncheck v1.8.0` reported 11 distinct advisories at symbol level across the full repository and 10 for a Linux ARM64 scan rooted at `cmd/lambda`. All symbol-level matches were in the standard library; no third-party module had a symbol-level match. JSON mode exited zero despite findings, so its exit status was not treated as a clean result.

Treat this as a **medium toolchain maintenance finding**, not 10 or 11 proven production exploits. Several conditions do not apply: the Windows NUL-byte advisory does not affect Linux; ECH is not configured; the Lambda does not expose the local net/http listener; outbound clients normally contact AWS; and the reported `net/url` issue concerns relative resolution while the image parser calls `url.Parse`. Symbol reachability through shared interfaces does not establish attacker control over the vulnerable operation. Deployed binary/compiler versions were not inspected.

The build [uses the locally available Go compiler](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/Makefile:3), while [go.mod](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/go.mod:3) specifies only `go 1.26`. Update and pin the build toolchain to a current supported patch, rebuild, and rerun the scan; Go's [release feed](https://go.dev/dl/?mode=json) lists **1.26.8** for the existing release line at audit time. Raising the minimum/pinning the compiler is suitable for bounded remediation, with an explicit rebuild/deployment follow-up. The evidence JSON preserves advisory IDs and fixed-version data.

Frontend `npm audit` reported **6 affected package entries: 4 high and 2 moderate**. This includes transitive/effect entries and is not six independent exploitable vulnerabilities. The installed dependency tree matched the locked versions below.

| Package | Locked version | Remediation floor for reported advisories | Application relevance |
|---|---|---|---|
| `react-router` | 7.18.1 | 7.18.2 | Production dependency, but the advisory requires unstable RSC APIs; this app uses client-side `BrowserRouter`. No applicable RSC attack path found. |
| `postcss` | 8.5.16 | 8.5.23 | Build dependency through Vite. File disclosure requires attacker-controlled CSS/source-map input; user card text is not compiled as CSS. |
| `nanoid` | 3.3.15 | 3.3.18 | Build dependency through PostCSS. Reported loops require invalid generator sizes; no user-controlled generator size found. |
| `undici` | 7.28.0 | 7.29.0 | Test dependency through jsdom. Cache/retry/cookie/body advisories; no production Node server or application cache interceptor configured. |
| `vitest`, `@vitest/mocker` | 4.1.9 | 4.1.11 | Test dependencies. Standalone mocker/interceptor plugins are not installed in the Vite config; tests use jsdom. |

Version floors are for the advisories returned on the audit date, not a guarantee against future advisories. Primary details: [React Router](https://github.com/remix-run/react-router/security/advisories/GHSA-qwww-vcr4-c8h2), [PostCSS](https://github.com/postcss/postcss/security/advisories/GHSA-fxqj-rqcc-2cmp), [Undici](https://github.com/nodejs/undici/security/advisories/GHSA-4cwx-7wf7-3272), [Vitest](https://github.com/vitest-dev/vitest/security/advisories/GHSA-82fw-gwwq-j7x9). npm labels the Router match high while its maintainer advisory labels it moderate; neither establishes impact in this SPA. Refresh the lockfile to patched compatible versions, review changes, then run tests/build/audit. This is suitable for bounded automated remediation without forcing major upgrades.

**Validation and evidence**

| Check | Result |
|---|---|
| `go test ./...` | Passed all backend test packages |
| `go vet ./...` | Passed |
| `npm test -- --reporter=dot` | 15 files, 102 tests passed; existing duplicate React-key and unmatched MSW-handler warnings were emitted |
| Production frontend build with dummy API configuration | Passed TypeScript and Vite build; fake key present in JavaScript |
| Five temporary security probes | Confirmed absent application auth, forged image deletion flow, permissive presign input, missing signed constraints, and caller-selected S3 bucket/key |
| Gitleaks Git-history scans with full redaction | No detected secrets in 19 backend / 27 frontend commits |
| Gitleaks backend directory scan | No detected secrets |
| npm and Go vulnerability database queries | Completed; findings triaged above |

The [evidence directory](/Users/zhaohanqing/Documents/GitHub/Flashcard-Lambda/docs/security-audit-2026-09-20) contains the local-only probes, a runner using a temporary Go overlay, and a compact dependency result snapshot. Run `python3 docs/security-audit-2026-09-20/run_probes.py` from the backend repository. These are diagnostic probes that assert the audited behavior; passing means the behavior remains present, not that the application is secure. They use dummy credentials/mocks and do not perform AWS network I/O. Go may need its normal local build cache or module downloads.

Positive controls observed: DynamoDB expressions use bound attribute values; typed conditional writes protect cross-entity update/delete operations; reviews enforce a 4 KiB body limit, reject unknown fields/trailing JSON, calculate timestamps/schedules server-side, and use atomic revision/idempotency checks. React renders content as text, with no `dangerouslySetInnerHTML` or dynamic code execution found in application source. These controls do not address F1's missing identity boundary.

Limits: this was a source/configuration audit with local validation, not a live penetration test. AWS account controls, effective deployed IAM, Amplify access settings/headers, logs, data sensitivity, historical incidents, and deployed artifact versions remain unverified. Secret scanning is detection-based and does not prove no secret has ever leaked. Fix F1 first, contain F2/F3, restrict local binding, then patch dependencies and resolve the media-privacy decision.
