# Configuration

google-workspace-github-sync supports three configuration sources, applied in order of precedence:

1. **CLI flags** (highest priority)
2. **Environment variables**
3. **YAML config file** (lowest priority)

---

## Config File (`config.yaml`)

```yaml
google:
  admin_email: admin@yourdomain.com           # Google Workspace admin for impersonation
  credentials_file: ./credentials.json        # Path to service account JSON (CLI mode)
  credentials_secret: google-workspace-github-sync/creds # AWS Secrets Manager key (Lambda mode)
  members_group: github-members@yourdomain.com # Google group → GitHub "member" role
  owners_group: github-owners@yourdomain.com   # Google group → GitHub "admin" role

github:
  organization: your-github-org               # GitHub organization name
  token: ghp_xxx                              # Personal Access Token (CLI mode)
  token_secret: google-workspace-github-sync/token      # AWS Secrets Manager key (Lambda mode)

sync:
  dry_run: true                               # Preview mode — no changes applied
  ignore_suspended: true                      # Skip suspended Google Workspace users
  remove_extra_members: false                 # Remove mode: conservative (false) or aggressive (true)

log:
  level: info                                 # Log level: debug, info, warn, error
  format: json                                # Log format: json or text

dynamodb:
  enabled: true                               # Enable invitation reconciliation
  table_name: invitation-mappings             # DynamoDB table name
  region: eu-west-1                           # AWS region for DynamoDB
  endpoint: http://localhost:8000             # Local endpoint (dev only, omit for AWS)
  ttl_days: 90                                # TTL for invitation records (days)
```

---

## Environment Variables

| Variable | Config Path | Description |
|----------|------------|-------------|
| `GOOGLE_ADMIN_EMAIL` | `google.admin_email` | Admin email for Google domain-wide delegation |
| `GOOGLE_CREDENTIALS_FILE` | `google.credentials_file` | Path to service account JSON key |
| `GOOGLE_CREDENTIALS_SECRET` | `google.credentials_secret` | Secrets Manager key for credentials |
| `GOOGLE_MEMBERS_GROUP` | `google.members_group` | Google group for org members |
| `GOOGLE_OWNERS_GROUP` | `google.owners_group` | Google group for org admins/owners |
| `GITHUB_ORG` | `github.organization` | GitHub organization name |
| `GITHUB_TOKEN` | `github.token` | GitHub Personal Access Token |
| `GITHUB_TOKEN_SECRET` | `github.token_secret` | Secrets Manager key for GitHub token |
| `DRY_RUN` | `sync.dry_run` | Enable dry-run mode (`true`/`false`) |
| `IGNORE_SUSPENDED` | `sync.ignore_suspended` | Skip suspended Google users (`true`/`false`) |
| `REMOVE_EXTRA_MEMBERS` | `sync.remove_extra_members` | Remove mode (`true`/`false`) |
| `LOG_LEVEL` | `log.level` | Log level |
| `LOG_FORMAT` | `log.format` | Log format |
| `DYNAMODB_ENABLED` | `dynamodb.enabled` | Enable DynamoDB invitation tracking |
| `DYNAMODB_TABLE_NAME` | `dynamodb.table_name` | DynamoDB table name |
| `DYNAMODB_REGION` | `dynamodb.region` | DynamoDB AWS region |
| `DYNAMODB_ENDPOINT` | `dynamodb.endpoint` | DynamoDB endpoint (local dev) |
| `DYNAMODB_TTL_DAYS` | `dynamodb.ttl_days` | TTL for records in days |

---

## CLI Flags

```bash
./google-workspace-github-sync [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `config.yaml` | Path to YAML config file |
| `--dry-run` | `true` | Preview mode |
| `--github-token` | — | GitHub PAT (overrides config/env) |
| `--github-org` | — | GitHub organization |
| `--google-admin` | — | Google admin email |
| `--google-credentials` | — | Path to credentials JSON |
| `--members-group` | — | Google members group email |
| `--owners-group` | — | Google owners group email |
| `--log-level` | `info` | Log level |
| `--log-format` | `json` | Log format |

CLI flags take highest precedence and override both config file and environment variables.

---

## Defaults

| Setting | Default Value |
|---------|---------------|
| `sync.dry_run` | `true` (safe by default) |
| `sync.ignore_suspended` | `true` |
| `sync.remove_extra_members` | `false` (conservative mode) |
| `log.level` | `info` |
| `log.format` | `json` |
| `dynamodb.table_name` | `invitation-mappings` |
| `dynamodb.region` | `eu-west-1` |
| `dynamodb.ttl_days` | `90` |
| `dynamodb.enabled` | `false` |

---

## Validation Rules

The configuration is validated at startup. The following rules apply:

| Rule | Condition |
|------|-----------|
| `google.admin_email` | Required, must be a valid email |
| `google.members_group` | Required, must be a valid email |
| `google.owners_group` | Required, must be a valid email |
| `github.organization` | Required |
| `google.credentials_file` | Required in CLI mode |
| `google.credentials_secret` | Required in Lambda mode |
| `github.token` | Required in CLI mode |
| `github.token_secret` | Required in Lambda mode |
| `dynamodb.table_name` | Required if DynamoDB enabled |
| `dynamodb.region` | Required if DynamoDB enabled |
| `dynamodb.ttl_days` | Must be > 0 if DynamoDB enabled |

---

## Lambda vs CLI Mode

The tool auto-detects its execution mode by checking the `AWS_LAMBDA_FUNCTION_NAME` environment variable.

| Feature | CLI Mode | Lambda Mode |
|---------|----------|-------------|
| Credentials source | `credentials_file` (local JSON) | `credentials_secret` (Secrets Manager) |
| GitHub token source | `token` (flag/env/config) | `token_secret` (Secrets Manager) |
| Trigger | Manual execution | EventBridge scheduled event |
| Config file | Loaded via `--config` flag | Environment variables only |
| DynamoDB endpoint | Can use local endpoint | Uses AWS DynamoDB service |

---

## Google Workspace Group Mapping

The tool maps two Google groups to GitHub organization roles:

| Google Group | GitHub Role | Description |
|-------------|-------------|-------------|
| `members_group` | `member` | Regular organization member |
| `owners_group` | `admin` | Organization owner/admin |

**Precedence rule**: If a user is in both groups, the **owners_group takes precedence** — they will be assigned the `admin` role.
