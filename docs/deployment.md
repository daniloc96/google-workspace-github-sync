# Deployment

## AWS Lambda Deployment (SAM)

The tool is packaged as an AWS SAM application for serverless deployment.

### Prerequisites

- [AWS SAM CLI](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/install-sam-cli.html)
- AWS credentials configured
- Google Workspace service account credentials stored in AWS Secrets Manager
- GitHub Personal Access Token stored in AWS Secrets Manager

### SAM Template Overview

The `template.yaml` provisions:

| Resource | Type | Description |
|----------|------|-------------|
| `GoogleGitHubSyncFunction` | `AWS::Serverless::Function` | Lambda function (Go, ARM64) |
| `InvitationMappingsTable` | `AWS::DynamoDB::Table` | DynamoDB table for invitation tracking |
| EventBridge Rule | (via `Events`) | Scheduled trigger |

### Lambda Configuration

| Setting | Value |
|---------|-------|
| Runtime | `provided.al2023` (custom Go runtime) |
| Architecture | `arm64` |
| Memory | 512 MB |
| Timeout | 300 seconds (5 minutes) |
| Reserved Concurrency | 1 (prevents parallel runs) |
| Handler | `bootstrap` |

### Template Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `ScheduleExpression` | `rate(15 minutes)` | EventBridge schedule |
| `LogLevel` | `info` | Log level |
| `LogFormat` | `json` | Log format |
| `DynamoDBEnabled` | `true` | Enable invitation tracking |
| `DynamoDBTableName` | `invitation-mappings` | DynamoDB table name |

### IAM Permissions

The Lambda function's execution role includes:

```yaml
# Secrets Manager (for Google credentials + GitHub token)
- secretsmanager:GetSecretValue  (Resource: *)

# CloudWatch Metrics
- cloudwatch:PutMetricData  (Resource: *)

# DynamoDB (invitation tracking)
- dynamodb:PutItem
- dynamodb:GetItem
- dynamodb:UpdateItem
- dynamodb:Query
# On both the table and its indexes
```

Plus the managed policy `AWSLambdaBasicExecutionRole` for CloudWatch Logs.

### DynamoDB Table

| Setting | Value |
|---------|-------|
| Billing Mode | PAY_PER_REQUEST |
| Primary Key | `pk` (String, HASH) + `sk` (String, RANGE) |
| GSI: `email-index` | `gsi1pk` (HASH) + `gsi1sk` (RANGE), ALL projection |
| GSI: `status-index` | `gsi2pk` (HASH) + `gsi2sk` (RANGE), ALL projection |
| TTL | Enabled on `ttl` attribute |

---

## Step-by-Step Deployment

### 1. Store secrets in AWS Secrets Manager

```bash
# Google Workspace service account credentials (JSON key file)
aws secretsmanager create-secret \
  --name google-workspace-github-sync/google-credentials \
  --secret-string file://credentials.json

# GitHub Personal Access Token
aws secretsmanager create-secret \
  --name google-workspace-github-sync/github-token \
  --secret-string '{"token":"ghp_xxxxxxxxxxxx"}'
```

### 2. Configure environment variables

Set the following in your `template.yaml` or as Lambda environment variables:

```yaml
Environment:
  Variables:
    GOOGLE_ADMIN_EMAIL: admin@yourdomain.com
    GOOGLE_CREDENTIALS_SECRET: google-workspace-github-sync/google-credentials
    GOOGLE_MEMBERS_GROUP: github-members@yourdomain.com
    GOOGLE_OWNERS_GROUP: github-owners@yourdomain.com
    GITHUB_ORG: your-github-org
    GITHUB_TOKEN_SECRET: google-workspace-github-sync/github-token
    DRY_RUN: "true"            # Start with dry-run!
    IGNORE_SUSPENDED: "true"
    REMOVE_EXTRA_MEMBERS: "false"
    LOG_LEVEL: info
    LOG_FORMAT: json
    DYNAMODB_ENABLED: "true"
    DYNAMODB_TABLE_NAME: invitation-mappings
    DYNAMODB_REGION: eu-west-1
```

### 3. Build and deploy

```bash
# Build the Go binary for Lambda
sam build

# Deploy (first time — interactive guided setup)
sam deploy --guided

# Subsequent deployments
sam deploy
```

Or use the Makefile:

```bash
make deploy
```

### 4. Verify

- Check CloudWatch Logs for the Lambda function
- Verify dry-run output shows correct planned actions
- Once satisfied, set `DRY_RUN=false` to enable real changes

---

## GitHub Token Requirements

The GitHub Personal Access Token needs the following scopes:

| Scope | Required For |
|-------|-------------|
| `admin:org` | List/invite/remove members, update roles, cancel invitations |
| `read:org` | List organization members and pending invitations |
| `audit_log` | Read audit log events (for invitation reconciliation) |

> **Recommended**: Use a **fine-grained PAT** with organization-level permissions for better security.

---

## Google Workspace Setup

### 1. Create a Google Cloud project

Enable the **Admin SDK API** in the Google Cloud Console.

### 2. Create a service account

1. Go to **IAM & Admin → Service Accounts**
2. Create a new service account
3. Generate a JSON key → this is your `credentials.json`

### 3. Configure domain-wide delegation

1. Go to **Google Workspace Admin Console → Security → API Controls → Domain-wide Delegation**
2. Add a new client with the service account's **Client ID**
3. Add the following scopes:
   ```
   https://www.googleapis.com/auth/admin.directory.group.member.readonly
   https://www.googleapis.com/auth/admin.directory.user.readonly
   https://www.googleapis.com/auth/admin.directory.group.readonly
   ```

### 4. Set admin email

The `admin_email` config must be a Google Workspace admin account. The service account impersonates this user for API access.

---

## Scheduling

The default schedule is `rate(15 minutes)`. You can customize it via the `ScheduleExpression` parameter:

```bash
sam deploy --parameter-overrides ScheduleExpression="rate(1 hour)"
```

Supported formats:
- `rate(15 minutes)`, `rate(1 hour)`, `rate(1 day)`
- `cron(0 */6 * * ? *)` — every 6 hours

---

## Monitoring

### CloudWatch Metrics

The tool emits the following custom metrics under the `GoogleGitHubSync` namespace:

| Metric | Description |
|--------|-------------|
| `Invited` | Number of invitations sent |
| `Removed` | Number of members removed |
| `RoleUpdated` | Number of role changes |
| `AlreadyInOrg` | Number of "already a member" results |
| `CancelledInvites` | Number of cancelled invitations |
| `ActionsPlanned` | Total planned actions |
| `ActionsExecuted` | Successfully executed actions |
| `ActionsFailed` | Failed actions |

### CloudWatch Logs

All output is structured JSON (when `log_format: json`), making it easy to create CloudWatch Insights queries:

```
# Find all sync summaries
fields @timestamp, @message
| filter msg = "invitation reconciliation completed"

# Find failed actions
fields @timestamp, @message
| filter level = "warning"
```

---

## Outputs

The SAM template exports:

| Output | Description |
|--------|-------------|
| `GoogleGitHubSyncFunctionArn` | ARN of the Lambda function |
| `InvitationMappingsTableArn` | ARN of the DynamoDB table |
