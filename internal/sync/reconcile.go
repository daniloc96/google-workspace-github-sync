package sync

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/daniloc96/google-workspace-github-sync/internal/config"
	"github.com/daniloc96/google-workspace-github-sync/internal/interfaces"
	"github.com/daniloc96/google-workspace-github-sync/internal/models"
	"github.com/sirupsen/logrus"
)

const invitationExpiryDays = 7

// Reconciler handles the invitation reconciliation flow:
// correlating pending invitations with audit log events to map email → GitHub username.
type Reconciler struct {
	store        interfaces.InvitationStore
	githubClient interfaces.GitHubClient
	cfg          *config.Config
}

// NewReconciler creates a new Reconciler.
func NewReconciler(store interfaces.InvitationStore, githubClient interfaces.GitHubClient, cfg *config.Config) *Reconciler {
	return &Reconciler{
		store:        store,
		githubClient: githubClient,
		cfg:          cfg,
	}
}

// Reconcile performs the full invitation reconciliation flow.
// It is designed to be called after ExecuteActions in the sync engine.
func (r *Reconciler) Reconcile(ctx context.Context, executedActions []models.SyncAction) (*models.ReconcileResult, error) {
	org := r.cfg.GitHub.Organization
	result := &models.ReconcileResult{}

	// Step 1: Save newly executed invitations to DynamoDB.
	saved, errs := r.saveNewInvitations(ctx, org, executedActions)
	result.NewInvitationsSaved = saved
	result.Errors = append(result.Errors, errs...)

	// Step 1b: Mark cancelled invitations in DynamoDB.
	cancelled, cancelErrs := r.markCancelledInvitations(ctx, org, executedActions)
	result.Cancelled = cancelled
	result.Errors = append(result.Errors, cancelErrs...)

	// Step 1c: Mark removed members in DynamoDB.
	removed, removeErrs := r.handleRemovedMembers(ctx, org, executedActions)
	result.MembersRemoved = removed
	result.Errors = append(result.Errors, removeErrs...)

	// Step 1d: Update roles in DynamoDB.
	roleUpdated, roleErrs := r.handleRoleUpdates(ctx, org, executedActions)
	result.RolesUpdated = roleUpdated
	result.Errors = append(result.Errors, roleErrs...)

	// Step 1e: Create DynamoDB records for already-in-org members resolved via search/verified emails.
	alreadyResolved, alreadyErrs := r.handleAlreadyInOrgMembers(ctx, org, executedActions)
	result.AlreadyInOrgResolved = alreadyResolved
	result.Errors = append(result.Errors, alreadyErrs...)

	// Step 2: Check pending invitations that now have login resolved via GitHub API.
	resolved, errs := r.resolvePendingWithLogin(ctx, org)
	result.Resolved += resolved
	result.Errors = append(result.Errors, errs...)

	// Step 3: Consult audit log for org.add_member events.
	resolvedAudit, errs := r.resolveFromAuditLog(ctx, org)
	result.Resolved += resolvedAudit
	result.Errors = append(result.Errors, errs...)

	// Step 4: Handle failed and expired invitations.
	failed, expired, errs := r.handleFailedAndExpired(ctx, org)
	result.Failed = failed
	result.Expired = expired
	result.Errors = append(result.Errors, errs...)

	logrus.WithFields(logrus.Fields{
		"new_saved":                result.NewInvitationsSaved,
		"resolved":                 result.Resolved,
		"failed":                   result.Failed,
		"expired":                  result.Expired,
		"cancelled":                result.Cancelled,
		"members_removed":          result.MembersRemoved,
		"roles_updated":            result.RolesUpdated,
		"already_in_org_resolved":  result.AlreadyInOrgResolved,
		"errors":                   len(result.Errors),
	}).Info("🔄 Invitation reconciliation completed")

	return result, nil
}

