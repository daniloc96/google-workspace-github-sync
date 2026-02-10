package sync

import (
	"context"
	"testing"
	"time"

	"github.com/daniloc96/google-workspace-github-sync/internal/config"
	ddb "github.com/daniloc96/google-workspace-github-sync/internal/dynamodb"
	"github.com/daniloc96/google-workspace-github-sync/internal/github"
	"github.com/daniloc96/google-workspace-github-sync/internal/models"
)

func reconcilerCfg() *config.Config {
	return &config.Config{
		GitHub:   config.GitHubConfig{Organization: "test-org"},
		DynamoDB: config.DynamoDBConfig{Enabled: true, TableName: "test-table", Region: "eu-west-1", TTLDays: 90},
	}
}

func TestReconcileSavesNewInvitations(t *testing.T) {
	store := &ddb.MockStore{}
	ghClient := &github.MockClient{
		ListPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
		GetAuditLogAddMemberEventsFunc: func(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error) {
			return nil, nil
		},
		ListFailedInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
	}
	store.GetPendingInvitationsFunc = func(ctx context.Context, org string) ([]models.InvitationMapping, error) {
		return nil, nil
	}

	r := NewReconciler(store, ghClient, reconcilerCfg())

	invID := int64(12345)
	role := models.RoleMember
	actions := []models.SyncAction{
		{Type: models.ActionInvite, Email: "user@example.com", TargetRole: &role, Executed: true, InvitationID: &invID},
	}

	result, err := r.Reconcile(context.Background(), actions)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.NewInvitationsSaved != 1 {
		t.Fatalf("expected 1 invitation saved, got %d", result.NewInvitationsSaved)
	}
	if len(store.SavedInvitations) != 1 {
		t.Fatalf("expected 1 saved invitation in store, got %d", len(store.SavedInvitations))
	}
	if store.SavedInvitations[0].Email != "user@example.com" {
		t.Fatalf("expected email user@example.com, got %s", store.SavedInvitations[0].Email)
	}
}

func TestReconcileSkipsNonInviteActions(t *testing.T) {
	store := &ddb.MockStore{}
	ghClient := &github.MockClient{
		ListPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
		GetAuditLogAddMemberEventsFunc: func(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error) {
			return nil, nil
		},
		ListFailedInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
	}
	store.GetPendingInvitationsFunc = func(ctx context.Context, org string) ([]models.InvitationMapping, error) {
		return nil, nil
	}

	r := NewReconciler(store, ghClient, reconcilerCfg())

	actions := []models.SyncAction{
		{Type: models.ActionRemove, Email: "user@example.com", Executed: true},
	}

	result, err := r.Reconcile(context.Background(), actions)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.NewInvitationsSaved != 0 {
		t.Fatalf("expected 0 invitations saved, got %d", result.NewInvitationsSaved)
	}
}

func TestReconcileResolvesLoginFromPending(t *testing.T) {
	invID := int64(100)
	username := "ghuser"
	store := &ddb.MockStore{
		GetInvitationFunc: func(ctx context.Context, org string, invitationID int64) (*models.InvitationMapping, error) {
			if invitationID == 100 {
				return &models.InvitationMapping{
					PK:     "ORG#test-org",
					SK:     "INV#100",
					Email:  "user@example.com",
					Status: models.InvitationPending,
				}, nil
			}
			return nil, nil
		},
		GetPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.InvitationMapping, error) {
			return nil, nil
		},
	}
	ghClient := &github.MockClient{
		ListPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return []models.GitHubOrgMember{
				{InvitationID: &invID, Username: &username, Email: ptrString("user@example.com")},
			}, nil
		},
		GetAuditLogAddMemberEventsFunc: func(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error) {
			return nil, nil
		},
		ListFailedInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
	}

	r := NewReconciler(store, ghClient, reconcilerCfg())
	result, err := r.Reconcile(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Resolved != 1 {
		t.Fatalf("expected 1 resolved, got %d", result.Resolved)
	}
	if len(store.ResolvedCalls) != 1 {
		t.Fatalf("expected 1 resolve call, got %d", len(store.ResolvedCalls))
	}
	if store.ResolvedCalls[0].GitHubLogin != "ghuser" {
		t.Fatalf("expected ghuser, got %s", store.ResolvedCalls[0].GitHubLogin)
	}
}

