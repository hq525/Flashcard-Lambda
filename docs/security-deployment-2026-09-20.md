# Private library: production security cutover

Prepared 20 September 2026 for a single owner's private library. The authorized deployment is documented in the [rollout record](security-rollout-2026-09-20.md). This public runbook uses example frontend URLs and template-default resource names; obtain actual identifiers from your own stack outputs and keep owner contact details and recovery inventory privately. Verify the AWS account, region and resources before running any command. Do not replay the legacy cutover on an already upgraded stack.

## Historical baseline before the cutover

Read-only inspection before remediation found the following. These are historical observations, not the current deployed controls:

| Setting | Before remediation |
|---|---|
| Lambda | Reserved concurrency 5 |
| DynamoDB | Point-in-time recovery disabled |
| S3 | Public bucket policy, versioning not enabled |
| Amplify | Automatic builds enabled; existing app-level hosting-password protection preserved during the cutover |
| Amplify variables | `VITE_API_BASE_URL`, `VITE_API_KEY` |
| Current SPA rewrite | `/<*>` → `/index.html`, status `404-200` |

The metadata-only media dry run completed one scan page with **zero image records and zero errors**. A separate bounded scan examined both table records and found none missing `entity_type`. S3 listing also returned **zero current objects**, with no further page. These are observations at inspection time, not a freeze or a guarantee of future state. Repeat before rollout. The image scanner selects typed records; older manually managed tables need `cmd/backfill` first if `entity_type` is missing.

The exposed browser key must be treated as public. Rebuilding the frontend alone cannot revoke it or protect existing public media. Historical access has not been investigated; the absence of current images does not prove that no data was previously accessed.

## 1. Prepare and review before maintenance

Keep both changes on security branches. **Pushing frontend `main` automatically deploys.** Finish the coordinated backend rollout before releasing that branch. Retain a copy of the previous deployment artifacts for diagnosis, but do not use them to reopen unauthenticated access.

Run the repository checks, then build a fresh backend artifact with Go 1.26.8 or newer. Do not reuse stale `.aws-sam/build` output. The approved rollout included a fresh SAM build and verified the deployed binary against it.

```bash
go test ./...
go vet ./...
sam validate --lint
go version
sam build
sam deploy --stack-name flashcard-prod --region ap-southeast-1 --resolve-s3 --capabilities CAPABILITY_IAM --parameter-overrides StageName=prod FrontendOrigin=https://flashcards.example.com --no-execute-changeset
```

Review the resulting change set before executing it. The table and media bucket must be **updated in place**, preserving names and logical IDs. Expected changes are Cognito resources; authenticated API methods; new Lambda code/configuration/permissions; removal of old API-key/usage-plan resources; private, encrypted and versioned media; DynamoDB recovery/TTL; exact CORS; and throttling. Stop on any unexpected resource replacement or deletion. `Retain` protects a resource from deletion but does not make accidental replacement an acceptable migration.

Backups, version retention and Cognito can change AWS charges. Method-level paid CloudWatch metrics are disabled; ordinary stage metrics remain available. Hashed frontend assets use long-lived caching, while private API JSON stays `no-store` and images permit only 60 seconds of private browser caching. Existing API/Lambda throughput settings and upload budgets are guardrails, not guaranteed account spending caps.

## 2. Freeze legacy access and back up

During the maintenance window, stop old writes before taking the final backup. The existing application will be temporarily unavailable.

```bash
aws lambda put-function-concurrency --function-name flashcard-backend-prod --reserved-concurrent-executions 0 --region ap-southeast-1
aws s3api put-public-access-block --bucket flash-card-app-media-prod --region ap-southeast-1 --public-access-block-configuration BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true
```

Reserved concurrency zero blocks new invocations; it does not stop work already executing. **Drain the old function before backup:** wait at least its configured 30-second timeout plus margin (use one minute), and verify no active invocations remain. Previously issued S3 PUT URLs also bypass Lambda and public-access blocking. Wait the old grant lifetime of **15 minutes after that drain** before final media inventory/backup, or explicitly deny the old Lambda role's legacy-prefix writes in the bucket policy and verify the deny first. Merely disabling the API key does not revoke signed S3 URLs. The default wait-based procedure therefore needs at least 16 minutes before backup, in addition to deployment time.

