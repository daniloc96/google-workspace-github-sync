package models

import (
	"time"

	"github.com/sirupsen/logrus"
)

// ActionType represents the type of sync action.
type ActionType string

const (
	ActionInvite       ActionType = "invite"
	ActionRemove       ActionType = "remove"
	ActionUpdateRole   ActionType = "update_role"
	ActionCancelInvite ActionType = "cancel_invite"
	ActionSkip         ActionType = "skip"
)

// SyncAction represents a single synchronization action.
type SyncAction struct {
	Type         ActionType `json:"type"`
	Email        string     `json:"email"`
	GoogleEmail  string     `json:"google_email,omitempty"` // Original Google email for DynamoDB lookup (set on remove/role-change from DynamoDB mappings)
	Username     string     `json:"username,omitempty"`     // Resolved GitHub username (set when already-in-org user is found via search or verified emails)
	CurrentRole  *OrgRole   `json:"current_role,omitempty"`
	TargetRole   *OrgRole   `json:"target_role,omitempty"`
	Reason       string     `json:"reason"`
	Executed     bool       `json:"executed"`
	AlreadyInOrg bool       `json:"already_in_org,omitempty"`
	Error        *string    `json:"error,omitempty"`
	Timestamp    *time.Time `json:"timestamp,omitempty"`
	InvitationID *int64     `json:"invitation_id,omitempty"`
}

// LogFields returns structured logging fields for this action.
func (a *SyncAction) LogFields() logrus.Fields {
	fields := logrus.Fields{
		"action": a.Type,
		"email":  a.Email,
		"reason": a.Reason,
	}
	if a.TargetRole != nil {
		fields["target_role"] = *a.TargetRole
	}
	if a.Error != nil {
		fields["error"] = *a.Error
	}
	return fields
}