func TestReconcileResolvesFromAuditLog(t *testing.T) {
	store := &ddb.MockStore{
		GetInvitationFunc: func(ctx context.Context, org string, invitationID int64) (*models.InvitationMapping, error) {
			if invitationID == 200 {
				return &models.InvitationMapping{
					PK:     "ORG#test-org",
					SK:     "INV#200",
					Email:  "user2@example.com",
					Status: models.InvitationPending,
				}, nil
			}
			return nil, nil
		},
		GetPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.InvitationMapping, error) {
			return nil, nil
		},
	}
	ghClient := &github.MockClient{
		ListPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
		GetAuditLogAddMemberEventsFunc: func(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error) {
			return []models.AuditLogEntry{
				{Timestamp: 1707500000000, Action: "org.add_member", User: "resolved-user", Org: "test-org", InvitationID: 200},
			}, nil
		},
		ListFailedInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
	}

	r := NewReconciler(store, ghClient, reconcilerCfg())
	result, err := r.Reconcile(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Resolved != 1 {
		t.Fatalf("expected 1 resolved from audit log, got %d", result.Resolved)
	}
	if len(store.ResolvedCalls) != 1 || store.ResolvedCalls[0].GitHubLogin != "resolved-user" {
		t.Fatalf("expected resolved-user, got %v", store.ResolvedCalls)
	}
	if len(store.SavedCursors) != 1 {
		t.Fatalf("expected 1 saved cursor, got %d", len(store.SavedCursors))
	}
	if store.SavedCursors[0].LastTimestamp != 1707500000000 {
		t.Fatalf("expected cursor timestamp 1707500000000, got %d", store.SavedCursors[0].LastTimestamp)
	}
}

func TestReconcileMarksFailedInvitations(t *testing.T) {
	failedInvID := int64(300)
	store := &ddb.MockStore{
		GetInvitationFunc: func(ctx context.Context, org string, invitationID int64) (*models.InvitationMapping, error) {
			if invitationID == 300 {
				return &models.InvitationMapping{
					PK:     "ORG#test-org",
					SK:     "INV#300",
					Email:  "failed@example.com",
					Status: models.InvitationPending,
				}, nil
			}
			return nil, nil
		},
		GetPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.InvitationMapping, error) {
			return nil, nil
		},
	}
	ghClient := &github.MockClient{
		ListPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
		GetAuditLogAddMemberEventsFunc: func(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error) {
			return nil, nil
		},
		ListFailedInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return []models.GitHubOrgMember{
				{InvitationID: &failedInvID, Email: ptrString("failed@example.com")},
			}, nil
		},
	}

	r := NewReconciler(store, ghClient, reconcilerCfg())
	result, err := r.Reconcile(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Failed != 1 {
		t.Fatalf("expected 1 failed, got %d", result.Failed)
	}
	if len(store.StatusCalls) != 1 || store.StatusCalls[0].Status != models.InvitationFailed {
		t.Fatalf("expected failed status update, got %v", store.StatusCalls)
	}
}

func TestReconcileMarksExpiredInvitations(t *testing.T) {
	store := &ddb.MockStore{
		GetPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.InvitationMapping, error) {
			return []models.InvitationMapping{
				{
					PK:        "ORG#test-org",
					SK:        "INV#400",
					Email:     "expired@example.com",
					Status:    models.InvitationPending,
					InvitedAt: time.Now().AddDate(0, 0, -10), // 10 days ago
				},
			}, nil
		},
		GetInvitationFunc: func(ctx context.Context, org string, invitationID int64) (*models.InvitationMapping, error) {
			return nil, nil
		},
	}
	ghClient := &github.MockClient{
		ListPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil // invitation not in pending anymore
		},
		GetAuditLogAddMemberEventsFunc: func(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error) {
			return nil, nil
		},
		ListFailedInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
	}

	r := NewReconciler(store, ghClient, reconcilerCfg())
	result, err := r.Reconcile(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Expired != 1 {
		t.Fatalf("expected 1 expired, got %d", result.Expired)
	}
	if len(store.StatusCalls) != 1 || store.StatusCalls[0].Status != models.InvitationExpired {
		t.Fatalf("expected expired status update, got %v", store.StatusCalls)
	}
}

