# Security production rollout — 20 September 2026

The owner authorized this rollout. Both repositories' security remediation, session usability refinements and aged npm upgrades are included. This public record preserves technical outcomes; account identifiers, owner contact details and backup identifiers are maintained in private operational records.

## Deployed resources

- Deployment region: `ap-southeast-1`.
- CloudFormation `flashcard-prod`: `UPDATE_COMPLETE`; fresh Go 1.26.8 ARM64 build deployed and the live bootstrap bytes verified against the local build.
- DynamoDB `flash-card-app-prod` and S3 `flash-card-app-media-prod` updated in place, without replacement.
- Amplify `main`: source commit `4af8f29d38dcf3cc18a81f500ccbcda055a045e3`; release job **9** succeeded with verified Cognito/API/media environment settings.
- Backend source commit `8b096f9` and frontend source changes pushed to their respective GitHub `main` branches.
- A single Cognito owner account was created with invitation suppressed; the owner set the permanent password through a local masked prompt. No password was logged or stored by the deployment tooling. Pool and public client identifiers were verified against stack outputs.

## Preservation and maintenance

- Recorded private deployment/configuration recovery snapshots outside both repositories.
- Set Lambda concurrency to zero, waited beyond its previous timeout, and verified an existing legacy presigned PUT returned `403 AccessDenied` after installing an explicit bucket deny.
- The permanent template retains the legacy `question-images/*` / `answer-images/*` PUT deny, so those grants cannot become usable again during policy updates.
- Created and verified an `AVAILABLE` DynamoDB backup; its name and ARN are recorded privately.
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
- Owner login completed in Chrome and the existing library loaded. Production returns to `/auth/callback/`; the frontend now accepts the exact callback with or without its trailing slash. Regression tests reproduced the failure before the fix. All **187 frontend tests** and the production build passed afterward.
- Created an isolated temporary category, deck and card through the UI; updated the card; completed a study review; deleted the temporary category and its descendants. A strongly consistent DynamoDB read then matched all original record contents exactly: **two original records, zero remaining test records**.
- A full page reload retained the owner session and successfully fetched the original library. The browser was left open on that library.
- DynamoDB TTL is enabled on `expires_at`.

## Verification limits

The live image-upload check could not select its synthetic PNG because the Chrome extension does not have file-URL access. No test media was uploaded. Image validation, image authorization and session-failure scenarios have automated coverage; their full browser smoke scenarios were not all repeated against production.

Direct HTTP header/artifact checks encountered the existing Amplify hosting-password gate. It was already enabled in the pre-deployment app snapshot and remains unchanged; Chrome's existing access reaches the app. The encrypted credential returned by Amplify cannot be reused as the plaintext hosting password. Header configuration was verified through Amplify settings; on-wire headers behind the gate were not independently inspected.

The initial automatic frontend build (job 7) ran before the new environment was applied. Job 8 rebuilt after configuration; job 9 includes the callback fix and is the current release. An AWS CLI stdin-JSON parsing incompatibility was resolved by using the installed AWS SDK for environment/password setup.

The local password setup program remains in the private deployment snapshot directory; it contains no password. Original app/database state and presigned diagnostic URLs were not committed to Git.
