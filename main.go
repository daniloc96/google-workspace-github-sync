package main

import (
	"context"
	"fmt"

	"github.com/daniloc96/google-workspace-github-sync/cmd"
	"github.com/daniloc96/google-workspace-github-sync/internal/config"
	store "github.com/daniloc96/google-workspace-github-sync/internal/dynamodb"
	"github.com/daniloc96/google-workspace-github-sync/internal/github"
	"github.com/daniloc96/google-workspace-github-sync/internal/google"
	"github.com/daniloc96/google-workspace-github-sync/internal/models"
	"github.com/daniloc96/google-workspace-github-sync/internal/secrets"
	"github.com/daniloc96/google-workspace-github-sync/internal/sync"
	"github.com/sirupsen/logrus"
)

func main() {
	cmd.SetLambdaHandler(HandleRequest)
	cmd.SetRunSync(runSync)
	cmd.Execute()
}

// HandleRequest is the AWS Lambda handler.
func HandleRequest(ctx context.Context, event models.LambdaEvent) (*models.LambdaResponse, error) {
	if event.Source != "" || event.DetailType != "" {
		if !isScheduledEvent(event) {
			return models.NewErrorResponse(fmt.Errorf("unsupported event source")), nil
		}
	}
	cfg, err := config.Load("")
	if err != nil {
		return models.NewErrorResponse(err), nil
	}

	cfg.Sync.DryRun = event.IsDryRun(cfg.Sync.DryRun)
	if err := config.Validate(cfg); err != nil {
		return models.NewErrorResponse(err), nil
	}

	result, err := runSync(ctx, cfg)
	if err != nil {
		return models.NewErrorResponse(err), nil
	}

	return models.NewSuccessResponse(result), nil
}

func isScheduledEvent(event models.LambdaEvent) bool {
	return event.Source == "aws.events" && event.DetailType == "Scheduled Event"
}

var runSync = func(ctx context.Context, cfg *config.Config) (*models.SyncResult, error) {
	googleCreds, err := secrets.ResolveSecretValue(cfg.Google.CredentialsSecret, cfg.Google.CredentialsFile)
	if err != nil {
		return nil, fmt.Errorf("google credentials: %w", err)
	}

	githubToken := cfg.GitHub.Token
	if githubToken == "" {
		token, tokenErr := secrets.ResolveSecretValue(cfg.GitHub.TokenSecret, "")
		if tokenErr != nil {
			return nil, fmt.Errorf("github token: %w", tokenErr)
		}
		githubToken = token
	}

	googleClient, err := google.NewClient(ctx, []byte(googleCreds), cfg.Google.AdminEmail)
	if err != nil {
		return nil, err
	}
	githubClient, err := github.NewClient(githubToken)
	if err != nil {
		return nil, err
	}

	engine := sync.NewEngine(googleClient, githubClient, cfg)

	// Initialize invitation reconciliation if DynamoDB is enabled.
	if cfg.DynamoDB.Enabled {
		dynamoStore, storeErr := store.NewStore(ctx, cfg.DynamoDB)
		if storeErr != nil {
			logrus.WithError(storeErr).Warn("⚠ DynamoDB store init failed — invitation reconciliation disabled")
		} else {
			reconciler := sync.NewReconciler(dynamoStore, githubClient, cfg)
			engine.SetReconciler(reconciler)
			logrus.WithFields(logrus.Fields{
				"table":    cfg.DynamoDB.TableName,
				"region":   cfg.DynamoDB.Region,
				"ttl_days": cfg.DynamoDB.TTLDays,
			}).Info("✅ Invitation reconciliation enabled (DynamoDB)")
		}
	}

	return engine.Sync(ctx)
}