func TestReconcileIdempotency(t *testing.T) {
	store := &ddb.MockStore{
		GetPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.InvitationMapping, error) {
			return nil, nil
		},
		GetInvitationFunc: func(ctx context.Context, org string, invitationID int64) (*models.InvitationMapping, error) {
			// Return already resolved mapping
			login := "resolved"
			return &models.InvitationMapping{
				PK:          "ORG#test-org",
				SK:          "INV#500",
				Email:       "already@example.com",
				GitHubLogin: &login,
				Status:      models.InvitationResolved,
			}, nil
		},
	}
	ghClient := &github.MockClient{
		ListPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			invID := int64(500)
			username := "resolved"
			return []models.GitHubOrgMember{
				{InvitationID: &invID, Username: &username},
			}, nil
		},
		GetAuditLogAddMemberEventsFunc: func(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error) {
			return nil, nil
		},
		ListFailedInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
	}

	r := NewReconciler(store, ghClient, reconcilerCfg())
	result, err := r.Reconcile(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Resolved != 0 {
		t.Fatalf("expected 0 resolved (already resolved), got %d", result.Resolved)
	}
	if len(store.ResolvedCalls) != 0 {
		t.Fatalf("expected no resolve calls for already-resolved invitation, got %d", len(store.ResolvedCalls))
	}
}

func TestReconcileMarksCancelledInvitations(t *testing.T) {
	store := &ddb.MockStore{
		GetPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.InvitationMapping, error) {
			return nil, nil
		},
		GetInvitationFunc: func(ctx context.Context, org string, invitationID int64) (*models.InvitationMapping, error) {
			return nil, nil
		},
	}
	ghClient := &github.MockClient{
		ListPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
		GetAuditLogAddMemberEventsFunc: func(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error) {
			return nil, nil
		},
		ListFailedInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
	}

	r := NewReconciler(store, ghClient, reconcilerCfg())

	invID := int64(600)
	actions := []models.SyncAction{
		{Type: models.ActionCancelInvite, Email: "cancelled@example.com", Executed: true, InvitationID: &invID},
	}

	result, err := r.Reconcile(context.Background(), actions)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Cancelled != 1 {
		t.Fatalf("expected 1 cancelled, got %d", result.Cancelled)
	}
	if len(store.StatusCalls) != 1 || store.StatusCalls[0].Status != models.InvitationCancelled {
		t.Fatalf("expected cancelled status update, got %v", store.StatusCalls)
	}
	if store.StatusCalls[0].InvitationID != 600 {
		t.Fatalf("expected invitation ID 600, got %d", store.StatusCalls[0].InvitationID)
	}
}

func TestReconcileMarksRemovedMembers(t *testing.T) {
	store := &ddb.MockStore{
		GetPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.InvitationMapping, error) {
			return nil, nil
		},
		GetInvitationFunc: func(ctx context.Context, org string, invitationID int64) (*models.InvitationMapping, error) {
			return nil, nil
		},
		GetByEmailFunc: func(ctx context.Context, email string, org string) ([]models.InvitationMapping, error) {
			if email == "removed@example.com" {
				return []models.InvitationMapping{
					{
						PK:     "ORG#test-org",
						SK:     "INV#700",
						Email:  "removed@example.com",
						Status: models.InvitationResolved,
					},
				}, nil
			}
			return nil, nil
		},
	}
	ghClient := &github.MockClient{
		ListPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
		GetAuditLogAddMemberEventsFunc: func(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error) {
			return nil, nil
		},
		ListFailedInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
	}

	r := NewReconciler(store, ghClient, reconcilerCfg())

	actions := []models.SyncAction{
		{
			Type:        models.ActionRemove,
			Email:       "ghuser-removed",
			GoogleEmail: "removed@example.com",
			Executed:    true,
		},
	}

	result, err := r.Reconcile(context.Background(), actions)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.MembersRemoved != 1 {
		t.Fatalf("expected 1 member removed in DynamoDB, got %d", result.MembersRemoved)
	}
	if len(store.StatusCalls) != 1 {
		t.Fatalf("expected 1 status update call, got %d", len(store.StatusCalls))
	}
	if store.StatusCalls[0].Status != models.InvitationRemoved {
		t.Fatalf("expected status %s, got %s", models.InvitationRemoved, store.StatusCalls[0].Status)
	}
	if store.StatusCalls[0].InvitationID != 700 {
		t.Fatalf("expected invitation ID 700, got %d", store.StatusCalls[0].InvitationID)
	}
}

