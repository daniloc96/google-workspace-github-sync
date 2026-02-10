package sync

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/daniloc96/google-workspace-github-sync/internal/config"
	"github.com/daniloc96/google-workspace-github-sync/internal/interfaces"
	"github.com/daniloc96/google-workspace-github-sync/internal/models"
	"github.com/sirupsen/logrus"
)

// Engine orchestrates a sync run.
type Engine struct {
	googleClient interfaces.GoogleClient
	githubClient interfaces.GitHubClient
	reconciler   *Reconciler
	cfg          *config.Config
	mu           sync.Mutex
	running      bool
}

// NewEngine creates a sync engine.
func NewEngine(googleClient interfaces.GoogleClient, githubClient interfaces.GitHubClient, cfg *config.Config) *Engine {
	return &Engine{googleClient: googleClient, githubClient: githubClient, cfg: cfg}
}

// SetReconciler sets the reconciler for invitation tracking. If nil, reconciliation is skipped.
func (e *Engine) SetReconciler(r *Reconciler) {
	e.reconciler = r
}

// Sync performs a synchronization run.
func (e *Engine) Sync(ctx context.Context) (*models.SyncResult, error) {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return nil, fmt.Errorf("sync already in progress")
	}
	e.running = true
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.running = false
		e.mu.Unlock()
	}()

	start := time.Now()

	membersGroup, err := e.googleClient.GetGroupMembers(ctx, e.cfg.Google.MembersGroup)
	if err != nil {
		return nil, err
	}
	ownersGroup, err := e.googleClient.GetGroupMembers(ctx, e.cfg.Google.OwnersGroup)
	if err != nil {
		return nil, err
	}

	if e.cfg.Sync.IgnoreSuspended {
		if err := applySuspensionStatus(ctx, e.googleClient, membersGroup, ownersGroup); err != nil {
			return nil, err
		}
	}

	githubMembers, err := e.githubClient.ListMembers(ctx, e.cfg.GitHub.Organization)
	if err != nil {
		return nil, err
	}

	pendingInvites, err := e.githubClient.ListPendingInvitations(ctx, e.cfg.GitHub.Organization)
	if err != nil {
		return nil, err
	}

	// Phase 1: Google groups loaded.
	logrus.WithFields(logrus.Fields{
		"members_group": len(membersGroup),
		"owners_group":  len(ownersGroup),
	}).Info("📋 [1/5] Google groups loaded")
	for _, m := range membersGroup {
		logrus.WithFields(logrus.Fields{"email": m.Email, "group": "members", "active": m.IsActive()}).Debug("  Google group member")
	}
	for _, m := range ownersGroup {
		logrus.WithFields(logrus.Fields{"email": m.Email, "group": "owners", "active": m.IsActive()}).Debug("  Google group member")
	}

	// Build email mappings from DynamoDB (if reconciler is available).
	var emailMappings *EmailMappings
	if e.reconciler != nil {
		emailMappings = e.buildEmailMappings(ctx)
	}

	// Fetch verified domain emails via GraphQL (Enterprise Cloud feature).
	// This maps verified-domain emails → GitHub usernames for all org members,
	// even when their email is private. Non-fatal: diff works without it.
	var verifiedEmails map[string]string
	verifiedEmails, err = e.githubClient.ListMembersWithVerifiedEmails(ctx, e.cfg.GitHub.Organization)
	if err != nil {
		logrus.WithError(err).Warn("⚠ Could not fetch verified domain emails via GraphQL (sync will continue without them)")
		verifiedEmails = nil
	}

	// Phase 2: GitHub org loaded.
	logrus.WithFields(logrus.Fields{
		"org":     e.cfg.GitHub.Organization,
		"members": len(githubMembers),
		"pending": len(pendingInvites),
	}).Info("🐙 [2/5] GitHub organization loaded")
	for _, m := range githubMembers {
		fields := logrus.Fields{"username": ptrVal(m.Username), "role": m.Role}
		if m.Email != nil {
			fields["email"] = *m.Email
		}
		logrus.WithFields(fields).Debug("  GitHub org member")
	}
	for _, inv := range pendingInvites {
		fields := logrus.Fields{"invitation_id": ptrInt64Val(inv.InvitationID), "role": inv.Role}
		if inv.Email != nil {
			fields["email"] = *inv.Email
		}
		if inv.Username != nil {
			fields["username"] = *inv.Username
		}
		logrus.WithFields(fields).Debug("  GitHub pending invitation")
	}

	actions := CalculateDiff(membersGroup, ownersGroup, githubMembers, pendingInvites, e.cfg.Sync.RemoveExtraMembers, emailMappings, verifiedEmails)
	logrus.WithField("actions", len(actions)).Info("🔍 [3/5] Diff calculated")
	if e.cfg.Sync.DryRun {
		for _, action := range actions {
			logrus.WithFields(action.LogFields()).Info("  [DRY RUN] would execute")
		}
	}
	if len(actions) > 0 {
		logrus.WithField("dry_run", e.cfg.Sync.DryRun).Info("⚡ [4/5] Executing actions")
	} else {
		logrus.Info("⚡ [4/5] No actions to execute")
	}
	updatedActions, err := ExecuteActions(ctx, e.githubClient, e.cfg.GitHub.Organization, actions, e.cfg.Sync.DryRun)
	if err != nil {
		return nil, err
	}

	// Invitation reconciliation (opt-in, non-fatal).
	var reconcileResult *models.ReconcileResult
	if e.reconciler != nil && !e.cfg.Sync.DryRun {
		logrus.Info("🔄 [5/5] Running invitation reconciliation")
		reconcileResult, err = e.reconciler.Reconcile(ctx, updatedActions)
		if err != nil {
			logrus.WithError(err).Warn("⚠ Reconciliation failed (non-fatal, sync results are still valid)")
		}

		// Ensure DynamoDB mappings exist for Google members matched via verified domain emails.
		// This handles users already in the org who are recognized by CalculateDiff (no invite
		// generated) but don't yet have a DynamoDB record for tracking.
		if reconcileResult != nil && verifiedEmails != nil {
			e.reconciler.EnsureVerifiedEmailMappings(ctx, verifiedEmails, membersGroup, ownersGroup, reconcileResult)
		}
	}

	end := time.Now()
	summary := buildSummary(append(membersGroup, ownersGroup...), githubMembers, pendingInvites, updatedActions)

	// Build detailed user lists
	invitedUsers, alreadyInOrgUsers := classifyInviteActions(updatedActions)
	orphanedUsers := findOrphanedGitHubUsers(membersGroup, ownersGroup, githubMembers, verifiedEmails, emailMappings)

	summary.AlreadyInOrg = len(alreadyInOrgUsers)
	summary.OrphanedGitHub = len(orphanedUsers)

	return &models.SyncResult{
		DryRun:              e.cfg.Sync.DryRun,
		StartTime:           start,
		EndTime:             end,
		DurationMs:          end.Sub(start).Milliseconds(),
		Actions:             updatedActions,
		Summary:             summary,
		InvitedUsers:        invitedUsers,
		AlreadyInOrgUsers:   alreadyInOrgUsers,
		OrphanedGitHubUsers: orphanedUsers,
		Reconciliation:      reconcileResult,
	}, nil
}

