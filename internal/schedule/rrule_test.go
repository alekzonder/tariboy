package schedule

import (
	"strings"
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestNormalizeRRuleAddsFloatingDTStartAndWholeSeconds(t *testing.T) {
	loc := mustLoc(t, "Europe/Moscow")
	now := time.Date(2026, 10, 18, 14, 7, 42, 0, loc)
	got, err := NormalizeRRule("RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR;BYHOUR=9;BYMINUTE=30", now, loc)
	if err != nil {
		t.Fatal(err)
	}
	want := "DTSTART:20261018T140700\nRRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR;BYHOUR=9;BYMINUTE=30;BYSECOND=0"
	if got != want {
		t.Fatalf("normalized = %q, want %q", got, want)
	}
	again, err := NormalizeRRule(got, now.Add(time.Hour), loc)
	if err != nil || again != got {
		t.Fatalf("normalize is not idempotent: %q, %v", again, err)
	}
}

func TestNormalizeRRuleAcceptsBareRuleAndSpaceSeparatedLines(t *testing.T) {
	now := time.Date(2026, 10, 18, 14, 7, 0, 0, time.UTC)
	got, err := NormalizeRRule("FREQ=DAILY", now, time.UTC)
	if err != nil || !strings.HasSuffix(got, "\nRRULE:FREQ=DAILY;BYSECOND=0") {
		t.Fatalf("bare rule = %q, %v", got, err)
	}
	got, err = NormalizeRRule("DTSTART;TZID=Asia/Tokyo:20261020T093000 RRULE:FREQ=DAILY;COUNT=1", now, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if got != "DTSTART;TZID=Asia/Tokyo:20261020T093000\nRRULE:FREQ=DAILY;COUNT=1;BYSECOND=0" {
		t.Fatalf("explicit TZID = %q", got)
	}
}

func TestNormalizeRRuleRejectsSecondPrecisionAndGarbage(t *testing.T) {
	now := time.Date(2026, 10, 18, 14, 7, 0, 0, time.UTC)
	for _, in := range []string{
		"",
		"FREQ=SECONDLY",
		"FREQ=MINUTELY;BYSECOND=15",
		"DTSTART:20261020T093015\nRRULE:FREQ=DAILY",
		"FREQ=FORTNIGHTLY",
		"BYHOUR=9",
		"hello",
		"DTSTART;TZID=Nowhere/City:20261020T093000\nRRULE:FREQ=DAILY",
		"RRULE:FREQ=DAILY\nEXDATE:20261020T093000",
	} {
		if got, err := NormalizeRRule(in, now, time.UTC); err == nil {
			t.Errorf("NormalizeRRule(%q) = %q, want error", in, got)
		}
	}
}

func TestNextRRuleExamples(t *testing.T) {
	loc := mustLoc(t, "Europe/Moscow")
	now := time.Date(2026, 10, 18, 14, 7, 0, 0, loc) // a Sunday
	cases := []struct {
		name, rule string
		after      time.Time
		want       time.Time
	}{
		{"weekdays at 09:30", "RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR;BYHOUR=9;BYMINUTE=30",
			now, time.Date(2026, 10, 19, 9, 30, 0, 0, loc)},
		{"15th of the month", "RRULE:FREQ=MONTHLY;BYMONTHDAY=15;BYHOUR=8;BYMINUTE=0",
			now, time.Date(2026, 11, 15, 8, 0, 0, 0, loc)},
		{"last day of the month", "RRULE:FREQ=MONTHLY;BYMONTHDAY=-1;BYHOUR=23;BYMINUTE=45",
			now, time.Date(2026, 10, 31, 23, 45, 0, 0, loc)},
		{"once", "DTSTART:20261020T093000\nRRULE:FREQ=DAILY;COUNT=1",
			now, time.Date(2026, 10, 20, 9, 30, 0, 0, loc)},
		{"every 2 hours", "RRULE:FREQ=HOURLY;INTERVAL=2",
			now, time.Date(2026, 10, 18, 16, 7, 0, 0, loc)},
		{"strictly after", "RRULE:FREQ=DAILY;BYHOUR=9;BYMINUTE=30",
			time.Date(2026, 10, 19, 9, 30, 0, 0, loc), time.Date(2026, 10, 20, 9, 30, 0, 0, loc)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			norm, err := NormalizeRRule(tc.rule, now, loc)
			if err != nil {
				t.Fatal(err)
			}
			got, ok := NextRRule(norm, tc.after, loc)
			if !ok || !got.Equal(tc.want) {
				t.Fatalf("next = %v (%v), want %v", got, ok, tc.want)
			}
		})
	}
}

func TestNextRRuleExplicitZoneAndFinishedRule(t *testing.T) {
	now := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	norm, err := NormalizeRRule("DTSTART;TZID=Asia/Tokyo:20261018T000000\nRRULE:FREQ=DAILY;BYHOUR=9;BYMINUTE=0", now, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := NextRRule(norm, now.Add(-time.Minute), time.UTC)
	if want := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC); !ok || !got.Equal(want) {
		t.Fatalf("09:00 Tokyo = %v (%v), want %v", got, ok, want)
	}

	once, err := NormalizeRRule("DTSTART:20261018T090000\nRRULE:FREQ=DAILY;COUNT=1", now, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := NextRRule(once, now.Add(48*time.Hour), time.UTC); ok {
		t.Fatalf("finished rule fired at %v", got)
	}
	if _, ok := NextRRule("", now, time.UTC); ok {
		t.Fatal("empty rule must never fire")
	}
}
