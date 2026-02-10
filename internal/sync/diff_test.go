package sync

import (
	"testing"

	"github.com/daniloc96/google-workspace-github-sync/internal/models"
)

func TestCalculateDiffInvitesNewMembers(t *testing.T) {
	googleMembers := []models.GoogleGroupMember{
		{Email: "user1@example.com", Type: "USER", Status: "ACTIVE"},
		{Email: "user2@example.com", Type: "USER", Status: "ACTIVE"},
	}
	owners := []models.GoogleGroupMember{}
	githubMembers := []models.GitHubOrgMember{
		{Email: ptrString("user1@example.com"), Role: models.RoleMember},
	}
	pendingInvites := []models.GitHubOrgMember{}

	actions := CalculateDiff(googleMembers, owners, githubMembers, pendingInvites, false, nil, nil)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Type != models.ActionInvite {
		t.Fatalf("expected invite action, got %s", actions[0].Type)
	}
	if actions[0].Email != "user2@example.com" {
		t.Fatalf("expected invite for user2@example.com, got %s", actions[0].Email)
	}
}

func TestCalculateDiffInvitesOwnersAsAdmin(t *testing.T) {
	members := []models.GoogleGroupMember{}
	owners := []models.GoogleGroupMember{{Email: "owner@example.com", Type: "USER", Status: "ACTIVE"}}
	actions := CalculateDiff(members, owners, nil, nil, false, nil, nil)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].TargetRole == nil || *actions[0].TargetRole != models.RoleOwner {
		t.Fatalf("expected owner role invite, got %#v", actions[0].TargetRole)
	}
}

func TestCalculateDiffOwnerPrecedence(t *testing.T) {
	members := []models.GoogleGroupMember{{Email: "user@example.com", Type: "USER", Status: "ACTIVE"}}
	owners := []models.GoogleGroupMember{{Email: "user@example.com", Type: "USER", Status: "ACTIVE"}}
	actions := CalculateDiff(members, owners, nil, nil, false, nil, nil)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].TargetRole == nil || *actions[0].TargetRole != models.RoleOwner {
		t.Fatalf("expected owner role due to precedence, got %#v", actions[0].TargetRole)
	}
}

func ptrString(value string) *string {
	return &value
}

func TestCalculateDiffRemovesExtraMembers(t *testing.T) {
	googleMembers := []models.GoogleGroupMember{{Email: "user@example.com", Type: "USER", Status: "ACTIVE"}}
	owners := []models.GoogleGroupMember{}
	githubMembers := []models.GitHubOrgMember{
		{Username: ptrString("user1")},
		{Username: ptrString("user2"), Email: ptrString("user@example.com"), Role: models.RoleMember},
	}
	actions := CalculateDiff(googleMembers, owners, githubMembers, nil, true, nil, nil)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Type != models.ActionRemove {
		t.Fatalf("expected remove action, got %s", actions[0].Type)
	}
	if actions[0].Email != "user1" {
		t.Fatalf("expected removal for user1, got %s", actions[0].Email)
	}
}

func TestCalculateDiffKeepsOwnersNotInMembers(t *testing.T) {
	members := []models.GoogleGroupMember{}
	owners := []models.GoogleGroupMember{{Email: "owner@example.com", Type: "USER", Status: "ACTIVE"}}
	githubMembers := []models.GitHubOrgMember{{Email: ptrString("owner@example.com")}}
	actions := CalculateDiff(members, owners, githubMembers, nil, true, nil, nil)
	if len(actions) != 0 {
		t.Fatalf("expected no actions, got %d", len(actions))
	}
}

func TestCalculateDiffRoleUpgrade(t *testing.T) {
	members := []models.GoogleGroupMember{}
	owners := []models.GoogleGroupMember{{Email: "user@example.com", Type: "USER", Status: "ACTIVE"}}
	githubMembers := []models.GitHubOrgMember{{Username: ptrString("user1"), Email: ptrString("user@example.com"), Role: models.RoleMember}}
	actions := CalculateDiff(members, owners, githubMembers, nil, false, nil, nil)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Type != models.ActionUpdateRole {
		t.Fatalf("expected update_role action, got %s", actions[0].Type)
	}
	if actions[0].TargetRole == nil || *actions[0].TargetRole != models.RoleOwner {
		t.Fatalf("expected target owner role, got %#v", actions[0].TargetRole)
	}
}

