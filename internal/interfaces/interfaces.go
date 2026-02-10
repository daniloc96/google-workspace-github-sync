package interfaces

import (
	"context"

	"github.com/daniloc96/google-workspace-github-sync/internal/models"
)

// GoogleClient defines operations needed from Google Workspace.
type GoogleClient interface {
	GetGroupMembers(ctx context.Context, groupEmail string) ([]models.GoogleGroupMember, error)
	GetUsersSuspendedStatus(ctx context.Context, emails []string) (map[string]bool, error)
}

// GitHubClient defines operations needed from GitHub Organization APIs.
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

// SyncEngine defines sync orchestration.
type SyncEngine interface {
	Sync(ctx context.Context) (*models.SyncResult, error)
}

// InvitationStore defines operations for persistent invitation tracking.
type InvitationStore interface {
	// SaveInvitation stores a new pending invitation mapping.
	SaveInvitation(ctx context.Context, mapping models.InvitationMapping) error

	// GetInvitation retrieves an invitation by org and invitation ID.
	GetInvitation(ctx context.Context, org string, invitationID int64) (*models.InvitationMapping, error)

	// GetPendingInvitations returns all pending invitations for an org.
	GetPendingInvitations(ctx context.Context, org string) ([]models.InvitationMapping, error)

	// ResolveInvitation updates an invitation with the resolved GitHub username.
	ResolveInvitation(ctx context.Context, org string, invitationID int64, githubLogin string) error

	// UpdateStatus changes the status of an invitation (failed, expired, cancelled, removed).
	UpdateStatus(ctx context.Context, org string, invitationID int64, status models.InvitationStatus) error

	// UpdateRole updates the role of an invitation mapping.
	UpdateRole(ctx context.Context, org string, invitationID int64, role models.OrgRole) error

	// GetByEmail retrieves invitation mappings for a specific email.
	GetByEmail(ctx context.Context, email string, org string) ([]models.InvitationMapping, error)

	// GetAuditLogCursor retrieves the last processed audit log cursor.
	GetAuditLogCursor(ctx context.Context, org string) (*models.AuditLogCursor, error)

	// SaveAuditLogCursor stores the audit log cursor.
	SaveAuditLogCursor(ctx context.Context, cursor models.AuditLogCursor) error

	// GetAllResolvedMappings returns all resolved email→username mappings for an org.
	GetAllResolvedMappings(ctx context.Context, org string) (map[string]string, error)
}

// GitHubAuditLogClient defines operations for reading the GitHub Audit Log.
type GitHubAuditLogClient interface {
	// GetAddMemberEvents returns org.add_member events after the given timestamp.
	GetAddMemberEvents(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error)
}
