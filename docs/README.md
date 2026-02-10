# google-workspace-github-sync — Documentation

Comprehensive documentation for the Google Workspace → GitHub Organization membership sync tool.

## Table of Contents

| Document | Description |
|----------|-------------|
| [Architecture](architecture.md) | System design, component overview, data flow diagrams |
| [Configuration](configuration.md) | All config options, environment variables, CLI flags |
| [Sync Logic](sync-logic.md) | Diff algorithm, action types, remove modes, role changes |
| [Invitation Reconciliation](invitation-reconciliation.md) | DynamoDB tracking, email→username mapping, status lifecycle |
| [Deployment](deployment.md) | AWS SAM deployment, Lambda setup, EventBridge scheduling |
| [Local Development](local-development.md) | Building, testing, DynamoDB Local, troubleshooting |
| [API Reference](api-reference.md) | Internal packages, interfaces, key types |
