package github

import (
	"context"

	"github.com/daniloc96/google-workspace-github-sync/internal/models"
)

// MockClient is a simple mock implementation of the GitHub client.
type MockClient struct {
	ListMembersFunc                    func(ctx context.Context, org string) ([]models.GitHubOrgMember, error)
	ListPendingInvitationsFunc         func(ctx context.Context, org string) ([]models.GitHubOrgMember, error)
	CreateInvitationFunc               func(ctx context.Context, org string, email string, role models.OrgRole) (*models.GitHubOrgMember, error)
	RemoveMemberFunc                   func(ctx context.Context, org string, username string) error
	UpdateMemberRoleFunc               func(ctx context.Context, org string, username string, role models.OrgRole) error
	CancelInvitationFunc               func(ctx context.Context, org string, invitationID int64) error
	SearchUserByEmailFunc              func(ctx context.Context, email string) (string, error)
	GetAuditLogAddMemberEventsFunc     func(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error)
	ListFailedInvitationsFunc          func(ctx context.Context, org string) ([]models.GitHubOrgMember, error)
	ListMembersWithVerifiedEmailsFunc  func(ctx context.Context, org string) (map[string]string, error)
}

func (m *MockClient) ListMembers(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
	if m.ListMembersFunc == nil {
		return nil, nil
	}
	return m.ListMembersFunc(ctx, org)
}

func (m *MockClient) ListPendingInvitations(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
	if m.ListPendingInvitationsFunc == nil {
		return nil, nil
	}
	return m.ListPendingInvitationsFunc(ctx, org)
}

func (m *MockClient) CreateInvitation(ctx context.Context, org string, email string, role models.OrgRole) (*models.GitHubOrgMember, error) {
	if m.CreateInvitationFunc == nil {
		return nil, nil
	}
	return m.CreateInvitationFunc(ctx, org, email, role)
}

func (m *MockClient) RemoveMember(ctx context.Context, org string, username string) error {
	if m.RemoveMemberFunc == nil {
		return nil
	}
	return m.RemoveMemberFunc(ctx, org, username)
}

func (m *MockClient) UpdateMemberRole(ctx context.Context, org string, username string, role models.OrgRole) error {
	if m.UpdateMemberRoleFunc == nil {
		return nil
	}
	return m.UpdateMemberRoleFunc(ctx, org, username, role)
}

func (m *MockClient) CancelInvitation(ctx context.Context, org string, invitationID int64) error {
	if m.CancelInvitationFunc == nil {
		return nil
	}
	return m.CancelInvitationFunc(ctx, org, invitationID)
}

func (m *MockClient) SearchUserByEmail(ctx context.Context, email string) (string, error) {
	if m.SearchUserByEmailFunc == nil {
		return "", nil
	}
	return m.SearchUserByEmailFunc(ctx, email)
}

func (m *MockClient) GetAuditLogAddMemberEvents(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error) {
	if m.GetAuditLogAddMemberEventsFunc == nil {
		return nil, nil
	}
	return m.GetAuditLogAddMemberEventsFunc(ctx, org, afterTimestamp)
}

func (m *MockClient) ListFailedInvitations(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
	if m.ListFailedInvitationsFunc == nil {
		return nil, nil
	}
	return m.ListFailedInvitationsFunc(ctx, org)
}

func (m *MockClient) ListMembersWithVerifiedEmails(ctx context.Context, org string) (map[string]string, error) {
	if m.ListMembersWithVerifiedEmailsFunc == nil {
		return nil, nil
	}
	return m.ListMembersWithVerifiedEmailsFunc(ctx, org)
}
