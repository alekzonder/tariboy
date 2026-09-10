// Package auditexport builds a sensitive, operator-requested audit archive.
package auditexport

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/aiproxy/session"
	"github.com/alekzonder/tariboy/internal/audit"
)

// WriteZIP writes audit.md for human review and audit.jsonl for lossless
// machine analysis. An empty iteration includes every retained iteration.
func WriteZIP(ctx context.Context, dst io.Writer, agentsDir, agent, iteration string) error {
	events, iterations, err := exportData(agentsDir, agent, iteration)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(dst)
	markdown, err := createZipFile(zw, "audit.md")
	if err != nil {
		_ = zw.Close()
		return err
	}
	if err := writeMarkdown(ctx, markdown, agentsDir, agent, events, iterations); err != nil {
		_ = zw.Close()
		return err
	}
	jsonl, err := createZipFile(zw, "audit.jsonl")
	if err != nil {
		_ = zw.Close()
		return err
	}
	if err := writeJSONL(ctx, jsonl, agentsDir, agent, events, iterations); err != nil {
		_ = zw.Close()
		return err
	}
	return zw.Close()
}

// WriteMarkdown writes the same human-readable document included in WriteZIP.
func WriteMarkdown(ctx context.Context, dst io.Writer, agentsDir, agent, iteration string) error {
	events, iterations, err := exportData(agentsDir, agent, iteration)
	if err != nil {
		return err
	}
	return writeMarkdown(ctx, dst, agentsDir, agent, events, iterations)
}

func exportData(agentsDir, agent, iteration string) ([]audit.Event, []string, error) {
	layout := agentdir.New(agentsDir, agent)
	events, err := audit.ReadEvents(layout.AuditLog(), 0, 0)
	if err != nil {
		return nil, nil, err
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
		return nil, nil, err
	}
	return events, iterations, nil
}

func writeJSONL(ctx context.Context, dst io.Writer, agentsDir, agent string, events []audit.Event, iterations []string) error {
	for _, event := range events {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := writeJSONLine(dst, map[string]any{"record_type": "audit_event", "event": event}); err != nil {
			return err
		}
	}
	for _, id := range iterations {
		if err := session.WalkEntries(ctx, agentsDir, agent, id, func(index int, entry aiproxy.TranscriptEntry) error {
			return writeJSONLine(dst, map[string]any{
				"record_type": "proxy_transcript", "iteration_id": id, "call_index": index,
				"meta": entry.Meta, "request": string(entry.Request), "response": string(entry.Response),
			})
		}); err != nil {
			return err
		}
	}
	return nil
}

func writeMarkdown(ctx context.Context, dst io.Writer, agentsDir, agent string, events []audit.Event, iterations []string) error {
	if _, err := fmt.Fprintf(dst, "# Audit log — %s\n\n", agent); err != nil {
		return err
	}
	if _, err := io.WriteString(dst, "> Sensitive export: may contain prompts, reasoning, commands, tool arguments/results, model responses, and user data.\n\n"); err != nil {
		return err
	}
	for _, id := range iterations {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(dst, "## Iteration `%s`\n\n", id); err != nil {
			return err
		}
		for _, event := range events {
			if event.IterationID == id {
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := writeEventMarkdown(dst, event); err != nil {
					return err
				}
			}
		}
		wroteCalls := false
		if err := session.WalkCalls(ctx, agentsDir, agent, id, func(call session.Call) error {
			if !wroteCalls {
				wroteCalls = true
				if _, err := io.WriteString(dst, "\n### Agent activity\n\n"); err != nil {
					return err
				}
			}
			return writeCallMarkdown(dst, call)
		}); err != nil {
			return err
		}
		if _, err := io.WriteString(dst, "\n"); err != nil {
			return err
		}
	}
	return nil
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

func writeJSONLine(dst io.Writer, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	_, err = dst.Write(append(raw, '\n'))
	return err
}

func writeEventMarkdown(dst io.Writer, event audit.Event) error {
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
		for _, field := range []struct{ label, value string }{{"image", value("image_ref")}, {"version", value("image_version")}, {"digest", value("image_digest")}} {
			if field.value != "" {
				detail += fmt.Sprintf("; %s: %s", field.label, field.value)
			}
		}
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
	if _, err := fmt.Fprintf(dst, "- `%s` **%s**", event.TS, label); err != nil {
		return err
	}
	if detail != "" {
		if _, err := fmt.Fprintf(dst, " — %s", detail); err != nil {
			return err
		}
	}
	_, err := io.WriteString(dst, "\n")
	return err
}

func writeCallMarkdown(dst io.Writer, call session.Call) error {
	for _, message := range call.Delta {
		for _, block := range message.Blocks {
			if err := writeBlockMarkdown(dst, block); err != nil {
				return err
			}
		}
	}
	for _, block := range call.Response.Blocks {
		if err := writeBlockMarkdown(dst, block); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(dst, "  - AI call: `%s`, %d→%d tokens, $%.4f, %d ms\n", call.Model, call.Usage.Input, call.Usage.Output, call.CostUSD, call.LatencyMs)
	return err
}

func writeBlockMarkdown(dst io.Writer, block session.Block) error {
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
		_, err := fmt.Fprintf(dst, "- **%s** — %s\n", label, detail)
		return err
	}
	return nil
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

func createZipFile(zw *zip.Writer, name string) (io.Writer, error) {
	header := &zip.FileHeader{Name: filepath.ToSlash(name), Method: zip.Deflate}
	header.SetMode(0o600)
	return zw.CreateHeader(header)
}
