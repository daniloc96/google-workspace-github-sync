package config

import (
	"errors"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Load reads configuration from file, environment variables, and defaults.
func Load(configFile string) (*Config, error) {
	v := viper.New()
	v.SetDefault("sync.dry_run", true)
	v.SetDefault("sync.ignore_suspended", true)
	v.SetDefault("sync.remove_extra_members", false)
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
	v.SetDefault("dynamodb.enabled", false)
	v.SetDefault("dynamodb.table_name", "invitation-mappings")
	v.SetDefault("dynamodb.region", "eu-west-1")
	v.SetDefault("dynamodb.ttl_days", 90)

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	_ = v.BindEnv("google.admin_email", "GOOGLE_ADMIN_EMAIL")
	_ = v.BindEnv("google.credentials_file", "GOOGLE_CREDENTIALS_FILE")
	_ = v.BindEnv("google.credentials_secret", "GOOGLE_CREDENTIALS_SECRET")
	_ = v.BindEnv("google.members_group", "GOOGLE_MEMBERS_GROUP")
	_ = v.BindEnv("google.owners_group", "GOOGLE_OWNERS_GROUP")
	_ = v.BindEnv("github.organization", "GITHUB_ORG")
	_ = v.BindEnv("github.token", "GITHUB_TOKEN")
	_ = v.BindEnv("github.token_secret", "GITHUB_TOKEN_SECRET")
	_ = v.BindEnv("sync.dry_run", "DRY_RUN")
	_ = v.BindEnv("sync.ignore_suspended", "IGNORE_SUSPENDED")
	_ = v.BindEnv("sync.remove_extra_members", "REMOVE_EXTRA_MEMBERS")
	_ = v.BindEnv("log.level", "LOG_LEVEL")
	_ = v.BindEnv("log.format", "LOG_FORMAT")
	_ = v.BindEnv("dynamodb.enabled", "DYNAMODB_ENABLED")
	_ = v.BindEnv("dynamodb.table_name", "DYNAMODB_TABLE_NAME")
	_ = v.BindEnv("dynamodb.region", "DYNAMODB_REGION")
	_ = v.BindEnv("dynamodb.endpoint", "DYNAMODB_ENDPOINT")
	_ = v.BindEnv("dynamodb.ttl_days", "DYNAMODB_TTL_DAYS")

	if configFile != "" {
		v.SetConfigFile(configFile)
		if err := v.ReadInConfig(); err != nil {
			return nil, err
		}
	} else {
		v.SetConfigName("config")
		v.AddConfigPath(".")
		if err := v.ReadInConfig(); err != nil {
			var notFound viper.ConfigFileNotFoundError
			if !errors.As(err, &notFound) {
				return nil, err
			}
		}
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}

	// Explicitly map values to avoid tag mismatch issues.
	cfg.Google.AdminEmail = v.GetString("google.admin_email")
	cfg.Google.CredentialsFile = v.GetString("google.credentials_file")
	cfg.Google.CredentialsSecret = v.GetString("google.credentials_secret")
	cfg.Google.MembersGroup = v.GetString("google.members_group")
	cfg.Google.OwnersGroup = v.GetString("google.owners_group")

	cfg.GitHub.Organization = v.GetString("github.organization")
	cfg.GitHub.Token = v.GetString("github.token")
	cfg.GitHub.TokenSecret = v.GetString("github.token_secret")

	cfg.Sync.DryRun = v.GetBool("sync.dry_run")
	cfg.Sync.IgnoreSuspended = v.GetBool("sync.ignore_suspended")
	cfg.Sync.RemoveExtraMembers = v.GetBool("sync.remove_extra_members")

	cfg.Log.Level = v.GetString("log.level")
	cfg.Log.Format = v.GetString("log.format")

	cfg.DynamoDB.Enabled = v.GetBool("dynamodb.enabled")
	cfg.DynamoDB.TableName = v.GetString("dynamodb.table_name")
	cfg.DynamoDB.Region = v.GetString("dynamodb.region")
	cfg.DynamoDB.Endpoint = v.GetString("dynamodb.endpoint")
	cfg.DynamoDB.TTLDays = v.GetInt("dynamodb.ttl_days")

	cfg.IsLambda = os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != ""

	return cfg, nil
}
