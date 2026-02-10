package sync

import (
	"context"
	"sync"
	"testing"

	"github.com/daniloc96/google-workspace-github-sync/internal/config"
	"github.com/daniloc96/google-workspace-github-sync/internal/github"
	"github.com/daniloc96/google-workspace-github-sync/internal/google"
	"github.com/daniloc96/google-workspace-github-sync/internal/models"
)

func TestDryRunDoesNotExecuteActions(t *testing.T) {
	createCalled := false
	googleClient := &google.MockClient{
		GetGroupMembersFunc: func(ctx context.Context, groupEmail string) ([]models.GoogleGroupMember, error) {
			return []models.GoogleGroupMember{{Email: "user@example.com", Type: "USER", Status: "ACTIVE"}}, nil
		},
	}
	githubClient := &github.MockClient{
		CreateInvitationFunc: func(ctx context.Context, org string, email string, role models.OrgRole) (*models.GitHubOrgMember, error) {
			createCalled = true
			return &models.GitHubOrgMember{}, nil
		},
		ListMembersFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
		ListPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
	}

	cfg := &config.Config{
		Google: config.GoogleConfig{MembersGroup: "members@example.com", OwnersGroup: "owners@example.com"},
		GitHub: config.GitHubConfig{Organization: "example-org"},
		Sync:   config.SyncConfig{DryRun: true},
	}

	engine := NewEngine(googleClient, githubClient, cfg)
	result, err := engine.Sync(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if createCalled {
		t.Fatalf("expected no invitation calls in dry-run")
	}
	if len(result.Actions) != 1 || result.Actions[0].Executed {
		t.Fatalf("expected 1 non-executed action, got %#v", result.Actions)
	}
}

func TestDryRunIncludesActionReport(t *testing.T) {
	googleClient := &google.MockClient{
		GetGroupMembersFunc: func(ctx context.Context, groupEmail string) ([]models.GoogleGroupMember, error) {
			return []models.GoogleGroupMember{{Email: "user@example.com", Type: "USER", Status: "ACTIVE"}}, nil
		},
	}
	githubClient := &github.MockClient{
		ListMembersFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
		ListPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
	}

	cfg := &config.Config{
		Google: config.GoogleConfig{MembersGroup: "members@example.com", OwnersGroup: "owners@example.com"},
		GitHub: config.GitHubConfig{Organization: "example-org"},
		Sync:   config.SyncConfig{DryRun: true},
	}

	engine := NewEngine(googleClient, githubClient, cfg)
	result, err := engine.Sync(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result.Actions) != 1 {
		t.Fatalf("expected 1 action in report, got %d", len(result.Actions))
	}
}

func TestSyncConcurrentExecution(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once

	googleClient := &google.MockClient{
		GetGroupMembersFunc: func(ctx context.Context, groupEmail string) ([]models.GoogleGroupMember, error) {
			once.Do(func() { close(started) })
			<-release
			return []models.GoogleGroupMember{}, nil
		},
	}
	githubClient := &github.MockClient{
		ListMembersFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
		ListPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
	}

	cfg := &config.Config{
		Google: config.GoogleConfig{MembersGroup: "members@example.com", OwnersGroup: "owners@example.com"},
		GitHub: config.GitHubConfig{Organization: "example-org"},
		Sync:   config.SyncConfig{DryRun: true},
	}

	engine := NewEngine(googleClient, githubClient, cfg)

	go func() {
		_, _ = engine.Sync(context.Background())
	}()

	<-started
	if _, err := engine.Sync(context.Background()); err == nil {
		t.Fatalf("expected concurrent sync to be rejected")
	}
	close(release)
}

func TestSyncVerifiedEmailsPreventsInvite(t *testing.T) {
	// User is in Google group AND already in GitHub org.
	// No public email, but verified domain email resolves the match.
	// Expected: no invite, no actions.
	googleClient := &google.MockClient{
		GetGroupMembersFunc: func(ctx context.Context, groupEmail string) ([]models.GoogleGroupMember, error) {
			if groupEmail == "members@example.com" {
				return []models.GoogleGroupMember{{Email: "user@company.com", Type: "USER", Status: "ACTIVE"}}, nil
			}
			return nil, nil
		},
	}
	username := "ghuser"
	githubClient := &github.MockClient{
		ListMembersFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return []models.GitHubOrgMember{
				{Username: &username, Role: models.RoleMember}, // no email
			}, nil
		},
		ListPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
		ListMembersWithVerifiedEmailsFunc: func(ctx context.Context, org string) (map[string]string, error) {
			return map[string]string{"user@company.com": "ghuser"}, nil
		},
	}

	cfg := &config.Config{
		Google: config.GoogleConfig{MembersGroup: "members@example.com", OwnersGroup: "owners@example.com"},
		GitHub: config.GitHubConfig{Organization: "example-org"},
		Sync:   config.SyncConfig{DryRun: true},
	}

	engine := NewEngine(googleClient, githubClient, cfg)
	result, err := engine.Sync(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	for _, a := range result.Actions {
		if a.Type == models.ActionInvite {
			t.Fatalf("should NOT invite user matched via verified email, got: %+v", a)
		}
	}
}
