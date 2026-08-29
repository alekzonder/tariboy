package judge

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/schedule"
	"github.com/alekzonder/tariboy/internal/tasks"
)

const validAutomationJSON = `{
  "schema_version": 1,
  "enabled": true,
  "judge": {
    "lead": "summary-alpha",
    "workers": ["review-alpha", "review-beta"],
    "image_ref": "quality-gate:7"
  },
  "schedule": {"spec": "0 */3 * * *"},
  "targets": {
    "agents": ["maker-one", "maker-two"],
    "image_refs": ["maker:11", "maker:12"],
    "only_unprocessed": true
  }
}`

func validAutomationValidator() AutomationValidator {
	agents := map[string]bool{
		"summary-alpha": true, "review-alpha": true, "review-beta": true,
		"maker-one": true, "maker-two": true,
	}
	return AutomationValidator{
		Customer:    "operator",
		AgentExists: func(_ context.Context, name string) bool { return agents[name] },
		ImagePlugins: func(ref string) ([]string, error) {
			if ref == "quality-gate:7" {
				return []string{"llm-as-judge", "schedule", "tasks", "current-task", "messages", "loop"}, nil
			}
			if ref == "maker:11" || ref == "maker:12" {
				return nil, nil
			}
			return nil, errors.New("not found")
		},
		TargetImageUsed: func(_ context.Context, agents []string, ref string) bool {
			return len(agents) == 2 && strings.HasPrefix(ref, "maker:")
		},
	}
}

func TestAutomationValidationAcceptsNamesAndImagesOnlyFromJSON(t *testing.T) {
	parsed := ParseAutomation([]byte(validAutomationJSON))
	if len(parsed.Diagnostics) != 0 {
		t.Fatalf("parse diagnostics=%+v", parsed.Diagnostics)
	}
	result := validAutomationValidator().Validate(context.Background(), parsed.Config)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("validation diagnostics=%+v", result.Diagnostics)
	}
	if result.CanonicalJSON == "" || !strings.Contains(result.CanonicalJSON, `"summary-alpha"`) || !strings.Contains(result.CanonicalJSON, `"maker:12"`) {
		t.Fatalf("canonical json=%q", result.CanonicalJSON)
	}
}

func TestAutomationValidationReportsJSONPointerDiagnostics(t *testing.T) {
	raw := strings.Replace(validAutomationJSON, `"only_unprocessed": true`, `"only_unprocessed": true, "surprise": 1`, 1)
	parsed := ParseAutomation([]byte(raw))
	if len(parsed.Diagnostics) != 1 || parsed.Diagnostics[0].Path != "/targets/surprise" {
		t.Fatalf("diagnostics=%+v", parsed.Diagnostics)
	}

	parsed = ParseAutomation([]byte(validAutomationJSON))
	validator := validAutomationValidator()
	validator.Customer = ""
	result := validator.Validate(context.Background(), parsed.Config)
	if len(result.Diagnostics) == 0 || result.Diagnostics[0].Path != "/customer" {
		t.Fatalf("diagnostics=%+v", result.Diagnostics)
	}

	parsed = ParseAutomation([]byte(validAutomationJSON + `{}`))
	if len(parsed.Diagnostics) != 1 || parsed.Diagnostics[0].Message != "multiple JSON values are not allowed" {
		t.Fatalf("trailing diagnostics=%+v", parsed.Diagnostics)
	}
}

func TestAutomationRevisionRoundTrip(t *testing.T) {
	_, js := newJudgeStore(t)
	parsed := ParseAutomation([]byte(validAutomationJSON))
	validated := validAutomationValidator().Validate(context.Background(), parsed.Config)
	first, err := js.SaveAutomation(context.Background(), validated.CanonicalJSON)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != 1 || first.Hash == "" {
		t.Fatalf("revision=%+v", first)
	}
	got, err := js.ActiveAutomation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != first.Revision || got.Hash != first.Hash || got.CanonicalJSON != validated.CanonicalJSON {
		t.Fatalf("active=%+v want=%+v", got, first)
	}
}