Once old invocations and upload grants can no longer write:

```bash
aws dynamodb create-backup --table-name flash-card-app-prod --backup-name flashcard-pre-security-cutover --region ap-southeast-1
```

Record the returned backup ARN privately and use `describe-backup --backup-arn <arn>` until its status is `AVAILABLE`. Use a unique backup name on later attempts. Preserve the deployment configuration and record metadata outside public Git history. The new PITR setting protects future history; it does not retroactively create a restore point for the old deployment.

Repeat the media inventory while writes are frozen. If there are no objects, record the empty inventory. Otherwise, create and verify a **private** backup with a recovery identity before continuing; enabling versioning now does not recover previously deleted versions. Do not delete original media as part of this rollout.

Disable the legacy API key(s) belonging to this stack. Identify key IDs through the stack's resources and usage-plan associations; do not print key values or disable unrelated applications' keys. A known ID can be disabled with:

```bash
aws apigateway update-api-key --api-key <this-stack-key-id> --patch-operations op=replace,path=/enabled,value=false --region ap-southeast-1
```

Keep the function blocked and bucket private if any subsequent step fails. Check these controls after CloudFormation rollback: rollback may restore older insecure template settings.

## 3. Apply the authenticated backend

Execute the reviewed change set and wait for `UPDATE_COMPLETE`. Do not substitute a stale SAM build. Confirm all API methods now use Cognito, the backend has the new auth environment variables, and the bucket policy has no anonymous allow. All four S3 public-access blocks must be enabled.

Manual reserved concurrency is drift: CloudFormation may leave it at zero or reset it during an update. Check after deployment, and keep it at zero until owner provisioning, frontend preparation and any media migration are ready. Even if reset, the new API/backend must reject anonymous requests; there is no temporary API-key compatibility path.

Read the public stack outputs:

```bash
aws cloudformation describe-stacks --stack-name flashcard-prod --region ap-southeast-1 --query 'Stacks[0].Outputs' --output json
```

Outputs include `UserPoolId`, `AuthIssuer`, `AuthClientId`, `AuthDomain`, `MediaOrigin`, `ApiUrl`, `TableName` and `BucketName`. The new Cognito pool/client must be the same values in API Gateway, Lambda and the frontend.

## 4. Provision only the owner

Create the intended owner's account in the new pool and add only that account to `owner`. Admin-only signup is already in the template. Never assign `owner` to a general user group or enable self-registration. AWS administrators who can manage this pool remain trusted administrators of the library.

For a rollout without an invitation email, use the Cognito console or `AdminCreateUser` with `MessageAction=SUPPRESS`, the owner email and a temporary password entered through a masked prompt. Do not place passwords in shell arguments, source files, tool messages or logs. The example below uses a local terminal and requires `boto3`; it outputs no credentials and sends no email. Replace the pool ID with the stack output before running.

```python
import getpass
import boto3

pool = input("UserPoolId from flashcard-prod outputs: ").strip()
owner = input("Owner email address: ").strip()
password = getpass.getpass("Temporary owner password (14+ characters, upper/lower/number/symbol): ")
assert password == getpass.getpass("Repeat temporary password: "), "Passwords differ"
client = boto3.client("cognito-idp", region_name="ap-southeast-1")
client.admin_create_user(
    UserPoolId=pool, Username=owner, TemporaryPassword=password,
    MessageAction="SUPPRESS",
    UserAttributes=[{"Name": "email", "Value": owner}, {"Name": "email_verified", "Value": "true"}],
)
del password
client.admin_add_user_to_group(UserPoolId=pool, Username=owner, GroupName="owner")
```