// saveNewInvitations saves newly executed invite actions to DynamoDB.
func (r *Reconciler) saveNewInvitations(ctx context.Context, org string, actions []models.SyncAction) (int, []string) {
	saved := 0
	var errs []string

	for _, action := range actions {
		if action.Type != models.ActionInvite || !action.Executed || action.InvitationID == nil {
			continue
		}

		mapping := models.NewInvitationMapping(org, *action.InvitationID, action.Email, r.resolveRole(action), r.cfg.DynamoDB.TTLDays)

		if err := r.store.SaveInvitation(ctx, mapping); err != nil {
			errMsg := fmt.Sprintf("saving invitation for %s: %v", action.Email, err)
			logrus.WithError(err).WithField("email", action.Email).Warn("failed to save invitation mapping")
			errs = append(errs, errMsg)
			continue
		}

		logrus.WithFields(logrus.Fields{
			"email":         action.Email,
			"invitation_id": *action.InvitationID,
		}).Info("📋 Saved new invitation mapping")
		saved++
	}

	return saved, errs
}

// resolvePendingWithLogin checks GitHub pending invitations for any that now have a login.
func (r *Reconciler) resolvePendingWithLogin(ctx context.Context, org string) (int, []string) {
	resolved := 0
	var errs []string

	pendingInvites, err := r.githubClient.ListPendingInvitations(ctx, org)
	if err != nil {
		return 0, []string{fmt.Sprintf("listing pending invitations: %v", err)}
	}

	for _, invite := range pendingInvites {
		if invite.InvitationID == nil || invite.Username == nil || *invite.Username == "" {
			continue
		}

		// This invitation has a login resolved — check if we have it in DynamoDB as pending.
		mapping, err := r.store.GetInvitation(ctx, org, *invite.InvitationID)
		if err != nil {
			errs = append(errs, fmt.Sprintf("getting invitation %d: %v", *invite.InvitationID, err))
			continue
		}

		if mapping == nil || mapping.Status != models.InvitationPending || mapping.GitHubLogin != nil {
			continue
		}

		if err := r.store.ResolveInvitation(ctx, org, *invite.InvitationID, *invite.Username); err != nil {
			errs = append(errs, fmt.Sprintf("resolving invitation %d: %v", *invite.InvitationID, err))
			continue
		}

		logrus.WithFields(logrus.Fields{
			"email":         mapping.Email,
			"github_login":  *invite.Username,
			"invitation_id": *invite.InvitationID,
		}).Info("✅ Mapping resolved via pending invitation login")
		resolved++
	}

	return resolved, errs
}

// resolveFromAuditLog correlates org.add_member events with pending invitations.
func (r *Reconciler) resolveFromAuditLog(ctx context.Context, org string) (int, []string) {
	resolved := 0
	var errs []string

	// Get the cursor for where we left off.
	cursor, err := r.store.GetAuditLogCursor(ctx, org)
	if err != nil {
		return 0, []string{fmt.Sprintf("getting audit log cursor: %v", err)}
	}

	var afterTimestamp int64
	if cursor != nil {
		afterTimestamp = cursor.LastTimestamp
	}

	// Fetch new audit log events.
	entries, err := r.githubClient.GetAuditLogAddMemberEvents(ctx, org, afterTimestamp)
	if err != nil {
		return 0, []string{fmt.Sprintf("fetching audit log: %v", err)}
	}

	var lastTimestamp int64
	for _, entry := range entries {
		if entry.InvitationID == 0 || entry.User == "" {
			continue
		}

		if entry.Timestamp > lastTimestamp {
			lastTimestamp = entry.Timestamp
		}

		// Look up the invitation in DynamoDB.
		mapping, err := r.store.GetInvitation(ctx, org, entry.InvitationID)
		if err != nil {
			errs = append(errs, fmt.Sprintf("getting invitation %d from audit log: %v", entry.InvitationID, err))
			continue
		}

		if mapping == nil || mapping.Status != models.InvitationPending {
			continue
		}

		if err := r.store.ResolveInvitation(ctx, org, entry.InvitationID, entry.User); err != nil {
			errs = append(errs, fmt.Sprintf("resolving invitation %d from audit log: %v", entry.InvitationID, err))
			continue
		}

		logrus.WithFields(logrus.Fields{
			"email":         mapping.Email,
			"github_login":  entry.User,
			"invitation_id": entry.InvitationID,
		}).Info("✅ Mapping resolved via audit log (org.add_member)")
		resolved++
	}

	// Step 5: Save the cursor.
	if lastTimestamp > 0 {
		newCursor := models.AuditLogCursor{
			PK:           "ORG#" + org,
			SK:           "CURSOR#audit_log",
			LastTimestamp: lastTimestamp,
			LastRun:      time.Now().UTC(),
		}
		if err := r.store.SaveAuditLogCursor(ctx, newCursor); err != nil {
			errs = append(errs, fmt.Sprintf("saving audit log cursor: %v", err))
		}
	}

	return resolved, errs
}

