package models

// OrgRole represents a user's role in the GitHub organization.
type OrgRole string

const (
	RoleMember OrgRole = "member"
	RoleOwner  OrgRole = "admin"
)

// GitHubOrgMember represents a current or invited member of a GitHub Organization.
type GitHubOrgMember struct {
	Username     *string `json:"username,omitempty"`
	Email        *string `json:"email,omitempty"`
	Role         OrgRole `json:"role"`
	IsPending    bool    `json:"is_pending"`
	InvitationID *int64  `json:"invitation_id,omitempty"`
}

// Identifier returns the best identifier for this member (email or username).
func (m *GitHubOrgMember) Identifier() string {
	if m.Email != nil && *m.Email != "" {
		return *m.Email
	}
	if m.Username != nil {
		return *m.Username
	}
	return ""
}
