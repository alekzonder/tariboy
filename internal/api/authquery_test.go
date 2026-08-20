package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthQueryTokenScopedToEvents(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	h := AuthMiddleware("sekret", next)

	// SSE path with a query token → allowed (EventSource cannot set a header).
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/agents/foo/events?types=iteration&token=sekret", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("events?token= = %d, want 200", rr.Code)
	}

	// SSE path with a WRONG query token → 401.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/agents/foo/events?token=nope", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("events wrong token = %d, want 401", rr.Code)
	}

	// A NON-events path with a query token → still 401 (query token is scoped).
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/daemon/status?token=sekret", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("non-events ?token= = %d, want 401 (query token must be scoped to /events)", rr.Code)
	}

	// The header path still works on any route.
	rr = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/daemon/status", nil)
	req.Header.Set("Authorization", "Bearer sekret")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("header auth = %d, want 200", rr.Code)
	}

	// Terminal WS path with a query token → allowed (browser WebSocket cannot
	// set an Authorization header; mirrors the /events SSE carve-out).
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/agents/foo/terminal?cols=80&rows=24&token=sekret", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("terminal?token= = %d, want 200", rr.Code)
	}

	// Terminal WS path with a WRONG query token → 401.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/agents/foo/terminal?token=nope", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("terminal wrong token = %d, want 401", rr.Code)
	}

	// Tasks realtime uses a browser WebSocket too, so its one exact route
	// accepts the same query-token transport.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/api/tasks/ws?token=sekret", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("tasks ws?token= = %d, want 200", rr.Code)
	}
}
