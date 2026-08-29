package judge

import (
	"context"
	"errors"
	"strings"
	"testing"
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
