package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

var updateDoc = flag.Bool("update", false, "rewrite the Operator commands table in docs/docs/reference/commands.md")

const commandsDoc = "../../docs/docs/reference/commands.md"

// TestCommandsDocMatchesHelpJSON pins the "Operator commands" table to the
// `tariboy --help-json` tree. Regenerate it with
// `go test ./cmd/tariboy -run TestCommandsDocMatchesHelpJSON -update`.
func TestCommandsDocMatchesHelpJSON(t *testing.T) {
	isolate(t)
	out, errOut, code := runCLI(t, "--help-json")
	if code != 0 {
		t.Fatalf("--help-json: code=%d stderr=%q", code, errOut)
	}
	var tree map[string]any
	if err := json.Unmarshal([]byte(out), &tree); err != nil {
		t.Fatal(err)
	}
	want := operatorTable(tree)

	doc, err := os.ReadFile(commandsDoc)
	if err != nil {
		t.Fatal(err)
	}
	start, end, err := operatorTableBounds(doc)
	if err != nil {
		t.Fatal(err)
	}
	if *updateDoc {
		updated := append(append(append([]byte{}, doc[:start]...), want...), doc[end:]...)
		if err := os.WriteFile(commandsDoc, updated, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if got := string(doc[start:end]); got != want {
		t.Fatalf("%s Operator commands table differs from --help-json; regenerate with\n"+
			"  go test ./cmd/tariboy -run TestCommandsDocMatchesHelpJSON -update\n--- doc\n%s\n--- help-json\n%s", commandsDoc, got, want)
	}
}

// operatorTableBounds returns the byte range of the table rows under the
// "## Operator commands" heading, header row included.
func operatorTableBounds(doc []byte) (int, int, error) {
	heading := bytes.Index(doc, []byte("\n## Operator commands\n"))
	if heading < 0 {
		return 0, 0, fmt.Errorf("no Operator commands heading")
	}
	start := bytes.Index(doc[heading:], []byte("\n| Command |"))
	if start < 0 {
		return 0, 0, fmt.Errorf("no table under Operator commands")
	}
	start += heading + 1
	end := start
	for end < len(doc) && doc[end] == '|' {
		next := bytes.IndexByte(doc[end:], '\n')
		if next < 0 {
			return 0, 0, fmt.Errorf("unterminated table")
		}
		end += next + 1
	}
	return start, end, nil
}

// operatorTable renders one row per command of the --help-json tree, sorted
// by command, with its required positional arguments as NAME and a rest
// argument as [NAME...]. Optional arguments are flags in practice and are left
// to the command's --help.
func operatorTable(tree map[string]any) string {
	var rows []string
	var walk func(prefix string, node map[string]any)
	walk = func(prefix string, node map[string]any) {
		if args, ok := node["args"].([]any); ok {
			usage := prefix
			for _, raw := range args {
				arg := raw.(map[string]any)
				if arg["flag"] != "" {
					continue
				}
				name := strings.ToUpper(strings.ReplaceAll(arg["name"].(string), "_", "-"))
				if arg["rest"] == true {
					usage += " [" + name + "...]"
				} else if arg["required"] == true {
					usage += " " + name
				}
			}
			rows = append(rows, fmt.Sprintf("| `%s` | %s |", usage, escapeCell(node["summary"].(string))))
			return
		}
		for key, child := range node {
			if m, ok := child.(map[string]any); ok {
				walk(prefix+" "+key, m)
			}
		}
	}
	walk("tariboy", tree)
	sort.Strings(rows)
	return "| Command | Summary |\n| --- | --- |\n" + strings.Join(rows, "\n") + "\n"
}

func escapeCell(s string) string {
	return strings.NewReplacer("|", `\|`, "<", `\<`, ">", `\>`).Replace(s)
}
