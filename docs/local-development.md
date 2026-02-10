# Local Development

## Prerequisites

- **Go 1.25+** (check with `go version`)
- **Docker** (for DynamoDB Local)
- **golangci-lint** (optional, for linting)
- **AWS SAM CLI** (optional, for deploy)

---

## Building

```bash
# Build the binary
make build
# → produces bin/google-workspace-github-sync

# Or directly:
go build -o bin/google-workspace-github-sync .
```

---

## Running Locally

### CLI Mode

```bash
# With config.yaml in the current directory
./bin/google-workspace-github-sync --config config.yaml

# Dry-run (overrides config)
./bin/google-workspace-github-sync --config config.yaml --dry-run

# Override specific options via flags
./bin/google-workspace-github-sync \
  --config config.yaml \
  --github-org my-org \
  --log-level debug \
  --log-format text
```

### Minimal `config.yaml` for local development

```yaml
google:
  admin_email: admin@yourdomain.com
  credentials_file: credentials.json
  groups:
    members: github-members@yourdomain.com
    owners: github-owners@yourdomain.com

github:
  org: your-github-org
  token: ghp_xxxxxxxxxxxx  # Or use GITHUB_TOKEN env var

sync:
  dry_run: true              # Always start with dry-run!
  remove_extra_members: false
  ignore_suspended: true

logging:
  level: debug
  format: text

dynamodb:
  enabled: true
  table_name: invitation-mappings
  region: eu-west-1
  endpoint: http://localhost:8000   # DynamoDB Local
```

> **Tip**: For local development, set `logging.level: debug` and `logging.format: text` for readable output.

---

## Testing

```bash
# Run all tests (57 tests)
make test

# Or directly with verbose output:
go test ./... -v

# Run tests for a specific package:
go test ./internal/sync/... -v
go test ./internal/github/... -v
go test ./internal/config/... -v

# Run a single test:
go test ./internal/sync/... -v -run TestCalculateDiff_InviteNewMember
```

### Test structure

| Package | Test File | Coverage |
|---------|-----------|----------|
| `internal/config` | `config_test.go` | Config loading, validation, env vars |
| `internal/github` | `client_test.go` | GitHub API calls (faked `orgService`) |
| `internal/google` | `client_test.go` | Google API calls (mock transport) |
| `internal/sync` | `diff_test.go` | Diff algorithm, conservative/aggressive modes |
| `internal/sync` | `actions_test.go` | Action execution, invite upgrade logic |
| `internal/sync` | `engine_test.go` | Full sync orchestration |
| `internal/log` | `logger_test.go` | Logger configuration |
| `internal/metrics` | `cloudwatch_test.go` | CloudWatch metric publishing |
| `.` | `main_test.go` | Lambda handler integration |

---

## DynamoDB Local

### Starting DynamoDB Local

```bash
# Start DynamoDB Local and dynamodb-admin UI
make dynamodb-up
# → DynamoDB Local on http://localhost:8000
# → dynamodb-admin UI on http://localhost:8001

# Create the invitation-mappings table with indexes
make dynamodb-setup

# Stop DynamoDB Local
make dynamodb-down

# Full reset (remove volumes + recreate table)
make dynamodb-reset
```

### Docker Compose

The `docker-compose.yml` starts two containers:

| Service | Port | Description |
|---------|------|-------------|
| `dynamodb-local` | 8000 | DynamoDB Local (in-memory mode) |
| `dynamodb-admin` | 8001 | Web UI for browsing DynamoDB tables |

### Browsing DynamoDB data

Open http://localhost:8001 in your browser to inspect:

- Table items
- GSI contents (email-index, status-index)
- Invitation mapping records

### Manual table creation

If you need to create the table manually (instead of `make dynamodb-setup`):

