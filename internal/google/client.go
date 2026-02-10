package google

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2/google"
	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"github.com/daniloc96/google-workspace-github-sync/internal/models"
)

const (
	membersScope = "https://www.googleapis.com/auth/admin.directory.group.member.readonly"
	groupsScope  = "https://www.googleapis.com/auth/admin.directory.group.readonly"
	usersScope   = "https://www.googleapis.com/auth/admin.directory.user.readonly"
)

type memberLister interface {
	ListMembers(ctx context.Context, groupEmail string, pageToken string) ([]*admin.Member, string, error)
}

type userGetter interface {
	GetUser(ctx context.Context, email string) (*admin.User, error)
}

// Client implements Google group member operations.
type Client struct {
	memberLister memberLister
	userGetter   userGetter
}

// NewClient creates a Google Admin SDK client using domain-wide delegation.
func NewClient(ctx context.Context, credentialsJSON []byte, adminEmail string) (*Client, error) {
	if len(credentialsJSON) == 0 {
		return nil, fmt.Errorf("credentials JSON is required")
	}
	if adminEmail == "" {
		return nil, fmt.Errorf("admin email is required")
	}

	config, err := google.JWTConfigFromJSON(credentialsJSON, membersScope, groupsScope, usersScope)
	if err != nil {
		return nil, err
	}
	config.Subject = adminEmail

	svc, err := admin.NewService(ctx, option.WithTokenSource(config.TokenSource(ctx)))
	if err != nil {
		return nil, err
	}

	directory := &directoryService{svc: svc}
	return &Client{memberLister: directory, userGetter: directory}, nil
}

// GetGroupMembers returns group members filtered to user accounts.
func (c *Client) GetGroupMembers(ctx context.Context, groupEmail string) ([]models.GoogleGroupMember, error) {
	if groupEmail == "" {
		return nil, fmt.Errorf("group email is required")
	}

	var members []models.GoogleGroupMember
	pageToken := ""
	for {
		var (
			items     []*admin.Member
			nextToken string
			err       error
		)
		err = retryOnGoogleError(ctx, func() error {
			items, nextToken, err = c.memberLister.ListMembers(ctx, groupEmail, pageToken)
			return err
		})
		if err != nil {
			return nil, err
		}
		for _, member := range items {
			if member.Type != "USER" {
				continue
			}
			members = append(members, models.GoogleGroupMember{
				Email:  member.Email,
				Role:   member.Role,
				Type:   member.Type,
				Status: member.Status,
			})
		}
		if nextToken == "" {
			break
		}
		pageToken = nextToken
	}

	return members, nil
}

// GetUsersSuspendedStatus returns suspension status for given emails.
func (c *Client) GetUsersSuspendedStatus(ctx context.Context, emails []string) (map[string]bool, error) {
	result := make(map[string]bool, len(emails))
	for _, email := range emails {
		var user *admin.User
		var err error
		err = retryOnGoogleError(ctx, func() error {
			user, err = c.userGetter.GetUser(ctx, email)
			return err
		})
		if err != nil {
			return nil, err
		}
		result[email] = user.Suspended
	}
	return result, nil
}

func retryOnGoogleError(ctx context.Context, fn func() error) error {
	const maxRetries = 3
	backoff := 200 * time.Millisecond
	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		if !isRetryableGoogleError(err) || attempt == maxRetries {
			return err
		}
		if backoff > 2*time.Second {
			backoff = 2 * time.Second
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		backoff *= 2
	}
	return nil
}

func isRetryableGoogleError(err error) bool {
	apiErr, ok := err.(*googleapi.Error)
	if !ok {
		return false
	}
	return apiErr.Code == 429 || apiErr.Code == 503
}

type directoryService struct {
	svc *admin.Service
}

func (d *directoryService) ListMembers(ctx context.Context, groupEmail string, pageToken string) ([]*admin.Member, string, error) {
	call := d.svc.Members.List(groupEmail).IncludeDerivedMembership(true)
	if pageToken != "" {
		call = call.PageToken(pageToken)
	}
	resp, err := call.Context(ctx).Do()
	if err != nil {
		return nil, "", err
	}
	return resp.Members, resp.NextPageToken, nil
}

func (d *directoryService) GetUser(ctx context.Context, email string) (*admin.User, error) {
	return d.svc.Users.Get(email).Context(ctx).Do()
}
