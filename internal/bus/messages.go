package bus

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"
)

// messages.go holds the Phase P processed/reply lifecycle (design §3.3, §4) and
// the watch fingerprint for parameterized subscriptions (§5.2). All mutable
// per-consumer state lives on `deliveries`; messages stay immutable facts.

// InboxItem is one message in an agent's inbox joined with its aggregated
// delivery state. An agent can receive a single message through several
// subscriptions; the delivery fields are aggregated across all of the agent's
// deliveries of that message so the inbox is per-message-per-agent.
type InboxItem struct {
	Message
	Attempts    int
	DeliveredAt string
	ProcessedAt string
	Result      string
	DLQ         bool
}

// Inbox returns an agent's messages filtered by delivery status, newest-first,
// with cursor paging. status is one of "pending", "processed", "dlq", "all".
// before, when non-empty, pages the archive: only messages strictly older than
// the `before` cursor message are returned. Message ids are channel-prefixed
// (store.go nextMessageID = channel+"-"+ts+"-"+seq), so they are NOT globally
// time-ordered when an inbox spans >1 channel; paging keys on the composite
// (ts, id) — matching the ORDER BY — so it stays globally time-ordered across
// channels. Rows are grouped per message id — an agent's several deliveries of
// one message collapse to a single item with aggregated state.
func (b *Bus) Inbox(agent, status string, limit int, before string) ([]InboxItem, error) {
	if limit <= 0 {
		limit = 100
	}
	// Status maps to a HAVING predicate over the aggregated delivery state.
	// MarkProcessed sets processed_at/acked_at/result on ALL of an agent's
	// deliveries of a message uniformly, so MAX() over them is exact. dlq is the
	// exception: it is set per-delivery by Pending(), so it can diverge across an
	// agent's several deliveries of one message. The message-level DLQ rule is
	// all-dead — MIN(d.dlq)=1 — so a message counts as dead only when EVERY
	// delivery is dead. That agrees with Pending(), which keeps draining any live
	// (dlq=0) delivery: a message with one live delivery stays pending, not dlq
	// (P8-F5). MAX(d.dlq) would have flagged the whole message dead on the first
	// partial dead-letter while Pending kept re-delivering it — split-brain.
	var having string
	switch status {
	case "", "all":
		having = ""
	case "pending":
		having = "HAVING MAX(d.acked_at) IS NULL AND MIN(d.dlq) = 0"
	case "processed":
		having = "HAVING MAX(d.processed_at) IS NOT NULL"
	case "dlq":
		having = "HAVING MIN(d.dlq) = 1"
	default:
		return nil, fmt.Errorf("inbox: unknown status %q", status)
	}
	args := []any{agent}
	cursor := ""
	if before != "" {
		// Composite (ts, id) cursor: ids alone are channel-prefixed and not
		// globally time-ordered, so key on (ts, id) — the same ordering the
		// ORDER BY below uses — to page strictly older rows across channels.
		cursor = "AND (m.ts, m.id) < (SELECT ts, id FROM messages WHERE id = ?)"
		args = append(args, before)
	}
	args = append(args, limit)
	q := `SELECT m.id, m.channel, m.ts, m.source, m.type, m.subject, m.text, m.data,
		m.produced_by_agent, m.produced_in_iteration, m.produced_by_plugin,
		m.kind, m.correlation_id, m.in_reply_to, m.reply_to, m.deadline,
		MAX(d.attempts), MAX(d.delivered_at), MAX(d.processed_at), MAX(d.result), MIN(d.dlq)
		FROM deliveries d
		JOIN subscriptions s ON s.id = d.subscription_id
		JOIN messages m ON m.id = d.message_id
		WHERE s.agent = ? ` + cursor + `
		GROUP BY m.id ` + having + `
		ORDER BY m.ts DESC, m.id DESC
		LIMIT ?`
	rows, err := b.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InboxItem
	for rows.Next() {
		it, err := scanInboxItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func scanInboxItem(row scanner) (InboxItem, error) {
	var it InboxItem
	var m Message
	var subject string
	var text, data, pa, pi, pp sql.NullString
	var kind string
	var cid, irt, rt, dl sql.NullString
	var da, at, res sql.NullString
	var attempts, dlq int
	if err := row.Scan(&m.ID, &m.Channel, &m.TS, &m.Source, &m.Type, &subject, &text, &data,
		&pa, &pi, &pp, &kind, &cid, &irt, &rt, &dl,
		&attempts, &da, &at, &res, &dlq); err != nil {
		if err == sql.ErrNoRows {
			return InboxItem{}, ErrNotFound
		}
		return InboxItem{}, err
	}
	unmarshalMessage(&m, subject, text, data, pa, pi, pp)
	m.Kind, m.CorrelationID, m.InReplyTo, m.ReplyTo, m.Deadline = kind, cid.String, irt.String, rt.String, dl.String
	it.Message = m
	it.Attempts = attempts
	it.DeliveredAt = da.String
	it.ProcessedAt = at.String
	it.Result = res.String
	it.DLQ = dlq != 0
	return it, nil
}

// getInboxItem fetches a single agent's message as an aggregated InboxItem,
// regardless of status. ErrNotFound if the agent has no delivery of it.
func (b *Bus) getInboxItem(agent, msgID string) (InboxItem, error) {
	row := b.db.QueryRow(`SELECT m.id, m.channel, m.ts, m.source, m.type, m.subject, m.text, m.data,
		m.produced_by_agent, m.produced_in_iteration, m.produced_by_plugin,
		m.kind, m.correlation_id, m.in_reply_to, m.reply_to, m.deadline,
		MAX(d.attempts), MAX(d.delivered_at), MAX(d.processed_at), MAX(d.result), MIN(d.dlq)
		FROM deliveries d
		JOIN subscriptions s ON s.id = d.subscription_id
		JOIN messages m ON m.id = d.message_id
		WHERE s.agent = ? AND m.id = ?
		GROUP BY m.id`, agent, msgID)
	return scanInboxItem(row)
}

// MarkProcessed marks every one of the agent's deliveries of a message processed
// with a mandatory result (§3.3). Empty result is rejected. It is idempotent:
// re-processing an already-processed message changes nothing and returns the
// existing record. Allowed on DLQ'd deliveries (operator archive cleanup).
// Emits a message_processed audit event only on the pending→processed
// transition, so an idempotent re-call is silent.
func (b *Bus) MarkProcessed(agent, msgID, result string) (InboxItem, error) {
	if strings.TrimSpace(result) == "" {
		return InboxItem{}, fmt.Errorf("mark processed: result is required")
	}
	now := b.clock().UTC().Format(time.RFC3339Nano)
	// Set processed_at only where still unprocessed, so a re-call is a no-op.
	// acked_at is set here too — explicit processing is the only acking path now.
	// Clear dlq as well: processing is a terminal state, so a DLQ'd delivery an
	// operator archives must land in the 'processed' tab only, never in both the
	// 'processed' and 'dlq' tabs at once (P8-F6).
	res, err := b.db.Exec(`UPDATE deliveries SET processed_at=?, result=?, dlq=0, acked_at=COALESCE(acked_at, ?)
		WHERE message_id=? AND processed_at IS NULL
		  AND subscription_id IN (SELECT id FROM subscriptions WHERE agent=?)`,
		now, result, now, msgID, agent)
	if err != nil {
		return InboxItem{}, err
	}
	item, err := b.getInboxItem(agent, msgID)
	if err != nil {
		return InboxItem{}, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		b.emitAudit(agent, "message_processed", auditJSON(map[string]string{
			"message_id": msgID, "result": result,
		}))
	}
	return item, nil
}

// Requeue returns a message to the pending queue on all of the agent's
// deliveries of it (§3.3): it clears dlq and resets attempts, and also clears
// acked_at/processed_at/result. Clearing the ack state is essential — Pending()
// drains only rows WHERE acked_at IS NULL, and MarkProcessed sets acked_at, so a
// process-then-requeue that left acked_at behind would show pending in the inbox
// yet never re-drain (P8-F3). ErrNotFound if the agent has no delivery of the
// message. Emits message_requeued.
func (b *Bus) Requeue(agent, msgID string) error {
	res, err := b.db.Exec(`UPDATE deliveries SET dlq=0, attempts=0, acked_at=NULL, processed_at=NULL, result=NULL
		WHERE message_id=? AND subscription_id IN (SELECT id FROM subscriptions WHERE agent=?)`,
		msgID, agent)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	b.emitAudit(agent, "message_requeued", auditJSON(map[string]string{"message_id": msgID}))
	return nil
}

// getMessage loads one immutable message row by id, threading fields included.
func (b *Bus) getMessage(id string) (Message, error) {
	rows, err := b.db.Query(`SELECT id, channel, ts, source, type, subject, text, data,
		produced_by_agent, produced_in_iteration, produced_by_plugin,
		kind, correlation_id, in_reply_to, reply_to, deadline FROM messages WHERE id=?`, id)
	if err != nil {
		return Message{}, err
	}
	msgs, err := scanMessages(rows)
	if err != nil {
		return Message{}, err
	}
	if len(msgs) == 0 {
		return Message{}, ErrNotFound
	}
	return msgs[0], nil
}

// messageAuthor returns the agent name that produced m, or "" if it was not
// produced by an agent. Prefers the explicit ProducedByAgent attribution and
// falls back to parsing an "agent:<name>" source. Used by the fan-out to
// exclude an author from receiving its own published message.
func messageAuthor(m Message) string {
	if m.ProducedByAgent != "" {
		return m.ProducedByAgent
	}
	if name, ok := strings.CutPrefix(m.Source, "agent:"); ok {
		return name
	}
	return ""
}

// replyTarget resolves the channel a reply to M lands on (§4.1 rule): explicit
// reply_to first, else the source agent's inbox, else the originating channel.
func replyTarget(m Message) string {
	if m.ReplyTo != "" {
		return m.ReplyTo
	}
	if strings.HasPrefix(m.Source, "agent:") {
		return m.Source + ":inbox"
	}
	return m.Channel
}

// Reply publishes a kind=reply answer to the message msgID on its resolved
// target channel (§4.1), threading it with in_reply_to and the exchange's
// correlation id (the original's, or the original id if it had none). type is
// carried from the original unless typeOverride is set. If actor is an agent
// that holds a delivery of the original, replying auto-processes it with result
// "replied: <reply id>". A wired deadline (a reply to a request) is cancelled.
// Returns the published reply. Emits message_replied.
// inheritSubject copies orig's subject for use on a reply, dropping the 'watch'
// key (per-subscription fingerprint, not per-reply). Returns nil for an
// empty/nil source so replies without routing keys stay subject-less. Reply-
// specific keys, if any, are overlaid by the caller and win on conflict.
func inheritSubject(orig map[string]any) map[string]any {
	if len(orig) == 0 {
		return nil
	}
	subject := make(map[string]any, len(orig))
	for k, v := range orig {
		if k == "watch" {
			continue
		}
		subject[k] = v
	}
	if len(subject) == 0 {
		return nil
	}
	return subject
}

func (b *Bus) Reply(actor, msgID, text string, data map[string]any, typeOverride string) (Message, error) {
	orig, err := b.getMessage(msgID)
	if err != nil {
		return Message{}, err
	}
	correlation := orig.CorrelationID
	if correlation == "" {
		correlation = orig.ID
	}
	typ := orig.Type
	if typeOverride != "" {
		typ = typeOverride
	}
	// The reply inherits every key of the original's subject except 'watch'
	// (the watch fingerprint is per-subscription, not per-reply). Sink plugins
	// route replies back to external systems via subject.chat_id/message_id and
	// have no bus read API, so the routing keys must ride along on the reply.
	subject := inheritSubject(orig.Subject)
	reply, err := b.Publish(Message{
		Channel:         replyTarget(orig),
		Type:            typ,
		Subject:         subject,
		Kind:            "reply",
		InReplyTo:       orig.ID,
		CorrelationID:   correlation,
		Text:            text,
		Data:            data,
		Source:          "agent:" + actor,
		ProducedByAgent: actor,
	})
	if err != nil {
		return Message{}, err
	}
	// Replying is handling: auto-process the actor's own delivery of the
	// original, if it has one. No delivery (e.g. an operator reply) → skip.
	if _, err := b.MarkProcessed(actor, orig.ID, "replied: "+reply.ID); err != nil && err != ErrNotFound {
		return Message{}, err
	}
	// A reply retires the request's deadline timeout, if one was armed.
	if b.cancelDeadline != nil {
		_ = b.cancelDeadline(correlation)
	}
	b.emitAudit(actor, "message_replied", auditJSON(map[string]string{
		"message_id": orig.ID, "reply_id": reply.ID, "correlation_id": correlation,
	}))
	return reply, nil
}

// Request publishes a kind=request message from fromAgent to targetChannel with
// a fresh correlation id (its own message id) and reply_to pointing at the
// requester's inbox (§4.2). With a non-empty deadline it arms a one-shot timeout
// (via the wired deadline hook) that fires a type=timeout event into the
// requester's inbox if no reply lands first. Returns the published request.
func (b *Bus) Request(fromAgent, targetChannel, text, deadline string) (Message, error) {
	// A deadline is only honest when a deadline hook is wired: without one, no
	// type=timeout event would ever fire and the requester could wait forever.
	// Reject --deadline up front (before publishing anything) rather than
	// silently no-op it. The full arming path lands with the schedule subsystem
	// (EPIC R); until then --deadline is not supported.
	if deadline != "" && b.armDeadline == nil {
		return Message{}, ErrDeadlineUnsupported
	}
	// Validate the deadline BEFORE publishing. armDeadline (which parses the
	// deadline) can only run after Publish because it keys the timeout on the
	// request's own id; so a malformed deadline must be caught here or it would
	// leave a live, timeout-less request in the member inbox while Request still
	// returns the parse error — a partial failure a retry would duplicate.
	if deadline != "" && b.validateDeadline != nil {
		if err := b.validateDeadline(deadline); err != nil {
			return Message{}, err
		}
	}
	req, err := b.Publish(Message{
		Channel:         targetChannel,
		Type:            "request",
		Kind:            "request",
		Text:            text,
		ReplyTo:         InboxChannel(fromAgent),
		Deadline:        deadline,
		Source:          "agent:" + fromAgent,
		ProducedByAgent: fromAgent,
	})
	if err != nil {
		return Message{}, err
	}
	// A fresh correlation id per request: use the request's own (unique) id, so
	// replies (correlation_id = orig.id) thread back and PendingRequests can
	// match the exchange by correlation id.
	if _, err := b.db.Exec(`UPDATE messages SET correlation_id=? WHERE id=?`, req.ID, req.ID); err != nil {
		return Message{}, err
	}
	req.CorrelationID = req.ID
	if deadline != "" && b.armDeadline != nil {
		if err := b.armDeadline(fromAgent, req.CorrelationID, deadline); err != nil {
			return Message{}, err
		}
	}
	return req, nil
}

// PendingRequests returns agent's outstanding requests — its kind=request
// messages whose correlation id has no kind=reply yet (§4.2). Derived, never
// stored. Oldest-first (longest-waiting leads the "# Awaiting replies" list).
func (b *Bus) PendingRequests(agent string) ([]Message, error) {
	rows, err := b.db.Query(`SELECT id, channel, ts, source, type, subject, text, data,
		produced_by_agent, produced_in_iteration, produced_by_plugin,
		kind, correlation_id, in_reply_to, reply_to, deadline FROM messages req
		WHERE req.kind='request' AND req.produced_by_agent=?
		  AND NOT EXISTS (SELECT 1 FROM messages r
		    WHERE r.kind='reply' AND r.correlation_id = req.correlation_id)
		ORDER BY req.ts, req.id`, agent)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

// --- watch fingerprint (§5.2) ---

// canonicalParams renders params as canonical JSON (Go's json.Marshal sorts map
// keys recursively, giving a deterministic form) so identical demand from
// different agents fingerprints identically. Empty params → "".
func canonicalParams(params map[string]any) (string, error) {
	if len(params) == 0 {
		return "", nil
	}
	b, err := json.Marshal(params)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ComputeWatch is the canonical (channel, params) fingerprint:
// sha256(channel + "\n" + canonical_params)[:16] hex. Empty params → "".
func ComputeWatch(channel string, params map[string]any) (string, error) {
	canon, err := canonicalParams(params)
	if err != nil {
		return "", err
	}
	if canon == "" {
		return "", nil
	}
	sum := sha256.Sum256([]byte(channel + "\n" + canon))
	return hex.EncodeToString(sum[:])[:16], nil
}

// SubscribeParams is Subscribe extended with opaque provider params (§5.2). When
// params is non-empty it computes the watch fingerprint, auto-injects
// subject.watch=<watch> into the matcher (so fan-out isolates this subscriber to
// the provider's messages stamped for this exact watch), and stores params +
// watch on the row. With empty params it is exactly Subscribe.
func (b *Bus) SubscribeParams(agent, channel string, m Matcher, typeFilter []string, params map[string]any) (Subscription, error) {
	if len(params) == 0 {
		return b.Subscribe(agent, channel, m, typeFilter)
	}
	// Provider contract gate (spec §6.1): validate params against the target
	// channel's declared params_schema before creating any channel/subscription
	// row, so a bad params object fails the subscribe loudly.
	if b.paramsValidator != nil {
		if err := b.paramsValidator(channel, params); err != nil {
			return Subscription{}, err
		}
	}
	watch, err := ComputeWatch(channel, params)
	if err != nil {
		return Subscription{}, err
	}
	// Auto-inject the watch into a copy of the matcher — never mutate the
	// caller's map, and the injected key makes the (agent,channel,matcher)
	// idempotency key differ per watch.
	inj := Matcher{}
	maps.Copy(inj, m)
	inj["subject.watch"] = watch

	now := b.clock().UTC()
	if err := ensureChannel(b.db, channel, now); err != nil {
		return Subscription{}, err
	}
	matcher := marshalMatcher(inj)
	tf := marshalTypeFilter(typeFilter)
	// type_filter is part of the idempotency key (review finding F2); see Subscribe.
	// NULL-safe IS so an empty filter matches only another empty filter.
	var existing string
	err = b.db.QueryRow(`SELECT id FROM subscriptions WHERE agent=? AND channel=? AND matcher=? AND type_filter IS ?`,
		agent, channel, matcher, tf).Scan(&existing)
	if err == nil {
		return b.getSubscription(existing)
	}
	if err != sql.ErrNoRows {
		return Subscription{}, err
	}
	id := "sub-" + agent + "-" + now.Format("20060102150405.000000000")
	paramsJSON, err := canonicalParams(params)
	if err != nil {
		return Subscription{}, err
	}
	if _, err := b.db.Exec(`INSERT INTO subscriptions(id, agent, channel, matcher, type_filter, created_at, params, watch)
		VALUES (?,?,?,?,?,?,?,?)`, id, agent, channel, matcher, tf, now.Format(time.RFC3339Nano), paramsJSON, watch); err != nil {
		return Subscription{}, err
	}
	b.emitSubscriptionHook(channel)
	return b.getSubscription(id)
}

// WatchInfo is one distinct unit of provider demand on a channel: the watch
// fingerprint, the params that produced it, and the agents that subscribed to it.
type WatchInfo struct {
	Watch       string
	Params      map[string]any
	Subscribers []string
}

// WatchList returns a channel's distinct (watch, params) pairs over its live
// parameterized subscriptions, each with its subscriber list (§5.2). This is the
// unit of work a provider reconciles against; ordered by watch for determinism.
func (b *Bus) WatchList(channel string) ([]WatchInfo, error) {
	rows, err := b.db.Query(`SELECT agent, watch, params FROM subscriptions
		WHERE channel=? AND watch IS NOT NULL AND watch<>'' ORDER BY watch, agent`, channel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byWatch := map[string]*WatchInfo{}
	var order []string
	for rows.Next() {
		var agent, watch string
		var params sql.NullString
		if err := rows.Scan(&agent, &watch, &params); err != nil {
			return nil, err
		}
		w, ok := byWatch[watch]
		if !ok {
			w = &WatchInfo{Watch: watch}
			if params.Valid && params.String != "" {
				w.Params = map[string]any{}
				_ = json.Unmarshal([]byte(params.String), &w.Params)
			}
			byWatch[watch] = w
			order = append(order, watch)
		}
		w.Subscribers = append(w.Subscribers, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(order)
	out := make([]WatchInfo, 0, len(order))
	for _, w := range order {
		out = append(out, *byWatch[w])
	}
	return out, nil
}

// auditJSON marshals a small string map into a stable JSON object for the audit
// data field; keys are emitted in sorted order (Go json.Marshal on a map).
func auditJSON(fields map[string]string) string {
	b, err := json.Marshal(fields)
	if err != nil {
		return "{}"
	}
	return string(b)
}
