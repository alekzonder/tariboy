package schedule

// rrule.go owns the agent loop schedule: an RFC 5545 DTSTART + RRULE pair
// that starts loop iterations at whole minutes. Unlike the cron schedules
// above (agent-authored message reminders in UTC), a DTSTART without TZID or
// Z is a floating time read in the daemon host's zone.

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/teambition/rrule-go"
)

// splitRRule accepts "RRULE:…", a bare "FREQ=…", or a DTSTART line followed by
// an RRULE line. Lines may be separated by newlines or spaces so a single-line
// field can carry both.
func splitRRule(text string) (dtstart, rule string, err error) {
	parts := strings.Fields(text)
	switch len(parts) {
	case 0:
		return "", "", errors.New("schedule is empty")
	case 1:
		rule = parts[0]
	case 2:
		dtstart, rule = parts[0], parts[1]
		if !strings.HasPrefix(strings.ToUpper(dtstart), "DTSTART") {
			return "", "", fmt.Errorf("expected DTSTART before RRULE, got %q", dtstart)
		}
	default:
		return "", "", errors.New("schedule takes one RRULE and an optional DTSTART")
	}
	if strings.HasPrefix(strings.ToUpper(rule), "RRULE:") {
		rule = rule[len("RRULE:"):]
	} else if strings.Contains(rule, ":") {
		return "", "", fmt.Errorf("expected an RRULE, got %q", rule)
	}
	return dtstart, rule, nil
}

func parseRRule(text string, loc *time.Location) (*rrule.ROption, error) {
	dtstart, rule, err := splitRRule(text)
	if err != nil {
		return nil, err
	}
	src := rule
	if dtstart != "" {
		src = dtstart + "\n" + rule
	}
	opt, err := rrule.StrToROptionInLocation(src, loc)
	if err != nil {
		return nil, err
	}
	if _, err := rrule.NewRRule(*opt); err != nil {
		return nil, err
	}
	return opt, nil
}

// NormalizeRRule validates an agent schedule and returns the form to store:
// a DTSTART line (now, truncated to the minute, when absent) and an RRULE
// with BYSECOND=0. Second precision is rejected: schedules run at whole
// minutes.
func NormalizeRRule(text string, now time.Time, loc *time.Location) (string, error) {
	opt, err := parseRRule(text, loc)
	if err != nil {
		return "", fmt.Errorf("bad schedule: %w", err)
	}
	if opt.Freq == rrule.SECONDLY {
		return "", errors.New("bad schedule: FREQ=SECONDLY is not supported; the smallest step is a minute")
	}
	if len(opt.Bysecond) > 1 || len(opt.Bysecond) == 1 && opt.Bysecond[0] != 0 {
		return "", errors.New("bad schedule: BYSECOND is not supported; schedules run at whole minutes")
	}
	if opt.Dtstart.IsZero() {
		opt.Dtstart = now.In(loc).Truncate(time.Minute)
	} else if opt.Dtstart.Second() != 0 || opt.Dtstart.Nanosecond() != 0 {
		return "", errors.New("bad schedule: DTSTART must be a whole minute")
	}
	opt.Bysecond = []int{0}
	return dtstartLine(opt.Dtstart, loc) + "\nRRULE:" + opt.RRuleString(), nil
}

// dtstartLine keeps a host-zone start floating so the rule follows the host's
// zone; any other zone is named explicitly.
func dtstartLine(t time.Time, loc *time.Location) string {
	switch t.Location() {
	case loc:
		return "DTSTART:" + t.Format(rrule.LocalDateTimeFormat)
	case time.UTC:
		return "DTSTART:" + t.Format(rrule.DateTimeFormat)
	default:
		return "DTSTART;TZID=" + t.Location().String() + ":" + t.Format(rrule.LocalDateTimeFormat)
	}
}

// NextRRule returns the first occurrence strictly after after. ok is false for
// an empty, invalid, or finished rule.
func NextRRule(text string, after time.Time, loc *time.Location) (time.Time, bool) {
	if strings.TrimSpace(text) == "" {
		return time.Time{}, false
	}
	opt, err := parseRRule(text, loc)
	if err != nil {
		return time.Time{}, false
	}
	r, err := rrule.NewRRule(*opt)
	if err != nil {
		return time.Time{}, false
	}
	next := r.After(after, false)
	return next, !next.IsZero()
}