// buildEmailMappings fetches resolved and pending mappings from DynamoDB.
// Returns nil if fetching fails (non-fatal — diff will work without enrichment).
func (e *Engine) buildEmailMappings(ctx context.Context) *EmailMappings {
	org := e.cfg.GitHub.Organization

	resolved, err := e.reconciler.store.GetAllResolvedMappings(ctx, org)
	if err != nil {
		logrus.WithError(err).Warn("⚠ Could not fetch resolved mappings from DynamoDB (sync will continue without enrichment)")
		return nil
	}

	pendingMappings, err := e.reconciler.store.GetPendingInvitations(ctx, org)
	if err != nil {
		logrus.WithError(err).Warn("⚠ Could not fetch pending mappings from DynamoDB (sync will continue without enrichment)")
		return nil
	}

	pending := make(map[string]int64, len(pendingMappings))
	for _, m := range pendingMappings {
		invIDStr := strings.TrimPrefix(m.SK, "INV#")
		var invID int64
		if _, err := fmt.Sscanf(invIDStr, "%d", &invID); err == nil {
			pending[m.Email] = invID
		}
	}

	logrus.WithFields(logrus.Fields{
		"resolved_mappings": len(resolved),
		"pending_mappings":  len(pending),
	}).Debug("loaded email mappings from DynamoDB")

	return &EmailMappings{
		Resolved:           resolved,
		PendingInvitations: pending,
	}
}

