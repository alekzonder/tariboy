package schedule

import (
	"testing"
	"time"
)

func TestCronNext(t *testing.T) {
	base := time.Date(2026, 7, 6, 10, 30, 0, 0, time.UTC)
	cases := []struct {
		expr string
		want string // RFC3339 of next firing after base
	}{
		{"* * * * *", "2026-07-06T10:31:00Z"},
		{"0 * * * *", "2026-07-06T11:00:00Z"},
		{"*/15 * * * *", "2026-07-06T10:45:00Z"},
		{"30 10 * * *", "2026-07-07T10:30:00Z"}, // already 10:30 -> next day
		{"0 0 1 * *", "2026-08-01T00:00:00Z"},
		{"0 9 * * 1-5", "2026-07-07T09:00:00Z"}, // Mon-Fri 09:00; base is Monday 10:30 -> Tue
	}
	for _, c := range cases {
		cr, err := Parse(c.expr)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.expr, err)
		}
		got, ok := cr.Next(base)
		if !ok || got.Format(time.RFC3339) != c.want {
			t.Fatalf("Next(%q) = %v ok=%v, want %s", c.expr, got.Format(time.RFC3339), ok, c.want)
		}
	}
}

func TestCronNextEdgeCases(t *testing.T) {
	cases := []struct {
		expr string
		from string // RFC3339 base
		want string // RFC3339 of next firing
	}{
		// Leap day: Feb 29 exists in 2028, not 2027.
		{"0 0 29 2 *", "2026-03-01T00:00:00Z", "2028-02-29T00:00:00Z"},
		// End-of-month rollover: last minute of Jan -> Feb 1.
		{"0 0 1 * *", "2026-01-31T23:59:00Z", "2026-02-01T00:00:00Z"},
		// End-of-year rollover.
		{"0 0 1 1 *", "2026-12-31T12:00:00Z", "2027-01-01T00:00:00Z"},
		// Vixie OR: dom=13 OR dow=Fri(5) both restricted -> matches either.
		// 2026-07-06 is Monday; next Friday is 2026-07-10, next 13th is 2026-07-13.
		{"0 0 13 * 5", "2026-07-06T10:00:00Z", "2026-07-10T00:00:00Z"},
		// Both restricted, the 13th comes before any Friday after the 10th? pick day-13 case.
		{"0 0 13 * 5", "2026-07-11T00:00:00Z", "2026-07-13T00:00:00Z"},
		// dom restricted, dow star -> AND (just dom).
		{"0 0 15 * *", "2026-07-06T00:00:00Z", "2026-07-15T00:00:00Z"},
		// dow restricted, dom star -> AND (just dow). Sunday via 0.
		{"0 12 * * 0", "2026-07-06T00:00:00Z", "2026-07-12T12:00:00Z"},
		// Sunday via 7.
		{"0 12 * * 7", "2026-07-06T00:00:00Z", "2026-07-12T12:00:00Z"},
		// Step over a range: minutes 0-30 every 10 -> 0,10,20,30.
		{"0-30/10 * * * *", "2026-07-06T10:05:00Z", "2026-07-06T10:10:00Z"},
		{"0-30/10 * * * *", "2026-07-06T10:31:00Z", "2026-07-06T11:00:00Z"},
		// List values.
		{"5,25,45 * * * *", "2026-07-06T10:10:00Z", "2026-07-06T10:25:00Z"},
		// Step with explicit start via range a-b/n on hours.
		{"0 9-17/4 * * *", "2026-07-06T10:00:00Z", "2026-07-06T13:00:00Z"},
		// Already exactly on a match -> strictly after.
		{"* * * * *", "2026-07-06T10:30:00Z", "2026-07-06T10:31:00Z"},
	}
	for _, c := range cases {
		cr, err := Parse(c.expr)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.expr, err)
		}
		from, _ := time.Parse(time.RFC3339, c.from)
		got, ok := cr.Next(from)
		if !ok || got.Format(time.RFC3339) != c.want {
			t.Fatalf("Next(%q, %s) = %v ok=%v, want %s", c.expr, c.from, got.Format(time.RFC3339), ok, c.want)
		}
	}
}

func TestCronNextImpossible(t *testing.T) {
	// Feb 30 never exists.
	cr, err := Parse("0 0 30 2 *")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, ok := cr.Next(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); ok {
		t.Fatalf("expected no firing, got %v", got)
	}
}

func TestCronParseErrors(t *testing.T) {
	for _, bad := range []string{"", "* * * *", "60 * * * *", "* 24 * * *", "a * * * *", "*/0 * * * *"} {
		if _, err := Parse(bad); err == nil {
			t.Fatalf("Parse(%q) should error", bad)
		}
	}
}
