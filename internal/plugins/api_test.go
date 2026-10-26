package plugins

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

// capDirectory knows agent alice with an inbox cap of 2, the customer and the
// plugin echo.
type capDirectory struct{}

func (capDirectory) AgentExists(name string) (bool, error)       { return name == "alice", nil }
func (capDirectory) InboxCap(string) (int, error)                { return 2, nil }
func (capDirectory) CustomerLogin() string                       { return "customer" }
func (capDirectory) PluginInstalled(name string) (bool, error)   { return name == "echo", nil }
func (capDirectory) QueueExists(string) (bool, error)            { return false, nil }
func (capDirectory) GroupMembers(string) ([]string, bool, error) { return nil, false, nil }

func newAPI(t *testing.T) (*API, *mail.Mail, *TokenRegistry) {
	t.Helper()
	s := storetest.Open(t)
	m := mail.New(s, func() time.Time { return time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC) }, capDirectory{})
	reg := NewTokenRegistry(nil)
	a := NewAPI(reg, m, slog.New(slog.NewTextHandler(io.Discard, nil)), func(string, string, string) {})
	return a, m, reg
}

func postMail(a *API, token, body string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/plugin/mail", strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	a.ServeHTTP(rr, req)
	return rr
}

func TestPluginMailSendsAsThePlugin(t *testing.T) {
	t.Parallel()
	a, m, reg := newAPI(t)
	tok, _ := reg.Mint(Identity{Name: "echo", Send: true})
	rr := postMail(a, tok, `{"to":"agent:alice","subject":"hi","text":"hello","from":"agent:forged","idempotency_key":"update-9"}`)
	if rr.Code != 200 {
		t.Fatalf("send code = %d body=%s", rr.Code, rr.Body)
	}
	var env struct {
		Result mail.MailSendResult `json:"result"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil || len(env.Result.Letters) != 1 {
		t.Fatalf("send result = %s (%v)", rr.Body, err)
	}
	got := env.Result.Letters[0]
	if got.From != "plugin:echo" || got.To != "agent:alice" || !got.NeedsReply || got.Body != "hello" {
		t.Fatalf("letter = %+v", got)
	}
	// A plugin-local key deduplicates retries without colliding with other
	// plugins or daemon producers.
	if rr := postMail(a, tok, `{"to":"agent:alice","text":"duplicate","idempotency_key":"update-9"}`); rr.Code != 200 {
		t.Fatalf("idempotent retry code = %d body=%s", rr.Code, rr.Body)
	}
	if letters, _, _ := m.List(mail.BoxInbox, "agent:alice", 10, ""); len(letters) != 1 || letters[0].Body != "hello" {
		t.Fatalf("inbox after a retry = %+v", letters)
	}
	if rr := postMail(a, tok, `{"to":"agent:ghost","text":"x"}`); rr.Code != 400 || !strings.Contains(rr.Body.String(), "unknown_recipient") {
		t.Fatalf("unknown recipient = %d %s", rr.Code, rr.Body)
	}
}

func TestPluginMailRequiresTokenAndSendCapability(t *testing.T) {
	t.Parallel()
	a, _, reg := newAPI(t)
	if rr := postMail(a, "plg-bogus", `{"to":"agent:alice"}`); rr.Code != 401 {
		t.Fatalf("bad token code = %d", rr.Code)
	}
	if rr := postMail(a, "", `{"to":"agent:alice"}`); rr.Code != 401 {
		t.Fatalf("no token code = %d", rr.Code)
	}
	tok, _ := reg.Mint(Identity{Name: "echo"})
	if rr := postMail(a, tok, `{"to":"agent:alice","text":"x"}`); rr.Code != 403 {
		t.Fatalf("send without mail.send code = %d", rr.Code)
	}
}

// TestPluginMailInboxFullReturns409: at the recipient's cap the letter is
// stored dead with queue_limit and the route answers 409 inbox_full.
func TestPluginMailInboxFullReturns409(t *testing.T) {
	t.Parallel()
	a, m, reg := newAPI(t)
	tok, _ := reg.Mint(Identity{Name: "echo", Send: true})
	for i := range 2 {
		if rr := postMail(a, tok, `{"to":"agent:alice","text":"fits"}`); rr.Code != 200 {
			t.Fatalf("letter %d code = %d body=%s", i, rr.Code, rr.Body)
		}
	}
	rr := postMail(a, tok, `{"to":"agent:alice","text":"over the cap"}`)
	if rr.Code != 409 || !strings.Contains(rr.Body.String(), `"inbox_full"`) {
		t.Fatalf("over the cap = %d %s", rr.Code, rr.Body)
	}
	archive, _, err := m.List(mail.BoxArchive, "agent:alice", 10, "")
	if err != nil || len(archive) != 1 || archive[0].DeadReason != mail.DeadQueueLimit {
		t.Fatalf("archive = %+v, %v; want the dead letter", archive, err)
	}
}

func TestRemovedPluginRoutesAnswerNotFound(t *testing.T) {
	t.Parallel()
	a, _, reg := newAPI(t)
	tok, _ := reg.Mint(Identity{Name: "echo", Send: true})
	for _, route := range []string{"/api/plugin/publish", "/api/plugin/subscriptions", "/api/plugin/watches"} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", route, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		a.ServeHTTP(rr, req)
		if rr.Code != 404 {
			t.Fatalf("%s = %d; want 404", route, rr.Code)
		}
	}
}
