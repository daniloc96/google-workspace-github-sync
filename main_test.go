package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/daniloc96/google-workspace-github-sync/internal/config"
	"github.com/daniloc96/google-workspace-github-sync/internal/models"
)

func TestHandleRequest(t *testing.T) {
	originalRunSync := runSync
	defer func() { runSync = originalRunSync }()

	os.Setenv("GOOGLE_ADMIN_EMAIL", "admin@example.com")
	os.Setenv("GOOGLE_MEMBERS_GROUP", "members@example.com")
	os.Setenv("GOOGLE_OWNERS_GROUP", "owners@example.com")
	os.Setenv("GOOGLE_CREDENTIALS_FILE", "/tmp/creds.json")
	os.Setenv("GITHUB_ORG", "example-org")
	os.Setenv("GITHUB_TOKEN", "ghp_test")
	os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")

	runSync = func(ctx context.Context, cfg *config.Config) (*models.SyncResult, error) {
		return &models.SyncResult{
			DryRun: cfg.Sync.DryRun,
			StartTime: time.Now(),
			EndTime: time.Now(),
			Summary: models.SyncSummary{ActionsPlanned: 1},
		}, nil
	}

	dryRun := false
	event := models.LambdaEvent{DryRun: &dryRun}
	resp, err := HandleRequest(context.Background(), event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d (%s)", resp.StatusCode, resp.Message)
	}
	if resp.Result == nil || resp.Result.DryRun != dryRun {
		t.Fatalf("expected dry_run %v, got %#v", dryRun, resp.Result)
	}
}

func TestHandleRequestDryRunMessage(t *testing.T) {
	originalRunSync := runSync
	defer func() { runSync = originalRunSync }()

	os.Setenv("GOOGLE_ADMIN_EMAIL", "admin@example.com")
	os.Setenv("GOOGLE_MEMBERS_GROUP", "members@example.com")
	os.Setenv("GOOGLE_OWNERS_GROUP", "owners@example.com")
	os.Setenv("GOOGLE_CREDENTIALS_FILE", "/tmp/creds.json")
	os.Setenv("GITHUB_ORG", "example-org")
	os.Setenv("GITHUB_TOKEN", "ghp_test")
	os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")

	runSync = func(ctx context.Context, cfg *config.Config) (*models.SyncResult, error) {
		return &models.SyncResult{
			DryRun: cfg.Sync.DryRun,
			StartTime: time.Now(),
			EndTime: time.Now(),
			Summary: models.SyncSummary{ActionsPlanned: 2},
		}, nil
	}

	dryRun := true
	event := models.LambdaEvent{DryRun: &dryRun}
	resp, err := HandleRequest(context.Background(), event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d (%s)", resp.StatusCode, resp.Message)
	}
	if resp.Message == "" || !strings.HasPrefix(resp.Message, "[DRY RUN]") {
		t.Fatalf("expected dry-run message, got %s", resp.Message)
	}
}

func TestHandleRequestScheduledEvent(t *testing.T) {
	originalRunSync := runSync
	defer func() { runSync = originalRunSync }()

	os.Setenv("GOOGLE_ADMIN_EMAIL", "admin@example.com")
	os.Setenv("GOOGLE_MEMBERS_GROUP", "members@example.com")
	os.Setenv("GOOGLE_OWNERS_GROUP", "owners@example.com")
	os.Setenv("GOOGLE_CREDENTIALS_FILE", "/tmp/creds.json")
	os.Setenv("GITHUB_ORG", "example-org")
	os.Setenv("GITHUB_TOKEN", "ghp_test")
	os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")

	runSync = func(ctx context.Context, cfg *config.Config) (*models.SyncResult, error) {
		return &models.SyncResult{
			DryRun: cfg.Sync.DryRun,
			StartTime: time.Now(),
			EndTime: time.Now(),
			Summary: models.SyncSummary{ActionsPlanned: 0},
		}, nil
	}

	event := models.LambdaEvent{Source: "aws.events", DetailType: "Scheduled Event"}
	resp, err := HandleRequest(context.Background(), event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d (%s)", resp.StatusCode, resp.Message)
	}
}
