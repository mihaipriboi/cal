// Package ical parses a useful subset of iCalendar (RFC 5545) for
// read-only feed subscriptions and file imports: VEVENT summary,
// description, location, URL, DTSTART/DTEND in all three date forms,
// RRULE expansion (daily/weekly/monthly/yearly) and EXDATE.
package ical

import (
	"fmt"
	"strings"
	"time"

	_ "time/tzdata" // containers may lack zoneinfo
)

// Event is a flattened calendar occurrence (or the seed of a recurring one).
type Event struct {
	UID         string
	Summary     string
	Description string
	Location    string
	URL         string
	Image       string // RFC 7986 IMAGE;VALUE=URI
	Start       time.Time
	End         time.Time
	AllDay      bool
	Recurrence  *RRule
	Exdates     []time.Time
	Cancelled   bool
	Completed   bool
	// AlarmMin is minutes-before-start from VALARM/TRIGGER (negative duration).
	AlarmMin *int
	// TZ, when set, emits DTSTART;TZID=… with local wall time instead of UTC.
	TZ *time.Location
}

type RRule struct {
	Freq       string // DAILY, WEEKLY, MONTHLY, YEARLY
	Interval   int
	Count      int
	Until      time.Time
	HasUntil   bool
	ByWeekday  []time.Weekday
	ByMonthDay int
}

type prop struct {
	name   string
	params map[string]string
	value  string
}

// Parse decodes an iCalendar stream into VEVENT records.
func Parse(data string) ([]Event, error) {
	var events []Event
	var cur *Event
	inAlarm := false
	for _, line := range unfold(data) {
		p := splitProp(line)
		if p == nil {
			continue
		}
		switch {
		case p.name == "BEGIN" && p.value == "VEVENT":
			cur = &Event{}
		case p.name == "BEGIN" && p.value == "VALARM":
			inAlarm = true
		case p.name == "END" && p.value == "VALARM":
			inAlarm = false
		case p.name == "TRIGGER" && inAlarm && cur != nil:
			// Negative duration = before start → remind minutes.
			if d, err := parseDuration(strings.TrimPrefix(p.value, "-")); err == nil && strings.HasPrefix(p.value, "-") {
				mins := int(d.Minutes())
				if cur.AlarmMin == nil || mins < *cur.AlarmMin {
					cur.AlarmMin = &mins
				}
			}
		case p.name == "END" && p.value == "VEVENT" && cur != nil:
			if !cur.Start.IsZero() {
				events = append(events, *cur)
			}
			cur = nil
		case cur == nil:
			// outside a VEVENT
		case p.name == "UID":
			cur.UID = p.value
		case p.name == "SUMMARY":
			cur.Summary = unescape(p.value)
		case p.name == "DESCRIPTION":
			cur.Description = unescape(p.value)
		case p.name == "LOCATION":
			cur.Location = unescape(p.value)
		case p.name == "URL":
			cur.URL = p.value
		case p.name == "IMAGE":
			cur.Image = p.value
		case p.name == "DTSTART":
			t, allDay, err := parseTime(p)
			if err != nil {
				return nil, fmt.Errorf("DTSTART: %w", err)
			}
			cur.Start, cur.AllDay = t, allDay
		case p.name == "DTEND":
			t, _, err := parseTime(p)
			if err != nil {
				return nil, fmt.Errorf("DTEND: %w", err)
			}
			cur.End = t
		case p.name == "DURATION" && cur.End.IsZero():
			if d, err := parseDuration(p.value); err == nil {
				cur.End = cur.Start.Add(d)
			}
		case p.name == "RRULE":
			if r, err := parseRRule(p.value); err == nil {
				cur.Recurrence = r
			}
		case p.name == "EXDATE":
			for _, part := range strings.Split(p.value, ",") {
				q := *p
				q.value = part
				if t, _, err := parseTime(&q); err == nil {
					cur.Exdates = append(cur.Exdates, t)
				}
			}
		case p.name == "STATUS" && p.value == "CANCELLED":
			cur.Cancelled = true
		}
	}
	for i := range events {
		if events[i].End.IsZero() {
			if events[i].AllDay {
				events[i].End = events[i].Start.AddDate(0, 0, 1)
			} else {
				events[i].End = events[i].Start.Add(time.Hour)
			}
		}
	}
	return events, nil
}

