package commands

import (
	"net/http"
	"strings"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/tasks"
)

// The operator mail commands (mailbox design §4). Every command acts as the
// principal in as, which defaults to the customer.

func requireMail(c *registry.Ctx) (*mail.Mail, error) {
	if c.Mail == nil {
		return nil, api.UserError{Status: http.StatusServiceUnavailable, Code: "unavailable", Msg: "mail is not available"}
	}
	return c.Mail, nil
}

// mailAs is the principal a command acts as: the as argument, or the customer.
func mailAs(c *registry.Ctx, p registry.Request) string {
	if as := p.Trimmed("as"); as != "" {
		return as
	}
	login := c.Operator
	if login == "" {
		login = tasks.DefaultCustomerLogin
	}
	return "user:" + login
}

var mailAsArg = registry.Arg{Name: "as", Flag: "as", Type: registry.String, Help: "principal to act as (default the customer)"}

// mailCommand builds one mail command whose handler gets the mail service and
// the principal it acts as.
func mailCommand(path, summary, method, route string, args []registry.Arg, run func(*mail.Mail, string, registry.Request) (any, error)) registry.Command {
	return registry.Command{
		Path: path, Summary: summary, Args: append(args, mailAsArg),
		HTTP: &registry.HTTPRoute{Method: method, Path: route},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			m, err := requireMail(c)
			if err != nil {
				return nil, err
			}
			return run(m, mailAs(c, p), p)
		},
	}
}