func applySuspensionStatus(ctx context.Context, client interfaces.GoogleClient, membersGroup []models.GoogleGroupMember, ownersGroup []models.GoogleGroupMember) error {
	emails := make([]string, 0, len(membersGroup)+len(ownersGroup))
	seen := map[string]struct{}{}
	for _, member := range append(membersGroup, ownersGroup...) {
		if member.Email == "" {
			continue
		}
		if _, exists := seen[member.Email]; exists {
			continue
		}
		seen[member.Email] = struct{}{}
		emails = append(emails, member.Email)
	}
	statuses, err := client.GetUsersSuspendedStatus(ctx, emails)
	if err != nil {
		return err
	}
	for i := range membersGroup {
		membersGroup[i].IsSuspended = statuses[membersGroup[i].Email]
	}
	for i := range ownersGroup {
		ownersGroup[i].IsSuspended = statuses[ownersGroup[i].Email]
	}
	return nil
}

func buildSummary(googleMembers []models.GoogleGroupMember, githubMembers []models.GitHubOrgMember, pendingInvites []models.GitHubOrgMember, actions []models.SyncAction) models.SyncSummary {
	summary := models.SyncSummary{
		TotalGoogleMembers: len(googleMembers),
		TotalGitHubMembers: len(githubMembers),
		PendingInvitations: len(pendingInvites),
		ActionsPlanned:     len(actions),
	}

	for _, action := range actions {
		if action.Executed {
			summary.ActionsExecuted++
		}
		if action.Error != nil {
			summary.ActionsFailed++
		}
		switch action.Type {
		case models.ActionInvite:
			summary.Invited++
		case models.ActionRemove:
			summary.Removed++
		case models.ActionUpdateRole:
			summary.RoleUpdated++
		case models.ActionCancelInvite:
			summary.CancelledInvites++
		case models.ActionSkip:
			summary.Skipped++
		}
	}

	return summary
}

// classifyInviteActions splits invite actions into successfully invited and already-in-org.
func classifyInviteActions(actions []models.SyncAction) (invited []string, alreadyInOrg []string) {
	for _, action := range actions {
		if action.Type != models.ActionInvite {
			continue
		}
		if action.AlreadyInOrg {
			alreadyInOrg = append(alreadyInOrg, action.Email)
		} else if action.Executed {
			invited = append(invited, action.Email)
		}
	}
	return
}

// findOrphanedGitHubUsers returns GitHub members whose identifier is not found in any Google group.
// It uses verifiedEmails and emailMappings to reverse-lookup GitHub usernames → Google emails.
func findOrphanedGitHubUsers(membersGroup []models.GoogleGroupMember, ownersGroup []models.GoogleGroupMember, githubMembers []models.GitHubOrgMember, verifiedEmails map[string]string, emailMappings *EmailMappings) []string {
	googleEmails := map[string]struct{}{}
	for _, m := range membersGroup {
		if m.Email != "" {
			googleEmails[strings.ToLower(m.Email)] = struct{}{}
		}
	}
	for _, m := range ownersGroup {
		if m.Email != "" {
			googleEmails[strings.ToLower(m.Email)] = struct{}{}
		}
	}

	// Build reverse lookup: lowercase GitHub username → lowercase Google email.
	usernameToEmail := map[string]string{}
	if emailMappings != nil {
		for email, username := range emailMappings.Resolved {
			usernameToEmail[strings.ToLower(username)] = strings.ToLower(email)
		}
	}
	if verifiedEmails != nil {
		for email, username := range verifiedEmails {
			lowerUser := strings.ToLower(username)
			if _, exists := usernameToEmail[lowerUser]; !exists {
				usernameToEmail[lowerUser] = strings.ToLower(email)
			}
		}
	}

	var orphaned []string
	for _, ghMember := range githubMembers {
		if ghMember.IsPending {
			continue
		}
		id := strings.ToLower(ghMember.Identifier())
		if id == "" {
			continue
		}

		// Direct match: GitHub identifier (email/username) is in Google groups.
		if _, exists := googleEmails[id]; exists {
			continue
		}

		// Reverse lookup: GitHub username → Google email via verified emails or DynamoDB.
		if ghMember.Username != nil {
			if email, ok := usernameToEmail[strings.ToLower(*ghMember.Username)]; ok {
				if _, exists := googleEmails[email]; exists {
					continue
				}
			}
		}

		// Use username if available, otherwise the identifier.
		display := id
		if ghMember.Username != nil && *ghMember.Username != "" {
			display = *ghMember.Username
		}
		orphaned = append(orphaned, display)
	}
	return orphaned
}

func ptrVal(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

func ptrInt64Val(p *int64) int64 {
	if p != nil {
		return *p
	}
	return 0
}
