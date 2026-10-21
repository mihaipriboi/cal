package ical

import (
	"strings"
	"testing"
	"time"
)

func TestEncodeEventRoundTrip(t *testing.T) {
	ev := Event{
		UID:         "test-1",
		Summary:     "Dinner, maybe?",
		Description: "line one\nline two; with semicolon",
		Start:       time.Date(2026, 9, 12, 18, 30, 0, 0, time.UTC),
		End:         time.Date(2026, 9, 12, 20, 0, 0, 0, time.UTC),
	}
	out := EncodeEvent(ev)
	if !strings.Contains(out, "SUMMARY:Dinner\\, maybe?") {
		t.Errorf("summary not escaped:\n%s", out)
	}
	if !strings.Contains(out, "DTSTART:20260912T183000Z") {
		t.Errorf("dtstart:\n%s", out)
	}
	parsed, err := Parse(out)
	if err != nil || len(parsed) != 1 {
		t.Fatalf("round-trip parse failed: %v", err)
	}
	if parsed[0].UID != "test-1" || parsed[0].Summary != "Dinner, maybe?" {
		t.Errorf("round-trip mismatch: %+v", parsed[0])
	}
}

func TestEncodeAllDay(t *testing.T) {
	ev := Event{UID: "d1", Summary: "Holiday", AllDay: true, Start: time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)}
	out := EncodeEvent(ev)
	if !strings.Contains(out, "DTSTART;VALUE=DATE:20261225") {
		t.Errorf("all-day dtstart:\n%s", out)
	}
	if !strings.Contains(out, "DTEND;VALUE=DATE:20261226") {
		t.Errorf("all-day dtend should default to +1d:\n%s", out)
	}
}

// CR/LF in any field must not start a new property line: URL and IMAGE are
// URIs (not TEXT-escaped), so control characters are dropped there.
func TestEncodeNoPropertyInjection(t *testing.T) {
	long := "https://example.com/" + strings.Repeat("a", 120)
	ev := Event{
		UID:         "inj-1",
		Summary:     "Title\rX-INJECT-SUMMARY:1",
		Description: "a\r\nX-INJECT-DESC:1",
		URL:         "https://example.com/x\r\nATTENDEE:mailto:evil@example.com",
		Image:       long + "\nX-INJECT-IMAGE:1",
		Start:       time.Date(2026, 9, 12, 18, 30, 0, 0, time.UTC),
		End:         time.Date(2026, 9, 12, 20, 0, 0, 0, time.UTC),
	}
	out := EncodeEvent(ev)
	for _, line := range strings.Split(out, "\r\n") {
		if strings.ContainsAny(line, "\r\n") {
			t.Fatalf("bare CR/LF in line %q", line)
		}
		for _, bad := range []string{"ATTENDEE", "X-INJECT"} {
			if strings.HasPrefix(line, bad) {
				t.Fatalf("injected property line %q in:\n%s", line, out)
			}
		}
	}
	parsed, err := Parse(out)
	if err != nil || len(parsed) != 1 {
		t.Fatalf("parse: %v", err)
	}
	if got := parsed[0].URL; got != "https://example.com/xATTENDEE:mailto:evil@example.com" {
		t.Errorf("URL = %q", got)
	}
	if got := parsed[0].Image; got != long+"X-INJECT-IMAGE:1" {
		t.Errorf("long IMAGE did not round-trip through folding: %q", got)
	}
	if got := parsed[0].Summary; got != "Title\nX-INJECT-SUMMARY:1" {
		t.Errorf("Summary = %q", got)
	}
}
