package store

import (
	"errors"
	"fmt"
	"time"
)

// TimeLayout is the one stored time format: UTC with exactly nine fractional
// digits. Every value has the same width and zone, so SQLite compares the
// text in the same order as the times, and ts/_at indexes serve range
// filters directly. RFC3339Nano strips trailing zeros instead: 10ns becomes
// ".00000001Z" and 11ns ".000000011Z", and "...011" sorts before "...01Z".
const TimeLayout = "2006-01-02T15:04:05.000000000Z"

// SQLNow is the SQL expression for the current time in TimeLayout. SQLite
// has millisecond precision, so the last six digits are always zero.
const SQLNow = "strftime('%Y-%m-%dT%H:%M:%f000000Z','now')"

// inputLayout is RFC 3339 with an optional fraction and any offset.
const inputLayout = time.RFC3339Nano

// ErrBadTime reports a value that is not in TimeLayout.
var ErrBadTime = errors.New("time is not in the stored format")

// FormatTime returns t in TimeLayout, converted to UTC.
func FormatTime(t time.Time) string {
	return t.UTC().Format(TimeLayout)
}

// ParseTime reads a stored time. It accepts only TimeLayout.
func ParseTime(s string) (time.Time, error) {
	t, err := time.Parse(TimeLayout, s)
	if err != nil || len(s) != len(TimeLayout) {
		return time.Time{}, fmt.Errorf("%w: %q", ErrBadTime, s)
	}
	return t.UTC(), nil
}

// ParseInputTime reads a time from outside the database: an API or CLI
// parameter, a worker report, a manifest. It accepts any RFC 3339 value and
// returns it in UTC.
func ParseInputTime(s string) (time.Time, error) {
	t, err := time.Parse(inputLayout, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// FormatInputTime formats t as RFC 3339 with its own offset and no fraction,
// for text people read or type: CLI output and help.
func FormatInputTime(t time.Time) string {
	return t.Format(time.RFC3339)
}

// NormalizeTimeValue rewrites any RFC 3339 value to TimeLayout. The empty
// string stays empty. A value that is not a time is returned unchanged with
// false.
func NormalizeTimeValue(v string) (string, bool) {
	if v == "" {
		return "", true
	}
	t, err := ParseInputTime(v)
	if err != nil {
		return v, false
	}
	return FormatTime(t), true
}

// timeColumns lists every column of the main database that holds a time in
// TimeLayout. The backfill migration and backup restore normalize these;
// TestTimeColumnsCoverSchema fails when a new time column is missing here.
var timeColumns = map[string][]string{
	"agents":                       {"created_at", "last_goal_delivery_at", "status_updated"},
	"ai_requests":                  {"ts"},
	"budgets":                      {"created_at"},
	"channels":                     {"created_at"},
	"chat_participants":            {"joined_at", "read_ts"},
	"chats":                        {"created_at", "archived_at"},
	"deliveries":                   {"delivered_at", "acked_at", "processed_at"},
	"eval_results":                 {"created_at"},
	"events":                       {"ts"},
	"groups":                       {"created_at"},
	"image_provenance":             {"built_at"},
	"image_source_snapshots":       {"created_at"},
	"iterations":                   {"started_at", "ended_at", "timeout_deadline", "hard_timeout_deadline", "timeout_triggered_at", "last_ai_request_at"},
	"messages":                     {"ts", "deadline"},
	"plugins":                      {"installed_at"},
	"proxy_rules":                  {"created_at"},
	"schedules":                    {"next_fire_at", "created_at"},
	"schema_migrations":            {"applied_at"},
	"script_result_outbox":         {"next_attempt_at", "published_at"},
	"script_runs":                  {"created_at", "started_at", "finished_at"},
	"scripts":                      {"created_at", "next_run_at"},
	"subscriptions":                {"created_at", "revoked_at"},
	"task_agent_pools":             {"created_at", "updated_at"},
	"task_artifacts":               {"created_at"},
	"task_comments":                {"created_at", "updated_at"},
	"task_events":                  {"created_at"},
	"task_idempotency":             {"created_at"},
	"task_key_aliases":             {"created_at"},
	"task_legacy_keys_migrated":    {"migrated_at"},
	"task_notification_outbox":     {"next_attempt_at", "published_at"},
	"task_notification_state":      {"read_at", "dismissed_at"},
	"task_queue_secrets":           {"updated_at"},
	"task_queue_source_items":      {"created_at"},
	"task_queue_source_runs":       {"started_at", "finished_at"},
	"task_queue_workflow_triggers": {"created_at", "updated_at"},
	"task_queue_workflows":         {"updated_at"},
	"task_queues":                  {"created_at", "updated_at"},
	"task_relations":               {"created_at"},
	"task_reminders":               {"activity_at", "sent_at"},
	"task_script_runs":             {"created_at", "started_at", "finished_at"},
	"task_status_visits":           {"entered_at", "left_at", "next_watch_at", "resumed_at"},
	"task_transition_requests":     {"created_at", "finished_at"},
	"task_waiting_for":             {"requested_at", "resolved_at"},
	"task_workflow_holders":        {"dispatched_at", "unavailable_since"},
	"task_workflow_images":         {"built_at"},
	"task_workflow_versions":       {"created_at", "updated_at", "published_at"},
	"tasks":                        {"created_at", "updated_at", "completed_at", "started_at"},
}

// IsTimeColumn reports whether table.column holds a stored time.
func IsTimeColumn(table, column string) bool {
	for _, c := range timeColumns[table] {
		if c == column {
			return true
		}
	}
	return false
}
