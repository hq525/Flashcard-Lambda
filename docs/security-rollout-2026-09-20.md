# Security production rollout — 20 September 2026

Authorized by the owner in this session. Both repositories' security remediation, session usability refinements and aged npm upgrades are included.

## Deployed resources

- AWS account `725020099811`, region `ap-southeast-1`.
- CloudFormation `flashcard-prod`: `UPDATE_COMPLETE`; fresh Go 1.26.8 ARM64 build deployed and the live bootstrap bytes verified against the local build.
- DynamoDB `flash-card-app-prod` and S3 `flash-card-app-media-prod` updated in place, without replacement.
- Amplify app `d21qooye31nta2`, `main`: source commit `db1e4695084208352a49dc18ca864d44aec6e48c`; release job **8** succeeded with verified Cognito/API/media environment settings.
- Live app: https://main.d21qooye31nta2.amplifyapp.com/
- Sole Cognito owner: `zhaohanqing96@gmail.com`; pool `ap-southeast-1_4DchsrDuF`, public client `1ndjces5ihpbn7ce55c99oj8lf`. Account created with invitation suppressed; the owner set the permanent password through a local masked prompt. No password was logged or stored by the deployment tooling.

## Preservation and maintenance

- Recorded private deployment/configuration recovery snapshots outside both repositories.
- Set Lambda concurrency to zero, waited beyond its previous timeout, and verified an existing legacy presigned PUT returned `403 AccessDenied` after installing an explicit bucket deny.
- The permanent template retains the legacy `question-images/*` / `answer-images/*` PUT deny, so those grants cannot become usable again during policy updates.
- Created and verified an `AVAILABLE` DynamoDB backup: `flashcard-pre-security-20260920T060414Z`.
- Frozen inventory: **two records, no current media objects**. Both records contained `entity_type`. Media migration dry run: one page, zero images, zero errors; no data migration required.
- After deployment, both original records were unchanged in a strongly consistent DynamoDB read.
- Restored Lambda reserved concurrency to **5** after the secured backend and frontend release were ready.

## Verified controls

- Anonymous API calls, invalid tokens and the retired browser key return `401`; CORS headers and authenticated-request preflight are correct.
- Legacy API key deleted by CloudFormation. API methods use Cognito. Runtime auth issuer/client configuration matches the new pool and client.
- S3 public access blocks, private bucket policy, TLS enforcement, legacy-upload deny, versioning, encryption and DynamoDB PITR enabled.
- One pool user and one `owner` member; self-registration disabled. Optional TOTP remains available but was not enrolled automatically.
- Amplify public configuration updated at app level, conflicting branch overrides removed, old `VITE_API_KEY` removed. Current build spec and security/cache headers supplied to Amplify.
- The production frontend build passed with its actual public environment settings. Chrome renders the owner-only sign-in screen and reaches the correct Cognito authorization-code + S256 PKCE flow.

## Remaining live checks

Owner sign-in and authenticated CRUD/upload/session smoke checks are in progress. Direct HTTP header/artifact checks encountered the existing Amplify hosting-password gate. It was already enabled in the pre-deployment app snapshot and remains unchanged; Chrome's existing access reaches the app. The encrypted credential returned by Amplify cannot be reused as the plaintext hosting password.

The initial automatic frontend build (job 7) ran before the new environment was applied. Job 8 rebuilt the same source after configuration and is the intended release. An AWS CLI stdin-JSON parsing incompatibility was resolved by using the installed AWS SDK for environment/password setup.

The local password setup program remains in the private deployment snapshot directory; it contains no password. Original app/database state and presigned diagnostic URLs were not committed to Git.
