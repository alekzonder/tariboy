package scriptcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func decodeNumber(raw json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("bad response from daemon: %w", err)
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// formatValue is format_value of scripts.py: Go-style text for one value.
func formatValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "<nil>"
	case bool:
		if v {
			return "true"
		}
		return "false"
	case []any:
		items := make([]string, len(v))
		for i, item := range v {
			items[i] = formatValue(item)
		}
		return "[" + strings.Join(items, " ") + "]"
	case map[string]any:
		items := make([]string, 0, len(v))
		for _, key := range sortedKeys(v) {
			items = append(items, key+":"+formatValue(v[key]))
		}
		return "map[" + strings.Join(items, " ") + "]"
	case json.Number:
		// shortcut: the literal of the response, while Python prints its float()
		// (1e-06 for 0.000001); the script routes return only integers.
		return v.String()
	case string:
		return v
	}
	return fmt.Sprint(value)
}

// pyDumps is json.dumps(value, separators=(",", ":")) of Python: compact, the
// key order of the response, and ASCII-only strings.
func pyDumps(raw json.RawMessage) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var b strings.Builder
	if err := writePy(dec, &b); err != nil {
		return "", fmt.Errorf("bad response from daemon: %w", err)
	}
	return b.String(), nil
}

func writePy(dec *json.Decoder, b *strings.Builder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	switch t := tok.(type) {
	case json.Delim:
		return writePyContainer(dec, b, t)
	case string:
		b.WriteString(pyString(t))
	case json.Number:
		b.WriteString(t.String())
	case bool:
		fmt.Fprint(b, t)
	case nil:
		b.WriteString("null")
	}
	return nil
}

func writePyContainer(dec *json.Decoder, b *strings.Builder, open json.Delim) error {
	object := open == '{'
	b.WriteRune(rune(open))
	for i := 0; dec.More(); i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		if object {
			key, err := dec.Token()
			if err != nil {
				return err
			}
			b.WriteString(pyString(key.(string)) + ":")
		}
		if err := writePy(dec, b); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	if object {
		b.WriteByte('}')
	} else {
		b.WriteByte(']')
	}
	return nil
}

var pyShortEscapes = map[rune]string{'"': `\"`, '\\': `\\`, '\n': `\n`, '\r': `\r`, '\t': `\t`, '\b': `\b`, '\f': `\f`}

// pyString is a JSON string as Python's json.dumps writes it with
// ensure_ascii: every rune outside space..~ becomes \uXXXX in lower case.
func pyString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch short, ok := pyShortEscapes[r]; {
		case ok:
			b.WriteString(short)
		case r >= ' ' && r <= '~':
			b.WriteRune(r)
		case r > 0xFFFF:
			r -= 0x10000
			fmt.Fprintf(&b, `\u%04x\u%04x`, 0xD800+(r>>10), 0xDC00+(r&0x3FF))
		default:
			fmt.Fprintf(&b, `\u%04x`, r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
