package plugins

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/mail"
)

// Sender is the slice of the mail service the plugin API needs.
type Sender interface {
	Send(mail.Letter) ([]mail.Letter, error)
}

// API is the plugin-facing daemon surface: a plugin that declares mail.send
// sends letters as plugin:<name>, authenticated by its plugin-token.
type API struct {
	tokens *TokenRegistry
	mail   Sender
	log    *slog.Logger
	audit  func(plugin, action, detail string)
}

func NewAPI(tokens *TokenRegistry, m Sender, log *slog.Logger, audit func(plugin, action, detail string)) *API {
	if log == nil {
		log = slog.Default()
	}
	if audit == nil {
		audit = func(string, string, string) {}
	}
	return &API{tokens: tokens, mail: m, log: log, audit: audit}
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/plugin/mail":
		a.send(w, r)
	default:
		api.WriteErr(w, http.StatusNotFound, "not_found", "unknown plugin route "+r.Method+" "+r.URL.Path)
	}
}

// token extracts the plugin-token from Authorization: Bearer or X-Plugin-Token.
func token(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	return strings.TrimSpace(r.Header.Get("X-Plugin-Token"))
}

func (a *API) auth(w http.ResponseWriter, r *http.Request) (Identity, bool) {
	id, ok := a.tokens.Resolve(token(r))
	if !ok {
		api.WriteErr(w, http.StatusUnauthorized, "unauthorized", "invalid plugin token")
		return Identity{}, false
	}
	return id, true
}

// send is POST /api/plugin/mail {to, subject, text, kind, data, notice,
// idempotency_key}: the letter goes from plugin:<name>, a request unless
// notice is set. A letter stored dead because the recipient's inbox is full
// answers 409 inbox_full, so the plugin can tell its user.
func (a *API) send(w http.ResponseWriter, r *http.Request) {
	id, ok := a.auth(w, r)
	if !ok {
		return
	}
	if !id.Send {
		api.WriteErr(w, http.StatusForbidden, "forbidden", "plugin "+id.Name+" does not declare mail.send")
		return
	}
	var body struct {
		To             string         `json:"to"`
		Subject        string         `json:"subject"`
		Text           string         `json:"text"`
		Kind           string         `json:"kind"`
		Data           map[string]any `json:"data"`
		Notice         bool           `json:"notice"`
		IdempotencyKey string         `json:"idempotency_key"`
	}
	if !api.DecodeJSONBody(w, r, maxPluginBodyBytes, &body) {
		return
	}
	if len(body.IdempotencyKey) > 256 {
		api.WriteErr(w, http.StatusBadRequest, "bad_idempotency_key", "idempotency_key is too long")
		return
	}
	letters, err := a.mail.Send(mail.Letter{
		From: "plugin:" + id.Name, To: body.To, Subject: body.Subject, Body: body.Text, Kind: body.Kind,
		Data: body.Data, NeedsReply: !body.Notice, IdempotencyKey: pluginIdempotencyKey(id.Name, body.IdempotencyKey),
	})
	if err != nil {
		api.WriteError(w, a.log, "plugin "+id.Name+" mail", err)
		return
	}
	for _, l := range letters {
		if l.DeadReason == mail.DeadQueueLimit {
			api.WriteErrData(w, http.StatusConflict, "inbox_full", "the inbox of "+l.To+" is full; the letter is stored dead",
				map[string]any{"letter": l})
			return
		}
	}
	a.audit(id.Name, "mail", body.To)
	api.WriteOK(w, mail.MailSendResult{Letters: letters})
}

// maxPluginBodyBytes bounds a plugin's letter.
const maxPluginBodyBytes = 1 << 20

func pluginIdempotencyKey(plugin, key string) string {
	if key == "" {
		return ""
	}
	return "plugin:" + plugin + ":" + key
}
