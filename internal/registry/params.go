package registry

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// Params are the arguments of one command call. Bind gives every declared
// argument its Go type, so the accessors below never fail; for an absent key
// or a value of another type they return the zero value. They also read the
// JSON and query forms of undeclared keys, which Bind passes through as sent.
type Params map[string]any

// String returns the value as sent, without trimming.
func (p Params) String(key string) string {
	s, _ := p[key].(string)
	return s
}

// Trimmed returns the string value without surrounding whitespace.
func (p Params) Trimmed(key string) string { return strings.TrimSpace(p.String(key)) }

// OptString returns nil when the key is absent or not a string, so a caller can
// tell an omitted value from an empty one.
func (p Params) OptString(key string) *string {
	s, ok := p[key].(string)
	if !ok {
		return nil
	}
	return &s
}

// Int returns the whole-number value, or zero.
func (p Params) Int(key string) int { return p.IntOr(key, 0) }

// Int64 is Int for 64-bit ids and revisions.
func (p Params) Int64(key string) int64 {
	n, _ := wholeNumber(p[key])
	return n
}

// IntOr returns def when the key is absent or not a whole number.
func (p Params) IntOr(key string, def int) int {
	n, ok := wholeNumber(p[key])
	if !ok {
		return def
	}
	return int(n)
}

// Bool returns true only for true, "true" and "1".
func (p Params) Bool(key string) bool {
	v, _ := p.OptBool(key)
	return v
}

// OptBool reports the boolean value and whether one was sent.
func (p Params) OptBool(key string) (bool, bool) {
	return boolValue(p[key])
}

// Strings reads a list sent as a JSON array or as a comma-separated string.
// Items are trimmed and empty items are dropped.
func (p Params) Strings(key string) []string {
	var items []string
	switch v := p[key].(type) {
	case string:
		items = strings.Split(v, ",")
	case []string:
		items = v
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				items = append(items, s)
			}
		}
	}
	var out []string
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// Has reports whether the key carries a value; JSON null counts as absent.
func (p Params) Has(key string) bool { return p[key] != nil }

// Decode converts the value under key into out through its JSON form.
func (p Params) Decode(key string, out any) error {
	raw, err := json.Marshal(p[key])
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func wholeNumber(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) || math.Trunc(n) != n || math.Abs(n) > math.MaxInt64 {
			return 0, false
		}
		return int64(n), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func boolValue(v any) (bool, bool) {
	switch b := v.(type) {
	case bool:
		return b, true
	case string:
		switch b {
		case "true", "1":
			return true, true
		case "false", "0":
			return false, true
		}
	}
	return false, false
}
