package dynamodb

import (
	"testing"
	"time"

	"github.com/daniloc96/google-workspace-github-sync/internal/models"
)

func TestNewInvitationMapping(t *testing.T) {
	mapping := models.NewInvitationMapping("my-org", 42, "test@example.com", models.RoleMember, 90)

	if mapping.PK != "ORG#my-org" {
		t.Fatalf("expected PK ORG#my-org, got %s", mapping.PK)
	}
	if mapping.SK != "INV#42" {
		t.Fatalf("expected SK INV#42, got %s", mapping.SK)
	}
	if mapping.Email != "test@example.com" {
		t.Fatalf("expected email test@example.com, got %s", mapping.Email)
	}
	if mapping.Status != models.InvitationPending {
		t.Fatalf("expected status pending, got %s", mapping.Status)
	}
	if mapping.GSI1PK != "EMAIL#test@example.com" {
		t.Fatalf("expected GSI1PK EMAIL#test@example.com, got %s", mapping.GSI1PK)
	}
	if mapping.GSI2SK != "STATUS#pending" {
		t.Fatalf("expected GSI2SK STATUS#pending, got %s", mapping.GSI2SK)
	}
	if mapping.TTL == 0 {
		t.Fatalf("expected non-zero TTL")
	}
	expectedTTL := time.Now().UTC().AddDate(0, 0, 90).Unix()
	// Allow 60 seconds tolerance for test execution time
	if mapping.TTL < expectedTTL-60 || mapping.TTL > expectedTTL+60 {
		t.Fatalf("TTL %d is not within expected range around %d", mapping.TTL, expectedTTL)
	}
}

func TestMockStoreTracking(t *testing.T) {
	store := &MockStore{}

	mapping := models.NewInvitationMapping("test-org", 12345, "user@example.com", models.RoleMember, 90)

	ctx := t.Context()
	if err := store.SaveInvitation(ctx, mapping); err != nil {
		t.Fatalf("SaveInvitation failed: %v", err)
	}
	if len(store.SavedInvitations) != 1 {
		t.Fatalf("expected 1 saved invitation, got %d", len(store.SavedInvitations))
	}
	if store.SavedInvitations[0].Email != "user@example.com" {
		t.Fatalf("expected email user@example.com, got %s", store.SavedInvitations[0].Email)
	}
}
