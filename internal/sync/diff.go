package sync

import (
	"strings"

	"github.com/daniloc96/google-workspace-github-sync/internal/models"
)

// EmailMappings holds resolved email→username mappings and pending invitation info from DynamoDB.
type EmailMappings struct {
	// Resolved maps lowercase email → GitHub username for accepted invitations.
	Resolved map[string]string
	// PendingInvitations maps lowercase email → invitation ID for pending invitations.
	PendingInvitations map[string]int64
}

// CalculateDiff determines sync actions for Google members and owners not in GitHub.
// emailMappings (optional) enriches the diff with DynamoDB email→username data to:
// - remove users by username when their GitHub email isn't public
// - update roles by username when their GitHub email isn't public
// - cancel pending invitations when the user is removed from Google groups
// verifiedEmails (optional) maps lowercase verified-domain email → GitHub username,
// loaded via the GraphQL organizationVerifiedDomainEmails API.
func CalculateDiff(membersGroup []models.GoogleGroupMember, ownersGroup []models.GoogleGroupMember, githubMembers []models.GitHubOrgMember, pendingInvites []models.GitHubOrgMember, removeExtraMembers bool, emailMappings *EmailMappings, verifiedEmails map[string]string) []models.SyncAction {
	// Build a set of known identifiers in GitHub (email or username).
	known := map[string]struct{}{}
	for _, member := range githubMembers {
		id := strings.ToLower(member.Identifier())
		if id != "" {
			known[id] = struct{}{}
		}
	}
	for _, invite := range pendingInvites {
		id := strings.ToLower(invite.Identifier())
		if id != "" {
			known[id] = struct{}{}
		}
	}

	// Build a username→GitHubOrgMember index for quick lookups.
	ghByUsername := map[string]*models.GitHubOrgMember{}
	for i := range githubMembers {
		if githubMembers[i].Username != nil {
			ghByUsername[strings.ToLower(*githubMembers[i].Username)] = &githubMembers[i]
		}
	}

	// If we have DynamoDB mappings, also consider resolved usernames as "known" identifiers
	// so we can map Google emails → GitHub usernames even when GitHub email isn't public.
	resolvedUserByEmail := map[string]string{}   // lowercase email → GitHub username
	emailByResolvedUser := map[string]string{}   // lowercase username → email (reverse lookup)
	if emailMappings != nil {
		for email, username := range emailMappings.Resolved {
			lowerEmail := strings.ToLower(email)
			lowerUser := strings.ToLower(username)
			resolvedUserByEmail[lowerEmail] = username
			emailByResolvedUser[lowerUser] = lowerEmail
			// If the email is known via DynamoDB, mark it as known in GitHub
			if _, exists := known[lowerEmail]; !exists {
				// Check if the username is actually in GitHub members
				if _, inGH := ghByUsername[lowerUser]; inGH {
					known[lowerEmail] = struct{}{}
				}
			}
		}
	}

	// If we have verified domain emails (from GraphQL), also mark them as "known"
	// and populate the reverse lookup maps. This handles users already in the org
	// whose email isn't public and who don't have a DynamoDB mapping yet.
	if verifiedEmails != nil {
		for email, username := range verifiedEmails {
			lowerEmail := strings.ToLower(email)
			lowerUser := strings.ToLower(username)
			// Only add if not already covered by DynamoDB mappings
			if _, exists := resolvedUserByEmail[lowerEmail]; !exists {
				resolvedUserByEmail[lowerEmail] = username
				emailByResolvedUser[lowerUser] = lowerEmail
			}
			if _, exists := known[lowerEmail]; !exists {
				if _, inGH := ghByUsername[lowerUser]; inGH {
					known[lowerEmail] = struct{}{}
				}
			}
		}
	}

	// Build desired state from Google groups (owners take precedence).
	type roleEntry struct {
		email string
		role  models.OrgRole
	}
	roleByEmail := map[string]roleEntry{}
	for _, member := range membersGroup {
		if !member.IsActive() {
			continue
		}
		roleByEmail[strings.ToLower(member.Email)] = roleEntry{email: member.Email, role: models.RoleMember}
	}
	for _, owner := range ownersGroup {
		if !owner.IsActive() {
			continue
		}
		roleByEmail[strings.ToLower(owner.Email)] = roleEntry{email: owner.Email, role: models.RoleOwner}
	}

	actions := make([]models.SyncAction, 0)

	// --- Invite: Google users not yet in GitHub ---
	for key, entry := range roleByEmail {
		if _, exists := known[key]; exists {
			continue
		}
		resolvedRole := entry.role
		actions = append(actions, models.SyncAction{
			Type:       models.ActionInvite,
			Email:      entry.email,
			TargetRole: &resolvedRole,
			Reason:     "missing in GitHub organization",
		})
	}

	// --- Remove: GitHub members not in any Google group ---
	// Build googleSet for remove/cancel logic (needed in both modes).
	googleSet := map[string]struct{}{}
	for key := range roleByEmail {
		googleSet[key] = struct{}{}
	}

	if removeExtraMembers {
		// Aggressive mode: remove ALL GitHub members not in any Google group.
		for _, member := range githubMembers {
			if member.IsPending || member.Username == nil {
				continue
			}
			identifier := strings.ToLower(member.Identifier())
			username := strings.ToLower(*member.Username)

			// Check by email/username identifier
			if identifier != "" {
				if _, exists := googleSet[identifier]; exists {
					continue
				}
			}

			// Check via DynamoDB reverse lookup: is this username linked to a Google email?
			if email, ok := emailByResolvedUser[username]; ok {
				if _, exists := googleSet[email]; exists {
					continue // user IS in Google groups, matched via DynamoDB
				}
			}

			// Carry the original Google email for DynamoDB record update during reconciliation.
			googleEmail := ""
			if e, ok := emailByResolvedUser[username]; ok {
				googleEmail = e
			}
			actions = append(actions, models.SyncAction{
				Type:        models.ActionRemove,
				Email:       *member.Username,
				GoogleEmail: googleEmail,
				Reason:      "missing from Google groups",
			})
		}
	} else if emailMappings != nil {
		// Conservative mode: only remove members tracked in DynamoDB (added by this tool)
		// who are no longer in any Google group. Pre-existing org members are untouched.
		for _, member := range githubMembers {
			if member.IsPending || member.Username == nil {
				continue
			}
			username := strings.ToLower(*member.Username)

			// Only act on members we know about via DynamoDB.
			googleEmail, tracked := emailByResolvedUser[username]
			if !tracked {
				continue // not tracked in DynamoDB → pre-existing member, skip
			}

			// Check if user is still in Google groups.
			if _, exists := googleSet[googleEmail]; exists {
				continue // still in Google groups
			}

			actions = append(actions, models.SyncAction{
				Type:        models.ActionRemove,
				Email:       *member.Username,
				GoogleEmail: googleEmail,
				Reason:      "removed from Google groups (tracked via DynamoDB)",
			})
		}
	}

	// Cancel pending invitations for users removed from Google groups.
	// Works in both modes as long as emailMappings is available.
	if emailMappings != nil {
		for _, invite := range pendingInvites {
			if invite.Email == nil || *invite.Email == "" || invite.InvitationID == nil {
				continue
			}
			email := strings.ToLower(*invite.Email)
			if _, exists := googleSet[email]; exists {
				continue // still in Google groups
			}
			invID := *invite.InvitationID
			actions = append(actions, models.SyncAction{
				Type:         models.ActionCancelInvite,
				Email:        *invite.Email,
				InvitationID: &invID,
				Reason:       "removed from Google groups, cancelling pending invitation",
			})
		}
		// Also cancel from DynamoDB pending mappings (for invitations we know about)
		for email, invID := range emailMappings.PendingInvitations {
			if _, exists := googleSet[strings.ToLower(email)]; exists {
				continue // still in Google groups
			}
			// Check it's not already covered by the pendingInvites loop above
			alreadyCovered := false
			for _, invite := range pendingInvites {
				if invite.InvitationID != nil && *invite.InvitationID == invID {
					alreadyCovered = true
					break
				}
			}
			if alreadyCovered {
				continue
			}
			id := invID
			actions = append(actions, models.SyncAction{
				Type:         models.ActionCancelInvite,
				Email:        email,
				InvitationID: &id,
				Reason:       "removed from Google groups, cancelling pending invitation",
			})
		}
	}

	// --- Role changes: GitHub members whose role doesn't match Google desired role ---
	for _, member := range githubMembers {
		if member.IsPending || member.Username == nil {
			continue
		}
		identifier := strings.ToLower(member.Identifier())
		username := strings.ToLower(*member.Username)

		// Try direct match by email/username
		desired, exists := roleByEmail[identifier]
		if !exists {
			// Try DynamoDB reverse lookup: username → email → desired role
			if email, ok := emailByResolvedUser[username]; ok {
				desired, exists = roleByEmail[email]
			}
		}
		if !exists {
			continue
		}
		if member.Role == desired.role {
			continue
		}
		current := member.Role
		target := desired.role
		// Carry the original Google email for DynamoDB record update during reconciliation.
		googleEmail := ""
		if e, ok := emailByResolvedUser[username]; ok {
			googleEmail = e
		}
		actions = append(actions, models.SyncAction{
			Type:        models.ActionUpdateRole,
			Email:       *member.Username,
			GoogleEmail: googleEmail,
			CurrentRole: &current,
			TargetRole:  &target,
			Reason:      "role mismatch",
		})
	}

	return actions
}
