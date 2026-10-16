package taskcli

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"
)

// printTasks prints a list page as a table, then the command for the next
// page when there is one. It reports false when raw is not a task page.
func printTasks(parsed request, raw json.RawMessage, stdout io.Writer) bool {
	var page struct {
		Tasks *[]struct {
			Key       string `json:"key"`
			Priority  string `json:"priority"`
			Status    string `json:"status"`
			Category  string `json:"category"`
			Assignee  string `json:"assignee"`
			UpdatedAt string `json:"updated_at"`
			Title     string `json:"title"`
			Blocked   bool   `json:"blocked"`
		} `json:"tasks"`
		NextCursor string `json:"next_cursor"`
	}
	if json.Unmarshal(raw, &page) != nil || page.Tasks == nil {
		return false
	}
	if len(*page.Tasks) == 0 {
		fmt.Fprintln(stdout, "no tasks match")
		return true
	}
	table := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "  KEY\tPRI\tSTATUS\tASSIGNEE\tUPDATED\tTITLE")
	for _, task := range *page.Tasks {
		mark := " "
		if task.Blocked {
			mark = "B"
		}
		status := task.Status
		if task.Category != "" && task.Category != task.Status {
			status += " (" + task.Category + ")"
		}
		updated := task.UpdatedAt
		if at, err := time.Parse(time.RFC3339Nano, updated); err == nil {
			updated = at.UTC().Format("2006-01-02 15:04")
		}
		fmt.Fprintf(table, "%s %s\t%s\t%s\t%s\t%s\t%s\n", mark, task.Key, task.Priority, status, dash(task.Assignee), dash(updated), cutTitle(task.Title))
	}
	table.Flush()
	if page.NextCursor != "" {
		fmt.Fprintf(stdout, "next: %s\n", nextListCommand(parsed, page.NextCursor))
	}
	return true
}

// cutTitle keeps a title on one line and at most 60 characters.
func cutTitle(title string) string {
	runes := []rune(strings.Join(strings.Fields(title), " "))
	if len(runes) > 60 {
		return string(runes[:59]) + "…"
	}
	return string(runes)
}

// nextListCommand repeats the command with --after replaced by the cursor.
func nextListCommand(parsed request, cursor string) string {
	words := []string{"ttasks", parsed.action}
	for i := 0; i < len(parsed.args); i++ {
		arg := parsed.args[i]
		if arg == "--after" {
			i++
			continue
		}
		if strings.HasPrefix(arg, "--after=") {
			continue
		}
		words = append(words, shellWord(arg))
	}
	return strings.Join(append(words, "--after", shellWord(cursor)), " ")
}

var plainShellWord = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

func shellWord(word string) string {
	if plainShellWord.MatchString(word) {
		return word
	}
	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}
