package models

import (
	"fmt"
	"time"
)

// SyncResult contains the outcome of a sync operation.
type SyncResult struct {
	DryRun              bool            `json:"dry_run"`
	StartTime           time.Time       `json:"start_time"`
	EndTime             time.Time       `json:"end_time"`
	DurationMs          int64           `json:"duration_ms"`
	Actions             []SyncAction    `json:"actions"`
	Summary             SyncSummary     `json:"summary"`
	Errors              []string          `json:"errors,omitempty"`
	InvitedUsers        []string          `json:"invited_users,omitempty"`
	AlreadyInOrgUsers   []string          `json:"already_in_org_users,omitempty"`
	OrphanedGitHubUsers []string          `json:"orphaned_github_users,omitempty"`
	Reconciliation      *ReconcileResult  `json:"reconciliation,omitempty"`
}

// SyncSummary provides aggregate statistics.
type SyncSummary struct {
	TotalGoogleMembers int `json:"total_google_members"`
	TotalGitHubMembers int `json:"total_github_members"`
	PendingInvitations int `json:"pending_invitations"`
	ActionsPlanned     int `json:"actions_planned"`
	ActionsExecuted    int `json:"actions_executed"`
	ActionsFailed      int `json:"actions_failed"`
	Invited            int `json:"invited"`
	AlreadyInOrg       int `json:"already_in_org"`
	Removed            int `json:"removed"`
	RoleUpdated        int `json:"role_updated"`
	CancelledInvites   int `json:"cancelled_invites"`
	Skipped            int `json:"skipped"`
	OrphanedGitHub     int `json:"orphaned_github"`
}

// IsSuccess returns true if no errors occurred.
func (r *SyncResult) IsSuccess() bool {
	return len(r.Errors) == 0 && r.Summary.ActionsFailed == 0
}

// String returns a human-readable representation of the sync summary.
func (s SyncSummary) String() string {
	return fmt.Sprintf(
		"sync completed — Google: %d members, GitHub: %d members, Pending invites: %d, "+
			"Actions: %d planned / %d executed / %d failed, "+
			"Invited: %d, Already in org: %d, Removed: %d, Role updated: %d, Skipped: %d, "+
			"Orphaned: %d",
		s.TotalGoogleMembers, s.TotalGitHubMembers, s.PendingInvitations,
		s.ActionsPlanned, s.ActionsExecuted, s.ActionsFailed,
		s.Invited, s.AlreadyInOrg, s.Removed, s.RoleUpdated, s.Skipped,
		s.OrphanedGitHub,
	)
}