// Expand flattens recurring events into occurrences inside [from, to].
// Non-recurring events pass through when they overlap the range.
func Expand(events []Event, from, to time.Time) []Event {
	var out []Event
	for _, e := range events {
		if e.Cancelled {
			continue
		}
		if e.Recurrence == nil {
			if e.Start.Before(to) && e.End.After(from) {
				out = append(out, e)
			}
			continue
		}
		// Occurrences may start before `from` but still overlap it.
		cursor := e.Start
		limit := e.Recurrence.Count
		if limit <= 0 {
			limit = 1000 // jarvis: ceiling, prevents runaway feeds
		}
		excluded := map[int64]bool{}
		for _, ex := range e.Exdates {
			excluded[ex.Unix()] = true
		}
		for i := 0; i < limit; i++ {
			if i > 0 {
				cursor = nextOccurrence(cursor, e.Recurrence, e.Start)
				if cursor.IsZero() {
					break
				}
			}
			if e.Recurrence.HasUntil && cursor.After(e.Recurrence.Until) {
				break
			}
			if cursor.After(to) {
				break
			}
			occ := e
			occ.Start = cursor
			occ.End = cursor.Add(e.End.Sub(e.Start))
			occ.Recurrence = nil
			if !excluded[cursor.Unix()] && occ.Start.Before(to) && occ.End.After(from) {
				out = append(out, occ)
			}
		}
	}
	return out
}

func nextOccurrence(prev time.Time, r *RRule, seed time.Time) time.Time {
	interval := r.Interval
	if interval <= 0 {
		interval = 1
	}
	switch r.Freq {
	case "DAILY":
		return prev.AddDate(0, 0, interval)
	case "WEEKLY":
		if len(r.ByWeekday) == 0 {
			return prev.AddDate(0, 0, 7*interval)
		}
		// Advance to the next BYDAY in week order after `prev`; wraps to the
		// next interval week.
		for step := 1; step <= 7*interval; step++ {
			cand := prev.AddDate(0, 0, step)
			if weekdayIn(cand.Weekday(), r.ByWeekday) && (step < 7 || interval == 1 || weekIndex(cand, seed)%interval == 0) {
				return cand
			}
		}
		return time.Time{}
	case "MONTHLY":
		return addMonthsClamped(prev, interval, r.ByMonthDay, seed.Day())
	case "YEARLY":
		return addMonthsClamped(prev, 12*interval, r.ByMonthDay, seed.Day())
	}
	return time.Time{}
}

func weekIndex(t, seed time.Time) int {
	return int(t.Sub(seed).Hours() / 24 / 7)
}

func weekdayIn(w time.Weekday, list []time.Weekday) bool {
	for _, x := range list {
		if x == w {
			return true
		}
	}
	return false
}

// addMonthsClamped keeps the day-of-month stable against the seed (Jan 31 ->
// Feb 28 -> Mar 31, never drifting to Mar 28).
func addMonthsClamped(t time.Time, months int, byMonthDay, seedDay int) time.Time {
	y, m, _ := t.Date()
	target := time.Date(y, m+time.Month(months), 1, t.Hour(), t.Minute(), t.Second(), 0, t.Location())
	day := seedDay
	if byMonthDay != 0 {
		day = byMonthDay
	}
	if last := lastDay(target); day > last {
		day = last
	}
	return time.Date(target.Year(), target.Month(), day, t.Hour(), t.Minute(), t.Second(), 0, t.Location())
}

func lastDay(t time.Time) int {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
}

func unfold(data string) []string {
	data = strings.ReplaceAll(data, "\r\n", "\n")
	raw := strings.Split(data, "\n")
	var lines []string
	for _, line := range raw {
		if line == "" {
			continue
		}
		if (line[0] == ' ' || line[0] == '\t') && len(lines) > 0 {
			lines[len(lines)-1] += line[1:]
		} else {
			lines = append(lines, line)
		}
	}
	return lines
}

func splitProp(line string) *prop {
	// Split at the first ':' that isn't inside a quoted param.
	inQuote := false
	for i, r := range line {
		switch r {
		case '"':
			inQuote = !inQuote
		case ':':
			if !inQuote {
				head, value := line[:i], line[i+1:]
				p := &prop{value: value, params: map[string]string{}}
				parts := strings.Split(head, ";")
				p.name = strings.ToUpper(parts[0])
				for _, kv := range parts[1:] {
					if eq := strings.IndexByte(kv, '='); eq > 0 {
						p.params[strings.ToUpper(kv[:eq])] = strings.Trim(kv[eq+1:], `"`)
					}
				}
				return p
			}
		}
	}
	return nil
}

