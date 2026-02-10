package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/daniloc96/google-workspace-github-sync/internal/config"
	"github.com/daniloc96/google-workspace-github-sync/internal/log"
	"github.com/daniloc96/google-workspace-github-sync/internal/models"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	cfgFile     string
	flagDryRun  bool
	flagLogLevel  string
	flagLogFormat string
	flagGoogleAdmin string
	flagGoogleCreds string
	flagMembersGroup string
	flagOwnersGroup  string
	flagGitHubOrg    string
	flagGitHubToken  string

	lambdaHandler func(ctx context.Context, event models.LambdaEvent) (*models.LambdaResponse, error)
	runSync       func(ctx context.Context, cfg *config.Config) (*models.SyncResult, error)
)

// SetLambdaHandler registers the Lambda handler used in Lambda mode.
func SetLambdaHandler(handler func(ctx context.Context, event models.LambdaEvent) (*models.LambdaResponse, error)) {
	lambdaHandler = handler
}

// SetRunSync registers the sync runner used by the CLI.
func SetRunSync(handler func(ctx context.Context, cfg *config.Config) (*models.SyncResult, error)) {
	runSync = handler
}

var rootCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync Google Workspace users to GitHub Organization",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}

		overrideConfigFromFlags(cmd, cfg)
		if err := config.Validate(cfg); err != nil {
			return err
		}

		logger := log.NewLogger(cfg.Log.Level, cfg.Log.Format)
		logrus.SetFormatter(logger.Formatter)
		logrus.SetLevel(logger.Level)
		logrus.SetOutput(logger.Out)

		if runSync == nil {
			return fmt.Errorf("sync engine is not configured")
		}

		result, err := runSync(context.Background(), cfg)
		if err != nil {
			return err
		}

		logrus.WithFields(logrus.Fields{
			"dry_run":         result.DryRun,
			"duration_ms":     result.DurationMs,
		}).Info(result.Summary.String())

		// Print detailed user lists with clear separators
		logrus.Info("──────────────────────────────────────────")
		printUserList("✉️  Invited users", result.InvitedUsers)
		printUserList("👤 Already in organization (skipped)", result.AlreadyInOrgUsers)
		printUserList("👻 GitHub members NOT in any Google group (orphaned)", result.OrphanedGitHubUsers)
		logrus.Info("──────────────────────────────────────────")

		return nil
	},
}

// Execute runs the CLI or Lambda handler depending on environment.
func Execute() {
	if isLambda() {
		if lambdaHandler == nil {
			logrus.Fatal("lambda handler is not configured")
		}
		lambda.Start(lambdaHandler)
		return
	}

	if err := rootCmd.Execute(); err != nil {
		logrus.Fatal(err)
	}
}

func printUserList(title string, users []string) {
	if len(users) == 0 {
		logrus.Infof("%s: (none)", title)
		return
	}
	logrus.Infof("%s (%d):", title, len(users))
	for i, user := range users {
		logrus.Infof("  %d. %s", i+1, user)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")
	rootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", true, "Preview changes without applying")
	rootCmd.PersistentFlags().StringVar(&flagGoogleAdmin, "google-admin", "", "Google Workspace admin email for impersonation")
	rootCmd.PersistentFlags().StringVar(&flagGoogleCreds, "google-creds", "", "Path to Google service account JSON")
	rootCmd.PersistentFlags().StringVar(&flagMembersGroup, "members-group", "", "Google Group email for organization members")
	rootCmd.PersistentFlags().StringVar(&flagOwnersGroup, "owners-group", "", "Google Group email for organization owners")
	rootCmd.PersistentFlags().StringVar(&flagGitHubOrg, "github-org", "", "GitHub organization name")
	rootCmd.PersistentFlags().StringVar(&flagGitHubToken, "github-token", "", "GitHub Personal Access Token")
	rootCmd.PersistentFlags().StringVar(&flagLogLevel, "log-level", "", "Log level: debug, info, warn, error")
	rootCmd.PersistentFlags().StringVar(&flagLogFormat, "log-format", "", "Log format: text or json")
}

func isLambda() bool {
	return os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != ""
}

func overrideConfigFromFlags(cmd *cobra.Command, cfg *config.Config) {
	if cmd.Flags().Changed("dry-run") {
		cfg.Sync.DryRun = flagDryRun
	}
	if cmd.Flags().Changed("google-admin") {
		cfg.Google.AdminEmail = flagGoogleAdmin
	}
	if cmd.Flags().Changed("google-creds") {
		cfg.Google.CredentialsFile = flagGoogleCreds
	}
	if cmd.Flags().Changed("members-group") {
		cfg.Google.MembersGroup = flagMembersGroup
	}
	if cmd.Flags().Changed("owners-group") {
		cfg.Google.OwnersGroup = flagOwnersGroup
	}
	if cmd.Flags().Changed("github-org") {
		cfg.GitHub.Organization = flagGitHubOrg
	}
	if cmd.Flags().Changed("github-token") {
		cfg.GitHub.Token = flagGitHubToken
	}
	if cmd.Flags().Changed("log-level") {
		cfg.Log.Level = flagLogLevel
	}
	if cmd.Flags().Changed("log-format") {
		cfg.Log.Format = flagLogFormat
	}
}
