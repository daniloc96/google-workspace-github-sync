package google

import (
	"context"
	"errors"
	"testing"

	admin "google.golang.org/api/admin/directory/v1"
)

type fakeMemberLister struct {
	pages []*admin.Members
	call  int
	errOn int
}

func (f *fakeMemberLister) ListMembers(ctx context.Context, groupEmail string, pageToken string) ([]*admin.Member, string, error) {
	if f.errOn > 0 && f.call+1 == f.errOn {
		return nil, "", errors.New("boom")
	}
	if f.call >= len(f.pages) {
		return nil, "", nil
	}
	page := f.pages[f.call]
	f.call++
	return page.Members, page.NextPageToken, nil
}

type fakeUserGetter struct{}

func (f *fakeUserGetter) GetUser(ctx context.Context, email string) (*admin.User, error) {
	return &admin.User{Suspended: false}, nil
}

type fakeSuspendedUserGetter struct {
	suspended map[string]bool
}

func (f *fakeSuspendedUserGetter) GetUser(ctx context.Context, email string) (*admin.User, error) {
	return &admin.User{Suspended: f.suspended[email]}, nil
}

func TestGetGroupMembersFiltersUsers(t *testing.T) {
	lister := &fakeMemberLister{
		pages: []*admin.Members{
			{
				Members: []*admin.Member{
					{Email: "user1@example.com", Type: "USER", Status: "ACTIVE", Role: "MEMBER"},
					{Email: "group@example.com", Type: "GROUP", Status: "ACTIVE", Role: "MEMBER"},
				},
				NextPageToken: "next",
			},
			{
				Members: []*admin.Member{
					{Email: "user2@example.com", Type: "USER", Status: "ACTIVE", Role: "MEMBER"},
				},
			},
		},
	}

	client := &Client{memberLister: lister, userGetter: &fakeUserGetter{}}
	members, err := client.GetGroupMembers(context.Background(), "group@example.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 user members, got %d", len(members))
	}
	if members[0].Email != "user1@example.com" || members[1].Email != "user2@example.com" {
		t.Fatalf("unexpected members: %#v", members)
	}
}

func TestGetGroupMembersReturnsError(t *testing.T) {
	lister := &fakeMemberLister{
		pages: []*admin.Members{{Members: []*admin.Member{{Email: "user1@example.com", Type: "USER"}}}},
		errOn: 1,
	}

	client := &Client{memberLister: lister, userGetter: &fakeUserGetter{}}
	_, err := client.GetGroupMembers(context.Background(), "group@example.com")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestGetGroupMembersOwnersGroup(t *testing.T) {
	lister := &fakeMemberLister{
		pages: []*admin.Members{{Members: []*admin.Member{{Email: "owner@example.com", Type: "USER", Status: "ACTIVE", Role: "OWNER"}}}},
	}

	client := &Client{memberLister: lister, userGetter: &fakeUserGetter{}}
	members, err := client.GetGroupMembers(context.Background(), "owners@example.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	if members[0].Role != "OWNER" {
		t.Fatalf("expected role OWNER, got %s", members[0].Role)
	}
}

func TestGetUsersSuspendedStatus(t *testing.T) {
	client := &Client{userGetter: &fakeSuspendedUserGetter{suspended: map[string]bool{"a@example.com": true}}}
	statuses, err := client.GetUsersSuspendedStatus(context.Background(), []string{"a@example.com", "b@example.com"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !statuses["a@example.com"] {
		t.Fatalf("expected a@example.com to be suspended")
	}
	if statuses["b@example.com"] {
		t.Fatalf("expected b@example.com to be active")
	}
}
