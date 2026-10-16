package identity_test

import (
	"context"
	"testing"
)

// An operator looks for an account by what they already know: the login name or
// the name shown in the console. Without this the only way to reach a user is to
// page through every account in creation order.
func TestListUserViewsSearchesUsernameAndDisplayName(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	for _, user := range []struct{ username, displayName string }{
		{"ana.ops", "Ana Ruiz"},
		{"ben.ops", "Ben Okafor"},
		{"carla.ops", "Carla Nunez"},
	} {
		if _, err := s.CreateUserView(ctx, user.username, user.displayName, "MEMBER"); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name       string
		search     string
		want       int
		wantEither string
	}{
		{"login name, case-insensitive", "BEN.OPS", 1, "ben.ops"},
		{"display name, partial and case-insensitive", "nunez", 1, "Carla Nunez"},
		{"fragment shared by every account", "ops", 3, ""},
		{"no match is an empty page, not an error", "nobody-here", 0, ""},
		{"blank search does not filter", "   ", 3, ""},
	}
	for _, tc := range cases {
		got, err := s.ListUserViews(ctx, tc.search, 50, "")
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(got) != tc.want {
			t.Fatalf("%s: got %d users, want %d", tc.name, len(got), tc.want)
		}
		if tc.wantEither != "" {
			if got[0].Username != tc.wantEither && got[0].DisplayName != tc.wantEither {
				t.Fatalf("%s: matched %q / %q, want %q", tc.name, got[0].Username, got[0].DisplayName, tc.wantEither)
			}
		}
	}
}

// The search narrows a paged list, it does not replace paging: continuing from a
// cursor must keep returning matches and must stop cleanly.
func TestListUserViewsSearchComposesWithPaging(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	for _, user := range []struct{ username, displayName string }{
		{"ana.ops", "Ana Ruiz"},
		{"ben.ops", "Ben Okafor"},
		{"carla.ops", "Carla Nunez"},
		{"dana.support", "Dana Silva"},
	} {
		if _, err := s.CreateUserView(ctx, user.username, user.displayName, "MEMBER"); err != nil {
			t.Fatal(err)
		}
	}

	first, err := s.ListUserViews(ctx, "ops", 2, "")
	if err != nil || len(first) != 2 {
		t.Fatalf("first page=%d err=%v", len(first), err)
	}
	second, err := s.ListUserViews(ctx, "ops", 2, first[len(first)-1].ID)
	if err != nil || len(second) != 1 {
		t.Fatalf("second page=%d err=%v", len(second), err)
	}
	for _, view := range append(first, second...) {
		if view.Username != "ana.ops" && view.Username != "ben.ops" && view.Username != "carla.ops" {
			t.Fatalf("paged search returned an account outside the match set: %q", view.Username)
		}
	}
	if second[0].ID <= first[len(first)-1].ID {
		t.Fatalf("cursor did not advance: %q then %q", first[len(first)-1].ID, second[0].ID)
	}
}
