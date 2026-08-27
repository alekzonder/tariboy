package bus

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// maxAttempts caps redelivery: a delivery returned this many times without an
// ack is flagged dlq and dropped from Pending.
const maxAttempts = 5

// seqWidth is the zero-pad width of the numeric suffix in a message id, chosen
// so ids sort lexicographically in (ts, seq) order.
const seqWidth = 9

var ErrNotFound = errors.New("not found")
var ErrPublishGuardDenied = errors.New("publish guard denied")

// ErrDeadlineUnsupported is returned by Request when a --deadline is given but
// no deadline hook is wired (SetDeadlineHooks), so no timeout could ever fire.
// The full arming path lands with the schedule subsystem (EPIC R).
var ErrDeadlineUnsupported = errors.New("deadline not supported yet")

// dbtx is the subset of *sql.DB and *sql.Tx used by the bus helpers so they can
// run either directly or inside a transaction.
type dbtx interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

type Bus struct {
	db    *sql.DB
	clock func() time.Time
	hook  func(Message, []string)
	// audit, if set, receives the per-agent audit events the lifecycle methods
	// emit (message_processed, message_replied, message_requeued). The daemon
	// wires it to its audit sink; nil in unit tests / before wiring.
	audit func(agent, kind, dataJSON string)
	// armDeadline / cancelDeadline are the deadline seam for Request (§4.2): the
	// bus decides WHEN a request's timeout is armed or cancelled; the daemon
	// decides HOW (a one-shot schedule entry publishing type=timeout into the
	// requester's inbox). Both are no-ops when unset.
	armDeadline    func(*sql.Tx, string, string, string) error
	cancelDeadline func(*sql.Tx, string) error
	// validateDeadline, if set, checks a deadline string for well-formedness
	// BEFORE Request publishes anything (§4.2). The daemon wires the same format
	// check its transactional arm hook uses (time.ParseDuration); nil = no gate.
	validateDeadline func(deadline string) error
	// paramsValidator, if set, gates parameterized subscribes (SubscribeParams):
	// the daemon supplies the provider contract's params_schema check (spec §6.1)
	// so a subscription with params to a provided channel fails loudly on a bad
	// params object instead of silently producing nothing. nil = no gate (unit
	// tests / before wiring); the bus itself never interprets params.
	paramsValidator func(channel string, params map[string]any) error
	// subscriptionHook, if set, is fired with the affected channel after a
	// subscription change that can actually alter the channel's provider watch
	// list — i.e. a watch-bearing (parameterized) subscribe or unsubscribe (spec
	// §6.2). The daemon maps the channel to its provider plugin and pushes the
	// current watch list. Plain (watch-less) subscribes/unsubscribes and no-ops
	// never fire it: WatchList excludes null-watch rows, so they push nothing new
	// — and firing would drag a plugin-store List()+provider scan onto the core
	// subscribe path, whose latency must not scale with plugin-store size (review
	// findings #3/#6). Same seam pattern as publishHook; the bus stays ignorant
	// of who provides what, gating only on its own watch state.
	subscriptionHook func(channel string)
}

func New(s *store.Store, clock func() time.Time) *Bus {
	if clock == nil {
		clock = time.Now
	}
	return &Bus{db: s.DB, clock: clock}
}

func (b *Bus) SetPublishHook(fn func(msg Message, deliveredTo []string)) { b.hook = fn }

// SetAuditHook wires the per-agent audit sink. The delivery/thread lifecycle
// methods emit their audit event through it (mirrors SetPublishHook's seam).
func (b *Bus) SetAuditHook(fn func(agent, kind, dataJSON string)) { b.audit = fn }

// SetDeadlineHooks wires request-deadline handling to the schedule subsystem
// (§4.2). arm registers a one-shot timeout for a request's correlation id;
// cancel removes it when a reply lands first. Either may be nil.
func (b *Bus) SetDeadlineHooks(arm func(*sql.Tx, string, string, string) error, cancel func(*sql.Tx, string) error) {
	b.armDeadline, b.cancelDeadline = arm, cancel
}

