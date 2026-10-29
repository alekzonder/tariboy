package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// PrintHuman prints a command result for a person: one "key: value" line per
// top-level key, nested values as indented JSON.
func PrintHuman(raw json.RawMessage, out io.Writer) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err == nil {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			switch m[k].(type) {
			case map[string]any, []any:
				b, _ := json.MarshalIndent(m[k], "", "  ")
				fmt.Fprintf(out, "%s:\n%s\n", k, indentBlock(string(b)))
			default:
				fmt.Fprintf(out, "%s: %v\n", k, m[k])
			}
		}
		return
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		fmt.Fprintln(out, s)
		return
	}
	fmt.Fprintln(out, strings.Trim(string(raw), "\"\n"))
}

// indentBlock prefixes each line of s with two spaces for nested-value display.
func indentBlock(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = "  " + l
	}
	return strings.Join(lines, "\n")
}
