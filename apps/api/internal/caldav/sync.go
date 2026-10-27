// Two-way sync between Cal entries and a CalDAV collection.
// Pull: remote events upsert into entries; remotely-deleted entries drop.
// Push: dirty entries PUT; tombstoned deletes DELETE.
// Conflict rule: last write wins per object (remote wins while local dirty —
// jarvis: acceptable for a personal planner, a conflict UI is overkill).

package caldav

import (
	"context"
	"log"
	"strings"
	"time"

	"cal/apps/api/internal/ical"
	"cal/apps/api/internal/store"

	"github.com/google/uuid"
)

type Syncer struct {
	store *store.Store
}

func NewSyncer(s *store.Store) *Syncer {
	return &Syncer{store: s}
}

func (sy *Syncer) clientFor(ctx context.Context, a store.CaldavAccount) (*Client, error) {
	password, err := sy.store.Decrypt(ctx, a.PasswordEnc)
	if err != nil {
		return nil, err
	}
	return New(a.URL, a.Username, password), nil
}

// Test verifies the collection URL + credentials before the account is saved.
func (sy *Syncer) Test(ctx context.Context, url, username, password string) error {
	return New(url, username, password).TestConnection(ctx)
}

// SyncAccount performs one full sync round for an account.
func (sy *Syncer) SyncAccount(ctx context.Context, userID string, a store.CaldavAccount) error {
	client, err := sy.clientFor(ctx, a)
	if err != nil {
		return err
	}
	remote, err := client.ListEvents(ctx)
	if err != nil {
		return err
	}
	local, err := sy.store.AccountEntries(ctx, userID, a.ID)
	if err != nil {
		return err
	}
	// Entries hold wall-clock times in the user's Settings zone.
	loc := sy.store.UserLocation(ctx, userID)
	byUID := map[string]store.Entry{}
	for _, e := range local {
		if e.ExternalUID != nil {
			byUID[*e.ExternalUID] = e
		}
	}

	// 1. Deletions propagate first so a remote tombstone doesn't get re-created.
	tombstones, _ := sy.store.Tombstones(ctx, a.ID)
	for _, href := range tombstones {
		if err := client.DeleteEvent(ctx, href); err != nil {
			log.Printf("caldav delete %s: %v", href, err)
			continue
		}
		_ = sy.store.ClearTombstone(ctx, a.ID, href)
	}

	// 2. Push local changes (dirty) — creates and updates alike.
	remoteByUID := map[string]RemoteEvent{}
	var keepUIDs []string
	for _, r := range remote {
		if uid := uidOf(r.ICS); uid != "" {
			remoteByUID[uid] = r
			keepUIDs = append(keepUIDs, uid)
		}
	}
	for _, e := range local {
		if !e.Dirty {
			continue
		}
		uid := uidFor(e)
		href := hrefFor(e)
		ics := encodeEntry(e, uid, loc)
		etag := ""
		if e.ExternalETag != nil {
			etag = *e.ExternalETag
		}
		newTag, err := client.PutEvent(ctx, href, ics, etag)
		if err != nil {
			log.Printf("caldav put %s: %v", href, err)
			continue
		}
		if e.ExternalUID == nil {
			_ = sy.store.SetExternalRef(ctx, e.ID, uid, href, newTag)
		} else {
			_ = sy.store.ClearDirty(ctx, e.ID, newTag)
		}
		byUID[uid] = e // keep for the pull pass
		keepUIDs = append(keepUIDs, uid)
	}

	// 3. Pull remote changes.
	for _, r := range remote {
		uid := uidOf(r.ICS)
		if uid == "" {
			continue
		}
		existing, ok := byUID[uid]
		same := ok && existing.ExternalETag != nil && *existing.ExternalETag == r.ETag && r.ETag != ""
		remoteChanged := ok && (existing.ExternalETag == nil || *existing.ExternalETag != r.ETag)
		if same {
			continue
		}
		if ok && existing.Dirty && remoteChanged {
			// Both sides moved: remote wins the shared object, but the local
			// edit survives as a detached copy tagged `conflict`.
			sy.preserveConflict(ctx, userID, existing)
		} else if ok && existing.Dirty {
			continue // local-only change; pushed in step 2, remote copy is ours
		}
		evs, err := ical.ParseIn(r.ICS, loc)
		if err != nil || len(evs) == 0 {
			continue
		}
		entry := entryFrom(evs[0], loc)
		entry.ID = uuid.NewString()
		if ok {
			entry.ID = existing.ID
		}
		entry.ExternalUID = &uid
		entry.ExternalHref = &r.Href
		entry.ExternalETag = &r.ETag
		if err := sy.store.UpsertSyncedEntry(ctx, userID, a.ID, entry); err != nil {
			log.Printf("caldav upsert %s: %v", uid, err)
		}
	}

	// 4. Remote deletes → local deletes (only for clean synced rows).
	if err := sy.store.DeleteSyncedMissing(ctx, userID, a.ID, keepUIDs); err != nil {
		return err
	}
	return sy.store.TouchCaldavSync(ctx, a.ID)
}

