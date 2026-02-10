package github

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-github/v60/github"
	"github.com/daniloc96/google-workspace-github-sync/internal/models"
)

type fakeOrgService struct {
	memberPages      [][]*github.User
	adminMemberPages [][]*github.User // Pages returned when Role="admin"
	invitationPages  [][]*github.Invitation
	memberCalls      int
	adminCalls       int
	invitationCalls  int
	memberErr        error
	memberErrs       []error
	lastInvitation   *github.CreateOrgInvitationOptions
	removedUsers     []string
	lastMembership   *github.Membership
}

func (f *fakeOrgService) ListMembers(ctx context.Context, org string, opts *github.ListMembersOptions) ([]*github.User, *github.Response, error) {
	if len(f.memberErrs) > 0 {
		err := f.memberErrs[0]
		f.memberErrs = f.memberErrs[1:]
		return nil, nil, err
	}
	if f.memberErr != nil {
		return nil, nil, f.memberErr
	}

	// Route to admin pages or regular pages based on Role filter.
	if opts != nil && opts.Role == "admin" {
		if f.adminCalls >= len(f.adminMemberPages) {
			return nil, &github.Response{}, nil
		}
		page := f.adminMemberPages[f.adminCalls]
		f.adminCalls++
		resp := &github.Response{NextPage: f.adminCalls + 1}
		if f.adminCalls >= len(f.adminMemberPages) {
			resp.NextPage = 0
		}
		return page, resp, nil
	}

	if f.memberCalls >= len(f.memberPages) {
		return nil, &github.Response{}, nil
	}
	page := f.memberPages[f.memberCalls]
	f.memberCalls++
	resp := &github.Response{NextPage: f.memberCalls + 1}
	if f.memberCalls >= len(f.memberPages) {
		resp.NextPage = 0
	}
	return page, resp, nil
}

func (f *fakeOrgService) ListPendingOrgInvitations(ctx context.Context, org string, opts *github.ListOptions) ([]*github.Invitation, *github.Response, error) {
	if f.invitationCalls >= len(f.invitationPages) {
		return nil, &github.Response{}, nil
	}
	page := f.invitationPages[f.invitationCalls]
	f.invitationCalls++
	resp := &github.Response{NextPage: f.invitationCalls + 1}
	if f.invitationCalls >= len(f.invitationPages) {
		resp.NextPage = 0
	}
	return page, resp, nil
}

func (f *fakeOrgService) CreateOrgInvitation(ctx context.Context, org string, opts *github.CreateOrgInvitationOptions) (*github.Invitation, *github.Response, error) {
	f.lastInvitation = opts
	return &github.Invitation{}, &github.Response{}, nil
}

func (f *fakeOrgService) RemoveMember(ctx context.Context, org, user string) (*github.Response, error) {
	f.removedUsers = append(f.removedUsers, user)
	return &github.Response{}, nil
}

func (f *fakeOrgService) EditOrgMembership(ctx context.Context, user, org string, membership *github.Membership) (*github.Membership, *github.Response, error) {
	f.lastMembership = membership
	return membership, &github.Response{}, nil
}

func TestListMembersPagination(t *testing.T) {
	service := &fakeOrgService{
		adminMemberPages: [][]*github.User{}, // no admins
		memberPages: [][]*github.User{
			{{Login: github.String("user1"), Email: github.String("user1@example.com")}},
			{{Login: github.String("user2")}},
		},
	}

	client := &Client{orgService: service}
	members, err := client.ListMembers(context.Background(), "example-org")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
	if members[0].Email == nil || *members[0].Email != "user1@example.com" {
		t.Fatalf("expected email for first member, got %#v", members[0].Email)
	}
}

func TestListMembersDetectsAdminRole(t *testing.T) {
	service := &fakeOrgService{
		adminMemberPages: [][]*github.User{
			{{Login: github.String("admin-user")}},
		},
		memberPages: [][]*github.User{
			{
				{Login: github.String("admin-user")},
				{Login: github.String("regular-user")},
			},
		},
	}

	client := &Client{orgService: service}
	members, err := client.ListMembers(context.Background(), "example-org")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}

	adminFound := false
	memberFound := false
	for _, m := range members {
		if *m.Username == "admin-user" && m.Role == models.RoleOwner {
			adminFound = true
		}
		if *m.Username == "regular-user" && m.Role == models.RoleMember {
			memberFound = true
		}
	}
	if !adminFound {
		t.Fatalf("expected admin-user to have role admin/owner")
	}
	if !memberFound {
		t.Fatalf("expected regular-user to have role member")
	}
}

func TestListPendingInvitationsPagination(t *testing.T) {
	service := &fakeOrgService{
		invitationPages: [][]*github.Invitation{
			{{ID: github.Int64(1), Email: github.String("a@example.com")}},
			{{ID: github.Int64(2), Email: github.String("b@example.com")}},
		},
	}

	client := &Client{orgService: service}
	invites, err := client.ListPendingInvitations(context.Background(), "example-org")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(invites) != 2 {
		t.Fatalf("expected 2 invitations, got %d", len(invites))
	}
}

func TestListMembersError(t *testing.T) {
	service := &fakeOrgService{memberErr: errors.New("boom")}
	client := &Client{orgService: service}
	_, err := client.ListMembers(context.Background(), "example-org")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestCreateInvitationAdminRole(t *testing.T) {
	service := &fakeOrgService{}
	client := &Client{orgService: service}
	_, err := client.CreateInvitation(context.Background(), "example-org", "owner@example.com", models.RoleOwner)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if service.lastInvitation == nil || service.lastInvitation.Role == nil {
		t.Fatalf("expected invitation options to be captured")
	}
	if *service.lastInvitation.Role != "admin" {
		t.Fatalf("expected role admin, got %s", *service.lastInvitation.Role)
	}
}

func TestRemoveMember(t *testing.T) {
	service := &fakeOrgService{}
	client := &Client{orgService: service}
	err := client.RemoveMember(context.Background(), "example-org", "user1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(service.removedUsers) != 1 || service.removedUsers[0] != "user1" {
		t.Fatalf("expected user1 to be removed, got %#v", service.removedUsers)
	}
}

func TestUpdateMemberRole(t *testing.T) {
	service := &fakeOrgService{}
	client := &Client{orgService: service}
	err := client.UpdateMemberRole(context.Background(), "example-org", "user1", models.RoleOwner)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if service.lastMembership == nil || service.lastMembership.Role == nil {
		t.Fatalf("expected membership update to be captured")
	}
	if *service.lastMembership.Role != "admin" {
		t.Fatalf("expected role admin, got %s", *service.lastMembership.Role)
	}
}

func TestListMembersRetriesOnRateLimit(t *testing.T) {
	rateErr := &github.RateLimitError{Rate: github.Rate{Reset: github.Timestamp{Time: time.Now()}}}
	service := &fakeOrgService{
		memberErrs:       []error{rateErr},                                              // first call (admin) hits rate limit, retry succeeds
		adminMemberPages: [][]*github.User{},                                             // no admins
		memberPages:      [][]*github.User{{{Login: github.String("user1")}}}, // 1 regular member
	}

	client := &Client{orgService: service}
	members, err := client.ListMembers(context.Background(), "example-org")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected 1 member after retry, got %d", len(members))
	}
}
