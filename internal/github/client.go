package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/v60/github"
	"github.com/daniloc96/google-workspace-github-sync/internal/models"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

// ErrAlreadyMember is returned when an invitation fails because the user is already an org member.
type ErrAlreadyMember struct {
	Email string
}

func (e *ErrAlreadyMember) Error() string {
	return fmt.Sprintf("user %s is already a member of the organization", e.Email)
}

// IsAlreadyMemberError checks whether an error indicates the user is already in the org.
func IsAlreadyMemberError(err error) bool {
	if _, ok := err.(*ErrAlreadyMember); ok {
		return true
	}
	return false
}

type orgService interface {
	ListMembers(ctx context.Context, org string, opts *github.ListMembersOptions) ([]*github.User, *github.Response, error)
	ListPendingOrgInvitations(ctx context.Context, org string, opts *github.ListOptions) ([]*github.Invitation, *github.Response, error)
	CreateOrgInvitation(ctx context.Context, org string, opts *github.CreateOrgInvitationOptions) (*github.Invitation, *github.Response, error)
	RemoveMember(ctx context.Context, org, user string) (*github.Response, error)
	EditOrgMembership(ctx context.Context, user, org string, membership *github.Membership) (*github.Membership, *github.Response, error)
}

// Client implements GitHub organization operations.
type Client struct {
	orgService orgService
	httpClient *http.Client
	token      string
}

// NewClient creates a GitHub client using a personal access token.
func NewClient(token string) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("github token is required")
	}
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(ctx, ts)
	client := github.NewClient(httpClient)
	return &Client{orgService: client.Organizations, httpClient: httpClient, token: token}, nil
}

