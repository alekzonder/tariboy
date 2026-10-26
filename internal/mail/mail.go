// Package mail is the per-principal mailbox (mailbox design §1–2). A letter
// goes from one principal to another and is stored as one row per recipient
// in the mail table. A letter is a request (needs_reply) that its recipient
// answers, or a notice that its recipient reads. mail.go holds the types, the
// coded errors and the Directory seam; store.go the SQL, send.go Send,
// reply.go Reply and the other state changes, render.go the prompt drain and
// hooks.go the after-commit hooks.
package mail

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// maxAttempts caps prompt renders: a letter whose next render would be the
// sixth is marked dead with reason attempts instead.
const maxAttempts = 5

// Letter kinds the mail package itself treats specially.
const (
	KindMessage      = "message"
	KindTaskQuestion = "task.question"
)

// Dead reasons.
const (
	DeadAttempts    = "attempts"
	DeadQueueLimit  = "queue_limit"
	DeadDecodeError = "decode_error"
)

// CloseNoteRead is the close note of a notice its recipient has read.
const CloseNoteRead = "read"

// Mailbox views of List.
const (
	BoxInbox   = "inbox"
	BoxSent    = "sent"
	BoxArchive = "archive"
	// BoxAll is every letter to the principal, open and closed (tmail inbox --all).
	BoxAll = "all"
)

// Letter is one row of the mail table. The JSON field names are the fixed
// Letter JSON every API route, tool route and `tmail --json` returns.
//
// On Send, To may hold several principals separated by commas, and NeedsReply
// selects a request (true) or a notice (false). Deadline accepts a Go duration
// such as 5m or an RFC 3339 time and is stored as the absolute time.
type Letter struct {
	ID                  string         `json:"id"`
	Seq                 int64          `json:"seq"`
	From                string         `json:"from"`
	To                  string         `json:"to"`
	Subject             string         `json:"subject"`
	Body                string         `json:"body"`
	Kind                string         `json:"kind"`
	Data                map[string]any `json:"data"`
	NeedsReply          bool           `json:"needs_reply"`
	InReplyTo           string         `json:"in_reply_to"`
	Deadline            string         `json:"deadline"`
	CreatedAt           string         `json:"created_at"`
	ReadAt              string         `json:"read_at"`
	AnsweredAt          string         `json:"answered_at"`
	AnswerID            string         `json:"answer_id"`
	CloseNote           string         `json:"close_note"`
	Attempts            int            `json:"attempts"`
	DeadAt              string         `json:"dead_at"`
	DeadReason          string         `json:"dead_reason"`
	ProducedInIteration string         `json:"produced_in_iteration"`
	// IdempotencyKey is the producer retry guard, unique per recipient. It is
	// not part of the Letter JSON.
	IdempotencyKey string `json:"-"`
}

// Open reports whether the letter is still in its recipient's inbox.
func (l Letter) Open() bool { return l.AnsweredAt == "" && l.DeadAt == "" }

// ReplyRequest is one Reply. As must be the recipient of letter ID. Ask makes
// the reply a request instead of a notice. OnBehalf marks a reply the operator
// writes as another principal (data.on_behalf = "operator").
type ReplyRequest struct {
	As       string
	ID       string
	Text     string
	Ask      bool
	OnBehalf bool
	// Iteration is the iteration the reply was written in, if any.
	Iteration string
}

// Hint is the live refetch hint for one written row (the /api/mail/ws frame).
type Hint struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
	TS   string `json:"ts"`
}

// MailUnread counts the open letters of one principal: Unread is the open
// letters not read yet, and BySender groups every open letter by sender and
// kind, which is what the Desktop badges count.
type MailUnread struct {
	As       string        `json:"as"`
	Open     int           `json:"open"`
	Unread   int           `json:"unread"`
	BySender []SenderCount `json:"by_sender"`
}

// SenderCount is one group of Unread.BySender.
type SenderCount struct {
	Sender string `json:"sender"`
	Kind   string `json:"kind"`
	Count  int    `json:"count"`
}

// Directory answers the recipient and sender questions mail cannot answer
// from its own table, so mail never reads agent, plugin, queue or group
// tables. Mail calls it before it opens a write transaction.
type Directory interface {
	AgentExists(name string) (bool, error)
	// InboxCap is the agent's messages_max_queue; zero or less means no cap.
	InboxCap(agent string) (int, error)
	CustomerLogin() string
	PluginInstalled(name string) (bool, error)
	QueueExists(prefix string) (bool, error)
	// GroupMembers returns the agent names of a group and whether it exists.
	GroupMembers(group string) (members []string, exists bool, err error)
}

// Deadlines arms and cancels the one-shot schedule of a request deadline,
// keyed on the request letter id, inside the mail write transaction.
type Deadlines interface {
	ArmTx(tx *sql.Tx, request Letter) error
	CancelTx(tx *sql.Tx, requestID string) error
}

// Error is a mail failure with a stable code the API layer maps to a status.
type Error struct {
	Status int
	Code   string
	Msg    string
}

func (e *Error) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return e.Code
}

func newError(status int, code, format string, args ...any) *Error {
	return &Error{Status: status, Code: code, Msg: fmt.Sprintf(format, args...)}
}

func errUnknownRecipient(p string) error {
	return newError(http.StatusBadRequest, "unknown_recipient", "unknown recipient %q", p)
}

func errInvalid(format string, args ...any) error {
	return newError(http.StatusBadRequest, "invalid_argument", format, args...)
}

func errNotFound(id string) error {
	return newError(http.StatusNotFound, "not_found", "letter %q not found", id)
}

func errForbidden(as, id string) error {
	return newError(http.StatusForbidden, "forbidden", "%s is not the recipient of letter %s", as, id)
}

// ErrorCode returns the code of a mail Error, or "" for any other error.
func ErrorCode(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// ErrorStatus returns the HTTP status of a mail Error, or 500.
func ErrorStatus(err error) int {
	var e *Error
	if errors.As(err, &e) && e.Status != 0 {
		return e.Status
	}
	return http.StatusInternalServerError
}

// Mail is the mailbox service over the mail table.
type Mail struct {
	db    *sql.DB
	clock func() time.Time
	dir   Directory

	hookMu     sync.RWMutex
	wake       func(agents []string)
	pluginWake func(plugins []string)
	event      func(Hint)
	audit      func(agent, kind, dataJSON string)
	deadlines  Deadlines
}

// New returns the mailbox over st. clock defaults to time.Now.
func New(st *store.Store, clock func() time.Time, dir Directory) *Mail {
	if clock == nil {
		clock = time.Now
	}
	return &Mail{db: st.DB, clock: clock, dir: dir}
}

func (m *Mail) now() time.Time { return m.clock().UTC() }

// agentPrincipal accepts an agent name or an agent principal.
func agentPrincipal(agent string) string {
	if strings.HasPrefix(agent, "agent:") {
		return agent
	}
	return "agent:" + agent
}
