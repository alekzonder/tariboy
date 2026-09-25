package bus

// unansweredSQL is the answering obligation of one participant in one chat: a
// message another participant sent, at or after this participant joined, with no
// reply of this participant pointing at it.
//
// It is deliberately NOT the delivery queue. Processing a delivery satisfies the
// transport; answering satisfies the conversation. Keeping them apart is what
// stops a "processed" mark from passing itself off as an answer.
var unansweredSQL = `
	SELECT ` + messageColumns + `
	FROM (SELECT m.id, m.channel, m.ts, m.source, m.type, m.subject, m.text, m.data,
	             m.produced_by_agent, m.produced_in_iteration, m.produced_by_plugin,
	             m.kind, m.correlation_id, m.in_reply_to, m.reply_to, m.deadline
	      FROM messages m
	      JOIN chats c ON c.channel = m.channel
	      JOIN chat_participants p ON p.chat_id = c.id AND p.principal = ?
	      WHERE c.id = ?
	        AND m.sender <> ?
	        AND m.ts >= p.joined_at
	        AND NOT EXISTS (
	            SELECT 1 FROM messages r
	            WHERE r.channel = m.channel AND r.in_reply_to = m.id
	              AND r.sender = ?
	        ))
	ORDER BY ts, id`

// Unanswered is the queue of messages principal still owes an answer in chatID,
// oldest first. A principal that is not a participant, and a chat that does not
// exist, both have an empty queue.
func (b *Bus) Unanswered(chatID, principal string) ([]Message, error) {
	rows, err := b.db.Query(unansweredSQL, principal, chatID, principal, principal)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

// UnansweredForPrincipal is the same queue across every chat principal takes
// part in, keyed by chat id. A chat with nothing outstanding is absent, so the
// result is the whole answering obligation and nothing else.
func (b *Bus) UnansweredForPrincipal(principal string) (map[string][]Message, error) {
	rows, err := b.db.Query(`SELECT chat_id FROM chat_participants WHERE principal = ? ORDER BY chat_id`, principal)
	if err != nil {
		return nil, err
	}
	var ids []string
	func() {
		defer rows.Close()
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				return
			}
			ids = append(ids, id)
		}
		err = rows.Err()
	}()
	if err != nil {
		return nil, err
	}
	out := map[string][]Message{}
	for _, id := range ids {
		msgs, err := b.Unanswered(id, principal)
		if err != nil {
			return nil, err
		}
		if len(msgs) > 0 {
			out[id] = msgs
		}
	}
	return out, nil
}