The owner changes this temporary password at first sign-in; it expires after three days. If provisioning partially succeeds, inspect the existing user and finish group assignment instead of recreating it. Use a password manager for the permanent password. Do not paste credentials into the assistant conversation.

The pool supports optional authenticator-app MFA, but optional MFA **does not automatically enroll the owner** in Cognito hosted login. Enroll it with Cognito's supported API workflow, or deliberately change the pool to required TOTP MFA so hosted login prompts enrollment. Record a recovery procedure before requiring MFA. [AWS MFA behavior](https://docs.aws.amazon.com/cognito/latest/developerguide/user-pool-settings-mfa.html).

## 5. Migrate media only if the new dry run finds records

Run the same command without `--apply` first; choose a fresh report path each time. The command reads projected metadata, not image bytes, in dry-run mode.

```bash
go run ./cmd/migrate-media --table flash-card-app-prod --bucket flash-card-app-media-prod --source-buckets flash-card-app-media-prod --region ap-southeast-1 --report /tmp/flashcard-media-cutover-dryrun.json
```

If records are planned, resolve all dry-run errors first. Add another source bucket only after verifying ownership and intended image prefixes. Then, during the freeze and after the backup:

```bash
go run ./cmd/migrate-media --table flash-card-app-prod --bucket flash-card-app-media-prod --source-buckets flash-card-app-media-prod --region ap-southeast-1 --report /tmp/flashcard-media-cutover-apply.json --apply
```

The administrator needs DynamoDB Scan/UpdateItem, read access to approved source objects and GetObject/PutObject on destination `images/*`. The runtime Lambda intentionally lacks migration/legacy-bucket permissions. Do not broaden it for this command.

Apply validates source bytes up to 10 MiB, re-encodes to safe JPEG/static PNG, and conditionally attaches an ID-bound `storage_key`. It preserves record IDs, source URLs and originals. It never overwrites a different destination object; reruns resume only if normalized bytes match exactly. Larger, malformed or unsupported originals need manual handling; do not bypass validation. A failed conditional update can leave a private unattached object for later reconciliation. Dry-run metadata success alone does not prove source image bytes are valid.

Rerun dry-run after apply and confirm all image records are migrated/skipped with zero errors. Keep all source buckets private too: making the destination private does not revoke old public URLs in another bucket. Scope any legacy bucket policy changes carefully if shared with another application.

## 6. Release the frontend and reopen authenticated use

Set these five **public** Amplify variables from the stack outputs; preserve unrelated settings and remove `VITE_API_KEY` at both app and branch override levels:

| Frontend variable | Stack output |
|---|---|
| `VITE_API_BASE_URL` | `ApiUrl` |
| `VITE_AUTH_AUTHORITY` | `AuthIssuer` |
| `VITE_AUTH_CLIENT_ID` | `AuthClientId` |
| `VITE_AUTH_DOMAIN` | `AuthDomain` |
| `VITE_MEDIA_ORIGIN` | `MediaOrigin` |