func TestReconcileUpdatesRoleInDynamoDB(t *testing.T) {
	store := &ddb.MockStore{
		GetPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.InvitationMapping, error) {
			return nil, nil
		},
		GetInvitationFunc: func(ctx context.Context, org string, invitationID int64) (*models.InvitationMapping, error) {
			return nil, nil
		},
		GetByEmailFunc: func(ctx context.Context, email string, org string) ([]models.InvitationMapping, error) {
			if email == "rolechange@example.com" {
				return []models.InvitationMapping{
					{
						PK:     "ORG#test-org",
						SK:     "INV#800",
						Email:  "rolechange@example.com",
						Status: models.InvitationResolved,
						Role:   models.RoleMember,
					},
				}, nil
			}
			return nil, nil
		},
	}
	ghClient := &github.MockClient{
		ListPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
		GetAuditLogAddMemberEventsFunc: func(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error) {
			return nil, nil
		},
		ListFailedInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
	}

	r := NewReconciler(store, ghClient, reconcilerCfg())

	current := models.RoleMember
	target := models.RoleOwner
	actions := []models.SyncAction{
		{
			Type:        models.ActionUpdateRole,
			Email:       "ghuser-rolechange",
			GoogleEmail: "rolechange@example.com",
			CurrentRole: &current,
			TargetRole:  &target,
			Executed:    true,
		},
	}

	result, err := r.Reconcile(context.Background(), actions)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.RolesUpdated != 1 {
		t.Fatalf("expected 1 role updated in DynamoDB, got %d", result.RolesUpdated)
	}
	if len(store.RoleCalls) != 1 {
		t.Fatalf("expected 1 role update call, got %d", len(store.RoleCalls))
	}
	if store.RoleCalls[0].InvitationID != 800 {
		t.Fatalf("expected invitation ID 800, got %d", store.RoleCalls[0].InvitationID)
	}
	if store.RoleCalls[0].Role != models.RoleOwner {
		t.Fatalf("expected role %s, got %s", models.RoleOwner, store.RoleCalls[0].Role)
	}
}

func TestReconcileSkipsRemoveWithoutGoogleEmail(t *testing.T) {
	store := &ddb.MockStore{
		GetPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.InvitationMapping, error) {
			return nil, nil
		},
		GetInvitationFunc: func(ctx context.Context, org string, invitationID int64) (*models.InvitationMapping, error) {
			return nil, nil
		},
	}
	ghClient := &github.MockClient{
		ListPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
		GetAuditLogAddMemberEventsFunc: func(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error) {
			return nil, nil
		},
		ListFailedInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
	}

	r := NewReconciler(store, ghClient, reconcilerCfg())

	// Remove action WITHOUT GoogleEmail — should not touch DynamoDB.
	actions := []models.SyncAction{
		{
			Type:     models.ActionRemove,
			Email:    "ghuser-direct",
			Executed: true,
			// GoogleEmail is empty — user was matched by email, no DynamoDB record
		},
	}

	result, err := r.Reconcile(context.Background(), actions)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.MembersRemoved != 0 {
		t.Fatalf("expected 0 members removed in DynamoDB, got %d", result.MembersRemoved)
	}
	if len(store.StatusCalls) != 0 {
		t.Fatalf("expected 0 status calls, got %d", len(store.StatusCalls))
	}
}