func TestCalculateDiffRoleDowngrade(t *testing.T) {
	members := []models.GoogleGroupMember{{Email: "user@example.com", Type: "USER", Status: "ACTIVE"}}
	owners := []models.GoogleGroupMember{}
	githubMembers := []models.GitHubOrgMember{{Username: ptrString("user1"), Email: ptrString("user@example.com"), Role: models.RoleOwner}}
	actions := CalculateDiff(members, owners, githubMembers, nil, false, nil, nil)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Type != models.ActionUpdateRole {
		t.Fatalf("expected update_role action, got %s", actions[0].Type)
	}
	if actions[0].TargetRole == nil || *actions[0].TargetRole != models.RoleMember {
		t.Fatalf("expected target member role, got %#v", actions[0].TargetRole)
	}
}

// --- Tests for DynamoDB-enhanced diff ---

func TestCalculateDiffRemoveViaDynamoDBMapping(t *testing.T) {
	// User removed from Google groups. GitHub member has no public email,
	// but DynamoDB knows email → username.
	googleMembers := []models.GoogleGroupMember{} // empty = user removed
	owners := []models.GoogleGroupMember{}
	githubMembers := []models.GitHubOrgMember{
		{Username: ptrString("ghuser1"), Role: models.RoleMember}, // no email on GitHub
	}

	mappings := &EmailMappings{
		Resolved:           map[string]string{"removed@example.com": "ghuser1"},
		PendingInvitations: map[string]int64{},
	}

	actions := CalculateDiff(googleMembers, owners, githubMembers, nil, true, mappings, nil)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d: %+v", len(actions), actions)
	}
	if actions[0].Type != models.ActionRemove {
		t.Fatalf("expected remove action, got %s", actions[0].Type)
	}
	if actions[0].Email != "ghuser1" {
		t.Fatalf("expected removal for ghuser1, got %s", actions[0].Email)
	}
}

func TestCalculateDiffKeepViaDynamoDBMapping(t *testing.T) {
	// User IS in Google groups. GitHub member has no public email,
	// but DynamoDB maps email → username. Should NOT be removed.
	googleMembers := []models.GoogleGroupMember{{Email: "user@example.com", Type: "USER", Status: "ACTIVE"}}
	owners := []models.GoogleGroupMember{}
	githubMembers := []models.GitHubOrgMember{
		{Username: ptrString("ghuser1"), Role: models.RoleMember}, // no email on GitHub
	}

	mappings := &EmailMappings{
		Resolved:           map[string]string{"user@example.com": "ghuser1"},
		PendingInvitations: map[string]int64{},
	}

	actions := CalculateDiff(googleMembers, owners, githubMembers, nil, true, mappings, nil)
	if len(actions) != 0 {
		t.Fatalf("expected 0 actions (user is in Google via DynamoDB mapping), got %d: %+v", len(actions), actions)
	}
}

func TestCalculateDiffRoleChangeViaDynamoDBMapping(t *testing.T) {
	// User moved from members → owners group. GitHub member has no public email,
	// but DynamoDB maps email → username. Should update role.
	members := []models.GoogleGroupMember{}
	owners := []models.GoogleGroupMember{{Email: "user@example.com", Type: "USER", Status: "ACTIVE"}}
	githubMembers := []models.GitHubOrgMember{
		{Username: ptrString("ghuser1"), Role: models.RoleMember}, // no email, current role: member
	}

	mappings := &EmailMappings{
		Resolved:           map[string]string{"user@example.com": "ghuser1"},
		PendingInvitations: map[string]int64{},
	}

	actions := CalculateDiff(members, owners, githubMembers, nil, false, mappings, nil)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d: %+v", len(actions), actions)
	}
	if actions[0].Type != models.ActionUpdateRole {
		t.Fatalf("expected update_role, got %s", actions[0].Type)
	}
	if actions[0].TargetRole == nil || *actions[0].TargetRole != models.RoleOwner {
		t.Fatalf("expected target owner, got %#v", actions[0].TargetRole)
	}
}

