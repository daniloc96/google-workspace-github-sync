package models

// GoogleGroupMember represents a member of a Google Workspace Group.
type GoogleGroupMember struct {
	Email       string `json:"email"`
	Role        string `json:"role"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	IsSuspended bool   `json:"is_suspended"`
}

// IsActive returns true if the member is an active, non-suspended user.
func (m *GoogleGroupMember) IsActive() bool {
	return m.Type == "USER" && m.Status == "ACTIVE" && !m.IsSuspended
}