// ListMembers lists current organization members with accurate roles.
// It fetches admins first to build a set, then fetches all members and tags admins.
func (c *Client) ListMembers(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
	if org == "" {
		return nil, fmt.Errorf("org is required")
	}

	// Step 1: Fetch admin users to build an admin set.
	adminSet := make(map[string]struct{})
	adminOpts := &github.ListMembersOptions{Role: "admin", ListOptions: github.ListOptions{PerPage: 100}}
	for {
		var (
			users []*github.User
			resp  *github.Response
			err   error
		)
		err = retryOnRateLimit(ctx, func() error {
			users, resp, err = c.orgService.ListMembers(ctx, org, adminOpts)
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("listing admin members: %w", err)
		}
		for _, user := range users {
			adminSet[user.GetLogin()] = struct{}{}
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		adminOpts.Page = resp.NextPage
	}

	// Step 2: Fetch all members and tag admins.
	allOpts := &github.ListMembersOptions{Role: "all", ListOptions: github.ListOptions{PerPage: 100}}
	var result []models.GitHubOrgMember
	for {
		var (
			users []*github.User
			resp  *github.Response
			err   error
		)
		err = retryOnRateLimit(ctx, func() error {
			users, resp, err = c.orgService.ListMembers(ctx, org, allOpts)
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("listing all members: %w", err)
		}
		for _, user := range users {
			login := user.GetLogin()
			email := user.GetEmail()
			var emailPtr *string
			if email != "" {
				emailPtr = &email
			}
			role := models.RoleMember
			if _, isAdmin := adminSet[login]; isAdmin {
				role = models.RoleOwner
			}
			result = append(result, models.GitHubOrgMember{
				Username: &login,
				Email:    emailPtr,
				Role:     role,
			})
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		allOpts.Page = resp.NextPage
	}

	// Step 3: Enrich members with public profile email where missing.
	// The List Members API doesn't return user emails; fetch individual profiles.
	for i := range result {
		if result[i].Email == nil && result[i].Username != nil {
			if email := c.getUserPublicEmail(ctx, *result[i].Username); email != "" {
				e := email
				result[i].Email = &e
			}
		}
	}

	return result, nil
}

// getUserPublicEmail fetches a user's public profile email via GET /users/{login}.
// Returns empty string if the email is not public or the request fails.
func (c *Client) getUserPublicEmail(ctx context.Context, login string) string {
	if c.httpClient == nil {
		return ""
	}
	url := fmt.Sprintf("https://api.github.com/users/%s", login)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var profile struct {
		Email *string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return ""
	}
	if profile.Email != nil {
		return *profile.Email
	}
	return ""
}

// SearchUserByEmail searches GitHub for a user by email address.
// Uses the Search Users API: GET /search/users?q={email}+in:email
// Returns the login (username) if exactly one match is found, empty string otherwise.
func (c *Client) SearchUserByEmail(ctx context.Context, email string) (string, error) {
	if c.httpClient == nil || email == "" {
		return "", nil
	}
	url := fmt.Sprintf("https://api.github.com/search/users?q=%s+in:email", email)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("creating search request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("searching user by email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("search users API returned status %d: %s", resp.StatusCode, string(body))
	}

	var searchResult struct {
		TotalCount int `json:"total_count"`
		Items      []struct {
			Login string `json:"login"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
		return "", fmt.Errorf("decoding search response: %w", err)
	}

	if searchResult.TotalCount == 1 && len(searchResult.Items) == 1 {
		return searchResult.Items[0].Login, nil
	}

	if searchResult.TotalCount > 1 {
		logrus.WithFields(logrus.Fields{
			"email":   email,
			"matches": searchResult.TotalCount,
		}).Warn("multiple GitHub users found for email, skipping")
	}

	return "", nil
}

// ListPendingInvitations lists pending organization invitations.
func (c *Client) ListPendingInvitations(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
	if org == "" {
		return nil, fmt.Errorf("org is required")
	}
	opts := &github.ListOptions{PerPage: 100}
	var result []models.GitHubOrgMember

	for {
		var (
			invites []*github.Invitation
			resp    *github.Response
			err     error
		)
		err = retryOnRateLimit(ctx, func() error {
			invites, resp, err = c.orgService.ListPendingOrgInvitations(ctx, org, opts)
			return err
		})
		if err != nil {
			return nil, err
		}
		for _, invite := range invites {
			role := models.RoleMember
			if invite.GetRole() == "admin" {
				role = models.RoleOwner
			}
			member := models.GitHubOrgMember{
				Role:         role,
				IsPending:    true,
				InvitationID: invite.ID,
			}
			if email := invite.GetEmail(); email != "" {
				member.Email = &email
			}
			if login := invite.GetLogin(); login != "" {
				member.Username = &login
			}
			result = append(result, member)
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return result, nil
}

// CreateInvitation sends an organization invitation.
func (c *Client) CreateInvitation(ctx context.Context, org string, email string, role models.OrgRole) (*models.GitHubOrgMember, error) {
	if org == "" || email == "" {
		return nil, fmt.Errorf("org and email are required")
	}
	roleValue := "direct_member"
	if role == models.RoleOwner {
		roleValue = "admin"
	}

	var invitation *github.Invitation
	var err error
	err = retryOnRateLimit(ctx, func() error {
		invitation, _, err = c.orgService.CreateOrgInvitation(ctx, org, &github.CreateOrgInvitationOptions{
			Email: github.String(email),
			Role:  github.String(roleValue),
		})
		return err
	})
	if err != nil {
		// Detect "already a member" errors (HTTP 422 from GitHub API)
		if isAlreadyMemberAPIError(err) {
			return nil, &ErrAlreadyMember{Email: email}
		}
		logGitHubError(err, logrus.Fields{
			"operation": "create_invitation",
			"org":       org,
			"email":     email,
			"role":      roleValue,
		})
		return nil, err
	}

	return &models.GitHubOrgMember{
		Email:        github.String(email),
		Role:         role,
		IsPending:    true,
		InvitationID: invitation.ID,
	}, nil
}

func logGitHubError(err error, fields logrus.Fields) {
	if err == nil {
		return
	}
	if respErr, ok := err.(*github.ErrorResponse); ok {
		fields["status_code"] = respErr.Response.StatusCode
		fields["status"] = respErr.Response.Status
		fields["message"] = respErr.Message
		if len(respErr.Errors) > 0 {
			fields["errors"] = respErr.Errors
		}
		logrus.WithFields(fields).Debug("GitHub API error")
		return
	}
	logrus.WithFields(fields).WithError(err).Debug("GitHub API error")
}

// isAlreadyMemberAPIError checks if a GitHub API error indicates the user is already an org member.
// GitHub returns HTTP 422 with a message like "Validation Failed" and an error
// containing "already a member" or "is already a part of this organization".
func isAlreadyMemberAPIError(err error) bool {
	if respErr, ok := err.(*github.ErrorResponse); ok {
		if respErr.Response != nil && respErr.Response.StatusCode == 422 {
			msg := strings.ToLower(respErr.Message)
			if strings.Contains(msg, "already") {
				return true
			}
			for _, e := range respErr.Errors {
				errMsg := strings.ToLower(e.Message)
				if strings.Contains(errMsg, "already") {
					return true
				}
			}
		}
	}
	return false
}

// RemoveMember removes a member from the organization.
func (c *Client) RemoveMember(ctx context.Context, org string, username string) error {
	if org == "" || username == "" {
		return fmt.Errorf("org and username are required")
	}
	return retryOnRateLimit(ctx, func() error {
		_, err := c.orgService.RemoveMember(ctx, org, username)
		return err
	})
}

// UpdateMemberRole updates a member's role in the organization.
func (c *Client) UpdateMemberRole(ctx context.Context, org string, username string, role models.OrgRole) error {
	if org == "" || username == "" {
		return fmt.Errorf("org and username are required")
	}
	roleValue := "member"
	if role == models.RoleOwner {
		roleValue = "admin"
	}
	return retryOnRateLimit(ctx, func() error {
		_, _, err := c.orgService.EditOrgMembership(ctx, username, org, &github.Membership{Role: github.String(roleValue)})
		return err
	})
}

// CancelInvitation cancels a pending organization invitation.
// Uses DELETE /orgs/{org}/invitations/{invitation_id} (not available in go-github v60).
func (c *Client) CancelInvitation(ctx context.Context, org string, invitationID int64) error {
	if org == "" {
		return fmt.Errorf("org is required")
	}
	url := fmt.Sprintf("https://api.github.com/orgs/%s/invitations/%d", org, invitationID)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("creating cancel invitation request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("cancelling invitation: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("cancel invitation API returned status %d", resp.StatusCode)
	}

	logrus.WithFields(logrus.Fields{
		"org":           org,
		"invitation_id": invitationID,
	}).Info("🚫 Cancelled pending invitation")

	return nil
}

func retryOnRateLimit(ctx context.Context, fn func() error) error {
	const maxRetries = 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		if wait, ok := rateLimitWait(err); ok {
			if attempt == maxRetries {
				return err
			}
			if wait > 100*time.Millisecond {
				wait = 100 * time.Millisecond
			}
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		return err
	}
	return nil
}

func rateLimitWait(err error) (time.Duration, bool) {
	if rateErr, ok := err.(*github.RateLimitError); ok {
		wait := time.Until(rateErr.Rate.Reset.Time)
		if wait < 0 {
			return 0, true
		}
		return wait, true
	}
	if abuseErr, ok := err.(*github.AbuseRateLimitError); ok {
		wait := abuseErr.GetRetryAfter()
		return wait, true
	}
	return 0, false
}

// auditLogRawEntry represents a raw entry from the GitHub Audit Log API.
type auditLogRawEntry struct {
	Timestamp int64  `json:"@timestamp"`
	Action    string `json:"action"`
	Actor     string `json:"actor"`
	User      string `json:"user"`
	Org       string `json:"org"`
	Data      *struct {
		InvitationID *int64 `json:"invitation_id,omitempty"`
	} `json:"data,omitempty"`
	// invitation_id may also be at top level depending on API version
	InvitationID *int64 `json:"invitation_id,omitempty"`
}

// GetAuditLogAddMemberEvents fetches org.add_member events from the audit log.
// Requires Enterprise Cloud + admin:org scope.
func (c *Client) GetAuditLogAddMemberEvents(ctx context.Context, org string, afterTimestamp int64) ([]models.AuditLogEntry, error) {
	if org == "" {
		return nil, fmt.Errorf("org is required")
	}

	var entries []models.AuditLogEntry
	url := fmt.Sprintf("https://api.github.com/orgs/%s/audit-log?phrase=action:org.add_member&per_page=100&order=asc", org)

	for url != "" {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("creating audit log request: %w", err)
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching audit log: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading audit log response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			logrus.WithFields(logrus.Fields{
				"status_code": resp.StatusCode,
				"body":        string(body),
			}).Debug("audit log API error")
			return nil, fmt.Errorf("audit log API returned status %d", resp.StatusCode)
		}

		var rawEntries []auditLogRawEntry
		if err := json.Unmarshal(body, &rawEntries); err != nil {
			return nil, fmt.Errorf("parsing audit log response: %w", err)
		}

		for _, raw := range rawEntries {
			if raw.Timestamp <= afterTimestamp {
				continue
			}

			entry := models.AuditLogEntry{
				Timestamp: raw.Timestamp,
				Action:    raw.Action,
				Actor:     raw.Actor,
				User:      raw.User,
				Org:       raw.Org,
			}

			// invitation_id can be nested under "data" or at top level
			if raw.Data != nil && raw.Data.InvitationID != nil {
				entry.InvitationID = *raw.Data.InvitationID
			} else if raw.InvitationID != nil {
				entry.InvitationID = *raw.InvitationID
			}

			entries = append(entries, entry)
		}

		// Parse Link header for pagination
		url = parseLinkNext(resp.Header.Get("Link"))
	}

	return entries, nil
}

// ListFailedInvitations fetches failed invitations for an org.
func (c *Client) ListFailedInvitations(ctx context.Context, org string) ([]models.GitHubOrgMember, error) {
	if org == "" {
		return nil, fmt.Errorf("org is required")
	}

	var result []models.GitHubOrgMember
	url := fmt.Sprintf("https://api.github.com/orgs/%s/failed_invitations?per_page=100", org)

	for url != "" {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("creating failed invitations request: %w", err)
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching failed invitations: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading failed invitations response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed invitations API returned status %d", resp.StatusCode)
		}

		var invitations []struct {
			ID    int64  `json:"id"`
			Email string `json:"email"`
			Login string `json:"login"`
		}
		if err := json.Unmarshal(body, &invitations); err != nil {
			return nil, fmt.Errorf("parsing failed invitations: %w", err)
		}

		for _, inv := range invitations {
			email := inv.Email
			id := inv.ID
			result = append(result, models.GitHubOrgMember{
				Email:        &email,
				InvitationID: &id,
			})
		}

		url = parseLinkNext(resp.Header.Get("Link"))
	}

	return result, nil
}

// parseLinkNext extracts the "next" URL from a GitHub Link header.
func parseLinkNext(linkHeader string) string {
	if linkHeader == "" {
		return ""
	}
	for _, part := range strings.Split(linkHeader, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, `rel="next"`) {
			start := strings.Index(part, "<")
			end := strings.Index(part, ">")
			if start >= 0 && end > start {
				return part[start+1 : end]
			}
		}
	}
	return ""
}

// ListMembersWithVerifiedEmails fetches all org members with their verified domain emails
// using the GitHub GraphQL API. Returns a map of lowercase email → GitHub username.
// Requires Enterprise Cloud and a verified domain on the organization.
func (c *Client) ListMembersWithVerifiedEmails(ctx context.Context, org string) (map[string]string, error) {
	if org == "" {
		return nil, fmt.Errorf("org is required")
	}

	const query = `query($org: String!, $cursor: String) {
		organization(login: $org) {
			membersWithRole(first: 100, after: $cursor) {
				pageInfo {
					hasNextPage
					endCursor
				}
				nodes {
					login
					organizationVerifiedDomainEmails(login: $org)
				}
			}
		}
	}`

	type graphQLRequest struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}

	type graphQLResponse struct {
		Data struct {
			Organization struct {
				MembersWithRole struct {
					PageInfo struct {
						HasNextPage bool    `json:"hasNextPage"`
						EndCursor   *string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []struct {
						Login                            string   `json:"login"`
						OrganizationVerifiedDomainEmails []string `json:"organizationVerifiedDomainEmails"`
					} `json:"nodes"`
				} `json:"membersWithRole"`
			} `json:"organization"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}

	result := make(map[string]string)
	var cursor *string

	for {
		variables := map[string]interface{}{"org": org}
		if cursor != nil {
			variables["cursor"] = *cursor
		}

		reqBody := graphQLRequest{Query: query, Variables: variables}
		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("marshaling GraphQL request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", "https://api.github.com/graphql", strings.NewReader(string(bodyBytes)))
		if err != nil {
			return nil, fmt.Errorf("creating GraphQL request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("executing GraphQL request: %w", err)
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading GraphQL response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GraphQL API returned status %d: %s", resp.StatusCode, string(respBody))
		}

		var gqlResp graphQLResponse
		if err := json.Unmarshal(respBody, &gqlResp); err != nil {
			return nil, fmt.Errorf("parsing GraphQL response: %w", err)
		}

		if len(gqlResp.Errors) > 0 {
			return nil, fmt.Errorf("GraphQL errors: %s", gqlResp.Errors[0].Message)
		}

		members := gqlResp.Data.Organization.MembersWithRole
		for _, node := range members.Nodes {
			if node.Login == "" {
				continue
			}
			for _, email := range node.OrganizationVerifiedDomainEmails {
				if email != "" {
					result[strings.ToLower(email)] = node.Login
				}
			}
		}

		logrus.WithFields(logrus.Fields{
			"page_members":  len(members.Nodes),
			"total_mapped":  len(result),
			"has_next_page": members.PageInfo.HasNextPage,
		}).Debug("fetched verified domain emails page")

		if !members.PageInfo.HasNextPage || members.PageInfo.EndCursor == nil {
			break
		}
		cursor = members.PageInfo.EndCursor
	}

	logrus.WithFields(logrus.Fields{
		"org":            org,
		"total_mappings": len(result),
	}).Info("📧 Loaded verified domain email mappings via GraphQL")

	return result, nil
}