func TestCalculateDiffCancelsPendingInviteOnRemoval(t *testing.T) {
	// User removed from Google groups while invitation is still pending.
	googleMembers := []models.GoogleGroupMember{} // empty = removed
	owners := []models.GoogleGroupMember{}
	githubMembers := []models.GitHubOrgMember{}
	invID := int64(999)
	pendingInvites := []models.GitHubOrgMember{
		{Email: ptrString("removed@example.com"), InvitationID: &invID, IsPending: true},
	}

	mappings := &EmailMappings{
		Resolved:           map[string]string{},
		PendingInvitations: map[string]int64{},
	}

	actions := CalculateDiff(googleMembers, owners, githubMembers, pendingInvites, true, mappings, nil)
	found := false
	for _, a := range actions {
		if a.Type == models.ActionCancelInvite {
			found = true
			if a.InvitationID == nil || *a.InvitationID != 999 {
				t.Fatalf("expected invitation ID 999, got %v", a.InvitationID)
			}
			if a.Email != "removed@example.com" {
				t.Fatalf("expected email removed@example.com, got %s", a.Email)
			}
		}
	}
	if !found {
		t.Fatalf("expected cancel_invite action, got: %+v", actions)
	}
}

func TestCalculateDiffDoesNotCancelActiveInvite(t *testing.T) {
	// User IS in Google groups and has pending invitation. Should NOT cancel.
	googleMembers := []models.GoogleGroupMember{{Email: "user@example.com", Type: "USER", Status: "ACTIVE"}}
	owners := []models.GoogleGroupMember{}
	githubMembers := []models.GitHubOrgMember{}
	invID := int64(888)
	pendingInvites := []models.GitHubOrgMember{
		{Email: ptrString("user@example.com"), InvitationID: &invID, IsPending: true},
	}

	mappings := &EmailMappings{
		Resolved:           map[string]string{},
		PendingInvitations: map[string]int64{},
	}

	actions := CalculateDiff(googleMembers, owners, githubMembers, pendingInvites, true, mappings, nil)
	for _, a := range actions {
		if a.Type == models.ActionCancelInvite {
			t.Fatalf("should not cancel invite for user still in Google groups")
		}
	}
}

// --- Tests for conservative mode (remove_extra_members=false) ---

func TestConservativeMode_RemovesTrackedMember(t *testing.T) {
	// User tracked in DynamoDB, removed from Google groups → should be removed.
	googleMembers := []models.GoogleGroupMember{} // empty = removed
	owners := []models.GoogleGroupMember{}
	githubMembers := []models.GitHubOrgMember{
		{Username: ptrString("ghuser1"), Role: models.RoleMember},
	}

	mappings := &EmailMappings{
		Resolved:           map[string]string{"tracked@example.com": "ghuser1"},
		PendingInvitations: map[string]int64{},
	}

	actions := CalculateDiff(googleMembers, owners, githubMembers, nil, false, mappings, nil)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d: %+v", len(actions), actions)
	}
	if actions[0].Type != models.ActionRemove {
		t.Fatalf("expected remove action, got %s", actions[0].Type)
	}
	if actions[0].Email != "ghuser1" {
		t.Fatalf("expected removal for ghuser1, got %s", actions[0].Email)
	}
	if actions[0].GoogleEmail != "tracked@example.com" {
		t.Fatalf("expected GoogleEmail tracked@example.com, got %s", actions[0].GoogleEmail)
	}
}

func TestConservativeMode_SkipsUntrackedMember(t *testing.T) {
	// User NOT tracked in DynamoDB, not in Google groups → should NOT be removed.
	// This is a pre-existing org member that was added before the tool was deployed.
	googleMembers := []models.GoogleGroupMember{}
	owners := []models.GoogleGroupMember{}
	githubMembers := []models.GitHubOrgMember{
		{Username: ptrString("preexisting-user"), Role: models.RoleMember},
	}

	mappings := &EmailMappings{
		Resolved:           map[string]string{}, // no DynamoDB records
		PendingInvitations: map[string]int64{},
	}

	actions := CalculateDiff(googleMembers, owners, githubMembers, nil, false, mappings, nil)
	for _, a := range actions {
		if a.Type == models.ActionRemove {
			t.Fatalf("should NOT remove pre-existing member not tracked in DynamoDB, got: %+v", a)
		}
	}
}