func TestReconcileCreatesAlreadyInOrgMapping(t *testing.T) {
	store := &ddb.MockStore{
		GetPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.InvitationMapping, error) {
			return nil, nil
		},
		GetByEmailFunc: func(ctx context.Context, email string, org string) ([]models.InvitationMapping, error) {
			return nil, nil // no existing mapping
		},
	}
	ghClient := &github.MockClient{
		ListPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
		GetAuditLogAddMemberEventsFunc: func(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error) {
			return nil, nil
		},
		ListFailedInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
	}

	r := NewReconciler(store, ghClient, reconcilerCfg())

	role := models.RoleMember
	ts := time.Now()
	actions := []models.SyncAction{
		{
			Type:         models.ActionUpdateRole,
			Email:        "found-user",
			GoogleEmail:  "user@company.com",
			Username:     "found-user",
			TargetRole:   &role,
			Executed:     true,
			AlreadyInOrg: true,
			Timestamp:    &ts,
		},
	}

	result, err := r.Reconcile(context.Background(), actions)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.AlreadyInOrgResolved != 1 {
		t.Fatalf("expected 1 already-in-org resolved, got %d", result.AlreadyInOrgResolved)
	}
	if len(store.SavedInvitations) != 1 {
		t.Fatalf("expected 1 saved invitation, got %d", len(store.SavedInvitations))
	}
	saved := store.SavedInvitations[0]
	if saved.SK != "EXISTING#found-user" {
		t.Fatalf("expected SK 'EXISTING#found-user', got '%s'", saved.SK)
	}
	if saved.Email != "user@company.com" {
		t.Fatalf("expected email 'user@company.com', got '%s'", saved.Email)
	}
	if saved.GitHubLogin == nil || *saved.GitHubLogin != "found-user" {
		t.Fatalf("expected github_login 'found-user', got %v", saved.GitHubLogin)
	}
	if saved.Status != models.InvitationResolved {
		t.Fatalf("expected status 'resolved', got '%s'", saved.Status)
	}
}

func TestReconcileSkipsAlreadyInOrgWithExistingMapping(t *testing.T) {
	login := "found-user"
	store := &ddb.MockStore{
		GetPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.InvitationMapping, error) {
			return nil, nil
		},
		GetByEmailFunc: func(ctx context.Context, email string, org string) ([]models.InvitationMapping, error) {
			return []models.InvitationMapping{
				{
					PK:          "ORG#test-org",
					SK:          "EXISTING#found-user",
					Email:       email,
					GitHubLogin: &login,
					Status:      models.InvitationResolved,
				},
			}, nil
		},
	}
	ghClient := &github.MockClient{
		ListPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
		GetAuditLogAddMemberEventsFunc: func(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error) {
			return nil, nil
		},
		ListFailedInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
	}

	r := NewReconciler(store, ghClient, reconcilerCfg())

	role := models.RoleMember
	ts := time.Now()
	actions := []models.SyncAction{
		{
			Type:         models.ActionUpdateRole,
			Email:        "found-user",
			GoogleEmail:  "user@company.com",
			Username:     "found-user",
			TargetRole:   &role,
			Executed:     true,
			AlreadyInOrg: true,
			Timestamp:    &ts,
		},
	}

	result, err := r.Reconcile(context.Background(), actions)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.AlreadyInOrgResolved != 0 {
		t.Fatalf("expected 0 already-in-org resolved (duplicate), got %d", result.AlreadyInOrgResolved)
	}
	if len(store.SavedInvitations) != 0 {
		t.Fatalf("expected 0 saved invitations (already exists), got %d", len(store.SavedInvitations))
	}
}