// handleFailedAndExpired marks invitations as failed or expired.
func (r *Reconciler) handleFailedAndExpired(ctx context.Context, org string) (int, int, []string) {
	failed := 0
	expired := 0
	var errs []string

	// Check failed invitations from GitHub API.
	failedInvites, err := r.githubClient.ListFailedInvitations(ctx, org)
	if err != nil {
		errs = append(errs, fmt.Sprintf("listing failed invitations: %v", err))
	} else {
		for _, invite := range failedInvites {
			if invite.InvitationID == nil {
				continue
			}

			mapping, err := r.store.GetInvitation(ctx, org, *invite.InvitationID)
			if err != nil {
				errs = append(errs, fmt.Sprintf("getting invitation %d for failure check: %v", *invite.InvitationID, err))
				continue
			}

			if mapping == nil || mapping.Status != models.InvitationPending {
				continue
			}

			if err := r.store.UpdateStatus(ctx, org, *invite.InvitationID, models.InvitationFailed); err != nil {
				errs = append(errs, fmt.Sprintf("marking invitation %d as failed: %v", *invite.InvitationID, err))
				continue
			}

			logrus.WithFields(logrus.Fields{
				"email":         mapping.Email,
				"invitation_id": *invite.InvitationID,
			}).Warn("❌ Invitation failed")
			failed++
		}
	}

	// Check for expired invitations (pending > 7 days).
	pendingMappings, err := r.store.GetPendingInvitations(ctx, org)
	if err != nil {
		errs = append(errs, fmt.Sprintf("getting pending invitations for expiry check: %v", err))
		return failed, expired, errs
	}

	// Build a set of currently pending invitation IDs from GitHub.
	currentPending, err := r.githubClient.ListPendingInvitations(ctx, org)
	if err != nil {
		errs = append(errs, fmt.Sprintf("listing current pending invitations for expiry check: %v", err))
		return failed, expired, errs
	}
	pendingIDs := make(map[int64]struct{})
	for _, inv := range currentPending {
		if inv.InvitationID != nil {
			pendingIDs[*inv.InvitationID] = struct{}{}
		}
	}

	// Build a set of failed invitation IDs.
	failedIDs := make(map[int64]struct{})
	for _, inv := range failedInvites {
		if inv.InvitationID != nil {
			failedIDs[*inv.InvitationID] = struct{}{}
		}
	}

	now := time.Now().UTC()
	for _, mapping := range pendingMappings {
		if now.Sub(mapping.InvitedAt) <= invitationExpiryDays*24*time.Hour {
			continue
		}

		// Extract invitation ID from SK (format: INV#<id>)
		invIDStr := strings.TrimPrefix(mapping.SK, "INV#")
		var invID int64
		if _, err := fmt.Sscanf(invIDStr, "%d", &invID); err != nil {
			continue
		}

		// Skip if already handled by failed check.
		if _, isFailed := failedIDs[invID]; isFailed {
			continue
		}

		// If not in current pending AND not in failed → likely expired.
		if _, isPending := pendingIDs[invID]; !isPending {
			if err := r.store.UpdateStatus(ctx, org, invID, models.InvitationExpired); err != nil {
				errs = append(errs, fmt.Sprintf("marking invitation %d as expired: %v", invID, err))
				continue
			}

			logrus.WithFields(logrus.Fields{
				"email":         mapping.Email,
				"invitation_id": invID,
				"invited_at":    mapping.InvitedAt,
			}).Warn("⏰ Invitation expired (>7 days, no longer pending)")
			expired++
		}
	}

	return failed, expired, errs
}

