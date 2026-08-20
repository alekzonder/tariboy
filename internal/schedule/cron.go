// Package schedule owns cron/one-shot schedules (spec §6) and the daemon
// scheduler. cron.go is a minimal deterministic 5-field parser.
package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Cron is a parsed 5-field expression (minute hour dom month dow).
type Cron struct {
	min, hour, dom, month, dow map[int]bool
	domStar, dowStar           bool
}

type fieldSpec struct {
	name     string
	min, max int
}

var fields = []fieldSpec{
	{"minute", 0, 59}, {"hour", 0, 23}, {"dom", 1, 31}, {"month", 1, 12}, {"dow", 0, 7},
}

func Parse(expr string) (Cron, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return Cron{}, fmt.Errorf("cron: want 5 fields, got %d in %q", len(parts), expr)
	}
	sets := make([]map[int]bool, 5)
	stars := make([]bool, 5)
	for i, f := range fields {
		set, star, err := parseField(parts[i], f)
		if err != nil {
			return Cron{}, err
		}
		sets[i], stars[i] = set, star
	}
	return Cron{min: sets[0], hour: sets[1], dom: sets[2], month: sets[3], dow: sets[4],
		domStar: stars[2], dowStar: stars[4]}, nil
}

func parseField(s string, f fieldSpec) (map[int]bool, bool, error) {
	out := map[int]bool{}
	star := false
	for _, part := range strings.Split(s, ",") {
		step := 1
		rng := part
		if i := strings.Index(part, "/"); i >= 0 {
			var err error
			step, err = strconv.Atoi(part[i+1:])
			if err != nil || step <= 0 {
				return nil, false, fmt.Errorf("cron %s: bad step %q", f.name, part)
			}
			rng = part[:i]
		}
		lo, hi := f.min, f.max
		switch {
		case rng == "*":
			star = true
		case strings.Contains(rng, "-"):
			ab := strings.SplitN(rng, "-", 2)
			a, err1 := strconv.Atoi(ab[0])
			b, err2 := strconv.Atoi(ab[1])
			if err1 != nil || err2 != nil {
				return nil, false, fmt.Errorf("cron %s: bad range %q", f.name, part)
			}
			lo, hi = a, b
		default:
			v, err := strconv.Atoi(rng)
			if err != nil {
				return nil, false, fmt.Errorf("cron %s: bad value %q", f.name, part)
			}
			lo, hi = v, v
		}
		if lo < f.min || hi > f.max || lo > hi {
			return nil, false, fmt.Errorf("cron %s: %q out of range [%d,%d]", f.name, part, f.min, f.max)
		}
		for v := lo; v <= hi; v += step {
			out[v] = true
		}
	}
	return out, star, nil
}

// Next returns the first firing strictly after `after` (truncated to the
// minute), or ok=false if none within ~4 years.
func (c Cron) Next(after time.Time) (time.Time, bool) {
	t := after.Truncate(time.Minute).Add(time.Minute)
	const cap = 4 * 366 * 24 * 60
	for i := 0; i < cap; i++ {
		if c.matches(t) {
			return t, true
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, false
}

func (c Cron) matches(t time.Time) bool {
	if !c.min[t.Minute()] || !c.hour[t.Hour()] || !c.month[int(t.Month())] {
		return false
	}
	dom := c.dom[t.Day()]
	dow := c.dow[int(t.Weekday())] // Sunday=0
	if t.Weekday() == time.Sunday {
		dow = dow || c.dow[7]
	}
	// Standard cron day semantics: OR when both restricted, else AND.
	if !c.domStar && !c.dowStar {
		return dom || dow
	}
	return dom && dow
}
