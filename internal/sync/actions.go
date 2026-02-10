package sync

import (
	"context"
	"time"

	"github.com/daniloc96/google-workspace-github-sync/internal/interfaces"
	"github.com/daniloc96/google-workspace-github-sync/internal/models"
	ghclient "github.com/daniloc96/google-workspace-github-sync/internal/github"
	"github.com/sirupsen/logrus"
)

// ExecuteActions executes sync actions unless dry-run is enabled.
func ExecuteActions(ctx context.Context, client interfaces.GitHubClient, org string, actions []models.SyncAction, dryRun bool) ([]models.SyncAction, error) {
	for i := range actions {
		action := &actions[i]
		if dryRun {
			action.Executed = false
			continue
		}

		switch action.Type {
		case models.ActionInvite:
			if action.TargetRole == nil {
				errMsg := "target role is required"
				action.Error = &errMsg
				continue
			}
			invResult, err := client.CreateInvitation(ctx, org, action.Email, *action.TargetRole)
			if err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"email":       action.Email,
					"target_role": *action.TargetRole,
					"is_already":  ghclient.IsAlreadyMemberError(err),
				}).Info("invite failed — checking error type")
				if ghclient.IsAlreadyMemberError(err) {
					action.AlreadyInOrg = true
					// User is already in the org — try to update their role if we have a target role.
					if action.TargetRole != nil {
						logrus.WithFields(logrus.Fields{
							"email":       action.Email,
							"target_role": *action.TargetRole,
						}).Info("🔍 searching GitHub username by email for role update")
						username, searchErr := client.SearchUserByEmail(ctx, action.Email)
						if searchErr != nil {
							logrus.WithError(searchErr).WithField("email", action.Email).Warn("failed to search user by email for role update")
						}
						logrus.WithFields(logrus.Fields{
							"email":    action.Email,
							"username": username,
						}).Info("🔍 search result")
						if username != "" {
							roleErr := client.UpdateMemberRole(ctx, org, username, *action.TargetRole)
							if roleErr != nil {
								logrus.WithError(roleErr).WithFields(logrus.Fields{
									"email":    action.Email,
									"username": username,
									"role":     *action.TargetRole,
								}).Warn("failed to update role for already-in-org user")
								errMsg := roleErr.Error()
								action.Error = &errMsg
								continue
							}
							// Successfully upgraded invite → role update.
							logrus.WithFields(logrus.Fields{
								"email":    action.Email,
								"username": username,
								"role":     *action.TargetRole,
							}).Info("🔄 invite upgraded to role update (user already in org)")
							action.Type = models.ActionUpdateRole
							action.Executed = true
							action.AlreadyInOrg = true
							action.Username = username
							action.GoogleEmail = action.Email
							action.Reason = "invite upgraded: user already in org, role updated"
							t := time.Now()
							action.Timestamp = &t
							continue
						}
					}
					// Could not find username — fall through to mark as already-in-org without role update.
					errMsg := err.Error()
					action.Error = &errMsg
					action.Executed = false
					continue
				}
				errMsg := err.Error()
				action.Error = &errMsg
				continue
			}
			action.Executed = true
			if invResult != nil {
				action.InvitationID = invResult.InvitationID
			}
			t := time.Now()
			action.Timestamp = &t
		case models.ActionRemove:
			err := client.RemoveMember(ctx, org, action.Email)
			if err != nil {
				errMsg := err.Error()
				action.Error = &errMsg
				continue
			}
			action.Executed = true
			t := time.Now()
			action.Timestamp = &t
		case models.ActionUpdateRole:
			if action.TargetRole == nil {
				errMsg := "target role is required"
				action.Error = &errMsg
				continue
			}
			logrus.WithFields(logrus.Fields{
				"email":       action.Email,
				"target_role": *action.TargetRole,
			}).Info("executing role update")
			err := client.UpdateMemberRole(ctx, org, action.Email, *action.TargetRole)
			if err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"email":       action.Email,
					"target_role": *action.TargetRole,
				}).Warn("role update failed")
				errMsg := err.Error()
				action.Error = &errMsg
				continue
			}
			action.Executed = true
			t := time.Now()
			action.Timestamp = &t
		case models.ActionCancelInvite:
			if action.InvitationID == nil {
				errMsg := "invitation ID is required for cancel"
				action.Error = &errMsg
				continue
			}
			err := client.CancelInvitation(ctx, org, *action.InvitationID)
			if err != nil {
				errMsg := err.Error()
				action.Error = &errMsg
				continue
			}
			action.Executed = true
			t := time.Now()
			action.Timestamp = &t
		default:
			continue
		}
	}

	return actions, nil
}
