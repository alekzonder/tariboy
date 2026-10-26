package mail

import (
	"encoding/json"
	"net/http"
)

// toolBody is the JSON body of an agent tool action. Identity never comes from
// the body: the sender and the mailbox are the calling agent.
type toolBody struct {
	All      bool           `json:"all"`
	Limit    int            `json:"limit"`
	Before   string         `json:"before"`
	ID       string         `json:"id"`
	To       string         `json:"to"`
	Subject  string         `json:"subject"`
	Text     string         `json:"text"`
	Kind     string         `json:"kind"`
	Data     map[string]any `json:"data"`
	Deadline string         `json:"deadline"`
	Notice   bool           `json:"notice"`
	Ask      bool           `json:"ask"`
}

// AgentAction runs one /tools/mail/<action> call of agent (mailbox design §3):
// inbox, sent, archive, show, send, reply and requeue. iterationID, when set,
// is recorded on a sent letter.
func (m *Mail) AgentAction(agent, iterationID, action string, body map[string]any) (any, error) {
	var in toolBody
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, errInvalid("mail %s: %v", action, err)
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, errInvalid("mail %s: %v", action, err)
	}
	me := agentPrincipal(agent)
	switch action {
	case "inbox":
		if in.All {
			return m.ListBox(BoxAll, me, in.Limit, in.Before)
		}
		return m.ListBox(BoxInbox, me, in.Limit, in.Before)
	case BoxSent, BoxArchive:
		return m.ListBox(action, me, in.Limit, in.Before)
	case "show":
		return m.Thread(me, in.ID)
	case "send":
		letters, err := m.Send(Letter{From: me, To: in.To, Subject: in.Subject, Body: in.Text, Kind: in.Kind,
			Data: in.Data, Deadline: in.Deadline, NeedsReply: !in.Notice, ProducedInIteration: iterationID})
		if err != nil {
			return nil, err
		}
		return MailSendResult{Letters: letters}, nil
	case "reply":
		reply, closed, err := m.Reply(ReplyRequest{As: me, ID: in.ID, Text: in.Text, Ask: in.Ask, Iteration: iterationID})
		if err != nil {
			return nil, err
		}
		return MailReplyResult{Letter: reply, Closed: closed}, nil
	case "requeue":
		l, err := m.Requeue(me, in.ID)
		if err != nil {
			return nil, err
		}
		return MailRequeueResult{Letter: l}, nil
	}
	return nil, newError(http.StatusNotFound, "not_found", "unknown mail action %q", action)
}