```bash
aws dynamodb create-table \
  --table-name invitation-mappings \
  --attribute-definitions \
    AttributeName=pk,AttributeType=S \
    AttributeName=sk,AttributeType=S \
    AttributeName=gsi1pk,AttributeType=S \
    AttributeName=gsi1sk,AttributeType=S \
    AttributeName=gsi2pk,AttributeType=S \
    AttributeName=gsi2sk,AttributeType=S \
  --key-schema AttributeName=pk,KeyType=HASH AttributeName=sk,KeyType=RANGE \
  --global-secondary-indexes \
    '[{"IndexName":"email-index","KeySchema":[{"AttributeName":"gsi1pk","KeyType":"HASH"},{"AttributeName":"gsi1sk","KeyType":"RANGE"}],"Projection":{"ProjectionType":"ALL"}},
      {"IndexName":"status-index","KeySchema":[{"AttributeName":"gsi2pk","KeyType":"HASH"},{"AttributeName":"gsi2sk","KeyType":"RANGE"}],"Projection":{"ProjectionType":"ALL"}}]' \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url http://localhost:8000
```

---

## Makefile Targets

| Target | Command | Description |
|--------|---------|-------------|
| `build` | `go build -o bin/google-workspace-github-sync .` | Compile the binary |
| `test` | `go test ./... -v` | Run all tests |
| `lint` | `golangci-lint run ./...` | Run linter |
| `deploy` | `sam deploy --guided` | Deploy to AWS |
| `dynamodb-up` | `docker compose up -d` | Start DynamoDB Local |
| `dynamodb-down` | `docker compose down` | Stop DynamoDB Local |
| `dynamodb-setup` | `./scripts/setup-local-dynamodb.sh` | Create table + indexes |
| `dynamodb-reset` | Down + setup | Full DynamoDB reset |

---

## Linting

```bash
# Install golangci-lint (macOS)
brew install golangci-lint

# Run linter
make lint
```

---

## Environment Variables for Local Development

You can use environment variables instead of (or to override) `config.yaml`:

```bash
export GOOGLE_ADMIN_EMAIL=admin@yourdomain.com
export GOOGLE_CREDENTIALS_FILE=./credentials.json
export GOOGLE_MEMBERS_GROUP=github-members@yourdomain.com
export GOOGLE_OWNERS_GROUP=github-owners@yourdomain.com
export GITHUB_ORG=your-github-org
export GITHUB_TOKEN=ghp_xxxxxxxxxxxx
export DRY_RUN=true
export LOG_LEVEL=debug
export LOG_FORMAT=text
export DYNAMODB_ENABLED=true
export DYNAMODB_TABLE_NAME=invitation-mappings
export DYNAMODB_REGION=eu-west-1
export DYNAMODB_ENDPOINT=http://localhost:8000
```

---

## Troubleshooting

### "role update returned nil membership"

The `EditOrgMembership` API returned an empty response. This usually means:
- The user is not actually in the organization
- The GitHub token lacks `admin:org` scope

### "already a member of the organization" on invite

Expected behavior — the tool auto-upgrades to a role update by:
1. Searching for the user by email (`SearchUserByEmail`)
2. If found, calling `UpdateMemberRole` instead

If `SearchUserByEmail` returns empty, the user's email may not be publicly associated with their GitHub account. The action is marked `already_in_org` and skipped.

### DynamoDB connection refused

Make sure DynamoDB Local is running:

```bash
make dynamodb-up
```

And verify the endpoint in `config.yaml` matches:

```yaml
dynamodb:
  endpoint: http://localhost:8000
```

### Google API "insufficient permissions"

Verify:
1. Domain-wide delegation is configured for the service account
2. The required scopes are listed in the delegation configuration
3. The `admin_email` is an actual Workspace admin

### Debug logging

Set log level to `debug` for maximum output:

```bash
./bin/google-workspace-github-sync --config config.yaml --log-level debug
```

This shows:
- All Google group members fetched
- All GitHub org members and their roles
- DynamoDB email→username mappings
- Individual action execution details
- API request/response diagnostics
