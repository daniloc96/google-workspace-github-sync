package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/daniloc96/google-workspace-github-sync/internal/config"
	"github.com/daniloc96/google-workspace-github-sync/internal/models"
)

// Store implements the InvitationStore interface using DynamoDB.
type Store struct {
	client    *dynamodb.Client
	tableName string
	ttlDays   int
}

// NewStore creates a new DynamoDB-backed InvitationStore.
func NewStore(ctx context.Context, cfg config.DynamoDBConfig) (*Store, error) {
	var opts []func(*awsconfig.LoadOptions) error
	opts = append(opts, awsconfig.WithRegion(cfg.Region))

	if cfg.Endpoint != "" {
		// Local development: use static credentials and custom endpoint.
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("local", "local", ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	var clientOpts []func(*dynamodb.Options)
	if cfg.Endpoint != "" {
		clientOpts = append(clientOpts, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		})
	}

	client := dynamodb.NewFromConfig(awsCfg, clientOpts...)

	ttlDays := cfg.TTLDays
	if ttlDays <= 0 {
		ttlDays = 90
	}

	return &Store{
		client:    client,
		tableName: cfg.TableName,
		ttlDays:   ttlDays,
	}, nil
}

// SaveInvitation stores a new pending invitation mapping.
func (s *Store) SaveInvitation(ctx context.Context, mapping models.InvitationMapping) error {
	item, err := attributevalue.MarshalMap(mapping)
	if err != nil {
		return fmt.Errorf("marshaling invitation: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("saving invitation: %w", err)
	}

	return nil
}

// GetInvitation retrieves an invitation by org and invitation ID.
func (s *Store) GetInvitation(ctx context.Context, org string, invitationID int64) (*models.InvitationMapping, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "ORG#" + org},
			"sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("INV#%d", invitationID)},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("getting invitation: %w", err)
	}

	if result.Item == nil {
		return nil, nil
	}

	var mapping models.InvitationMapping
	if err := attributevalue.UnmarshalMap(result.Item, &mapping); err != nil {
		return nil, fmt.Errorf("unmarshaling invitation: %w", err)
	}

	return &mapping, nil
}

// GetPendingInvitations returns all pending invitations for an org using status-index GSI.
func (s *Store) GetPendingInvitations(ctx context.Context, org string) ([]models.InvitationMapping, error) {
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("status-index"),
		KeyConditionExpression: aws.String("gsi2pk = :pk AND gsi2sk = :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "ORG#" + org},
			":sk": &types.AttributeValueMemberS{Value: "STATUS#pending"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("querying pending invitations: %w", err)
	}

	var mappings []models.InvitationMapping
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &mappings); err != nil {
		return nil, fmt.Errorf("unmarshaling pending invitations: %w", err)
	}

	return mappings, nil
}

// ResolveInvitation updates an invitation with the resolved GitHub username.
func (s *Store) ResolveInvitation(ctx context.Context, org string, invitationID int64, githubLogin string) error {
	now := time.Now().UTC()
	ttl := now.AddDate(0, 0, s.ttlDays).Unix()

	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "ORG#" + org},
			"sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("INV#%d", invitationID)},
		},
		UpdateExpression: aws.String("SET github_login = :login, #st = :status, resolved_at = :resolved, gsi2sk = :gsi2sk, #ttl = :ttl"),
		ExpressionAttributeNames: map[string]string{
			"#st":  "status",
			"#ttl": "ttl",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":login":    &types.AttributeValueMemberS{Value: githubLogin},
			":status":   &types.AttributeValueMemberS{Value: string(models.InvitationResolved)},
			":resolved": &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
			":gsi2sk":   &types.AttributeValueMemberS{Value: "STATUS#" + string(models.InvitationResolved)},
			":ttl":      &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", ttl)},
		},
	})
	if err != nil {
		return fmt.Errorf("resolving invitation: %w", err)
	}

	return nil
}

// UpdateStatus changes the status of an invitation (failed, expired, cancelled, removed).
func (s *Store) UpdateStatus(ctx context.Context, org string, invitationID int64, status models.InvitationStatus) error {
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "ORG#" + org},
			"sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("INV#%d", invitationID)},
		},
		UpdateExpression: aws.String("SET #st = :status, gsi2sk = :gsi2sk"),
		ExpressionAttributeNames: map[string]string{
			"#st": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status": &types.AttributeValueMemberS{Value: string(status)},
			":gsi2sk": &types.AttributeValueMemberS{Value: "STATUS#" + string(status)},
		},
	})
	if err != nil {
		return fmt.Errorf("updating invitation status: %w", err)
	}

	return nil
}

// UpdateRole updates the role of an invitation mapping.
func (s *Store) UpdateRole(ctx context.Context, org string, invitationID int64, role models.OrgRole) error {
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "ORG#" + org},
			"sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("INV#%d", invitationID)},
		},
		UpdateExpression: aws.String("SET #r = :role"),
		ExpressionAttributeNames: map[string]string{
			"#r": "role",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":role": &types.AttributeValueMemberS{Value: string(role)},
		},
	})
	if err != nil {
		return fmt.Errorf("updating invitation role: %w", err)
	}

	return nil
}

// GetByEmail retrieves invitation mappings for a specific email using email-index GSI.
func (s *Store) GetByEmail(ctx context.Context, email string, org string) ([]models.InvitationMapping, error) {
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("email-index"),
		KeyConditionExpression: aws.String("gsi1pk = :pk AND gsi1sk = :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "EMAIL#" + email},
			":sk": &types.AttributeValueMemberS{Value: "ORG#" + org},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("querying by email: %w", err)
	}

	var mappings []models.InvitationMapping
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &mappings); err != nil {
		return nil, fmt.Errorf("unmarshaling email results: %w", err)
	}

	return mappings, nil
}

// GetAuditLogCursor retrieves the last processed audit log cursor.
func (s *Store) GetAuditLogCursor(ctx context.Context, org string) (*models.AuditLogCursor, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "ORG#" + org},
			"sk": &types.AttributeValueMemberS{Value: "CURSOR#audit_log"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("getting audit log cursor: %w", err)
	}

	if result.Item == nil {
		return nil, nil
	}

	var cursor models.AuditLogCursor
	if err := attributevalue.UnmarshalMap(result.Item, &cursor); err != nil {
		return nil, fmt.Errorf("unmarshaling audit log cursor: %w", err)
	}

	return &cursor, nil
}

// SaveAuditLogCursor stores the audit log cursor.
func (s *Store) SaveAuditLogCursor(ctx context.Context, cursor models.AuditLogCursor) error {
	item, err := attributevalue.MarshalMap(cursor)
	if err != nil {
		return fmt.Errorf("marshaling cursor: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("saving cursor: %w", err)
	}

	return nil
}

// GetAllResolvedMappings returns all resolved email→username mappings for an org.
func (s *Store) GetAllResolvedMappings(ctx context.Context, org string) (map[string]string, error) {
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("status-index"),
		KeyConditionExpression: aws.String("gsi2pk = :pk AND gsi2sk = :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "ORG#" + org},
			":sk": &types.AttributeValueMemberS{Value: "STATUS#resolved"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("querying resolved mappings: %w", err)
	}

	var mappings []models.InvitationMapping
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &mappings); err != nil {
		return nil, fmt.Errorf("unmarshaling resolved mappings: %w", err)
	}

	resolved := make(map[string]string, len(mappings))
	for _, m := range mappings {
		if m.GitHubLogin != nil {
			resolved[m.Email] = *m.GitHubLogin
		}
	}

	return resolved, nil
}