func TestAutomationApplyCreatesQueuesAndOneRecurringSchedule(t *testing.T) {
	db, js := newJudgeStore(t)
	clock := func() time.Time { return time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC) }
	service := NewAutomationService(js, schedule.NewStore(db, clock), validAutomationValidator(), clock)
	var activated []string
	service.SetActivator(func(names []string) error { activated = append(activated, names...); return nil })

	first, err := service.Apply(context.Background(), []byte(validAutomationJSON))
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Apply(context.Background(), []byte(validAutomationJSON))
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision.Revision != second.Revision.Revision || first.Schedule.ID == "" || second.Schedule.ID == "" {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	for _, prefix := range []string{"JUDGE", "IMPROVE"} {
		var responsible string
		if err := db.DB.QueryRow(`SELECT responsible_agent FROM task_queues WHERE prefix=?`, prefix).Scan(&responsible); err != nil {
			t.Fatalf("queue %s: %v", prefix, err)
		}
		if prefix == "JUDGE" && responsible != "summary-alpha" {
			t.Fatalf("JUDGE responsible=%q", responsible)
		}
	}
	var schedules int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM schedules WHERE enabled=1 AND kind='cron'`).Scan(&schedules); err != nil || schedules != 1 {
		t.Fatalf("schedules=%d err=%v", schedules, err)
	}
	if strings.Join(activated, ",") != "summary-alpha,review-alpha,review-beta,summary-alpha,review-alpha,review-beta" {
		t.Fatalf("activated=%v", activated)
	}
}

func TestAutomationRunOnceUsesActiveRevisionAndLimit(t *testing.T) {
	db, js := newJudgeStore(t)
	clock := func() time.Time { return time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC) }
	service := NewAutomationService(js, schedule.NewStore(db, clock), validAutomationValidator(), clock)
	if _, err := service.Apply(context.Background(), []byte(validAutomationJSON)); err != nil {
		t.Fatal(err)
	}
	one, err := service.RunOnce(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if one.Kind != "oneshot" || !strings.Contains(one.MessageTemplate, `"limit":3`) || !strings.Contains(one.MessageTemplate, `"config_revision":1`) {
		t.Fatalf("oneshot=%+v", one)
	}
	var recurring int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM schedules WHERE enabled=1 AND kind='cron'`).Scan(&recurring); err != nil || recurring != 1 {
		t.Fatalf("recurring=%d err=%v", recurring, err)
	}
}

func TestAutomationBeginCreatesOneTaskAndOneRunPerDelivery(t *testing.T) {
	db, js := newJudgeStore(t)
	for _, name := range []string{"summary-alpha", "review-alpha", "review-beta"} {
		seedJudgeAgent(t, db.DB, name)
	}
	for i, name := range []string{"maker-one", "maker-two", "maker-one"} {
		id := []string{"target-one", "target-two", "target-three"}[i]
		seedTarget(t, db.DB, id, name, "done", time.Date(2026, 8, 20+i, 9, 0, 0, 0, time.UTC).Format(time.RFC3339))
		if _, err := db.DB.Exec(`UPDATE iterations SET image_ref=? WHERE id=?`, []string{"maker:11", "maker:12", "maker:12"}[i], id); err != nil {
			t.Fatal(err)
		}
	}
	clock := func() time.Time { return time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC) }
	service := NewAutomationService(js, schedule.NewStore(db, clock), validAutomationValidator(), clock)
	service.tasks = tasks.NewService(db.DB, "operator", clock)
	enqueued := ""
	service.enqueue = func(id string) { enqueued = id }
	if _, err := service.Apply(context.Background(), []byte(validAutomationJSON)); err != nil {
		t.Fatal(err)
	}

	first, err := service.Begin(context.Background(), "summary-alpha", "lead-iteration", 1, "delivery-1", 3)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Begin(context.Background(), "summary-alpha", "lead-iteration", 1, "delivery-1", 3)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.TaskKey == "" || first.RunID == "" || enqueued != first.RunID {
		t.Fatalf("first=%+v second=%+v enqueued=%q", first, second, enqueued)
	}
	var taskCount, runCount int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM tasks WHERE queue_prefix='JUDGE'`).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM judge_runs`).Scan(&runCount); err != nil {
		t.Fatal(err)
	}
	if taskCount != 1 || runCount != 1 {
		t.Fatalf("tasks=%d runs=%d", taskCount, runCount)
	}
}

func TestAutomationFinishCompletesLinkedTaskAndMentionsConfiguredCustomer(t *testing.T) {
	db, js := newJudgeStore(t)
	for _, name := range []string{"summary-alpha", "review-alpha", "review-beta"} {
		seedJudgeAgent(t, db.DB, name)
	}
	seedTarget(t, db.DB, "target-one", "maker-one", "done", "2026-08-20T09:00:00Z")
	if _, err := db.DB.Exec(`UPDATE iterations SET image_ref='maker:11' WHERE id='target-one'`); err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC) }
	service := NewAutomationService(js, schedule.NewStore(db, clock), validAutomationValidator(), clock)
	service.tasks = tasks.NewService(db.DB, "operator", clock)
	if _, err := service.Apply(context.Background(), []byte(validAutomationJSON)); err != nil {
		t.Fatal(err)
	}
	cycle, err := service.Begin(context.Background(), "summary-alpha", "lead-iteration", 1, "delivery-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Finish(context.Background(), cycle.RunID, "Three recommendations are ready."); err != nil {
		t.Fatal(err)
	}
	var status, body string
	if err := db.DB.QueryRow(`SELECT status FROM tasks WHERE task_key=?`, cycle.TaskKey).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := db.DB.QueryRow(`SELECT body FROM task_comments WHERE task_id=(SELECT id FROM tasks WHERE task_key=?) ORDER BY id DESC LIMIT 1`, cycle.TaskKey).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if status != tasks.StatusDone || !strings.Contains(body, "Three recommendations") || !strings.Contains(body, "@user:operator") {
		t.Fatalf("status=%q body=%q", status, body)
	}
}
