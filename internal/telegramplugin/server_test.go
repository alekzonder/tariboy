package telegramplugin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerHealthyDisabledStatusDoesNotExposeToken(t *testing.T) {
	state, err := OpenState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(state, nil)

	health := httptest.NewRecorder()
	server.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d", health.Code)
	}

	routes := httptest.NewRecorder()
	server.ServeHTTP(routes, httptest.NewRequest(http.MethodGet, "/routes", nil))
	if routes.Code != http.StatusOK {
		t.Fatalf("routes status = %d", routes.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(routes.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	status := body["status"].(map[string]any)
	if status["token_configured"] != false || status["allowlist_count"] != float64(0) {
		t.Fatalf("status = %#v", status)
	}
	if _, exposed := status["bot_token"]; exposed {
		t.Fatalf("status exposes token: %#v", status)
	}
}