func TestConservativeMode_KeepsTrackedMemberInGroup(t *testing.T) {
	// User tracked in DynamoDB, still in Google groups → should NOT be removed.
	googleMembers := []models.GoogleGroupMember{{Email: "user@example.com", Type: "USER", Status: "ACTIVE"}}
	owners := []models.GoogleGroupMember{}
	githubMembers := []models.GitHubOrgMember{
		{Username: ptrString("ghuser1"), Role: models.RoleMember},
	}

	mappings := &EmailMappings{
		Resolved:           map[string]string{"user@example.com": "ghuser1"},
		PendingInvitations: map[string]int64{},
	}

	actions := CalculateDiff(googleMembers, owners, githubMembers, nil, false, mappings, nil)
	for _, a := range actions {
		if a.Type == models.ActionRemove {
			t.Fatalf("should NOT remove tracked member still in Google groups")
		}
	}
}

func TestConservativeMode_CancelsPendingInvite(t *testing.T) {
	// Even in conservative mode, pending invitations for users removed from
	// Google groups should be cancelled.
	googleMembers := []models.GoogleGroupMember{} // empty = removed
	owners := []models.GoogleGroupMember{}
	githubMembers := []models.GitHubOrgMember{}
	invID := int64(777)
	pendingInvites := []models.GitHubOrgMember{
		{Email: ptrString("removed@example.com"), InvitationID: &invID, IsPending: true},
	}

	mappings := &EmailMappings{
		Resolved:           map[string]string{},
		PendingInvitations: map[string]int64{},
	}

	actions := CalculateDiff(googleMembers, owners, githubMembers, pendingInvites, false, mappings, nil)
	found := false
	for _, a := range actions {
		if a.Type == models.ActionCancelInvite && a.InvitationID != nil && *a.InvitationID == 777 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected cancel_invite action in conservative mode, got: %+v", actions)
	}
}

func TestConservativeMode_RoleChangeWorks(t *testing.T) {
	// Role changes should work in conservative mode (they always did, but verify).
	members := []models.GoogleGroupMember{{Email: "user@example.com", Type: "USER", Status: "ACTIVE"}}
	owners := []models.GoogleGroupMember{}
	githubMembers := []models.GitHubOrgMember{
		{Username: ptrString("ghuser1"), Role: models.RoleOwner}, // admin on GitHub
	}

	mappings := &EmailMappings{
		Resolved:           map[string]string{"user@example.com": "ghuser1"},
		PendingInvitations: map[string]int64{},
	}

	actions := CalculateDiff(members, owners, githubMembers, nil, false, mappings, nil)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d: %+v", len(actions), actions)
	}
	if actions[0].Type != models.ActionUpdateRole {
		t.Fatalf("expected update_role, got %s", actions[0].Type)
	}
	if actions[0].TargetRole == nil || *actions[0].TargetRole != models.RoleMember {
		t.Fatalf("expected downgrade to member, got %#v", actions[0].TargetRole)
	}
}

func TestConservativeMode_NoDynamoDB_SkipsRemoval(t *testing.T) {
	// Without DynamoDB (emailMappings=nil), conservative mode should not remove anyone
	// and should not cancel any invitations.
	googleMembers := []models.GoogleGroupMember{}
	owners := []models.GoogleGroupMember{}
	githubMembers := []models.GitHubOrgMember{
		{Username: ptrString("someone"), Role: models.RoleMember},
	}
	invID := int64(555)
	pendingInvites := []models.GitHubOrgMember{
		{Email: ptrString("pending@example.com"), InvitationID: &invID, IsPending: true},
	}

	actions := CalculateDiff(googleMembers, owners, githubMembers, pendingInvites, false, nil, nil)
	for _, a := range actions {
		if a.Type == models.ActionRemove || a.Type == models.ActionCancelInvite {
			t.Fatalf("should NOT remove or cancel without DynamoDB in conservative mode, got: %+v", a)
		}
	}
}

