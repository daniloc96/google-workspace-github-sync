# API Reference

Internal package and interface documentation for `google-workspace-github-sync`.

---

## Package Overview

```
internal/
├── config/       Configuration loading and validation
├── github/       GitHub API client implementation
├── google/       Google Workspace API client implementation
├── interfaces/   Interface definitions (contracts)
├── log/          Structured logging setup
├── metrics/      CloudWatch metrics publishing
├── models/       Domain types and data structures
├── secrets/      AWS Secrets Manager integration
└── sync/         Sync engine, diff, actions, reconciliation
```

---

## Interfaces

### `interfaces.GoogleClient`

```go
type GoogleClient interface {
    GetGroupMembers(ctx context.Context, groupEmail string) ([]models.GoogleGroupMember, error)
    GetUsersSuspendedStatus(ctx context.Context, emails []string) (map[string]bool, error)
}
```

| Method | Description |
|--------|-------------|
| `GetGroupMembers` | Fetches all members of a Google Workspace group. Returns email, role, type, and status. |
| `GetUsersSuspendedStatus` | Checks whether the given emails belong to suspended Google Workspace users. |

### `interfaces.GitHubClient`

```go
type GitHubClient interface {
    ListMembers(ctx context.Context, org string) ([]models.GitHubOrgMember, error)
    ListPendingInvitations(ctx context.Context, org string) ([]models.GitHubOrgMember, error)
    CreateInvitation(ctx context.Context, org string, email string, role models.OrgRole) (*models.GitHubOrgMember, error)
    RemoveMember(ctx context.Context, org string, username string) error
    UpdateMemberRole(ctx context.Context, org string, username string, role models.OrgRole) error
    CancelInvitation(ctx context.Context, org string, invitationID int64) error
    SearchUserByEmail(ctx context.Context, email string) (string, error)
    GetAuditLogAddMemberEvents(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error)
    ListFailedInvitations(ctx context.Context, org string) ([]models.GitHubOrgMember, error)
    ListMembersWithVerifiedEmails(ctx context.Context, org string) (map[string]string, error)
}
```

| Method | Description |
|--------|-------------|
| `ListMembers` | Lists all org members. Uses two-pass approach: first `member` role, then `admin` role, to correctly detect admin status. |
| `ListPendingInvitations` | Lists all pending org invitations. Includes invitation ID, email, and role. |
| `CreateInvitation` | Sends an org invitation by email. Returns the created member object or 422 if already a member. |
| `RemoveMember` | Removes a user from the org by username. |
| `UpdateMemberRole` | Changes a member's role (member ↔ admin). **Note**: uses `go-github`'s `EditOrgMembership(ctx, user, org, ...)` — user comes before org. |
| `CancelInvitation` | Cancels a pending invitation by ID. Uses raw HTTP `DELETE` to the GitHub API. |
| `SearchUserByEmail` | Searches GitHub users by email (`GET /search/users?q={email}+in:email`). Returns username or empty string. |
| `GetAuditLogAddMemberEvents` | Fetches `org.add_member` events from the GitHub audit log after a timestamp. |
| `ListFailedInvitations` | Lists invitations that have failed (for reconciliation). |
| `ListMembersWithVerifiedEmails` | Queries GitHub GraphQL API for `organizationVerifiedDomainEmails` across all org members. Returns `map[lowercase-email]username`. Requires Enterprise Cloud with a verified domain. Paginated via cursor. |

### `interfaces.SyncEngine`

```go
type SyncEngine interface {
    Sync(ctx context.Context) (*models.SyncResult, error)
}
```

Orchestrates the full sync pipeline: fetch → diff → execute → reconcile → summarize.

### `interfaces.InvitationStore`

```go
type InvitationStore interface {
    SaveInvitation(ctx context.Context, mapping models.InvitationMapping) error
    GetInvitation(ctx context.Context, org string, invitationID int64) (*models.InvitationMapping, error)
    GetPendingInvitations(ctx context.Context, org string) ([]models.InvitationMapping, error)
    ResolveInvitation(ctx context.Context, org string, invitationID int64, githubLogin string) error
    UpdateStatus(ctx context.Context, org string, invitationID int64, status models.InvitationStatus) error
    UpdateRole(ctx context.Context, org string, invitationID int64, role models.OrgRole) error
    GetByEmail(ctx context.Context, email string, org string) ([]models.InvitationMapping, error)
    GetAuditLogCursor(ctx context.Context, org string) (*models.AuditLogCursor, error)
    SaveAuditLogCursor(ctx context.Context, cursor models.AuditLogCursor) error
    GetAllResolvedMappings(ctx context.Context, org string) (map[string]string, error)
}
```

| Method | Description |
|--------|-------------|
| `SaveInvitation` | Persists a new `InvitationMapping` to DynamoDB. |
| `GetInvitation` | Retrieves by `PK=ORG#<org>`, `SK=INV#<id>`. |
| `GetPendingInvitations` | Queries `status-index` for `STATUS#pending`. |
| `ResolveInvitation` | Sets `github_login`, `status=resolved`, `resolved_at`. |
| `UpdateStatus` | Transitions status (e.g., `pending → cancelled`). |
| `UpdateRole` | Updates the role on an existing mapping. |
| `GetByEmail` | Queries `email-index` for `EMAIL#<email>`. |
| `GetAuditLogCursor` | Retrieves the saved audit log position. |
| `SaveAuditLogCursor` | Persists the audit log cursor. |
| `GetAllResolvedMappings` | Returns all `status=resolved` mappings as `email → username`. |

