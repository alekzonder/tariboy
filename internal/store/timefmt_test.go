package store

import (
	"sort"
	"strings"
	"testing"
	"time"
)

func TestFormatTimeIsFixedWidthUTC(t *testing.T) {
	base := time.Date(2026, 10, 19, 6, 33, 58, 0, time.FixedZone("MSK", 3*3600))
	for _, ns := range []int{0, 10, 11, 999_999_999} {
		got := FormatTime(base.Add(time.Duration(ns)))
		if len(got) != 30 || got[len(got)-1] != 'Z' {
			t.Fatalf("FormatTime(+%dns) = %q, want 30 chars ending in Z", ns, got)
		}
	}
	if got := FormatTime(base); got != "2026-10-19T03:33:58.000000000Z" {
		t.Fatalf("FormatTime = %q, want UTC conversion", got)
	}
}

func TestFormatTimeSortsChronologically(t *testing.T) {
	base := time.Date(2026, 10, 19, 6, 33, 58, 0, time.UTC)
	var times []time.Time
	for _, ns := range []int{11, 10, 999_999_999, 0, 100, 1} {
		times = append(times, base.Add(time.Duration(ns)))
	}
	strs := make([]string, len(times))
	for i, ts := range times {
		strs[i] = FormatTime(ts)
	}
	sort.Strings(strs)
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	for i := range times {
		if strs[i] != FormatTime(times[i]) {
			t.Fatalf("lexical order %v differs from chronological order", strs)
		}
	}
}

func TestParseTimeAcceptsOnlyCanonical(t *testing.T) {
	want := time.Date(2026, 10, 19, 6, 33, 58, 10, time.UTC)
	got, err := ParseTime("2026-10-19T06:33:58.000000010Z")
	if err != nil || !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("ParseTime = %v, %v", got, err)
	}
	for _, bad := range []string{
		"2026-10-19T06:33:58.00000001Z",
		"2026-10-19T06:33:58Z",
		"2026-10-19T06:33:58.123Z",
		"2026-10-19T09:33:58.000000010+03:00",
		"",
		"garbage",
	} {
		if _, err := ParseTime(bad); err == nil {
			t.Fatalf("ParseTime(%q) accepted a non-canonical value", bad)
		}
	}
}

func TestParseInputTimeAcceptsRFC3339Offsets(t *testing.T) {
	got, err := ParseInputTime("2026-10-19T09:33:58.5+03:00")
	if err != nil {
		t.Fatal(err)
	}
	if got.Location() != time.UTC || FormatTime(got) != "2026-10-19T06:33:58.500000000Z" {
		t.Fatalf("ParseInputTime = %v", got)
	}
	if _, err := ParseInputTime("yesterday"); err == nil {
		t.Fatal("ParseInputTime accepted a non-time value")
	}
}

func TestNormalizeTimeValue(t *testing.T) {
	cases := map[string]string{
		"":                                   "",
		"2026-10-19T06:33:58.123Z":           "2026-10-19T06:33:58.123000000Z",
		"2026-10-19T06:33:58Z":               "2026-10-19T06:33:58.000000000Z",
		"2026-10-19T09:33:58.00000001+03:00": "2026-10-19T06:33:58.000000010Z",
		"2026-10-19T06:33:58.000000010Z":     "2026-10-19T06:33:58.000000010Z",
	}
	for in, want := range cases {
		got, ok := NormalizeTimeValue(in)
		if !ok || got != want {
			t.Fatalf("NormalizeTimeValue(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if got, ok := NormalizeTimeValue("not a time"); ok || got != "not a time" {
		t.Fatalf("NormalizeTimeValue(garbage) = %q, %v; want unchanged and false", got, ok)
	}
}

func TestSQLNowIsCanonical(t *testing.T) {
	s := open(t)
	var now string
	if err := s.DB.QueryRow(`SELECT ` + SQLNow).Scan(&now); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseTime(now); err != nil {
		t.Fatalf("SQLNow = %q is not canonical: %v", now, err)
	}
}

// TestTimeColumnsCoverSchema fails when a new time column is not registered,
// so the backfill and backup restore keep every stored time canonical.
func TestTimeColumnsCoverSchema(t *testing.T) {
	s := open(t)
	rows, err := s.DB.Query(`SELECT m.name, p.name, p.type FROM sqlite_schema m
		JOIN pragma_table_info(m.name) p WHERE m.type = 'table'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	schema := map[string]bool{}
	for rows.Next() {
		var table, column, typ string
		if err := rows.Scan(&table, &column, &typ); err != nil {
			t.Fatal(err)
		}
		schema[table+"."+column] = true
		if typ == "TEXT" && looksLikeTimeColumn(column) && !IsTimeColumn(table, column) && !notTimeColumns[table+"."+column] {
			t.Errorf("time column %s.%s is not in timeColumns", table, column)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for table, columns := range timeColumns {
		for _, column := range columns {
			if !schema[table+"."+column] {
				t.Errorf("timeColumns names %s.%s, which the schema does not have", table, column)
			}
		}
	}
}

// notTimeColumns have a time-like name but hold something else.
var notTimeColumns = map[string]bool{
	"messages.deadline": true, // a Go duration such as 5m
}

func looksLikeTimeColumn(name string) bool {
	return name == "ts" || name == "deadline" || name == "status_updated" ||
		strings.HasSuffix(name, "_at") || strings.HasSuffix(name, "_ts") ||
		strings.HasSuffix(name, "_deadline") || strings.HasSuffix(name, "_since")
}