Use the hosted domain returned by `AuthDomain`. Keep `customHttp.yml` synchronized with the exact deployed API/media/Cognito origins. Register callback `https://flashcards.example.com/auth/callback` and logout `https://flashcards.example.com/`, replacing the example origin with your own; the SAM template derives both from `FrontendOrigin`. ID and access tokens are both five minutes; refresh tokens rotate and last at most one day. [AWS refresh-token behavior](https://docs.aws.amazon.com/cognito/latest/developerguide/amazon-cognito-user-pools-using-the-refresh-token.html).

Verify Amplify's SPA rewrite serves `/auth/callback` directly without rewriting actual JavaScript/CSS files. The existing `404-200` fallback must be tested after deployment; use AWS's documented SPA rewrite if needed. [Amplify rewrite examples](https://docs.aws.amazon.com/amplify/latest/userguide/redirect-rewrite-examples.html).

Run frontend `npm ci`, `npm test`, `npm run build`, and `npm audit`. Release the reviewed frontend to Amplify's connected `main` branch only when the new backend is ready. Wait for deployment success and inspect actual CSP, frame, referrer and cache headers; hosting configuration can override repository settings. Ensure the new bundle contains no API key. Remove obsolete Amplify key variables and invalidate old deployed assets/caches; possession of an old bundle must no longer authorize any request.

Restore the intended Lambda concurrency after confirming the new backend artifact/configuration:

```bash
aws lambda put-function-concurrency --function-name flashcard-backend-prod --reserved-concurrent-executions 5 --region ap-southeast-1
```

## 7. Verify before ending maintenance

- Anonymous API calls and the retired key alone fail with `401`/`403`; wrong-pool/client tokens and signed users outside `owner` fail. Valid owner access succeeds. The local server also rejects unauthenticated calls.
- Sign in as the owner, verify existing cards/reviews, create a temporary card, immediately upload two small images, reorder them and study a review. Keep this test card until image checks finish. Never test destructive operations against existing study content.
- An unsigned URL for a newly created test image returns `403`; its signed URL displays. Check expiry with an uncached network request: an already loaded image can remain visible. Visible image metadata refreshes after four minutes, but the displayed image must not download again solely because its signature changes. Uploads over 4 MiB and invalid image bytes fail. Then delete the temporary card and confirm its current image objects are no longer readable using an uncached network request.
- Wait beyond one five-minute token period: the session should refresh automatically. Logout immediately clears private screens/cache and attempts refresh-token revocation; a new request must require login. Check the callback on a fresh tab and page reload. While editing an unsaved temporary card with a selected file, simulate a transient refresh failure: valid-token editing continues during bounded retries. After expiry, private screens must lock and API calls must stop, with the draft kept only in memory. Same-owner popup reauthentication must restore the route, text and selected file; popup cancellation keeps the draft locked. Explicit logout or a different account discards it. A reload or closed tab cannot preserve an in-memory draft.
- While locked, a network reconnect or Escape key must not discard the hidden draft. After a lock longer than five minutes, reauthentication refreshes stale image metadata without resetting text fields. If a new card saves before an image upload is rejected for authentication, its pending batch remains and **Continue uploads** resumes explicitly after sign-in, without recreating the card or repeating confirmed images. If a write's response is lost or arrives after the session changed, use **Open saved card** to inspect the result; do not replay an uncertain write automatically.
- Confirm PITR, S3 encryption/versioning/public blocks, exact origins, Lambda role scope, throttle settings and `expires_at` TTL. The owner account is the only `owner` member. Confirm old stack keys are disabled/deleted, including any keys not removed by the change set.

The browser makes a bounded best-effort call to Cognito's [revocation endpoint](https://docs.aws.amazon.com/cognito/latest/developerguide/revocation-endpoint.html) on logout. If the network prevents that request, administrative global sign-out can revoke remaining refresh sessions. Locally verified JWTs and already issued S3 links can remain usable for up to their five-minute expiry, and previously downloaded image bytes can remain in the private browser cache for 60 seconds; do not describe logout as instant revocation of every copied credential or link. Previously downloaded public content cannot be recalled.

## Failure recovery and follow-up

If rollout fails, keep the function frozen and all media private. Recheck public blocks after CloudFormation rollback. Correct the secure release or restore data into a separate recovery table from the verified backup, validate it, and perform a deliberate table cutover. Never restore the browser-key/public-bucket security model merely to regain availability. Do not delete or replace the existing table/bucket to resolve a deploy error.

Review available API Gateway/Lambda logs, CloudTrail events, AWS usage and billing for unexpected historical access or deletions. Existing logging may not have captured past S3 reads or API data access, so lack of log evidence is inconclusive. Rotate any actual AWS credentials only if they were exposed; this audit found no committed AWS secret. The browser-distributed API key is the credential that must be retired regardless.

Retain backups according to the owner's recovery needs. Deleted images can remain privately recoverable in older S3 versions for 30 days; uncertain database writes can leave private unattached media. Reconcile such objects against live records before cleanup—never bulk-delete a prefix as part of this migration. Schedule dependency updates and test recovery periodically.