---

## Models

### `models.OrgRole`

```go
type OrgRole string

const (
    RoleMember OrgRole = "member"
    RoleOwner  OrgRole = "admin"
)
```

Maps to GitHub's organization roles. Google `MEMBER` → `RoleMember`, Google `OWNER`/`MANAGER` → `RoleOwner`.

### `models.ActionType`

```go
type ActionType string

const (
    ActionInvite       ActionType = "invite"
    ActionRemove       ActionType = "remove"
    ActionUpdateRole   ActionType = "update_role"
    ActionCancelInvite ActionType = "cancel_invite"
    ActionSkip         ActionType = "skip"
)
```

### `models.SyncAction`

```go
type SyncAction struct {
    Type         ActionType
    Email        string            // Target email or username
    GoogleEmail  string            // Original Google email (for DynamoDB lookup on remove/role-change)
    CurrentRole  *OrgRole          // Current role (for role changes)
    TargetRole   *OrgRole          // Desired role
    Reason       string            // Human-readable explanation
    Executed     bool              // Whether action was executed
    AlreadyInOrg bool              // User was already in org
    Error        *string           // Error message if failed
    Timestamp    *time.Time        // Execution time
    InvitationID *int64            // For cancel_invite actions
}
```

Methods:
- `LogFields() logrus.Fields` — returns structured log fields for the action.

### `models.GitHubOrgMember`

```go
type GitHubOrgMember struct {
    Username     *string
    Email        *string
    Role         OrgRole
    IsPending    bool
    InvitationID *int64
}
```

Methods:
- `Identifier() string` — returns `Email` if set, otherwise `Username`.

### `models.GoogleGroupMember`

```go
type GoogleGroupMember struct {
    Email       string
    Role        string     // "MEMBER", "OWNER", "MANAGER"
    Type        string     // "USER", "GROUP", "SERVICE_ACCOUNT"
    Status      string     // "ACTIVE", "SUSPENDED"
    IsSuspended bool       // Set by GetUsersSuspendedStatus
}
```

Methods:
- `IsActive() bool` — returns `true` if `Type == "USER"`, `Status == "ACTIVE"`, and not suspended.

### `models.InvitationMapping`

```go
type InvitationMapping struct {
    PK          string              // "ORG#<org>"
    SK          string              // "INV#<invitation_id>"
    Email       string
    GitHubLogin *string
    Status      InvitationStatus
    Role        OrgRole
    InvitedAt   time.Time
    ResolvedAt  *time.Time
    TTL         int64               // Unix timestamp (90-day expiry)
    GSI1PK      string              // "EMAIL#<email>"
    GSI1SK      string              // "ORG#<org>"
    GSI2PK      string              // "ORG#<org>"
    GSI2SK      string              // "STATUS#<status>"
}
```

Constructor:
- `NewInvitationMapping(org, invitationID, email, role, ttlDays)` — creates a fully-keyed mapping.

### `models.InvitationStatus`

```go
type InvitationStatus string

const (
    InvitationPending   InvitationStatus = "pending"
    InvitationResolved  InvitationStatus = "resolved"
    InvitationFailed    InvitationStatus = "failed"
    InvitationExpired   InvitationStatus = "expired"
    InvitationCancelled InvitationStatus = "cancelled"
    InvitationRemoved   InvitationStatus = "removed"
)
```

### `models.AuditLogEntry`

```go
type AuditLogEntry struct {
    Timestamp    int64
    Action       string     // e.g., "org.add_member"
    Actor        string
    User         string     // GitHub username that joined
    Org          string
    InvitationID int64
}
```

### `models.AuditLogCursor`

```go
type AuditLogCursor struct {
    PK            string      // "ORG#<org>"
    SK            string      // "CURSOR#audit_log"
    LastTimestamp  int64
    LastRun       time.Time
}
```

### `models.SyncResult`

```go
type SyncResult struct {
    DryRun              bool
    StartTime           time.Time
    EndTime             time.Time
    DurationMs          int64
    Actions             []SyncAction
    Summary             SyncSummary
    Errors              []string
    InvitedUsers        []string
    AlreadyInOrgUsers   []string
    OrphanedGitHubUsers []string
    Reconciliation      *ReconcileResult
}
```

Methods:
- `IsSuccess() bool` — no errors and no failed actions.

### `models.SyncSummary`

```go
type SyncSummary struct {
    TotalGoogleMembers  int
    TotalGitHubMembers  int
    PendingInvitations  int
    ActionsPlanned      int
    ActionsExecuted     int
    ActionsFailed       int
    Invited             int
    AlreadyInOrg        int
    Removed             int
    RoleUpdated         int
    CancelledInvites    int
    Skipped             int
    OrphanedGitHub      int
}
```