func mailCommands() []registry.Command {
	idArg := registry.Arg{Name: "id", Type: registry.String, Required: true, Help: "letter id"}
	return []registry.Command{
		registry.WithResult[mail.MailList](mailCommand("mail.ls", "List one mailbox: inbox, sent or archive", "GET", "/api/mail",
			[]registry.Arg{
				{Name: "box", Flag: "box", Type: registry.String, Default: mail.BoxInbox, Help: "inbox, sent or archive"},
				{Name: "limit", Flag: "limit", Type: registry.Int, Help: "most letters to return (default 100)"},
				{Name: "before", Flag: "before", Type: registry.String, Help: "created_at to page before (sent and archive)"},
			},
			func(m *mail.Mail, as string, p registry.Request) (any, error) {
				box := p.Trimmed("box")
				if box == "" {
					box = mail.BoxInbox
				}
				return m.ListBox(box, as, p.Int("limit"), p.Trimmed("before"))
			})),
		registry.WithResult[mail.MailThread](mailCommand("mail.show", "Show one letter and its reply", "GET", "/api/mail/{id}",
			[]registry.Arg{idArg},
			func(m *mail.Mail, as string, p registry.Request) (any, error) {
				return m.Thread(as, p.Trimmed("id"))
			})),
		registry.WithResult[mail.MailSendResult](mailCommand("mail.send", "Send a letter: a request unless notice is set", "POST", "/api/mail",
			[]registry.Arg{
				{Name: "to", Flag: "to", Type: registry.String, Repeatable: true, Required: true, Help: "recipient principals, comma separated"},
				{Name: "subject", Flag: "subject", Type: registry.String, Help: "subject"},
				{Name: "text", Flag: "text", Type: registry.String, Help: "body"},
				{Name: "kind", Flag: "kind", Type: registry.String, Help: "kind (default message)"},
				{Name: "data", Flag: "data", Type: registry.JSONObject, Help: "structured data object"},
				{Name: "deadline", Flag: "deadline", Type: registry.String, Help: "reply deadline: a duration such as 5m or an RFC 3339 time"},
				{Name: "notice", Flag: "notice", Type: registry.Bool, Help: "send a notice that needs no reply"},
			},
			func(m *mail.Mail, as string, p registry.Request) (any, error) {
				var data map[string]any
				if p.Has("data") {
					if err := p.Decode("data", &data); err != nil {
						return nil, api.UserError{Code: "invalid_argument", Msg: "data must be a JSON object"}
					}
				}
				letters, err := m.Send(mail.Letter{From: as, To: strings.Join(p.Strings("to"), ","),
					Subject: p.String("subject"), Body: p.String("text"), Kind: p.Trimmed("kind"),
					Data: data, Deadline: p.Trimmed("deadline"), NeedsReply: !p.Bool("notice")})
				if err != nil {
					return nil, err
				}
				return mail.MailSendResult{Letters: letters}, nil
			})),
		registry.WithResult[mail.MailReplyResult](mailCommand("mail.reply", "Reply to a letter, which closes it", "POST", "/api/mail/{id}/reply",
			[]registry.Arg{idArg,
				{Name: "text", Flag: "text", Type: registry.String, Required: true, Help: "reply body"},
				{Name: "ask", Flag: "ask", Type: registry.Bool, Help: "make the reply a request"},
			},
			func(m *mail.Mail, as string, p registry.Request) (any, error) {
				// A reply as an agent is the operator writing on its behalf.
				reply, closed, err := m.Reply(mail.ReplyRequest{As: as, ID: p.Trimmed("id"), Text: p.String("text"),
					Ask: p.Bool("ask"), OnBehalf: strings.HasPrefix(as, "agent:")})
				if err != nil {
					return nil, err
				}
				return mail.MailReplyResult{Letter: reply, Closed: closed}, nil
			})),
		registry.WithResult[mail.MailCloseResult](mailCommand("mail.close", "Archive a letter without a reply", "POST", "/api/mail/{id}/close",
			[]registry.Arg{idArg, {Name: "note", Flag: "note", Type: registry.String, Help: "archive note"}},
			func(m *mail.Mail, as string, p registry.Request) (any, error) {
				l, err := m.Close(as, p.Trimmed("id"), p.String("note"))
				if err != nil {
					return nil, err
				}
				return mail.MailCloseResult{Letter: l}, nil
			})),
		registry.WithResult[mail.MailReadResult](mailCommand("mail.read", "Mark letters read; a read notice closes", "POST", "/api/mail/read",
			[]registry.Arg{{Name: "ids", Flag: "id", Type: registry.String, Repeatable: true, Required: true, Help: "letter ids"}},
			func(m *mail.Mail, as string, p registry.Request) (any, error) {
				closed, read, err := m.MarkRead(as, p.Strings("ids"))
				if err != nil {
					return nil, err
				}
				return mail.MailReadResult{ClosedNotices: closed, Read: read}, nil
			})),
		registry.WithResult[mail.MailRequeueResult](mailCommand("mail.requeue", "Return a dead letter to the inbox", "POST", "/api/mail/{id}/requeue",
			[]registry.Arg{idArg},
			func(m *mail.Mail, as string, p registry.Request) (any, error) {
				l, err := m.Requeue(as, p.Trimmed("id"))
				if err != nil {
					return nil, err
				}
				return mail.MailRequeueResult{Letter: l}, nil
			})),
		registry.WithResult[mail.MailUnread](mailCommand("mail.unread", "Count the open letters by sender and kind", "GET", "/api/mail/unread",
			nil,
			func(m *mail.Mail, as string, _ registry.Request) (any, error) {
				return m.Unread(as)
			})),
		registry.WithResult[mail.MailClearResult](registry.Command{
			Path:    "agent.mail.clear",
			Summary: "Delete the open letters in an agent's inbox",
			Args:    []registry.Arg{{Name: "name", Type: registry.String, Required: true, Help: "agent name"}},
			HTTP:    &registry.HTTPRoute{Method: "POST", Path: "/api/agents/{name}/mail/clear"},
			Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
				m, err := requireMail(c)
				if err != nil {
					return nil, err
				}
				a, err := getAgent(c, p.String("name"))
				if err != nil {
					return nil, err
				}
				n, err := m.ClearInbox(a.Name)
				if err != nil {
					return nil, err
				}
				return mail.MailClearResult{Deleted: n}, nil
			},
		}),
	}
}

// AgentMessagesBatch is an agent's letters per iteration after
// agent.messages.batch.
type AgentMessagesBatch struct {
	Name          string `json:"name"`
	MessagesBatch int    `json:"messages_batch"`
}

// AgentMessagesMaxQueue is an agent's open inbox cap after
// agent.messages.max-queue.
type AgentMessagesMaxQueue struct {
	Name             string `json:"name"`
	MessagesMaxQueue int    `json:"messages_max_queue"`
}
