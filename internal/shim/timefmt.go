package shim

import "time"

// timeLayout is store.TimeLayout, the one stored and wire time format. The
// shim keeps a copy because importing the store would link SQLite into the
// shim binary; TestShimTimeLayoutMatchesStore keeps the two equal.
const timeLayout = "2006-01-02T15:04:05.000000000Z"

// inputLayout is RFC 3339. Go accepts a fractional second after the seconds
// field when parsing even though the layout has none, so this reads every
// RFC 3339 value, including ones an older daemon sends.
const inputLayout = "2006-01-02T15:04:05Z07:00"

// formatTime writes t for the shim protocol and result files.
func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

// parseInputTime reads a time from the daemon, which may be an older version.
func parseInputTime(s string) (time.Time, error) {
	t, err := time.Parse(inputLayout, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}