Methods:
- `String() string` — human-readable one-line summary.

### `models.ReconcileResult`

```go
type ReconcileResult struct {
    NewInvitationsSaved  int
    Resolved             int
    Failed               int
    Expired              int
    Cancelled            int
    MembersRemoved       int
    RolesUpdated         int
    AlreadyInOrgResolved int      // EXISTING# records created via 422 → SearchUserByEmail fallback
    VerifiedEmailsMapped int      // EXISTING# records created via verified domain email matching
    Errors               []string
}
```

### `models.LambdaEvent`

```go
type LambdaEvent struct {
    DryRun     *bool
    Source     string      // e.g., "aws.events"
    DetailType string     // e.g., "Scheduled Event"
}
```

Methods:
- `IsDryRun(defaultValue bool) bool` — returns the event's `DryRun` if set, otherwise the default.

### `models.LambdaResponse`

```go
type LambdaResponse struct {
    StatusCode int
    Message    string
    Result     *SyncResult
}
```

Constructors:
- `NewSuccessResponse(result)` — status 200, message with action count.
- `NewErrorResponse(err)` — status 500, error message.

---

## Key Functions

### `sync.CalculateDiff`

```go
func CalculateDiff(
    membersGroup   []models.GoogleGroupMember,
    ownersGroup    []models.GoogleGroupMember,
    githubMembers  []models.GitHubOrgMember,
    pendingInvites []models.GitHubOrgMember,
    removeExtraMembers bool,
    emailMappings  *EmailMappings,
    verifiedEmails map[string]string,
) []models.SyncAction
```

Computes the list of sync actions by comparing Google group membership (desired state) against GitHub org membership (current state). The `verifiedEmails` parameter (optional) provides `map[lowercase-email]username` from GraphQL `organizationVerifiedDomainEmails`, enabling proactive matching of users already in the org. See Sync Logic for the algorithm.

### `sync.ExecuteActions`

```go
func ExecuteActions(
    ctx    context.Context,
    client interfaces.GitHubClient,
    org    string,
    actions []models.SyncAction,
    dryRun  bool,
) ([]models.SyncAction, error)
```

Executes planned actions against the GitHub API. Handles:
- `invite` — calls `CreateInvitation`; auto-upgrades to `update_role` on "already a member" errors.
- `remove` — calls `RemoveMember`.
- `update_role` — calls `UpdateMemberRole`.
- `cancel_invite` — calls `CancelInvitation`.

In dry-run mode, actions are logged but not executed.

### `sync.NewEngine` / `Engine.Sync`

```go
func NewEngine(
    googleClient interfaces.GoogleClient,
    githubClient interfaces.GitHubClient,
    cfg          *config.Config,
) *Engine

func (e *Engine) Sync(ctx context.Context) (*models.SyncResult, error)
```

Full sync pipeline:
1. Fetch Google group members (members + owners groups)
2. Apply suspension status
3. Fetch GitHub org members and pending invitations
4. Build email mappings from DynamoDB (if enabled)
5. Fetch verified domain emails via GraphQL (non-fatal on error)
6. Calculate diff (with DynamoDB mappings + verified emails)
7. Execute actions (or log in dry-run)
8. Run reconciliation (if enabled)
9. Ensure verified email DynamoDB mappings (`EnsureVerifiedEmailMappings`)
10. Build and return `SyncResult`

### `sync.Engine.SetReconciler`

```go
func (e *Engine) SetReconciler(r *Reconciler)
```

Injects the optional reconciler for DynamoDB-based invitation tracking.

### Helper Functions

| Function | Package | Description |
|----------|---------|-------------|
| `buildEmailMappings` | `sync` | Queries DynamoDB for resolved email→username mappings. |
| `applySuspensionStatus` | `sync` | Enriches Google members with suspension status. |
| `buildSummary` | `sync` | Aggregates action counts into `SyncSummary`. |
| `classifyInviteActions` | `sync` | Splits invite results into invited vs already-in-org lists. |
| `findOrphanedGitHubUsers` | `sync` | Finds GitHub members not in any Google group (uses direct email match, DynamoDB reverse lookup, and verified email reverse lookup). |
| `ptrVal` / `ptrInt64Val` | `sync` | Safely dereference `*string` / `*int64` pointers. |

---

## Mock / Fake Implementations

### `github.MockClient`

Located in `internal/github/mock_client.go`. Implements `interfaces.GitHubClient` with function fields for each method, allowing per-test behavior:

```go
type MockClient struct {
    ListMembersFunc                  func(...) ([]models.GitHubOrgMember, error)
    CreateInvitationFunc             func(...) (*models.GitHubOrgMember, error)
    RemoveMemberFunc                 func(...) error
    UpdateMemberRoleFunc             func(...) error
    CancelInvitationFunc             func(...) error
    SearchUserByEmailFunc            func(...) (string, error)
    ListMembersWithVerifiedEmailsFn  func(...) (map[string]string, error)
    // ... etc.
}
```

### `google.MockClient`

Located in `internal/google/mock_client.go`. Same pattern as above for `interfaces.GoogleClient`.
