package config

// Config holds all configuration for the sync operation.
type Config struct {
	Google   GoogleConfig   `json:"google"`
	GitHub   GitHubConfig   `json:"github"`
	Sync     SyncConfig     `json:"sync"`
	Log      LogConfig      `json:"log"`
	DynamoDB DynamoDBConfig `json:"dynamodb"`
	IsLambda bool           `json:"-"`
}

// DynamoDBConfig holds DynamoDB settings for invitation tracking.
type DynamoDBConfig struct {
	TableName string `json:"table_name"`
	Region    string `json:"region"`
	Endpoint  string `json:"endpoint,omitempty"`
	Enabled   bool   `json:"enabled"`
	TTLDays   int    `json:"ttl_days"`
}

// GoogleConfig holds Google Workspace settings.
type GoogleConfig struct {
	AdminEmail        string `json:"admin_email"`
	MembersGroup      string `json:"members_group"`
	OwnersGroup       string `json:"owners_group"`
	CredentialsFile   string `json:"credentials_file,omitempty"`
	CredentialsSecret string `json:"credentials_secret,omitempty"`
}

// GitHubConfig holds GitHub settings.
type GitHubConfig struct {
	Organization string `json:"organization"`
	Token        string `json:"-"`
	TokenSecret  string `json:"token_secret,omitempty"`
}

// SyncConfig holds sync behavior settings.
type SyncConfig struct {
	DryRun            bool `json:"dry_run"`
	IgnoreSuspended   bool `json:"ignore_suspended"`
	RemoveExtraMembers bool `json:"remove_extra_members"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string `json:"level"`
	Format string `json:"format"`
}
