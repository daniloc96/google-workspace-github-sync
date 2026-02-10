package sync

import (
	"context"
	"testing"

	"github.com/daniloc96/google-workspace-github-sync/internal/github"
	"github.com/daniloc96/google-workspace-github-sync/internal/models"
)

func TestExecuteInviteAction(t *testing.T) {
	called := false
	mock := &github.MockClient{
		CreateInvitationFunc: func(ctx context.Context, org string, email string, role models.OrgRole) (*models.GitHubOrgMember, error) {
			called = true
			return &models.GitHubOrgMember{Email: &email, Role: role}, nil
		},
	}

	actions := []models.SyncAction{
		{
			Type:       models.ActionInvite,
			Email:      "user@example.com",
			TargetRole: ptrRole(models.RoleMember),
			Reason:     "missing in GitHub",
		},
	}

	updated, err := ExecuteActions(context.Background(), mock, "example-org", actions, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Fatalf("expected CreateInvitation to be called")
	}
	if !updated[0].Executed {
		t.Fatalf("expected action to be marked executed")
	}
}

func TestExecuteRemoveAction(t *testing.T) {
	called := false
	mock := &github.MockClient{
		RemoveMemberFunc: func(ctx context.Context, org string, username string) error {
			called = true
			return nil
		},
	}

	actions := []models.SyncAction{
		{
			Type:   models.ActionRemove,
			Email:  "user1",
			Reason: "missing from Google groups",
		},
	}

	updated, err := ExecuteActions(context.Background(), mock, "example-org", actions, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Fatalf("expected RemoveMember to be called")
	}
	if !updated[0].Executed {
		t.Fatalf("expected action to be marked executed")
	}
}

func TestExecuteUpdateRoleAction(t *testing.T) {
	called := false
	mock := &github.MockClient{
		UpdateMemberRoleFunc: func(ctx context.Context, org string, username string, role models.OrgRole) error {
			called = true
			return nil
		},
	}

	actions := []models.SyncAction{
		{
			Type:        models.ActionUpdateRole,
			Email:       "user1",
			CurrentRole: ptrRole(models.RoleMember),
			TargetRole:  ptrRole(models.RoleOwner),
			Reason:      "role mismatch",
		},
	}

	updated, err := ExecuteActions(context.Background(), mock, "example-org", actions, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Fatalf("expected UpdateMemberRole to be called")
	}
	if !updated[0].Executed {
		t.Fatalf("expected action to be marked executed")
	}
}

func ptrRole(role models.OrgRole) *models.OrgRole {
	return &role
}

func TestExecuteInviteAlreadyInOrgUpgradesToRoleUpdate(t *testing.T) {
	searchCalled := false
	updateCalled := false
	var updatedUsername string
	var updatedRole models.OrgRole

	mock := &github.MockClient{
		CreateInvitationFunc: func(ctx context.Context, org string, email string, role models.OrgRole) (*models.GitHubOrgMember, error) {
			return nil, &github.ErrAlreadyMember{Email: email}
		},
		SearchUserByEmailFunc: func(ctx context.Context, email string) (string, error) {
			searchCalled = true
			return "found-user", nil
		},
		UpdateMemberRoleFunc: func(ctx context.Context, org string, username string, role models.OrgRole) error {
			updateCalled = true
			updatedUsername = username
			updatedRole = role
			return nil
		},
	}

	actions := []models.SyncAction{
		{
			Type:       models.ActionInvite,
			Email:      "user@example.com",
			TargetRole: ptrRole(models.RoleOwner),
			Reason:     "missing in GitHub organization",
		},
	}

	updated, err := ExecuteActions(context.Background(), mock, "example-org", actions, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !searchCalled {
		t.Fatal("expected SearchUserByEmail to be called")
	}
	if !updateCalled {
		t.Fatal("expected UpdateMemberRole to be called")
	}
	if updatedUsername != "found-user" {
		t.Fatalf("expected username 'found-user', got '%s'", updatedUsername)
	}
	if updatedRole != models.RoleOwner {
		t.Fatalf("expected role 'admin', got '%s'", updatedRole)
	}
	if updated[0].Type != models.ActionUpdateRole {
		t.Fatalf("expected action type to be upgrade to update_role, got '%s'", updated[0].Type)
	}
	if !updated[0].Executed {
		t.Fatal("expected action to be marked executed")
	}
	if !updated[0].AlreadyInOrg {
		t.Fatal("expected AlreadyInOrg to be true")
	}
	if updated[0].Username != "found-user" {
		t.Fatalf("expected Username 'found-user', got '%s'", updated[0].Username)
	}
	if updated[0].GoogleEmail != "user@example.com" {
		t.Fatalf("expected GoogleEmail 'user@example.com', got '%s'", updated[0].GoogleEmail)
	}
}

func TestExecuteInviteAlreadyInOrgNoSearchResult(t *testing.T) {
	mock := &github.MockClient{
		CreateInvitationFunc: func(ctx context.Context, org string, email string, role models.OrgRole) (*models.GitHubOrgMember, error) {
			return nil, &github.ErrAlreadyMember{Email: email}
		},
		SearchUserByEmailFunc: func(ctx context.Context, email string) (string, error) {
			return "", nil // no match found
		},
	}

	actions := []models.SyncAction{
		{
			Type:       models.ActionInvite,
			Email:      "user@example.com",
			TargetRole: ptrRole(models.RoleMember),
			Reason:     "missing in GitHub organization",
		},
	}

	updated, err := ExecuteActions(context.Background(), mock, "example-org", actions, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !updated[0].AlreadyInOrg {
		t.Fatal("expected AlreadyInOrg to be true")
	}
	if updated[0].Executed {
		t.Fatal("expected action to NOT be executed (no username found)")
	}
	if updated[0].Error == nil {
		t.Fatal("expected error to be set")
	}
}
