package models

import (
	"fmt"
	"time"
)

// InvitationStatus represents the current state of an invitation mapping.
type InvitationStatus string

const (
	InvitationPending   InvitationStatus = "pending"
	InvitationResolved  InvitationStatus = "resolved"
	InvitationFailed    InvitationStatus = "failed"
	InvitationExpired   InvitationStatus = "expired"
	InvitationCancelled InvitationStatus = "cancelled"
	InvitationRemoved   InvitationStatus = "removed"
)

// InvitationMapping represents a tracked invitation in DynamoDB.
type InvitationMapping struct {
	PK          string           `dynamodbav:"pk"`
	SK          string           `dynamodbav:"sk"`
	Email       string           `dynamodbav:"email"`
	GitHubLogin *string          `dynamodbav:"github_login,omitempty"`
	Status      InvitationStatus `dynamodbav:"status"`
	Role        OrgRole          `dynamodbav:"role"`
	InvitedAt   time.Time        `dynamodbav:"invited_at"`
	ResolvedAt  *time.Time       `dynamodbav:"resolved_at,omitempty"`
	TTL         int64            `dynamodbav:"ttl"`

	// GSI keys
	GSI1PK string `dynamodbav:"gsi1pk"` // EMAIL#<email>
	GSI1SK string `dynamodbav:"gsi1sk"` // ORG#<org>
	GSI2PK string `dynamodbav:"gsi2pk"` // ORG#<org>
	GSI2SK string `dynamodbav:"gsi2sk"` // STATUS#<status>
}

// NewInvitationMapping creates a new InvitationMapping with all key attributes set.
func NewInvitationMapping(org string, invitationID int64, email string, role OrgRole, ttlDays int) InvitationMapping {
	now := time.Now().UTC()
	ttl := now.AddDate(0, 0, ttlDays).Unix()

	return InvitationMapping{
		PK:       "ORG#" + org,
		SK:       invitationSK(invitationID),
		Email:    email,
		Status:   InvitationPending,
		Role:     role,
		InvitedAt: now,
		TTL:      ttl,
		GSI1PK:   "EMAIL#" + email,
		GSI1SK:   "ORG#" + org,
		GSI2PK:   "ORG#" + org,
		GSI2SK:   "STATUS#" + string(InvitationPending),
	}
}

func invitationSK(invitationID int64) string {
	return "INV#" + formatInt64(invitationID)
}

func formatInt64(v int64) string {
	return fmt.Sprintf("%d", v)
}

// AuditLogEntry represents a parsed entry from the GitHub Audit Log.
type AuditLogEntry struct {
	Timestamp    int64  `json:"@timestamp"`
	Action       string `json:"action"`
	Actor        string `json:"actor"`
	User         string `json:"user"`
	Org          string `json:"org"`
	InvitationID int64  `json:"invitation_id,omitempty"`
}

// AuditLogCursor represents the saved position in the audit log.
type AuditLogCursor struct {
	PK            string    `dynamodbav:"pk"`
	SK            string    `dynamodbav:"sk"`
	LastTimestamp  int64     `dynamodbav:"last_timestamp"`
	LastRun       time.Time `dynamodbav:"last_run"`
}

// ReconcileResult holds the outcome of an invitation reconciliation run.
type ReconcileResult struct {
	NewInvitationsSaved  int      `json:"new_invitations_saved"`
	Resolved             int      `json:"resolved"`
	Failed               int      `json:"failed"`
	Expired              int      `json:"expired"`
	Cancelled            int      `json:"cancelled"`
	MembersRemoved       int      `json:"members_removed"`
	RolesUpdated         int      `json:"roles_updated"`
	AlreadyInOrgResolved int      `json:"already_in_org_resolved"`
	VerifiedEmailsMapped int      `json:"verified_emails_mapped"`
	Errors               []string `json:"errors,omitempty"`
}