// resolveRole extracts the target role from a SyncAction.
func (r *Reconciler) resolveRole(action models.SyncAction) models.OrgRole {
	if action.TargetRole != nil {
		return *action.TargetRole
	}
	return models.RoleMember
}

// markCancelledInvitations marks cancelled invitations in DynamoDB.
func (r *Reconciler) markCancelledInvitations(ctx context.Context, org string, actions []models.SyncAction) (int, []string) {
	cancelled := 0
	var errs []string

	for _, action := range actions {
		if action.Type != models.ActionCancelInvite || !action.Executed || action.InvitationID == nil {
			continue
		}

		if err := r.store.UpdateStatus(ctx, org, *action.InvitationID, models.InvitationCancelled); err != nil {
			errMsg := fmt.Sprintf("marking invitation %d as cancelled: %v", *action.InvitationID, err)
			logrus.WithError(err).WithField("invitation_id", *action.InvitationID).Warn("failed to mark invitation as cancelled in DynamoDB")
			errs = append(errs, errMsg)
			continue
		}

		logrus.WithFields(logrus.Fields{
			"email":         action.Email,
			"invitation_id": *action.InvitationID,
		}).Info("🚫 Invitation cancelled and marked in DynamoDB")
		cancelled++
	}

	return cancelled, errs
}

// handleRemovedMembers updates DynamoDB records for removed members.
// When a member is removed from the GitHub org, their invitation mapping is marked as "removed".
func (r *Reconciler) handleRemovedMembers(ctx context.Context, org string, actions []models.SyncAction) (int, []string) {
	removed := 0
	var errs []string

	for _, action := range actions {
		if action.Type != models.ActionRemove || !action.Executed || action.GoogleEmail == "" {
			continue
		}

		// Look up the DynamoDB record by the original Google email.
		mappings, err := r.store.GetByEmail(ctx, action.GoogleEmail, org)
		if err != nil {
			errMsg := fmt.Sprintf("looking up DynamoDB record for removed member %s (email: %s): %v", action.Email, action.GoogleEmail, err)
			logrus.WithError(err).WithFields(logrus.Fields{
				"username": action.Email,
				"email":    action.GoogleEmail,
			}).Warn("failed to look up DynamoDB record for removed member")
			errs = append(errs, errMsg)
			continue
		}

		// Find the resolved mapping and mark it as removed.
		for _, m := range mappings {
			if m.Status != models.InvitationResolved {
				continue
			}

			invIDStr := strings.TrimPrefix(m.SK, "INV#")
			var invID int64
			if _, err := fmt.Sscanf(invIDStr, "%d", &invID); err != nil {
				continue
			}

			if err := r.store.UpdateStatus(ctx, org, invID, models.InvitationRemoved); err != nil {
				errMsg := fmt.Sprintf("marking invitation %d as removed for %s: %v", invID, action.Email, err)
				logrus.WithError(err).WithFields(logrus.Fields{
					"username":      action.Email,
					"invitation_id": invID,
				}).Warn("failed to mark DynamoDB record as removed")
				errs = append(errs, errMsg)
				continue
			}

			logrus.WithFields(logrus.Fields{
				"username":      action.Email,
				"email":         action.GoogleEmail,
				"invitation_id": invID,
			}).Info("🗑️ Member removed — DynamoDB record marked as removed")
			removed++
		}
	}

	return removed, errs
}