// --- Tests for verified domain emails ---

func TestCalculateDiffVerifiedEmailPreventsInvite(t *testing.T) {
	// User is in Google group and already in GitHub org.
	// No public email on GitHub, no DynamoDB mapping, but verified domain email matches.
	googleMembers := []models.GoogleGroupMember{
		{Email: "user@company.com", Type: "USER", Status: "ACTIVE"},
	}
	owners := []models.GoogleGroupMember{}
	githubMembers := []models.GitHubOrgMember{
		{Username: ptrString("ghuser"), Role: models.RoleMember}, // no email
	}

	verifiedEmails := map[string]string{
		"user@company.com": "ghuser",
	}

	actions := CalculateDiff(googleMembers, owners, githubMembers, nil, false, nil, verifiedEmails)
	for _, a := range actions {
		if a.Type == models.ActionInvite {
			t.Fatalf("should NOT invite user already in org with verified email, got: %+v", a)
		}
	}
}

func TestCalculateDiffVerifiedEmailDetectsRoleMismatch(t *testing.T) {
	// User is in owners Google group but has member role on GitHub.
	// Matched via verified domain email — should generate a role update.
	members := []models.GoogleGroupMember{}
	owners := []models.GoogleGroupMember{
		{Email: "user@company.com", Type: "USER", Status: "ACTIVE"},
	}
	githubMembers := []models.GitHubOrgMember{
		{Username: ptrString("ghuser"), Role: models.RoleMember}, // should be admin
	}

	verifiedEmails := map[string]string{
		"user@company.com": "ghuser",
	}

	actions := CalculateDiff(members, owners, githubMembers, nil, false, nil, verifiedEmails)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action (role update), got %d: %+v", len(actions), actions)
	}
	if actions[0].Type != models.ActionUpdateRole {
		t.Fatalf("expected update_role, got %s", actions[0].Type)
	}
	if actions[0].TargetRole == nil || *actions[0].TargetRole != models.RoleOwner {
		t.Fatalf("expected target owner role, got %#v", actions[0].TargetRole)
	}
}

func TestCalculateDiffVerifiedEmailRemoveInAggressive(t *testing.T) {
	// User in GitHub org with verified email but NOT in any Google group.
	// Aggressive mode: should be removed. Verified email provides the reverse lookup.
	googleMembers := []models.GoogleGroupMember{}
	owners := []models.GoogleGroupMember{}
	githubMembers := []models.GitHubOrgMember{
		{Username: ptrString("ghuser"), Role: models.RoleMember},
	}

	verifiedEmails := map[string]string{
		"user@company.com": "ghuser",
	}

	actions := CalculateDiff(googleMembers, owners, githubMembers, nil, true, nil, verifiedEmails)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action (remove), got %d: %+v", len(actions), actions)
	}
	if actions[0].Type != models.ActionRemove {
		t.Fatalf("expected remove, got %s", actions[0].Type)
	}
}

func TestCalculateDiffVerifiedEmailDynamoDBTakesPrecedence(t *testing.T) {
	// Both DynamoDB and verified email have a mapping. DynamoDB should take precedence.
	googleMembers := []models.GoogleGroupMember{
		{Email: "user@company.com", Type: "USER", Status: "ACTIVE"},
	}
	owners := []models.GoogleGroupMember{}
	githubMembers := []models.GitHubOrgMember{
		{Username: ptrString("ghuser"), Role: models.RoleMember},
	}

	mappings := &EmailMappings{
		Resolved:           map[string]string{"user@company.com": "ghuser"},
		PendingInvitations: map[string]int64{},
	}
	verifiedEmails := map[string]string{
		"user@company.com": "ghuser",
	}

	actions := CalculateDiff(googleMembers, owners, githubMembers, nil, false, mappings, verifiedEmails)
	// No actions expected — user is known via DynamoDB mapping
	for _, a := range actions {
		if a.Type == models.ActionInvite {
			t.Fatalf("should NOT invite user known via DynamoDB + verified email, got: %+v", a)
		}
	}
}