func TestReconcileSkipsAlreadyInOrgWithoutUsername(t *testing.T) {
	store := &ddb.MockStore{
		GetPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.InvitationMapping, error) {
			return nil, nil
		},
	}
	ghClient := &github.MockClient{
		ListPendingInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
		GetAuditLogAddMemberEventsFunc: func(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error) {
			return nil, nil
		},
		ListFailedInvitationsFunc: func(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
			return nil, nil
		},
	}

	r := NewReconciler(store, ghClient, reconcilerCfg())

	// Already-in-org but no username resolved (search failed)
	errMsg := "user is already a member"
	actions := []models.SyncAction{
		{
			Type:         models.ActionInvite,
			Email:        "user@company.com",
			AlreadyInOrg: true,
			Executed:     false,
			Error:        &errMsg,
			// Username is empty — search failed
		},
	}

	result, err := r.Reconcile(context.Background(), actions)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.AlreadyInOrgResolved != 0 {
		t.Fatalf("expected 0 already-in-org resolved (no username), got %d", result.AlreadyInOrgResolved)
	}
	if len(store.SavedInvitations) != 0 {
		t.Fatalf("expected 0 saved invitations (no username), got %d", len(store.SavedInvitations))
	}
}

// ---------------------------------------------------------------------------
// EnsureVerifiedEmailMappings
// ---------------------------------------------------------------------------

func TestEnsureVerifiedEmailMappings_CreatesRecordWithMemberRole(t *testing.T) {
	store := &ddb.MockStore{
		GetByEmailFunc: func(ctx context.Context, email, org string) ([]models.InvitationMapping, error) {
			return nil, nil // no existing mapping
		},
	}
	ghClient := &github.MockClient{}

	r := NewReconciler(store, ghClient, reconcilerCfg())

	verifiedEmails := map[string]string{
		"user@company.com": "ghuser",
	}
	membersGroup := []models.GoogleGroupMember{
		{Email: "user@company.com", Type: "USER", Status: "ACTIVE"},
	}
	result := &models.ReconcileResult{}

	r.EnsureVerifiedEmailMappings(context.Background(), verifiedEmails, membersGroup, nil, result)

	if result.VerifiedEmailsMapped != 1 {
		t.Fatalf("expected 1 verified email mapped, got %d", result.VerifiedEmailsMapped)
	}
	if len(store.SavedInvitations) != 1 {
		t.Fatalf("expected 1 saved invitation, got %d", len(store.SavedInvitations))
	}
	saved := store.SavedInvitations[0]
	if saved.SK != "EXISTING#ghuser" {
		t.Fatalf("expected SK EXISTING#ghuser, got %s", saved.SK)
	}
	if saved.Email != "user@company.com" {
		t.Fatalf("expected email user@company.com, got %s", saved.Email)
	}
	if saved.Status != models.InvitationResolved {
		t.Fatalf("expected status resolved, got %s", saved.Status)
	}
	if saved.Role != models.RoleMember {
		t.Fatalf("expected role member, got %s", saved.Role)
	}
}

func TestEnsureVerifiedEmailMappings_CreatesRecordWithOwnerRole(t *testing.T) {
	store := &ddb.MockStore{
		GetByEmailFunc: func(ctx context.Context, email, org string) ([]models.InvitationMapping, error) {
			return nil, nil
		},
	}
	ghClient := &github.MockClient{}

	r := NewReconciler(store, ghClient, reconcilerCfg())

	verifiedEmails := map[string]string{
		"boss@company.com": "bossgithub",
	}
	// User is in owners group, not in members group.
	ownersGroup := []models.GoogleGroupMember{
		{Email: "boss@company.com", Type: "USER", Status: "ACTIVE"},
	}
	result := &models.ReconcileResult{}

	r.EnsureVerifiedEmailMappings(context.Background(), verifiedEmails, nil, ownersGroup, result)

	if result.VerifiedEmailsMapped != 1 {
		t.Fatalf("expected 1 verified email mapped, got %d", result.VerifiedEmailsMapped)
	}
	saved := store.SavedInvitations[0]
	if saved.Role != models.RoleOwner {
		t.Fatalf("expected role admin (owner), got %s", saved.Role)
	}
}