// SetDeadlineValidator wires the pre-publish deadline format check (§4.2). It
// runs inside Request before anything is published, so a malformed deadline is
// rejected up front instead of committing an un-armed request. Nil = no gate.
func (b *Bus) SetDeadlineValidator(validate func(deadline string) error) {
	b.validateDeadline = validate
}

// SetParamsValidator wires the provider contract's subscribe-time params gate
// (spec §6.1). fn is called for every non-empty-params SubscribeParams before
// the row is persisted; a non-nil error fails the subscribe. nil disables the
// gate. The bus stays ignorant of provider semantics — the daemon supplies fn.
func (b *Bus) SetParamsValidator(fn func(channel string, params map[string]any) error) {
	b.paramsValidator = fn
}

// SetSubscriptionHook wires the provider watch-lifecycle seam (spec §6.2). fn is
// called with the affected channel after a watch-affecting subscription change
// (a parameterized subscribe, or an unsubscribe that removed a watch-bearing
// row) commits; the daemon uses it to push the channel's current watch list to
// its provider plugin. Watch-less subscribes/unsubscribes and no-ops do not fire
// it (see the subscriptionHook field). nil disables the hook.
func (b *Bus) SetSubscriptionHook(fn func(channel string)) { b.subscriptionHook = fn }

// emitSubscriptionHook fires the wired subscription hook, if any.
func (b *Bus) emitSubscriptionHook(channel string) {
	if b.subscriptionHook != nil {
		b.subscriptionHook(channel)
	}
}

// emitAudit routes a lifecycle audit event to the wired sink, if any.
func (b *Bus) emitAudit(agent, kind, dataJSON string) {
	if b.audit != nil {
		b.audit(agent, kind, dataJSON)
	}
}

// ensureChannel inserts the channel row on first use (auto-create on publish or
// subscribe, spec §6). now is the injected-clock instant so callers can reuse a
// single tick per operation rather than burning one here.
func ensureChannel(x dbtx, name string, now time.Time) error {
	_, err := x.Exec(`INSERT INTO channels(name, kind, created_at) VALUES (?,?,?)
		ON CONFLICT(name) DO NOTHING`,
		name, ChannelKind(name), now.UTC().Format(time.RFC3339Nano))
	return err
}

// nextMessageID derives a collision-safe, (ts, seq)-sortable id of the form
// <channel>-<yyyymmddhhmmss>-<zero-padded seq>. The seq is MAX(existing seq for
// this channel+second)+1, computed inside the caller's tx. Using MAX (not
// COUNT) means pruning the oldest rows can never make a future seq collide with
// a survivor, and matching the prefix with substr (not LIKE) means channel
// names containing '_'/'%' cannot over-match.
func nextMessageID(x dbtx, channel string, now time.Time) (string, error) {
	prefix := channel + "-" + now.UTC().Format("20060102150405") + "-"
	var maxSeq int64
	// substr(id,1,len(prefix)) == prefix is an exact, metacharacter-free prefix
	// test; substr(id,len(prefix)+1) is the numeric suffix. channel=? lets the
	// (channel, ts, id) index narrow the scan.
	if err := x.QueryRow(`SELECT COALESCE(MAX(CAST(substr(id, ?) AS INTEGER)), 0)
		FROM messages WHERE channel = ? AND substr(id, 1, ?) = ?`,
		len(prefix)+1, channel, len(prefix), prefix).Scan(&maxSeq); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%0*d", prefix, seqWidth, maxSeq+1), nil
}

func (b *Bus) Publish(msg Message) (Message, error) {
	return b.PublishWithGuard(msg, nil)
}

// PublishWithGuard evaluates guard inside the same SQLite write transaction
// that inserts the message and deliveries. Callers can therefore authorize
// against durable state without a check/publish race.
func (b *Bus) PublishWithGuard(msg Message, guard func(*sql.Tx, time.Time) error) (Message, error) {
	now := b.clock().UTC()
	tx, err := b.db.Begin()
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback()
	msg, delivered, existing, err := b.publishTx(tx, msg, now, guard, false)
	if err != nil || existing {
		return msg, err
	}
	if err := tx.Commit(); err != nil {
		return Message{}, err
	}
	b.emitPublish(msg, delivered)
	return msg, nil
}

