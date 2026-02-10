# Invitation Reconciliation

The invitation reconciliation system tracks GitHub organization invitations in DynamoDB, enabling the tool to map Google Workspace emails to GitHub usernames — even when users don't have public emails on their GitHub profiles.

---

## Why It's Needed

GitHub's organization APIs have a fundamental limitation: they don't reliably expose the email address used to invite a user. Once a user accepts an invitation, their GitHub username appears in the members list, but there's no direct API to map that username back to the original Google email.

Without this mapping, the tool cannot:
- Detect when a user should be **removed** (their Google email left the group, but we don't know which GitHub username to remove)
- Detect when a user's **role should change** (they moved between Google groups)
- **Cancel pending invitations** for users removed from Google groups

The reconciliation system solves this by tracking invitations in DynamoDB and resolving the email→username mapping through multiple strategies.

---

## DynamoDB Table Design

### Single-table design

| Record Type | Partition Key (`pk`) | Sort Key (`sk`) |
|-------------|---------------------|-----------------|
| Invitation | `ORG#<org-name>` | `INV#<invitation-id>` || Existing member | `ORG#<org-name>` | `EXISTING#<github-username>` || Audit cursor | `ORG#<org-name>` | `CURSOR#audit_log` |

### Global Secondary Indexes

| GSI | Partition Key | Sort Key | Use Case |
|-----|--------------|----------|----------|
| `email-index` | `gsi1pk` = `ORG#<org>` | `gsi1sk` = `EMAIL#<email>` | Lookup by email |
| `status-index` | `gsi2pk` = `ORG#<org>` | `gsi2sk` = `STATUS#<status>` | Query by status |

### Invitation record schema

```json
{
  "pk":           "ORG#your-github-org",
  "sk":           "INV#71923550",
  "email":        "user@example.com",
  "role":         "member",
  "status":       "pending",
  "invitation_id": 71923550,
  "github_login": "",
  "invited_at":   "2026-02-09T15:00:00Z",
  "resolved_at":  "",
  "ttl":          1746000000,
  "gsi1pk":       "ORG#your-github-org",
  "gsi1sk":       "EMAIL#user@example.com",
  "gsi2pk":       "ORG#your-github-org",
  "gsi2sk":       "STATUS#pending"
}
```

### `EXISTING#` record (for pre-existing org members)

Created when a Google group member is found to already be in the GitHub org, either via:
- Verified domain email matching (GraphQL `organizationVerifiedDomainEmails`)
- Invite → 422 "already a member" → `SearchUserByEmail` resolution

```json
{
  "pk":           "ORG#your-github-org",
  "sk":           "EXISTING#jdoe",
  "email":        "jane.doe@example.com",
  "role":         "admin",
  "status":       "resolved",
  "github_login": "jdoe",
  "invited_at":   "2026-02-10T10:00:00Z",
  "resolved_at":  "2026-02-10T10:00:00Z",
  "ttl":          1746000000,
  "gsi1pk":       "ORG#your-github-org",
  "gsi1sk":       "EMAIL#jane.doe@example.com",
  "gsi2pk":       "ORG#your-github-org",
  "gsi2sk":       "STATUS#resolved"
}
```

These records are automatically included in `GetAllResolvedMappings` (they have `status=resolved`), enabling conservative-mode removals and role changes for pre-existing members.

### TTL

- Default: **90 days** from invitation creation (configurable via `dynamodb.ttl_days`)
- On resolution, TTL is refreshed to 90 days from the resolve timestamp
- DynamoDB automatically deletes expired records

### Billing

- **PAY_PER_REQUEST** (on-demand) — no capacity planning needed
- Suitable for low-frequency sync runs (e.g., every 15 minutes)

---

## Invitation Status Lifecycle

```
                         ┌─────────────┐
                    ┌───►│  resolved   │  (email → username mapped)
                    │    └─────────────┘
                    │
┌─────────┐   resolve   ┌─────────────┐
│ pending │────────┼───►│  failed     │  (GitHub reports failure)
└─────────┘        │    └─────────────┘
                    │
                    │    ┌─────────────┐
                    ├───►│  expired    │  (>7 days, not in pending set)
                    │    └─────────────┘
                    │
                    │    ┌─────────────┐
                    ├───►│  cancelled  │  (user removed from Google, invite cancelled)
                    │    └─────────────┘
                    │
                    │    ┌─────────────┐
                    └───►│  removed    │  (member removed from org)
                         └─────────────┘
```

| Status | Description |
|--------|-------------|
| `pending` | Invitation sent, waiting for user to accept |
| `resolved` | Email→username mapping established (user accepted or login detected) |
| `failed` | GitHub reports the invitation failed |
| `expired` | Invitation is >7 days old and no longer in GitHub's pending set |
| `cancelled` | Invitation was cancelled (user removed from Google groups) |
| `removed` | Member was removed from the GitHub org |

---

## Resolution Strategies

The reconciler uses multiple strategies to resolve `pending` → `resolved` (mapping email to GitHub username):

### Strategy 1: Pending invitation login

When a user clicks the invitation link, GitHub associates their username with the invitation. The reconciler checks:

```
for each pending DynamoDB record:
    check GitHub pending invitations for matching invitation_id
    if invitation now has a login → resolve(login)
```

### Strategy 2: Audit log

GitHub's audit log records `org.add_member` events with both the `invitation_id` and the `user` (login). The reconciler:

```
fetch audit log events after last cursor timestamp
for each org.add_member event:
    lookup DynamoDB record by invitation_id
    if found and pending → resolve(event.user)
save new cursor timestamp
```

### Strategy 3: Invite upgrade (at execution time)

When an invite fails with "already a member", `ExecuteActions` calls `SearchUserByEmail` to find the GitHub username. If found:
1. The action is upgraded to a role update
2. `action.Username` and `action.GoogleEmail` are set
3. During reconciliation, `handleAlreadyInOrgMembers` creates an `EXISTING#<username>` record

### Strategy 4: Verified domain emails (proactive)

When the GitHub org has a verified domain (Enterprise Cloud), the engine queries `ListMembersWithVerifiedEmails` via GraphQL before the diff runs. This provides a `map[email]username` for all org members with verified-domain emails.

- **At diff time**: Users are recognized as "known", preventing unnecessary invite attempts
- **After reconciliation**: `EnsureVerifiedEmailMappings` creates `EXISTING#<username>` records for matched users who don't yet have a DynamoDB mapping
- **Role detection**: The correct role (member/admin) is determined by which Google group the user belongs to (owners group takes precedence)

This is the most reliable strategy as it works even when:
- The user's email is private on GitHub
- `SearchUserByEmail` would return no results
- The user was already in the org before the tool was deployed

---

## Reconcile Flow

The reconciler runs after `ExecuteActions` on every non-dry-run sync:

```
Step 1:  Save new invitation records (for executed ActionInvite with invitation_id)
Step 1b: Mark cancelled invitations (for executed ActionCancelInvite)
Step 1c: Handle removed members (for executed ActionRemove with GoogleEmail)
         └─ Finds DynamoDB record by email, marks as "removed"
Step 1d: Handle role updates (for executed ActionUpdateRole with GoogleEmail)
         └─ Finds DynamoDB record by email, updates role field
Step 1e: Handle already-in-org members (for actions with AlreadyInOrg=true and Username set)
         └─ Creates EXISTING#<username> record (from 422 → SearchUserByEmail fallback)

Step 2:  Resolve pending invitations with login
         └─ Re-fetches GitHub pending invites, matches to DynamoDB records

Step 3:  Resolve from audit log
         └─ Fetches org.add_member events, resolves matching DynamoDB records

Step 4:  Handle failed and expired
         ├─ Fetches GitHub failed invitations, marks in DynamoDB
         └─ Checks all pending records >7 days old, marks as expired

Step 5:  Ensure verified email mappings (called by Engine after Reconcile)
         └─ For each verified-email-matched Google member without a DynamoDB record:
            create EXISTING#<username> with correct role (member/admin)
```

### Error handling

- Reconciliation is **non-fatal**: if any step fails, the error is logged as a warning and the sync continues
- Individual record errors are counted in `ReconcileResult.Errors`

---

## How It Enables Remove and Role Change

### Conservative remove (without DynamoDB)

If DynamoDB is disabled and `remove_extra_members: false`, **no removals happen**. The tool doesn't know which GitHub members were added by it vs. pre-existing.

### Conservative remove (with DynamoDB)

```
1. CalculateDiff loads resolved mappings: map[email] → username
2. Builds reverse index: map[username] → email
3. For each GitHub member:
   a. Look up username in reverse index
   b. If not found → pre-existing member, skip
   c. If found but email still in Google groups → keep
   d. If found but email NOT in Google groups → emit ActionRemove
```

### Role change

```
1. CalculateDiff loads resolved mappings
2. For each GitHub member:
   a. Check desired role by email (direct match)
   b. If no match, try DynamoDB reverse lookup: username → email → desired role
   c. If current role ≠ desired role → emit ActionUpdateRole
```

---

## Reconcile Result

Each reconcile run produces a result:

```json
{
  "new_saved": 2,
  "resolved": 1,
  "failed": 0,
  "expired": 0,
  "cancelled": 1,
  "members_removed": 0,
  "roles_updated": 1,
  "already_in_org_resolved": 0,
  "verified_emails_mapped": 3,
  "errors": 0
}
```

---

## Enabling/Disabling

Invitation reconciliation is **opt-in**:

```yaml
dynamodb:
  enabled: true        # Set to false to disable
  table_name: invitation-mappings
  region: eu-west-1
  ttl_days: 90
```

When disabled:
- No DynamoDB operations occur
- Conservative mode removals are skipped (no tracking data)
- Role changes only work via direct email match or invite upgrade mechanism
- Invitation tracking (cancel, expire, fail) is not available

When enabled:
- Table must exist (created by SAM template or manually)
- Requires IAM permissions for `PutItem`, `GetItem`, `UpdateItem`, `Query` on table and GSIs
