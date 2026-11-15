package domain

import "testing"

// TST-PMS-001-02: calendar boundaries, without inventing reservation overlap states.
func TestStayInterval(t *testing.T) {
	for _, tt := range []struct {
		name, start, end string
		bad              bool
	}{
		{"one night", "2026-11-15", "2026-11-16", false},
		{"adjacent next interval", "2026-11-16", "2026-11-17", false},
		{"leap date", "2028-02-29", "2028-03-01", false},
		{"DST spring calendar", "2026-03-07", "2026-03-09", false},
		{"DST fall calendar", "2026-10-31", "2026-11-02", false},
		{"past dates allowed", "2000-01-01", "2000-01-02", false},
		{"long interval allowed", "0001-01-01", "9999-12-31", false},
		{"zero night", "2026-11-15", "2026-11-15", true},
		{"reversed", "2026-11-16", "2026-11-15", true},
		{"bad leap date", "2026-02-29", "2026-03-01", true},
		{"impossible day", "2026-04-31", "2026-05-02", true},
		{"impossible month", "2026-13-01", "2027-01-02", true},
		{"missing", "", "2026-11-16", true},
		{"short label", "2026-1-01", "2026-11-16", true},
		{"whitespace", " 2026-11-15", "2026-11-16", true},
		{"timestamp", "2026-11-15T00:00:00Z", "2026-11-16", true},
		{"checkout timestamp", "2026-11-15", "2026-11-16T00:00:00+07:00", true},
		{"year zero", "0000-01-01", "0001-01-01", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewStayInterval(tt.start, tt.end)
			if (err != nil) != tt.bad {
				t.Fatalf("unexpected interval validation: %v", err)
			}
			if err == nil && (got.CheckIn != tt.start || got.CheckOut != tt.end) {
				t.Fatalf("calendar labels shifted: %+v", got)
			}
		})
	}
}
