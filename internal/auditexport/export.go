// Package auditexport builds a sensitive, operator-requested audit archive.
package auditexport

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/aiproxy/session"
	"github.com/alekzonder/tariboy/internal/audit"
)

// WriteZIP writes audit.md for human review and audit.jsonl for lossless
// machine analysis. An empty iteration includes every retained iteration.
func WriteZIP(dst io.Writer, agentsDir, agent, iteration string) error {
	markdown, jsonl, err := build(agentsDir, agent, iteration)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(dst)
	if err := writeZipFile(zw, "audit.md", markdown); err != nil {
		_ = zw.Close()
		return err
	}
	if err := writeZipFile(zw, "audit.jsonl", jsonl); err != nil {
		_ = zw.Close()
		return err
	}
	return zw.Close()
}

// WriteMarkdown writes the same human-readable document included in WriteZIP.
func WriteMarkdown(dst io.Writer, agentsDir, agent, iteration string) error {
	markdown, _, err := build(agentsDir, agent, iteration)
	if err != nil {
		return err
	}
	_, err = io.WriteString(dst, markdown)
	return err
}

func build(agentsDir, agent, iteration string) (string, string, error) {
	layout := agentdir.New(agentsDir, agent)
	events, err := audit.ReadEvents(layout.AuditLog(), 0, 0)
	if err != nil {
		return "", "", err
	}
	if iteration != "" {
		filtered := events[:0]
		for _, event := range events {
			if event.IterationID == iteration {
				filtered = append(filtered, event)
			}
		}
		events = filtered
	}
	iterations, err := iterationIDs(events, iteration, layout.IterationsDir())
	if err != nil {
		return "", "", err
	}

	var markdown strings.Builder
	fmt.Fprintf(&markdown, "# Audit log — %s\n\n", agent)
	markdown.WriteString("> Sensitive export: may contain prompts, reasoning, commands, tool arguments/results, model responses, and user data.\n\n")
	var jsonl strings.Builder
	for _, event := range events {
		writeJSONLine(&jsonl, map[string]any{"record_type": "audit_event", "event": event})
	}
	for _, id := range iterations {
		fmt.Fprintf(&markdown, "## Iteration `%s`\n\n", id)
		for _, event := range events {
			if event.IterationID == id {
				writeEventMarkdown(&markdown, event)
			}
		}
		entries, readErr := session.ReadEntries(agentsDir, agent, id)
		if readErr != nil {
			return "", "", readErr
		}
		for index, entry := range entries {
			writeJSONLine(&jsonl, map[string]any{
				"record_type": "proxy_transcript", "iteration_id": id, "call_index": index,
				"meta": entry.Meta, "request": string(entry.Request), "response": string(entry.Response),
			})
		}
		writeCallsMarkdown(&markdown, session.Build(entries))
	}

	return markdown.String(), jsonl.String(), nil
}

func iterationIDs(events []audit.Event, selected, iterationsDir string) ([]string, error) {
	if selected != "" {
		if !safeIterationID(selected) {
			return nil, fmt.Errorf("unsafe iteration ID")
		}
		return []string{selected}, nil
	}
	seen := map[string]bool{}
	for _, event := range events {
		if safeIterationID(event.IterationID) {
			seen[event.IterationID] = true
		}
	}
	entries, err := os.ReadDir(iterationsDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() && safeIterationID(entry.Name()) {
			seen[entry.Name()] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func safeIterationID(value string) bool {
	return value != "" && value != "." && value != ".." && len(value) <= 255 &&
		!strings.ContainsAny(value, "/\\\x00")
}

func writeJSONLine(dst *strings.Builder, value any) {
	raw, err := json.Marshal(value)
	if err == nil {
		dst.Write(raw)
		dst.WriteByte('\n')
	}
}

func writeEventMarkdown(dst *strings.Builder, event audit.Event) {
	value := func(key string) string {
		if text, ok := event.Data[key].(string); ok {
			return text
		}
		return ""
	}
	label, detail := event.Type, value("message")
	switch event.Type {
	case "iteration_started":
		label, detail = "Started", value("trigger")
	case "iteration_finished", "iteration_done":
		label, detail = "Finished", value("status")
	case "status":
		label = "Status"
	case "launching_harness":
		label, detail = "Harness", value("harness")
	case "harness_output", "shim":
		label, detail = "Harness", value("line")
	}
	if detail == "" && len(event.Data) != 0 {
		raw, _ := json.Marshal(event.Data)
		detail = string(raw)
	}
	fmt.Fprintf(dst, "- `%s` **%s**", event.TS, label)
	if detail != "" {
		fmt.Fprintf(dst, " — %s", detail)
	}
	dst.WriteString("\n")
}

func writeCallsMarkdown(dst *strings.Builder, timeline session.SessionTimeline) {
	if len(timeline.Calls) == 0 {
		dst.WriteString("\n")
		return
	}
	dst.WriteString("\n### Agent activity\n\n")
	for _, call := range timeline.Calls {
		for _, message := range call.Delta {
			for _, block := range message.Blocks {
				writeBlockMarkdown(dst, block)
			}
		}
		for _, block := range call.Response.Blocks {
			writeBlockMarkdown(dst, block)
		}
		fmt.Fprintf(dst, "  - AI call: `%s`, %d→%d tokens, $%.4f, %d ms\n", call.Model, call.Usage.Input, call.Usage.Output, call.CostUSD, call.LatencyMs)
	}
	dst.WriteString("\n")
}

func writeBlockMarkdown(dst *strings.Builder, block session.Block) {
	label, detail := "Message", block.Text
	switch block.Type {
	case "thinking":
		label = "Thinking"
	case "tool_result":
		label = "Result"
	case "tool_use":
		label, detail = toolLabel(block.ToolName), toolDetail(block.Input)
	}
	if detail != "" {
		fmt.Fprintf(dst, "- **%s** — %s\n", label, detail)
	}
}

func toolLabel(name string) string {
	lower := strings.ToLower(name)
	switch lower {
	case "exec_command", "command_execution", "bash", "shell", "local_shell":
		return "Command"
	}
	if lower == "skill" || strings.HasSuffix(lower, "__skill") || strings.HasSuffix(lower, ".skill") {
		return "Skill"
	}
	if name == "" {
		return "Tool"
	}
	return "Tool `" + name + "`"
}

func toolDetail(raw json.RawMessage) string {
	var input map[string]any
	if json.Unmarshal(raw, &input) != nil {
		return string(raw)
	}
	for _, key := range []string{"cmd", "command", "skill", "name", "query", "path", "prompt"} {
		if value, ok := input[key].(string); ok && value != "" {
			return value
		}
		if key == "command" {
			if parts, ok := input[key].([]any); ok {
				command := make([]string, 0, len(parts))
				for _, part := range parts {
					text, ok := part.(string)
					if !ok {
						command = nil
						break
					}
					command = append(command, text)
				}
				if len(command) != 0 {
					return strings.Join(command, " ")
				}
			}
		}
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := map[string]any{}
	for _, key := range keys {
		ordered[key] = input[key]
	}
	encoded, _ := json.Marshal(ordered)
	return string(encoded)
}

func writeZipFile(zw *zip.Writer, name, content string) error {
	header := &zip.FileHeader{Name: filepath.ToSlash(name), Method: zip.Deflate}
	header.SetMode(0o600)
	file, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.WriteString(file, content)
	return err
}