// preserveConflict writes the dirty local version back as a detached copy so
// neither side of a divergence is lost.
func (sy *Syncer) preserveConflict(ctx context.Context, userID string, e store.Entry) {
	tags := append(append([]string{}, e.Tags...), "conflict")
	_, err := sy.store.CreateEntry(ctx, userID, store.EntryInput{
		Title: e.Title + " (conflict)", Content: e.Content, Type: e.Type,
		LinkURL: e.LinkURL, Date: e.Date,
		StartTime: strOr(e.StartTime), EndTime: strOr(e.EndTime),
		Color: e.Color, Tags: tags, Recur: e.Recur, Remind: e.Remind,
	})
	if err != nil {
		log.Printf("caldav conflict copy %s: %v", e.ID, err)
	}
}

// SyncAll is the background loop tick: every account, best-effort.
func (sy *Syncer) SyncAll(ctx context.Context) {
	owners, err := sy.store.AllCaldavAccounts(ctx)
	if err != nil {
		log.Printf("caldav sync: %v", err)
		return
	}
	for _, o := range owners {
		if err := sy.SyncAccount(ctx, o.UserID, o.Account); err != nil {
			log.Printf("caldav sync %s: %v", o.Account.Name, err)
		}
	}
}

func strOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func uidFor(e store.Entry) string {
	if e.ExternalUID != nil && *e.ExternalUID != "" {
		return *e.ExternalUID
	}
	return "cal-" + e.ID
}

func hrefFor(e store.Entry) string {
	if e.ExternalHref != nil && *e.ExternalHref != "" {
		return *e.ExternalHref
	}
	return uidFor(e) + ".ics"
}

// uidOf extracts the UID from an ICS body without full parsing.
func uidOf(ics string) string {
	for _, line := range strings.Split(strings.ReplaceAll(ics, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "UID:") {
			return strings.TrimSpace(line[4:])
		}
	}
	return ""
}

// encodeEntry writes an entry as a VEVENT. Its times are wall clock in loc,
// and are written with that zone's TZID.
func encodeEntry(e store.Entry, uid string, loc *time.Location) string {
	d, _ := time.Parse(time.DateOnly, e.Date)
	ev := ical.Event{
		UID:         uid,
		Summary:     e.Title,
		Description: e.Content,
		URL:         e.LinkURL,
		TZ:          loc,
	}
	if e.StartTime == nil {
		ev.AllDay = true
		ev.Start = d
		ev.End = d.AddDate(0, 0, 1)
	} else {
		ev.Start, _ = ical.LocalTime(e.Date, *e.StartTime, loc)
		if e.EndTime != nil {
			ev.End, _ = ical.LocalTime(e.Date, *e.EndTime, loc)
		} else {
			ev.End = ev.Start.Add(time.Hour)
		}
	}
	return ical.EncodeEvent(ev)
}

// entryFrom turns a remote VEVENT into an entry, with its date and times in
// loc.
func entryFrom(ev ical.Event, loc *time.Location) store.Entry {
	e := store.Entry{
		Type:  "event",
		Title: ev.Summary,
		Color: "sky",
	}
	if e.Title == "" {
		e.Title = "Untitled"
	}
	e.Content = ev.Description
	e.LinkURL = ev.URL
	date, start, end := ical.LocalSlot(ev, loc)
	e.Date = date
	if start != "" {
		e.StartTime = &start
	}
	if end != "" {
		e.EndTime = &end
	}
	return e
}