// parseTime handles DATE, UTC ("Z"), TZID-qualified and floating local times.
func parseTime(p *prop) (time.Time, bool, error) {
	v := p.value
	if p.params["VALUE"] == "DATE" || len(v) == 8 {
		t, err := time.ParseInLocation("20060102", v, time.Local)
		return t, true, err
	}
	layout := "20060102T150405"
	if strings.HasSuffix(v, "Z") {
		t, err := time.Parse(layout+"Z", v)
		return t, false, err
	}
	loc := time.Local
	if tz := p.params["TZID"]; tz != "" {
		// Olson name; some feeds ship "W. Europe Standard Time"-style zones,
		// which we map to a few common equivalents before giving up.
		if l, err := time.LoadLocation(mapWindowsTZ(tz)); err == nil {
			loc = l
		}
	}
	t, err := time.ParseInLocation(layout, v, loc)
	return t, false, err
}

var windowsTZ = map[string]string{
	"W. Europe Standard Time":        "Europe/Berlin",
	"Central European Standard Time": "Europe/Budapest",
	"GMT Standard Time":              "Europe/London",
	"Eastern Standard Time":          "America/New_York",
	"Central Standard Time":          "America/Chicago",
	"Mountain Standard Time":         "America/Denver",
	"Pacific Standard Time":          "America/Los_Angeles",
	"UTC":                            "UTC",
}

func mapWindowsTZ(tz string) string {
	if mapped, ok := windowsTZ[tz]; ok {
		return mapped
	}
	return tz
}

var weekdays = map[string]time.Weekday{
	"SU": time.Sunday, "MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday,
	"TH": time.Thursday, "FR": time.Friday, "SA": time.Saturday,
}

func parseRRule(v string) (*RRule, error) {
	r := &RRule{Interval: 1}
	for _, part := range strings.Split(v, ";") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch strings.ToUpper(kv[0]) {
		case "FREQ":
			r.Freq = strings.ToUpper(kv[1])
		case "INTERVAL":
			fmt.Sscanf(kv[1], "%d", &r.Interval)
		case "COUNT":
			fmt.Sscanf(kv[1], "%d", &r.Count)
		case "UNTIL":
			p := &prop{value: kv[1]}
			if t, _, err := parseTime(p); err == nil {
				r.Until, r.HasUntil = t, true
			}
		case "BYDAY":
			for _, d := range strings.Split(kv[1], ",") {
				// Ignore ordinal prefixes (e.g. "2TU") — take the last 2 chars.
				if len(d) >= 2 {
					if w, ok := weekdays[strings.ToUpper(d[len(d)-2:])]; ok {
						r.ByWeekday = append(r.ByWeekday, w)
					}
				}
			}
		case "BYMONTHDAY":
			fmt.Sscanf(kv[1], "%d", &r.ByMonthDay)
		}
	}
	if r.Freq == "" {
		return nil, fmt.Errorf("missing FREQ")
	}
	return r, nil
}

func parseDuration(v string) (time.Duration, error) {
	// Minimal ISO-8601 duration: PT1H30M / P1D / P1W / PT45M.
	var d time.Duration
	v = strings.TrimPrefix(strings.ToUpper(v), "P")
	days := 0
	if i := strings.IndexByte(v, 'W'); i >= 0 {
		fmt.Sscanf(v[:i], "%d", &days)
		return time.Duration(days) * 7 * 24 * time.Hour, nil
	}
	if i := strings.IndexByte(v, 'D'); i >= 0 {
		fmt.Sscanf(v[:i], "%d", &days)
		v = v[i+1:]
	}
	v = strings.TrimPrefix(v, "T")
	num := 0
	for i := 0; i < len(v); i++ {
		if v[i] >= '0' && v[i] <= '9' {
			num = num*10 + int(v[i]-'0')
			continue
		}
		switch v[i] {
		case 'H':
			d += time.Duration(num) * time.Hour
		case 'M':
			d += time.Duration(num) * time.Minute
		case 'S':
			d += time.Duration(num) * time.Second
		}
		num = 0
	}
	return d + time.Duration(days)*24*time.Hour, nil
}

