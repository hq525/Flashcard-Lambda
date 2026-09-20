# Security remediation implementation plan

**Goal:** Close every audited issue for the existing single-owner deployment with tested code and an explicit migration/cutover runbook.

**Spec:** `docs/superpowers/specs/2026-09-20-security-remediation-design.md`.

The user has authorized the fixes and confirmed single-owner access. Independent authentication and frontend work run alongside storage and deployment work under the parallel-agent workflow. Changes stay on security branches; production changes are a separate reviewed step.

- [x] Authentication: add tested Cognito JWT owner verification and configuration, fail-closed application integration, loopback defaults and finite server timeouts.
- [x] Frontend: add PKCE login/logout/session handling, remove API keys, update image uploads and ordering requests, refresh signed reads and reject foreign image origins, fix environment ignores and hosting headers, patch dependency versions, verify tests/build/audit.
- [x] Storage: reproduce forged deletions and upload-policy failures; replace with server-validated image bytes, generated immutable IDs/keys, typed parent validation, private read signing, bounded quota, and safe deletion. Verify with fake AWS transports and real image fixtures.
- [x] Request/data bounds: strict bounded JSON/fields, typed entity reads, bounded query loops and response sizes; preserve review concurrency and cascade semantics.
- [x] Migration: dry-run-first media command validates explicit source allowlists, preserves originals, conditionally records managed keys, and resumes safely. Test malformed sources, duplicates, conflicts, and already migrated records.
- [x] Infrastructure/runbook: add owner-only Cognito, exact origins, private versioned media, least-privilege IAM, PITR and retained resources; document backup, maintenance, owner provisioning, migration, coordinated deploy, old-key revocation and safe rollback. Inspect current deployed resource names read-only.
- [x] Verification: full Go tests/vet, frontend tests/build, SAM validation, npm audit, Go vulnerability scans; review authorization, migration, image and deployment interactions and resolve failures.

Review focus: owner-group claims alone are insufficient without cryptographic token verification; logout must invalidate browser data and future requests; an existing storage key must match its record ID; invalid migrated media must never cause an arbitrary AWS request; old frontend/API credentials must not remain a compatibility bypass; no list or cascade may silently truncate data.

Completed local remediation and review. Production rollout remains a separate step: see `docs/security-deployment-2026-09-20.md`; this checklist does not mark deployment or migration as executed.
