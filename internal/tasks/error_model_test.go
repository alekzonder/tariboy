package tasks

import (
	"fmt"
	"net/http"
	"testing"
)

func TestErrorStatusAndCodeSurviveWrapping(t *testing.T) {
	t.Parallel()
	wrapped := fmt.Errorf("x: %w", newError(http.StatusNotFound, "run_not_found", "missing"))
	if got := ErrorStatus(wrapped); got != http.StatusNotFound {
		t.Fatalf("ErrorStatus = %d, want 404", got)
	}
	if got := ErrorCode(wrapped); got != "run_not_found" {
		t.Fatalf("ErrorCode = %q", got)
	}
	if got := ErrorStatus(fmt.Errorf("plain")); got != http.StatusInternalServerError {
		t.Fatalf("plain ErrorStatus = %d", got)
	}
}

func TestNotFoundAndWithData(t *testing.T) {
	t.Parallel()
	err := notFound("queue_not_found", "queue not found").WithData(map[string]any{"k": 1})
	if err.Status != http.StatusNotFound || err.Code != "queue_not_found" || err.Data["k"] != 1 {
		t.Fatalf("unexpected error %+v", err)
	}
}