func unescape(s string) string {
	r := strings.NewReplacer(`\n`, "\n", `\\`, `\`, `\,`, ",", `\;`, ";", `\N`, "\n")
	return r.Replace(s)
}

// EncodeEvent serializes an Event into a minimal valid VCALENDAR/VEVENT.
func EncodeEvent(e Event) string {
	return EncodeCalendar([]Event{e})
}

// EncodeCalendar serializes events into one VCALENDAR document.
func EncodeCalendar(events []Event) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Cal//EN\r\nCALSCALE:GREGORIAN\r\n")
	for _, e := range events {
		b.WriteString(encodeVevent(e))
	}
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

func encodeVevent(e Event) string {
	var b strings.Builder
	b.WriteString("BEGIN:VEVENT\r\n")
	b.WriteString("UID:" + fold(escapeText(e.UID)) + "\r\n")
	b.WriteString("DTSTAMP:" + time.Now().UTC().Format("20060102T150405Z") + "\r\n")
	b.WriteString("SUMMARY:" + fold(escapeText(e.Summary)) + "\r\n")
	if e.Description != "" {
		b.WriteString("DESCRIPTION:" + fold(escapeText(e.Description)) + "\r\n")
	}
	if e.Location != "" {
		b.WriteString("LOCATION:" + fold(escapeText(e.Location)) + "\r\n")
	}
	if u := uriValue(e.URL); u != "" {
		b.WriteString("URL:" + fold(u) + "\r\n")
	}
	if u := uriValue(e.Image); u != "" {
		b.WriteString("IMAGE;VALUE=URI:" + fold(u) + "\r\n")
	}
	if e.AllDay {
		b.WriteString("DTSTART;VALUE=DATE:" + e.Start.Format("20060102") + "\r\n")
		end := e.End
		if !end.After(e.Start) {
			end = e.Start.AddDate(0, 0, 1)
		}
		b.WriteString("DTEND;VALUE=DATE:" + end.Format("20060102") + "\r\n")
	} else if e.TZ != nil {
		b.WriteString("DTSTART;TZID=" + e.TZ.String() + ":" + e.Start.In(e.TZ).Format("20060102T150405") + "\r\n")
		if e.End.After(e.Start) {
			b.WriteString("DTEND;TZID=" + e.TZ.String() + ":" + e.End.In(e.TZ).Format("20060102T150405") + "\r\n")
		}
	} else {
		b.WriteString("DTSTART:" + e.Start.UTC().Format("20060102T150405Z") + "\r\n")
		if e.End.After(e.Start) {
			b.WriteString("DTEND:" + e.End.UTC().Format("20060102T150405Z") + "\r\n")
		}
	}
	if e.Completed {
		b.WriteString("STATUS:COMPLETED\r\n")
	}
	if e.Recurrence != nil && e.Recurrence.Freq != "" {
		rr := "RRULE:FREQ=" + e.Recurrence.Freq
		if e.Recurrence.Interval > 1 {
			rr += fmt.Sprintf(";INTERVAL=%d", e.Recurrence.Interval)
		}
		if e.Recurrence.Count > 0 {
			rr += fmt.Sprintf(";COUNT=%d", e.Recurrence.Count)
		}
		if e.Recurrence.HasUntil {
			rr += ";UNTIL=" + e.Recurrence.Until.UTC().Format("20060102T150405Z")
		}
		if len(e.Recurrence.ByWeekday) > 0 {
			days := []string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}
			parts := make([]string, len(e.Recurrence.ByWeekday))
			for i, d := range e.Recurrence.ByWeekday {
				parts[i] = days[int(d)%7]
			}
			rr += ";BYDAY=" + strings.Join(parts, ",")
		}
		if e.Recurrence.ByMonthDay != 0 {
			rr += fmt.Sprintf(";BYMONTHDAY=%d", e.Recurrence.ByMonthDay)
		}
		b.WriteString(rr + "\r\n")
	}
	b.WriteString("END:VEVENT\r\n")
	return b.String()
}

func escapeText(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "\r\n", `\n`, "\r", `\n`, "\n", `\n`, ",", `\,`, ";", `\;`)
	return r.Replace(s)
}

// uriValue drops control characters from a URI property value. URIs aren't
// TEXT-escaped, so a CR or LF would otherwise start a new property line.
func uriValue(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// fold wraps a content line at 75 octets per RFC 5545 (simplified to runes —
// fine for typical titles/descriptions).
func fold(s string) string {
	if len(s) <= 70 {
		return s
	}
	var b strings.Builder
	for len(s) > 70 {
		b.WriteString(s[:70] + "\r\n ")
		s = s[70:]
	}
	b.WriteString(s)
	return b.String()
}
