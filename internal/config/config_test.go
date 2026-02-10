package config

import "testing"

func TestValidateConfig(t *testing.T) {
	validLocal := Config{
		Google: GoogleConfig{
			AdminEmail:      "admin@example.com",
			MembersGroup:    "members@example.com",
			OwnersGroup:     "owners@example.com",
			CredentialsFile: "/tmp/creds.json",
		},
		GitHub: GitHubConfig{
			Organization: "example-org",
			Token:        "ghp_test",
		},
		Sync: SyncConfig{
			DryRun: true,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}

	cases := []struct {
		name    string
		cfg     Config
		isLambda bool
		wantErr bool
	}{
		{
			name:    "valid local config",
			cfg:     validLocal,
			isLambda: false,
			wantErr: false,
		},
		{
			name: "missing admin email",
			cfg: func() Config {
				c := validLocal
				c.Google.AdminEmail = ""
				return c
			}(),
			isLambda: false,
			wantErr: true,
		},
		{
			name: "invalid group email",
			cfg: func() Config {
				c := validLocal
				c.Google.MembersGroup = "not-an-email"
				return c
			}(),
			isLambda: false,
			wantErr: true,
		},
		{
			name: "lambda missing secrets",
			cfg: func() Config {
				c := validLocal
				c.Google.CredentialsFile = ""
				c.GitHub.Token = ""
				return c
			}(),
			isLambda: true,
			wantErr: true,
		},
		{
			name: "valid lambda config",
			cfg: func() Config {
				c := validLocal
				c.Google.CredentialsFile = ""
				c.GitHub.Token = ""
				c.Google.CredentialsSecret = "google-creds"
				c.GitHub.TokenSecret = "github-token"
				return c
			}(),
			isLambda: true,
			wantErr: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg
			cfg.IsLambda = tc.isLambda
			err := Validate(&cfg)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
