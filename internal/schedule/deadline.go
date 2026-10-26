package schedule

import (
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/alekzonder/tariboy/internal/mail"
)

// Deadlines arms the deadline of a mail request as a one-shot schedule keyed
// on the request letter's id. When it fires, the requester gets a notice from
// system:schedule, kind deadline, with data.request_id; the reply that closes
// the request cancels it in the same transaction.
type Deadlines struct{ Store *Store }

// ArmTx arms request's deadline. Only an agent requester has a schedule owner
// and a loop to wake, so a request from any other principal arms nothing.
func (d Deadlines) ArmTx(tx *sql.Tx, request mail.Letter) error {
	agent, ok := strings.CutPrefix(request.From, "agent:")
	if !ok {
		return nil
	}
	tmpl, err := json.Marshal(map[string]any{
		"kind": "deadline", "notice": true,
		"subject": "No reply by the deadline: " + request.Subject,
		"text":    "Your request " + request.ID + " to " + request.To + " has no reply by its deadline " + request.Deadline + ".",
		"data":    map[string]any{"request_id": request.ID},
	})
	if err != nil {
		return err
	}
	_, err = d.Store.AddTx(tx, Schedule{Agent: agent, Kind: "oneshot", Spec: request.Deadline,
		Recipient: request.From, MessageTemplate: string(tmpl), CorrelationID: request.ID})
	return err
}

// CancelTx cancels the deadline of request requestID, if one is armed.
func (d Deadlines) CancelTx(tx *sql.Tx, requestID string) error {
	return d.Store.CancelByCorrelationTx(tx, requestID)
}
