package mail

// Results of the mail operations. The operator commands and the agent tool
// routes return the same shapes, so `tmail --json` reads one format in both
// modes; the names are the OpenAPI component names.

// MailList is one mailbox view (mail.ls, tool inbox/sent/archive).
// NextBefore is the before value of the next page, or "" after the last one.
type MailList struct {
	Box        string   `json:"box"`
	As         string   `json:"as"`
	Letters    []Letter `json:"letters"`
	Count      int      `json:"count"`
	NextBefore string   `json:"next_before"`
}

// MailThread is one letter and its reply, if any (mail.show).
type MailThread struct {
	Letter Letter  `json:"letter"`
	Reply  *Letter `json:"reply,omitempty"`
}

// MailSendResult is the letters one send wrote, one per recipient (mail.send).
type MailSendResult struct {
	Letters []Letter `json:"letters"`
}

// MailReplyResult is the reply and whether it closed the letter (mail.reply).
type MailReplyResult struct {
	Letter Letter `json:"letter"`
	Closed bool   `json:"closed"`
}

// MailCloseResult is the archived letter (mail.close).
type MailCloseResult struct {
	Letter Letter `json:"letter"`
}

// MailReadResult counts the notices a read closed and the letters read for
// the first time (mail.read).
type MailReadResult struct {
	ClosedNotices int `json:"closed_notices"`
	Read          int `json:"read"`
}

// MailRequeueResult is the letter back in the inbox (mail.requeue).
type MailRequeueResult struct {
	Letter Letter `json:"letter"`
}

// MailClearResult counts the open letters a clear deleted (agent.mail.clear).
type MailClearResult struct {
	Deleted int `json:"deleted"`
}

// ListBox returns the MailList of one mailbox view.
func (m *Mail) ListBox(box, as string, limit int, before string) (MailList, error) {
	letters, next, err := m.List(box, as, limit, before)
	if err != nil {
		return MailList{}, err
	}
	return MailList{Box: box, As: as, Letters: letters, Count: len(letters), NextBefore: next}, nil
}

// Thread returns the MailThread of letter id as seen by as.
func (m *Mail) Thread(as, id string) (MailThread, error) {
	l, reply, err := m.Show(as, id)
	if err != nil {
		return MailThread{}, err
	}
	return MailThread{Letter: l, Reply: reply}, nil
}