func TestEnsureVerifiedEmailMappings_OwnerOverridesMember(t *testing.T) {
	store := &ddb.MockStore{
		GetByEmailFunc: func(ctx context.Context, email, org string) ([]models.InvitationMapping, error) {
			return nil, nil
		},
	}
	ghClient := &github.MockClient{}

	r := NewReconciler(store, ghClient, reconcilerCfg())

	verifiedEmails := map[string]string{
		"lead@company.com": "leaduser",
	}
	// User appears in both groups — owners should take precedence.
	membersGroup := []models.GoogleGroupMember{
		{Email: "lead@company.com", Type: "USER", Status: "ACTIVE"},
	}
	ownersGroup := []models.GoogleGroupMember{
		{Email: "lead@company.com", Type: "USER", Status: "ACTIVE"},
	}
	result := &models.ReconcileResult{}

	r.EnsureVerifiedEmailMappings(context.Background(), verifiedEmails, membersGroup, ownersGroup, result)

	if result.VerifiedEmailsMapped != 1 {
		t.Fatalf("expected 1 verified email mapped, got %d", result.VerifiedEmailsMapped)
	}
	saved := store.SavedInvitations[0]
	if saved.Role != models.RoleOwner {
		t.Fatalf("expected role admin (owner takes precedence), got %s", saved.Role)
	}
}

func TestEnsureVerifiedEmailMappings_SkipsAlreadyMapped(t *testing.T) {
	login := "ghuser"
	store := &ddb.MockStore{
		GetByEmailFunc: func(ctx context.Context, email, org string) ([]models.InvitationMapping, error) {
			return []models.InvitationMapping{
				{Status: models.InvitationResolved, GitHubLogin: &login},
			}, nil
		},
	}
	ghClient := &github.MockClient{}

	r := NewReconciler(store, ghClient, reconcilerCfg())

	verifiedEmails := map[string]string{
		"user@company.com": "ghuser",
	}
	membersGroup := []models.GoogleGroupMember{
		{Email: "user@company.com", Type: "USER", Status: "ACTIVE"},
	}
	result := &models.ReconcileResult{}

	r.EnsureVerifiedEmailMappings(context.Background(), verifiedEmails, membersGroup, nil, result)

	if result.VerifiedEmailsMapped != 0 {
		t.Fatalf("expected 0 verified emails mapped (already exists), got %d", result.VerifiedEmailsMapped)
	}
	if len(store.SavedInvitations) != 0 {
		t.Fatalf("expected 0 saved invitations (already exists), got %d", len(store.SavedInvitations))
	}
}

func TestEnsureVerifiedEmailMappings_SkipsNonGoogleMember(t *testing.T) {
	store := &ddb.MockStore{}
	ghClient := &github.MockClient{}

	r := NewReconciler(store, ghClient, reconcilerCfg())

	// Verified email exists but user is NOT in Google groups.
	verifiedEmails := map[string]string{
		"outsider@company.com": "outsider",
	}
	membersGroup := []models.GoogleGroupMember{
		{Email: "alice@company.com", Type: "USER", Status: "ACTIVE"},
	}
	result := &models.ReconcileResult{}

	r.EnsureVerifiedEmailMappings(context.Background(), verifiedEmails, membersGroup, nil, result)

	if result.VerifiedEmailsMapped != 0 {
		t.Fatalf("expected 0 (not a Google member), got %d", result.VerifiedEmailsMapped)
	}
	if len(store.SavedInvitations) != 0 {
		t.Fatalf("expected 0 saved invitations, got %d", len(store.SavedInvitations))
	}
}

func TestEnsureVerifiedEmailMappings_NilMap(t *testing.T) {
	store := &ddb.MockStore{}
	ghClient := &github.MockClient{}

	r := NewReconciler(store, ghClient, reconcilerCfg())
	result := &models.ReconcileResult{}

	// Should be a no-op when nil is passed.
	r.EnsureVerifiedEmailMappings(context.Background(), nil, nil, nil, result)

	if result.VerifiedEmailsMapped != 0 {
		t.Fatalf("expected 0, got %d", result.VerifiedEmailsMapped)
	}
}
