package httpapi

import (
	"context"
	"testing"

	"cal/apps/api/internal/store"

	"github.com/google/uuid"
)

// CalDAV UIDs are scoped per account: the same UID in two accounts gives two
// rows, re-syncing updates in place, and no caller can update a row that
// belongs to another user.
func TestCaldavUIDScopedPerAccount(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	alice, err := st.CreateUser(ctx, "alice@cal.local", "x")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetAllowRegistration(ctx, true); err != nil {
		t.Fatal(err)
	}
	bob, err := st.CreateUser(ctx, "bob@cal.local", "x")
	if err != nil {
		t.Fatal(err)
	}
	acct := func(userID, name string) string {
		a, err := st.CreateCaldavAccount(ctx, userID, name, "https://dav.example/"+name+"/", "u", "enc", "sky")
		if err != nil {
			t.Fatal(err)
		}
		return a.ID
	}
	aliceWork, aliceHome, bobCal := acct(alice.ID, "work"), acct(alice.ID, "home"), acct(bob.ID, "bob")

	uid := "shared-uid@example"
	upsert := func(userID, accountID, title string) {
		t.Helper()
		href, etag := "/"+title+".ics", "e-"+title
		e := store.Entry{ID: uuid.NewString(), Title: title, Date: "2026-11-02", Color: "sky",
			ExternalUID: &uid, ExternalHref: &href, ExternalETag: &etag}
		if err := st.UpsertSyncedEntry(ctx, userID, accountID, e); err != nil {
			t.Fatalf("upsert %s: %v", title, err)
		}
	}
	titles := func(userID, accountID string) []string {
		t.Helper()
		rows, err := st.AccountEntries(ctx, userID, accountID)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range rows {
			out = append(out, r.Title)
		}
		return out
	}
	expect := func(label string, got []string, want string) {
		t.Helper()
		if len(got) != 1 || got[0] != want {
			t.Fatalf("%s: got %v, want [%s]", label, got, want)
		}
	}

	upsert(alice.ID, aliceWork, "alice-work")
	upsert(bob.ID, bobCal, "bob")
	upsert(alice.ID, aliceHome, "alice-home")
	expect("alice work", titles(alice.ID, aliceWork), "alice-work")
	expect("alice home (same user, same UID, other account)", titles(alice.ID, aliceHome), "alice-home")
	expect("bob (other user, same UID)", titles(bob.ID, bobCal), "bob")

	// Re-sync updates in place.
	upsert(alice.ID, aliceWork, "alice-work-v2")
	expect("alice work after re-sync", titles(alice.ID, aliceWork), "alice-work-v2")

	// A write naming someone else's account must not touch their row.
	upsert(bob.ID, aliceWork, "hijack")
	expect("alice work after foreign write", titles(alice.ID, aliceWork), "alice-work-v2")
}