func (b *Bus) publishTx(tx *sql.Tx, msg Message, now time.Time, guard func(*sql.Tx, time.Time) error, selfCorrelation bool) (Message, []string, bool, error) {
	if msg.Channel == "" {
		return Message{}, nil, false, fmt.Errorf("publish: channel is required")
	}
	if guard != nil {
		if err := guard(tx, now); err != nil {
			return Message{}, nil, false, err
		}
	}

	if msg.IdempotencyKey != "" {
		existing, found, err := messageByIdempotency(tx, msg.IdempotencyKey)
		if err != nil {
			return Message{}, nil, false, err
		}
		if found {
			return existing, nil, true, nil
		}
	}
	if err := ensureChannel(tx, msg.Channel, now); err != nil {
		return Message{}, nil, false, err
	}
	id, err := nextMessageID(tx, msg.Channel, now)
	if err != nil {
		return Message{}, nil, false, err
	}
	msg.ID = id
	if selfCorrelation {
		msg.CorrelationID = id
	}
	// Fixed-width fractional seconds (always 9 digits) so the stored ts sorts
	// lexically the same as chronologically. RFC3339Nano strips trailing zeros,
	// which makes same-second timestamps of differing digit-widths sort wrong:
	// e.g. 10ns strips to ".00000001" and 11ns strips to ".000000011"; once each
	// is followed by the trailing "Z", "...011" < "...01Z" lexically (a digit
	// sorts below 'Z'), so 11ns would sort BEFORE 10ns even though 10ns is
	// earlier. Pending/Tail order by this column (spec §6: oldest first).
	msg.TS = now.Format("2006-01-02T15:04:05.000000000Z07:00")

	// kind defaults to "event" (matches the column default) so fire-and-forget
	// publishes need not set it; the stored row and the returned msg agree.
	if msg.Kind == "" {
		msg.Kind = "event"
	}

	subject := marshalMap(msg.Subject, "{}")
	data := marshalMapNullable(msg.Data)
	text := nullableStr(msg.Text)
	if _, err := tx.Exec(`INSERT INTO messages
		(id, channel, ts, source, type, subject, text, data,
		 produced_by_agent, produced_in_iteration, produced_by_plugin,
		 kind, correlation_id, in_reply_to, reply_to, deadline, idempotency_key)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		msg.ID, msg.Channel, msg.TS, msg.Source, msg.Type, subject, text, data,
		nullableStr(msg.ProducedByAgent), nullableStr(msg.ProducedInIteration), nullableStr(msg.ProducedByPlugin),
		msg.Kind, nullableStr(msg.CorrelationID), nullableStr(msg.InReplyTo), nullableStr(msg.ReplyTo), nullableStr(msg.Deadline),
		nullableStr(msg.IdempotencyKey)); err != nil {
		return Message{}, nil, false, err
	}
	if _, err := tx.Exec(`INSERT INTO task_workflow_message_sequence(message_id) VALUES (?)`, msg.ID); err != nil {
		return Message{}, nil, false, err
	}

	// Fan-out: create a delivery row for each matching subscription.
	subs, err := subscriptionsFor(tx, msg.Channel)
	if err != nil {
		return Message{}, nil, false, err
	}
	// An agent never receives its own OUTBOUND published message: whoever
	// authored this message is excluded from the fan-out so a reply an agent
	// publishes to a channel it is itself subscribed to does not echo back into
	// its own queue. This is a uniform bus rule (not a chat-channel special
	// case); every OTHER matching subscription — plugin sinks that forward the
	// message out, other agents, group members — still gets its delivery.
	//
	// The suppression is OUTBOUND-ONLY: it never applies on the author's own
	// inbox. ProducedByAgent is also the provenance tag on system messages
	// generated FOR an agent and delivered TO its inbox (request-reply timeout
	// events, proxy budget.warn, script.result, self-targeted schedules). Those
	// land on InboxChannel(author) with author == the recipient, and the inbox
	// is by definition where messages FOR that agent belong — so an inbox
	// delivery to the author must go through, even though the author "produced"
	// it. Suppress the author's own delivery only on NON-inbox channels.
	author := messageAuthor(msg)
	authorInbox := InboxChannel(author)
	deliveredSet := map[string]bool{}
	queueFull := map[string]bool{}
	queueChecked := map[string]bool{}
	for _, s := range subs {
		if author != "" && s.Agent == author && msg.Channel != authorInbox {
			continue
		}
		if !MatchType(s.TypeFilter, msg.Type) || !s.Matcher.Match(msg) {
			continue
		}
		if !queueChecked[s.Agent] {
			queueFull[s.Agent], err = queueLimitReached(tx, s.Agent)
			if err != nil {
				return Message{}, nil, false, err
			}
			queueChecked[s.Agent] = true
		}
		dlq, result := 0, any(nil)
		if queueFull[s.Agent] {
			dlq, result = 1, "queue_limit"
		}
		if _, err := tx.Exec(`INSERT INTO deliveries(subscription_id, message_id, dlq, result) VALUES (?,?,?,?)
			ON CONFLICT(subscription_id, message_id) DO NOTHING`, s.ID, msg.ID, dlq, result); err != nil {
			return Message{}, nil, false, err
		}
		if dlq == 0 {
			deliveredSet[s.Agent] = true
		}
	}
	delivered := make([]string, 0, len(deliveredSet))
	for a := range deliveredSet {
		delivered = append(delivered, a)
	}
	return msg, delivered, false, nil
}

func queueLimitReached(tx *sql.Tx, agent string) (bool, error) {
	var limit int
	if err := tx.QueryRow(`SELECT messages_max_queue FROM agents WHERE name=?`, agent).Scan(&limit); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	var pending int
	if err := tx.QueryRow(`SELECT COUNT(DISTINCT d.message_id)
		FROM deliveries d JOIN subscriptions s ON s.id=d.subscription_id
		WHERE s.agent=? AND d.acked_at IS NULL AND d.dlq=0`, agent).Scan(&pending); err != nil {
		return false, err
	}
	return pending >= limit, nil
}

func (b *Bus) emitPublish(msg Message, delivered []string) {
	if b.hook != nil {
		b.hook(msg, delivered)
	}
}

func messageByIdempotency(x dbtx, key string) (Message, bool, error) {
	var m Message
	var subject string
	var text, data, pa, pi, pp sql.NullString
	var kind string
	var cid, irt, rt, dl, idempotency sql.NullString
	err := x.QueryRow(`SELECT id, channel, ts, source, type, subject, text, data,
		produced_by_agent, produced_in_iteration, produced_by_plugin,
		kind, correlation_id, in_reply_to, reply_to, deadline, idempotency_key
		FROM messages WHERE idempotency_key = ?`, key).Scan(
		&m.ID, &m.Channel, &m.TS, &m.Source, &m.Type, &subject, &text, &data,
		&pa, &pi, &pp, &kind, &cid, &irt, &rt, &dl, &idempotency)
	if err == sql.ErrNoRows {
		return Message{}, false, nil
	}
	if err != nil {
		return Message{}, false, err
	}
	unmarshalMessage(&m, subject, text, data, pa, pi, pp)
	m.Kind, m.CorrelationID, m.InReplyTo, m.ReplyTo, m.Deadline =
		kind, cid.String, irt.String, rt.String, dl.String
	m.IdempotencyKey = idempotency.String
	return m, true, nil
}

func (b *Bus) Subscribe(agent, channel string, m Matcher, typeFilter []string) (Subscription, error) {
	now := b.clock().UTC()
	if err := ensureChannel(b.db, channel, now); err != nil {
		return Subscription{}, err
	}
	matcher := marshalMatcher(m)
	tf := marshalTypeFilter(typeFilter)
	// Idempotent on (agent,channel,matcher,type_filter): return the existing row
	// if present. type_filter is part of the key so two subscribes to the same
	// channel+matcher differing only by type globs each get their own row instead
	// of the second collapsing onto the first and silently dropping its filter
	// (review finding F2). type_filter is nullable, so compare with IS (NULL-safe)
	// rather than = , which never matches NULL against NULL.
	var existing string
	err := b.db.QueryRow(`SELECT id FROM subscriptions WHERE agent=? AND channel=? AND matcher=? AND type_filter IS ?`,
		agent, channel, matcher, tf).Scan(&existing)
	if err == nil {
		return b.getSubscription(existing)
	}
	if err != sql.ErrNoRows {
		return Subscription{}, err
	}
	id := "sub-" + agent + "-" + now.Format("20060102150405.000000000")
	if _, err := b.db.Exec(`INSERT INTO subscriptions(id, agent, channel, matcher, type_filter, created_at)
		VALUES (?,?,?,?,?,?)`, id, agent, channel, matcher, tf, now.Format(time.RFC3339Nano)); err != nil {
		return Subscription{}, err
	}
	// A plain subscription carries no watch, so it can never change any provider's
	// watch list (WatchList excludes null-watch rows). Deliberately skip the
	// subscription hook here: firing it would only push an unchanged list and
	// force a plugin-store List()+provider scan onto every core-channel subscribe
	// (review finding #3). Only the parameterized path (SubscribeParams) fires.
	return b.getSubscription(id)
}

// Unsubscribe removes agent's own subscription id. Scoped to (id, agent) so one
// agent can never delete another agent's subscription even though ids embed
// the owning agent's name (and are therefore guessable): a mismatched agent
// or missing id both report ErrNotFound rather than silently succeeding.
func (b *Bus) Unsubscribe(agent, id string) error {
	// Capture the channel (and whether the row bears a watch) before deleting so
	// the subscription hook (spec §6.2) can be fired with it — the row is gone by
	// the time we know the delete stuck. Only a watch-bearing removal can change a
	// provider's watch list, so the watch decides whether we fire (finding #3/#6).
	var channel string
	var watch sql.NullString
	err := b.db.QueryRow(`SELECT channel, watch FROM subscriptions WHERE id=? AND agent=?`, id, agent).Scan(&channel, &watch)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	res, err := b.db.Exec(`DELETE FROM subscriptions WHERE id=? AND agent=?`, id, agent)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	// Orphan deliveries for this subscription are harmless but tidy them. The
	// subscription row is already gone (and scoped to agent above), so scoping
	// this cleanup to the same id is sufficient - no other agent's deliveries
	// can share this subscription_id.
	_, _ = b.db.Exec(`DELETE FROM deliveries WHERE subscription_id=?`, id)
	if watch.Valid && watch.String != "" {
		b.emitSubscriptionHook(channel)
	}
	return nil
}

// UnsubscribeChannel removes every subscription (agent, channel) holds, regardless
// of matcher. Scoped to the given agent so it can never affect another agent's
// identically-named subscription. Returns the number removed; zero maps to
// ErrNotFound so callers distinguish "nothing to remove" from success.
func (b *Bus) UnsubscribeChannel(agent, channel string) (int, error) {
	// Whether any of the to-be-removed rows bears a watch decides if a provider's
	// watch list actually changes; a channel of only plain subs is a hook no-op,
	// so we keep the plugin-store List() off that path (finding #3/#6).
	var watchCount int
	if err := b.db.QueryRow(`SELECT COUNT(*) FROM subscriptions
		WHERE agent=? AND channel=? AND watch IS NOT NULL AND watch<>''`, agent, channel).Scan(&watchCount); err != nil {
		return 0, err
	}
	res, err := b.db.Exec(`DELETE FROM subscriptions WHERE agent=? AND channel=?`, agent, channel)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return 0, ErrNotFound
	}
	if watchCount > 0 {
		b.emitSubscriptionHook(channel)
	}
	return int(n), nil
}

func (b *Bus) ListSubscriptions(agent string) ([]Subscription, error) {
	rows, err := b.db.Query(`SELECT id, agent, channel, matcher, type_filter, created_at, params, watch, locked
		FROM subscriptions WHERE agent=? ORDER BY created_at, id`, agent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		s, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Pending returns an agent's undelivered/unacked messages ordered by ts, marking
// ONLY the returned deliveries delivered and incrementing their attempts. Rows
// past maxAttempts are flagged dlq. Deduped by message id.
//
// Delivery accounting is scoped strictly to messages actually returned. A single
// message can have several delivery rows (one per matching subscription); all of
// its rows are marked, but rows belonging to messages held back by `limit` are
// left completely untouched so a backlog can never be silently attempted into
// the DLQ without ever being read.
//
// The feeding SELECT is ordered by (m.ts, m.id) with no SQL LIMIT: cross-sub
// dedupe makes the number of rows needed for `limit` distinct messages depend on
// fan-out, so a fixed LIMIT can't be sized correctly. Instead we rely on the row
// stream being lazy and on all delivery rows of one message being contiguous
// (they share the same ts and id): once we hold `limit` distinct message ids and
// encounter a row for a new id, every earlier message's rows have already been
// seen, so we stop reading. That bounds work to the returned messages' rows.
func (b *Bus) Pending(agent string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := b.db.Query(`SELECT m.id, m.channel, m.ts, m.source, m.type, m.subject, m.text, m.data,
		m.produced_by_agent, m.produced_in_iteration, m.produced_by_plugin,
		m.kind, m.correlation_id, m.in_reply_to, m.reply_to, m.deadline,
		d.attempts, d.subscription_id
		FROM deliveries d
		JOIN subscriptions s ON s.id = d.subscription_id
		JOIN messages m ON m.id = d.message_id
		WHERE s.agent = ? AND d.acked_at IS NULL AND d.dlq = 0
		ORDER BY m.ts, m.id`, agent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := b.clock().UTC().Format(time.RFC3339Nano)
	// returned holds ONLY message ids appended to out; marks holds delivery rows
	// for those same ids (and only those) to be accounted after the read.
	returned := map[string]bool{}
	var out []Message
	type mark struct {
		subID, msgID string
		attempts     int
	}
	var marks []mark
	for rows.Next() {
		var m Message
		var subject, subID string
		var text, data, pa, pi, pp sql.NullString
		var kind string
		var cid, irt, rt, dl sql.NullString
		var attempts int
		if err := rows.Scan(&m.ID, &m.Channel, &m.TS, &m.Source, &m.Type, &subject, &text, &data,
			&pa, &pi, &pp, &kind, &cid, &irt, &rt, &dl, &attempts, &subID); err != nil {
			return nil, err
		}
		if returned[m.ID] {
			// Another delivery row for a message we already returned: account it.
			marks = append(marks, mark{subID: subID, msgID: m.ID, attempts: attempts})
			continue
		}
		if len(out) >= limit {
			// Full, and this row starts a new (held-back) message. Because rows
			// are grouped by (ts, id), no returned message has rows left below.
			break
		}
		unmarshalMessage(&m, subject, text, data, pa, pi, pp)
		m.Kind, m.CorrelationID, m.InReplyTo, m.ReplyTo, m.Deadline = kind, cid.String, irt.String, rt.String, dl.String
		out = append(out, m)
		returned[m.ID] = true
		marks = append(marks, mark{subID: subID, msgID: m.ID, attempts: attempts})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Release the connection before issuing updates: the store pins a single
	// connection (SetMaxOpenConns(1)), and after an early break the row iterator
	// still holds it, so the UPDATEs below would otherwise deadlock.
	rows.Close()
	// Account ONLY the returned messages' delivery rows.
	for _, mk := range marks {
		if mk.attempts+1 > maxAttempts {
			_, _ = b.db.Exec(`UPDATE deliveries SET dlq=1 WHERE subscription_id=? AND message_id=?`, mk.subID, mk.msgID)
			continue
		}
		_, _ = b.db.Exec(`UPDATE deliveries SET delivered_at=?, attempts=attempts+1
			WHERE subscription_id=? AND message_id=?`, now, mk.subID, mk.msgID)
	}
	return out, nil
}

// PendingTotal is the number of unacked, non-DLQ deliveries across all
// subscribers — the channel queue-depth gauge for telemetry (spec §14).
func (b *Bus) PendingTotal() (int, error) {
	var n int
	err := b.db.QueryRow(`SELECT COUNT(*) FROM deliveries
		WHERE acked_at IS NULL AND dlq = 0`).Scan(&n)
	return n, err
}

func (b *Bus) HasPending(agent string) (bool, error) {
	var n int
	err := b.db.QueryRow(`SELECT COUNT(*) FROM deliveries d
		JOIN subscriptions s ON s.id = d.subscription_id
		WHERE s.agent = ? AND d.acked_at IS NULL AND d.dlq = 0`, agent).Scan(&n)
	return n > 0, err
}

func (b *Bus) Ack(agent string, messageIDs []string) error {
	if len(messageIDs) == 0 {
		return nil
	}
	now := b.clock().UTC().Format(time.RFC3339Nano)
	for _, id := range messageIDs {
		if _, err := b.db.Exec(`UPDATE deliveries SET acked_at=?
			WHERE message_id=? AND acked_at IS NULL AND subscription_id IN
			  (SELECT id FROM subscriptions WHERE agent=?)`, now, id, agent); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bus) Announce(agent, channel string) error {
	_, err := b.db.Exec(`UPDATE deliveries SET acked_at=NULL, delivered_at=NULL, attempts=0, dlq=0
		WHERE subscription_id IN (SELECT id FROM subscriptions WHERE agent=? AND channel=?)`, agent, channel)
	return err
}

// EnsureChannel materializes the channel row for name so it lists in
// GET /api/channels before any message or subscription exists — the case of a
// bound-but-idle plugin chat (dev-t-dbu.1). Channel rows are otherwise created
// only as a side-effect of Publish/Subscribe (ensureChannel), so a chat bound
// via a plugin route but never messaged has no row and stays invisible to the
// agent Subscribe picker. Idempotent: an existing row is left untouched. Rejects
// a malformed name so a caller can't seed a junk row.
func (b *Bus) EnsureChannel(name string) error {
	if !ValidChannel(name) {
		return fmt.Errorf("ensure channel: invalid channel %q", name)
	}
	return ensureChannel(b.db, name, b.clock().UTC())
}

func (b *Bus) Channels() ([]Channel, error) {
	rows, err := b.db.Query(`SELECT name, kind, created_at FROM channels ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Channel
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.Name, &c.Kind, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (b *Bus) Tail(channel string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 50
	}
	// Newest `limit` rows, returned oldest-first.
	rows, err := b.db.Query(`SELECT id, channel, ts, source, type, subject, text, data,
		produced_by_agent, produced_in_iteration, produced_by_plugin,
		kind, correlation_id, in_reply_to, reply_to, deadline FROM messages
		WHERE channel=? ORDER BY ts DESC, id DESC LIMIT ?`, channel, limit)
	if err != nil {
		return nil, err
	}
	msgs, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	// Reverse to chronological order.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

func (b *Bus) MessagesSince(channel, afterID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := b.db.Query(`SELECT id, channel, ts, source, type, subject, text, data,
		produced_by_agent, produced_in_iteration, produced_by_plugin,
		kind, correlation_id, in_reply_to, reply_to, deadline FROM messages
		WHERE channel=? AND id>? ORDER BY ts, id LIMIT ?`, channel, afterID, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func (b *Bus) InspectChannel(name string) (Channel, int, error) {
	var c Channel
	err := b.db.QueryRow(`SELECT name, kind, created_at FROM channels WHERE name=?`, name).Scan(&c.Name, &c.Kind, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return Channel{}, 0, ErrNotFound
	}
	if err != nil {
		return Channel{}, 0, err
	}
	var count int
	if err := b.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE channel=?`, name).Scan(&count); err != nil {
		return Channel{}, 0, err
	}
	return c, count, nil
}

// DeleteChannel removes a channel and everything derived from it: its messages,
// the deliveries of its subscriptions, and the subscriptions themselves. Used
// by group teardown (spec §5.3 `down`). Deleting deliveries first, then
// subscriptions, then messages, then the channel keeps the removal self-
// contained (the bus tables carry no FK cascade).
func (b *Bus) DeleteChannel(name string) error {
	if _, err := b.db.Exec(`DELETE FROM deliveries WHERE subscription_id IN
		(SELECT id FROM subscriptions WHERE channel=?)`, name); err != nil {
		return err
	}
	if _, err := b.db.Exec(`DELETE FROM subscriptions WHERE channel=?`, name); err != nil {
		return err
	}
	if _, err := b.db.Exec(`DELETE FROM messages WHERE channel=?`, name); err != nil {
		return err
	}
	_, err := b.db.Exec(`DELETE FROM channels WHERE name=?`, name)
	return err
}

// --- helpers ---

func subscriptionsFor(x dbtx, channel string) ([]Subscription, error) {
	rows, err := x.Query(`SELECT id, agent, channel, matcher, type_filter, created_at, params, watch, locked
		FROM subscriptions WHERE channel=?`, channel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		s, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (b *Bus) getSubscription(id string) (Subscription, error) {
	row := b.db.QueryRow(`SELECT id, agent, channel, matcher, type_filter, created_at, params, watch, locked
		FROM subscriptions WHERE id=?`, id)
	return scanSubscription(row)
}

type scanner interface{ Scan(dest ...any) error }

func scanSubscription(row scanner) (Subscription, error) {
	var s Subscription
	var matcher string
	var tf, params, watch sql.NullString
	var locked int
	if err := row.Scan(&s.ID, &s.Agent, &s.Channel, &matcher, &tf, &s.CreatedAt, &params, &watch, &locked); err != nil {
		if err == sql.ErrNoRows {
			return Subscription{}, ErrNotFound
		}
		return Subscription{}, err
	}
	s.Matcher = Matcher{}
	if matcher != "" {
		_ = json.Unmarshal([]byte(matcher), &s.Matcher)
	}
	if tf.Valid && tf.String != "" {
		_ = json.Unmarshal([]byte(tf.String), &s.TypeFilter)
	}
	if params.Valid && params.String != "" {
		s.Params = map[string]any{}
		_ = json.Unmarshal([]byte(params.String), &s.Params)
	}
	s.Watch = watch.String
	s.Locked = locked != 0
	return s, nil
}

func scanMessages(rows *sql.Rows) ([]Message, error) {
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var subject string
		var text, data, pa, pi, pp sql.NullString
		var kind string
		var cid, irt, rt, dl sql.NullString
		if err := rows.Scan(&m.ID, &m.Channel, &m.TS, &m.Source, &m.Type, &subject, &text, &data, &pa, &pi, &pp,
			&kind, &cid, &irt, &rt, &dl); err != nil {
			return nil, err
		}
		unmarshalMessage(&m, subject, text, data, pa, pi, pp)
		m.Kind, m.CorrelationID, m.InReplyTo, m.ReplyTo, m.Deadline = kind, cid.String, irt.String, rt.String, dl.String
		out = append(out, m)
	}
	return out, rows.Err()
}

func unmarshalMessage(m *Message, subject string, text, data, pa, pi, pp sql.NullString) {
	m.Subject = map[string]any{}
	if subject != "" {
		_ = json.Unmarshal([]byte(subject), &m.Subject)
	}
	if data.Valid && data.String != "" {
		m.Data = map[string]any{}
		_ = json.Unmarshal([]byte(data.String), &m.Data)
	}
	m.Text = text.String
	m.ProducedByAgent = pa.String
	m.ProducedInIteration = pi.String
	m.ProducedByPlugin = pp.String
}

func marshalMap(m map[string]any, empty string) string {
	if len(m) == 0 {
		return empty
	}
	b, err := json.Marshal(m)
	if err != nil {
		return empty
	}
	return string(b)
}

func marshalMapNullable(m map[string]any) any {
	if len(m) == 0 {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return string(b)
}

func marshalMatcher(m Matcher) string {
	if len(m) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func marshalTypeFilter(tf []string) any {
	if len(tf) == 0 {
		return nil
	}
	b, _ := json.Marshal(tf)
	return string(b)
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
