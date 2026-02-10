package google

import (
	"context"

	"github.com/daniloc96/google-workspace-github-sync/internal/models"
)

// MockClient is a simple mock implementation of the Google client.
type MockClient struct {
	GetGroupMembersFunc        func(ctx context.Context, groupEmail string) ([]models.GoogleGroupMember, error)
	GetUsersSuspendedStatusFunc func(ctx context.Context, emails []string) (map[string]bool, error)
}

func (m *MockClient) GetGroupMembers(ctx context.Context, groupEmail string) ([]models.GoogleGroupMember, error) {
	if m.GetGroupMembersFunc == nil {
		return nil, nil
	}
	return m.GetGroupMembersFunc(ctx, groupEmail)
}

func (m *MockClient) GetUsersSuspendedStatus(ctx context.Context, emails []string) (map[string]bool, error) {
	if m.GetUsersSuspendedStatusFunc == nil {
		return map[string]bool{}, nil
	}
	return m.GetUsersSuspendedStatusFunc(ctx, emails)
}
