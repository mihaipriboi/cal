package httpapi

import (
	"testing"
	"time"
)

func TestGcalEvents(t *testing.T) {
	events := gcalEvents([]gcalItem{
		{ID: "a", Summary: "Trip", Start: gcalTime{Date: "2026-10-30"}, End: gcalTime{Date: "2026-11-03"}},
		{ID: "b", Summary: "Call", Start: gcalTime{DateTime: "2026-10-22T10:00:00+03:00"}, End: gcalTime{DateTime: "2026-10-22T11:00:00+03:00"}},
		{ID: "c", Summary: "Gone", Status: "cancelled", Start: gcalTime{Date: "2026-10-22"}},
	})
	if len(events) != 2 {
		t.Fatalf("got %d events", len(events))
	}
	// Google's all-day end date is exclusive, as DTEND is.
	if trip := events[0]; !trip.AllDay || feedEndDate(trip) != "2026-11-02" {
		t.Errorf("trip: all-day %v, last day %q", trip.AllDay, feedEndDate(trip))
	}
	if call := events[1]; !call.Start.Equal(time.Date(2026, 10, 22, 7, 0, 0, 0, time.UTC)) || call.End.Sub(call.Start) != time.Hour {
		t.Errorf("call: %v – %v", call.Start, call.End)
	}
}