// handleRoleUpdates updates the role in DynamoDB records when a member's role changes.
func (r *Reconciler) handleRoleUpdates(ctx context.Context, org string, actions []models.SyncAction) (int, []string) {
	updated := 0
	var errs []string

	for _, action := range actions {
		if action.Type != models.ActionUpdateRole || !action.Executed || action.GoogleEmail == "" || action.TargetRole == nil {
			continue
		}

		// Look up the DynamoDB record by the original Google email.
		mappings, err := r.store.GetByEmail(ctx, action.GoogleEmail, org)
		if err != nil {
			errMsg := fmt.Sprintf("looking up DynamoDB record for role update %s (email: %s): %v", action.Email, action.GoogleEmail, err)
			logrus.WithError(err).WithFields(logrus.Fields{
				"username": action.Email,
				"email":    action.GoogleEmail,
			}).Warn("failed to look up DynamoDB record for role update")
			errs = append(errs, errMsg)
			continue
		}

		// Find the resolved mapping and update its role.
		for _, m := range mappings {
			if m.Status != models.InvitationResolved {
				continue
			}

			invIDStr := strings.TrimPrefix(m.SK, "INV#")
			var invID int64
			if _, err := fmt.Sscanf(invIDStr, "%d", &invID); err != nil {
				continue
			}

			if err := r.store.UpdateRole(ctx, org, invID, *action.TargetRole); err != nil {
				errMsg := fmt.Sprintf("updating role for invitation %d to %s: %v", invID, *action.TargetRole, err)
				logrus.WithError(err).WithFields(logrus.Fields{
					"username":      action.Email,
					"invitation_id": invID,
				}).Warn("failed to update role in DynamoDB")
				errs = append(errs, errMsg)
				continue
			}

			logrus.WithFields(logrus.Fields{
				"username":      action.Email,
				"email":         action.GoogleEmail,
				"invitation_id": invID,
				"new_role":      *action.TargetRole,
			}).Info("🔄 Role updated in DynamoDB")
			updated++
		}
	}

	return updated, errs
}

// EnsureVerifiedEmailMappings creates DynamoDB records for Google group members
// whose email matches a verified domain email on a GitHub org member, but who
// don't yet have a DynamoDB mapping. This ensures that users recognized via
// GraphQL verified emails are tracked in DynamoDB for future syncs (role changes,
// removal in conservative mode, etc.).
func (r *Reconciler) EnsureVerifiedEmailMappings(ctx context.Context, verifiedEmails map[string]string, membersGroup []models.GoogleGroupMember, ownersGroup []models.GoogleGroupMember, result *models.ReconcileResult) {
	if verifiedEmails == nil || len(verifiedEmails) == 0 {
		return
	}

	org := r.cfg.GitHub.Organization

	// Build a map of Google email → desired role.
	// Members first, then owners (owners take precedence, matching CalculateDiff logic).
	googleRoleByEmail := make(map[string]models.OrgRole)
	for _, m := range membersGroup {
		if m.Email != "" && m.IsActive() {
			googleRoleByEmail[strings.ToLower(m.Email)] = models.RoleMember
		}
	}
	for _, o := range ownersGroup {
		if o.Email != "" && o.IsActive() {
			googleRoleByEmail[strings.ToLower(o.Email)] = models.RoleOwner
		}
	}

	for email, username := range verifiedEmails {
		lowerEmail := strings.ToLower(email)

		// Only process emails that are in Google groups (desired state).
		desiredRole, inGoogle := googleRoleByEmail[lowerEmail]
		if !inGoogle {
			continue
		}

		// Check if we already have a resolved mapping for this email.
		existing, err := r.store.GetByEmail(ctx, email, org)
		if err != nil {
			errMsg := fmt.Sprintf("checking existing mapping for verified email %s: %v", email, err)
			logrus.WithError(err).WithField("email", email).Warn("failed to check DynamoDB for verified email mapping")
			result.Errors = append(result.Errors, errMsg)
			continue
		}

		alreadyMapped := false
		for _, m := range existing {
			if m.Status == models.InvitationResolved && m.GitHubLogin != nil {
				alreadyMapped = true
				break
			}
		}
		if alreadyMapped {
			continue
		}

		// Create EXISTING# record with the correct role from Google groups.
		now := time.Now().UTC()
		ttl := now.AddDate(0, 0, r.cfg.DynamoDB.TTLDays).Unix()
		login := username

		mapping := models.InvitationMapping{
			PK:          "ORG#" + org,
			SK:          "EXISTING#" + username,
			Email:       email,
			GitHubLogin: &login,
			Status:      models.InvitationResolved,
			Role:        desiredRole,
			InvitedAt:   now,
			ResolvedAt:  &now,
			TTL:         ttl,
			GSI1PK:      "EMAIL#" + email,
			GSI1SK:      "ORG#" + org,
			GSI2PK:      "ORG#" + org,
			GSI2SK:      "STATUS#" + string(models.InvitationResolved),
		}

		if err := r.store.SaveInvitation(ctx, mapping); err != nil {
			errMsg := fmt.Sprintf("saving verified email mapping for %s (%s): %v", email, username, err)
			logrus.WithError(err).WithFields(logrus.Fields{
				"email":    email,
				"username": username,
			}).Warn("failed to save verified email DynamoDB mapping")
			result.Errors = append(result.Errors, errMsg)
			continue
		}

		logrus.WithFields(logrus.Fields{
			"email":    email,
			"username": username,
			"role":     desiredRole,
		}).Info("✅ Created DynamoDB mapping for existing org member via verified domain email")
		result.VerifiedEmailsMapped++
	}
}

