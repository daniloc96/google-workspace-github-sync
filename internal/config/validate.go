package config

import (
	"fmt"
	"net/mail"
	"strings"
)

// Validate ensures configuration is complete and well-formed.
func Validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	var errs []string

	requireEmail := func(value string, field string) {
		if value == "" {
			errs = append(errs, fmt.Sprintf("%s is required", field))
			return
		}
		if _, err := mail.ParseAddress(value); err != nil {
			errs = append(errs, fmt.Sprintf("%s must be a valid email", field))
		}
	}

	requireNonEmpty := func(value string, field string) {
		if value == "" {
			errs = append(errs, fmt.Sprintf("%s is required", field))
		}
	}

	requireEmail(cfg.Google.AdminEmail, "google.admin_email")
	requireEmail(cfg.Google.MembersGroup, "google.members_group")
	requireEmail(cfg.Google.OwnersGroup, "google.owners_group")
	requireNonEmpty(cfg.GitHub.Organization, "github.organization")

	if cfg.IsLambda {
		requireNonEmpty(cfg.Google.CredentialsSecret, "google.credentials_secret")
		requireNonEmpty(cfg.GitHub.TokenSecret, "github.token_secret")
	} else {
		requireNonEmpty(cfg.Google.CredentialsFile, "google.credentials_file")
		requireNonEmpty(cfg.GitHub.Token, "github.token")
	}

	if cfg.DynamoDB.Enabled {
		requireNonEmpty(cfg.DynamoDB.TableName, "dynamodb.table_name")
		requireNonEmpty(cfg.DynamoDB.Region, "dynamodb.region")
		if cfg.DynamoDB.TTLDays <= 0 {
			errs = append(errs, "dynamodb.ttl_days must be positive")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed: %s", strings.Join(errs, "; "))
	}

	return nil
}
