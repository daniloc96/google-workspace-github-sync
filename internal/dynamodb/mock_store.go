package dynamodb

import (
	"context"

	"github.com/daniloc96/google-workspace-github-sync/internal/models"
)

// MockStore implements InvitationStore for testing.
type MockStore struct {
	SaveInvitationFunc         func(ctx context.Context, mapping models.InvitationMapping) error
	GetInvitationFunc          func(ctx context.Context, org string, invitationID int64) (*models.InvitationMapping, error)
	GetPendingInvitationsFunc  func(ctx context.Context, org string) ([]models.InvitationMapping, error)
	ResolveInvitationFunc      func(ctx context.Context, org string, invitationID int64, githubLogin string) error
	UpdateStatusFunc           func(ctx context.Context, org string, invitationID int64, status models.InvitationStatus) error
	UpdateRoleFunc             func(ctx context.Context, org string, invitationID int64, role models.OrgRole) error
	GetByEmailFunc             func(ctx context.Context, email string, org string) ([]models.InvitationMapping, error)
	GetAuditLogCursorFunc      func(ctx context.Context, org string) (*models.AuditLogCursor, error)
	SaveAuditLogCursorFunc     func(ctx context.Context, cursor models.AuditLogCursor) error
	GetAllResolvedMappingsFunc func(ctx context.Context, org string) (map[string]string, error)

	// Track calls for assertions.
	SavedInvitations []models.InvitationMapping
	ResolvedCalls    []ResolveCall
	StatusCalls      []StatusCall
	RoleCalls        []RoleCall
	SavedCursors     []models.AuditLogCursor
}

// ResolveCall records a call to ResolveInvitation.
type ResolveCall struct {
	Org          string
	InvitationID int64
	GitHubLogin  string
}

// StatusCall records a call to UpdateStatus.
type StatusCall struct {
	Org          string
	InvitationID int64
	Status       models.InvitationStatus
}

// RoleCall records a call to UpdateRole.
type RoleCall struct {
	Org          string
	InvitationID int64
	Role         models.OrgRole
}

func (m *MockStore) SaveInvitation(ctx context.Context, mapping models.InvitationMapping) error {
	m.SavedInvitations = append(m.SavedInvitations, mapping)
	if m.SaveInvitationFunc != nil {
		return m.SaveInvitationFunc(ctx, mapping)
	}
	return nil
}

func (m *MockStore) GetInvitation(ctx context.Context, org string, invitationID int64) (*models.InvitationMapping, error) {
	if m.GetInvitationFunc != nil {
		return m.GetInvitationFunc(ctx, org, invitationID)
	}
	return nil, nil
}

func (m *MockStore) GetPendingInvitations(ctx context.Context, org string) ([]models.InvitationMapping, error) {
	if m.GetPendingInvitationsFunc != nil {
		return m.GetPendingInvitationsFunc(ctx, org)
	}
	return nil, nil
}

func (m *MockStore) ResolveInvitation(ctx context.Context, org string, invitationID int64, githubLogin string) error {
	m.ResolvedCalls = append(m.ResolvedCalls, ResolveCall{Org: org, InvitationID: invitationID, GitHubLogin: githubLogin})
	if m.ResolveInvitationFunc != nil {
		return m.ResolveInvitationFunc(ctx, org, invitationID, githubLogin)
	}
	return nil
}

func (m *MockStore) UpdateStatus(ctx context.Context, org string, invitationID int64, status models.InvitationStatus) error {
	m.StatusCalls = append(m.StatusCalls, StatusCall{Org: org, InvitationID: invitationID, Status: status})
	if m.UpdateStatusFunc != nil {
		return m.UpdateStatusFunc(ctx, org, invitationID, status)
	}
	return nil
}

func (m *MockStore) UpdateRole(ctx context.Context, org string, invitationID int64, role models.OrgRole) error {
	m.RoleCalls = append(m.RoleCalls, RoleCall{Org: org, InvitationID: invitationID, Role: role})
	if m.UpdateRoleFunc != nil {
		return m.UpdateRoleFunc(ctx, org, invitationID, role)
	}
	return nil
}

func (m *MockStore) GetByEmail(ctx context.Context, email string, org string) ([]models.InvitationMapping, error) {
	if m.GetByEmailFunc != nil {
		return m.GetByEmailFunc(ctx, email, org)
	}
	return nil, nil
}

func (m *MockStore) GetAuditLogCursor(ctx context.Context, org string) (*models.AuditLogCursor, error) {
	if m.GetAuditLogCursorFunc != nil {
		return m.GetAuditLogCursorFunc(ctx, org)
	}
	return nil, nil
}

func (m *MockStore) SaveAuditLogCursor(ctx context.Context, cursor models.AuditLogCursor) error {
	m.SavedCursors = append(m.SavedCursors, cursor)
	if m.SaveAuditLogCursorFunc != nil {
		return m.SaveAuditLogCursorFunc(ctx, cursor)
	}
	return nil
}

func (m *MockStore) GetAllResolvedMappings(ctx context.Context, org string) (map[string]string, error) {
	if m.GetAllResolvedMappingsFunc != nil {
		return m.GetAllResolvedMappingsFunc(ctx, org)
	}
	return map[string]string{}, nil
}