// handleAlreadyInOrgMembers creates DynamoDB records for users who were already in the org
// when an invitation was attempted. These users have been resolved via SearchUserByEmail
// or verified domain emails, and their role has been updated. We create an EXISTING# record
// so that future syncs recognize them in the known set and skip the invite attempt.
func (r *Reconciler) handleAlreadyInOrgMembers(ctx context.Context, org string, actions []models.SyncAction) (int, []string) {
	resolved := 0
	var errs []string

	for _, action := range actions {
		// Only handle actions that were upgraded from invite → update_role for already-in-org users.
		if !action.AlreadyInOrg || !action.Executed || action.Username == "" {
			continue
		}

		email := action.GoogleEmail
		if email == "" {
			email = action.Email
		}

		// Check if we already have a resolved mapping for this email to avoid duplicates.
		existing, err := r.store.GetByEmail(ctx, email, org)
		if err != nil {
			errMsg := fmt.Sprintf("checking existing mapping for %s: %v", email, err)
			logrus.WithError(err).WithField("email", email).Warn("failed to check existing DynamoDB mapping for already-in-org member")
			errs = append(errs, errMsg)
			continue
		}

		alreadyMapped := false
		for _, m := range existing {
			if m.Status == models.InvitationResolved && m.GitHubLogin != nil && strings.EqualFold(*m.GitHubLogin, action.Username) {
				alreadyMapped = true
				break
			}
		}
		if alreadyMapped {
			logrus.WithFields(logrus.Fields{
				"email":    email,
				"username": action.Username,
			}).Debug("already-in-org member already has a resolved DynamoDB mapping, skipping")
			continue
		}

		// Create a resolved mapping with EXISTING# SK convention.
		now := time.Now().UTC()
		ttl := now.AddDate(0, 0, r.cfg.DynamoDB.TTLDays).Unix()
		login := action.Username

		role := models.RoleMember
		if action.TargetRole != nil {
			role = *action.TargetRole
		}

		mapping := models.InvitationMapping{
			PK:          "ORG#" + org,
			SK:          "EXISTING#" + action.Username,
			Email:       email,
			GitHubLogin: &login,
			Status:      models.InvitationResolved,
			Role:        role,
			InvitedAt:   now,
			ResolvedAt:  &now,
			TTL:         ttl,
			GSI1PK:      "EMAIL#" + email,
			GSI1SK:      "ORG#" + org,
			GSI2PK:      "ORG#" + org,
			GSI2SK:      "STATUS#" + string(models.InvitationResolved),
		}

		if err := r.store.SaveInvitation(ctx, mapping); err != nil {
			errMsg := fmt.Sprintf("saving already-in-org mapping for %s (%s): %v", email, action.Username, err)
			logrus.WithError(err).WithFields(logrus.Fields{
				"email":    email,
				"username": action.Username,
			}).Warn("failed to save already-in-org DynamoDB mapping")
			errs = append(errs, errMsg)
			continue
		}

		logrus.WithFields(logrus.Fields{
			"email":    email,
			"username": action.Username,
			"role":     role,
		}).Info("✅ Created DynamoDB mapping for already-in-org member")
		resolved++
	}

	return resolved, errs
}
