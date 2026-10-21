package bus

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
var ErrRequiredDelivery = errors.New("required agent delivery missing")

// ErrChatExists is returned when a chat id is already owned. It is a conflict
// rather than an idempotent reuse: a second chat on one channel would merge two
// conversations into a single feed.
var ErrChatExists = errors.New("chat already exists")

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
	read  *sql.DB // read-only pool for the polled chat list
	clock func() time.Time
	hook  func(Message, []string)
	// txHook / retainedSQL are the transactional publish seam and the message
	// retention condition (SetPublishTxHook, SetMessageRetention).
	txHook      func(*sql.Tx, Message) error
	retainedSQL string
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
	return &Bus{db: s.DB, read: s.Reader(), clock: clock}
}

func (b *Bus) SetPublishHook(fn func(msg Message, deliveredTo []string)) { b.hook = fn }

// SetPublishTxHook wires a write that must commit together with every new
// message: fn runs inside the publish transaction after the message row is
// inserted, and its error rolls the publish back. Native Tasks uses it to
// number messages for workflow ingress. nil disables it.
func (b *Bus) SetPublishTxHook(fn func(tx *sql.Tx, msg Message) error) { b.txHook = fn }

// SetMessageRetention wires an SQL condition over messages.id that keeps a
// message from being deleted while its owner still needs it, even after every
// delivery is gone. "" retains nothing.
func (b *Bus) SetMessageRetention(retainedSQL string) { b.retainedSQL = retainedSQL }

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
		name, ChannelKind(name), store.FormatTime(now))
	return err
}

// channelSeqSQL reads the message-id counter by the channel primary key.
const channelSeqSQL = `SELECT id_second, id_seq FROM channels WHERE name = ?`

// nextMessageID derives a collision-safe, (ts, seq)-sortable id of the form
// <channel>-<yyyymmddhhmmss>-<zero-padded seq>, inside the caller's tx. The seq
// comes from the counter on the channel row (migration 0070), so the cost does
// not grow with the channel history. The stored ids are read only when the
// counter cannot answer: it is not seeded yet (an upgraded or recreated
// channel), or the clock stepped back before the counter's second.
func nextMessageID(x dbtx, channel string, now time.Time) (string, error) {
	second := now.UTC().Format("20060102150405")
	var counted sql.NullString
	var seq int64
	if err := x.QueryRow(channelSeqSQL, channel).Scan(&counted, &seq); err != nil {
		return "", err
	}
	switch {
	case counted.Valid && counted.String == second:
		seq++
	case counted.Valid && counted.String < second:
		seq = 1
	default:
		stored, err := maxStoredSeq(x, channel, second)
		if err != nil {
			return "", err
		}
		seq = stored + 1
	}
	if !counted.Valid || counted.String <= second {
		if _, err := x.Exec(`UPDATE channels SET id_second = ?, id_seq = ? WHERE name = ?`, second, seq, channel); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%s-%s-%0*d", channel, second, seqWidth, seq), nil
}

// maxStoredSeq is the highest stored seq for channel in second. Using MAX (not
// COUNT) means pruning the oldest rows can never make a future seq collide with
// a survivor, and matching the prefix with substr (not LIKE) means channel
// names containing '_'/'%' cannot over-match.
func maxStoredSeq(x dbtx, channel, second string) (int64, error) {
	prefix := channel + "-" + second + "-"
	var maxSeq int64
	// substr(id,1,len(prefix)) == prefix is an exact, metacharacter-free prefix
	// test; substr(id,len(prefix)+1) is the numeric suffix. channel=? lets the
	// (channel, ts, id) index narrow the scan.
	if err := x.QueryRow(`SELECT COALESCE(MAX(CAST(substr(id, ?) AS INTEGER)), 0)
		FROM messages WHERE channel = ? AND substr(id, 1, ?) = ?`,
		len(prefix)+1, channel, len(prefix), prefix).Scan(&maxSeq); err != nil {
		return 0, err
	}
	return maxSeq, nil
}

func (b *Bus) Publish(msg Message) (Message, error) {
	return b.PublishWithGuard(msg, nil)
}

// PublishRequiringDelivery persists only messages delivered to agent.
func (b *Bus) PublishRequiringDelivery(msg Message, agent string) (Message, error) {
	now := b.clock().UTC()
	tx, err := b.db.Begin()
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback()
	msg, delivered, existing, err := b.publishTx(tx, msg, now, nil, false)
	if err != nil {
		return Message{}, err
	}
	found := false
	for _, deliveredAgent := range delivered {
		if deliveredAgent == agent {
			found = true
			break
		}
	}
	if existing {
		err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM deliveries d JOIN subscriptions s ON s.id=d.subscription_id WHERE d.message_id=? AND d.dlq=0 AND s.agent=?)`, msg.ID, agent).Scan(&found)
		if err != nil {
			return Message{}, err
		}
	}
	if !found {
		return Message{}, ErrRequiredDelivery
	}
	if existing {
		return msg, nil
	}
	if err := tx.Commit(); err != nil {
		return Message{}, err
	}
	b.emitPublish(msg, delivered)
	return msg, nil
}

// DeliveryAgents returns agents with a non-DLQ delivery for messageID.
func (b *Bus) DeliveryAgents(messageID string) ([]string, error) {
	rows, err := b.db.Query(`SELECT DISTINCT s.agent FROM deliveries d JOIN subscriptions s ON s.id=d.subscription_id
		WHERE d.message_id=? AND d.dlq=0 ORDER BY s.agent`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var agents []string
	for rows.Next() {
		var agent string
		if err := rows.Scan(&agent); err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	return agents, rows.Err()
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
	// The canonical stored format sorts lexically in time order, which
	// Pending/Tail rely on (spec §6: oldest first).
	msg.TS = store.FormatTime(now)

	// kind defaults to "event" (matches the column default) so fire-and-forget
	// publishes need not set it; the stored row and the returned msg agree.
	if msg.Kind == "" {
		msg.Kind = "event"
	}

	subject := marshalMap(msg.Subject, "{}")
	data := marshalMapNullable(msg.Data)
	text := nullableStr(msg.Text)
	if _, err := tx.Exec(`INSERT INTO messages
		(`+messageColumns+`, idempotency_key)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		msg.ID, msg.Channel, msg.TS, msg.Source, msg.Type, subject, text, data,
		nullableStr(msg.ProducedByAgent), nullableStr(msg.ProducedInIteration), nullableStr(msg.ProducedByPlugin),
		msg.Kind, nullableStr(msg.CorrelationID), nullableStr(msg.InReplyTo), nullableStr(msg.ReplyTo), nullableStr(msg.Deadline),
		nullableStr(msg.IdempotencyKey)); err != nil {
		return Message{}, nil, false, err
	}
	// The sender is resolved by fromSQL over the stored row, so a new message
	// and a backfilled one (migration 0048) can never disagree.
	if _, err := tx.Exec(`UPDATE messages SET sender = `+fromSQL+` WHERE id = ?`, msg.ID); err != nil {
		return Message{}, nil, false, err
	}
	if b.txHook != nil {
		if err := b.txHook(tx, msg); err != nil {
			return Message{}, nil, false, err
		}
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
	// inbox or service chat. ProducedByAgent is also the provenance tag on
	// system messages generated FOR an agent (request-reply timeout events,
	// proxy budget.warn, script.result, self-targeted schedules). Those land on
	// InboxChannel(author) or the author's service chat with author == the
	// recipient, and both are by definition where messages FOR that agent
	// belong — so such a delivery to the author must go through, even though
	// the author "produced" it. Suppress the author's own delivery only on
	// other channels.
	author := messageAuthor(msg)
	ownChannel := msg.Channel == InboxChannel(author) || msg.Channel == ChatChannelFor(ChatIDService(author))
	deliveredSet := map[string]bool{}
	queueFull := map[string]bool{}
	queueChecked := map[string]bool{}
	for _, s := range subs {
		if author != "" && s.Agent == author && !ownChannel {
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
	pending, err := pendingCount(tx, agent)
	if err != nil {
		return false, err
	}
	return pending >= limit, nil
}

func pendingCount(x dbtx, agent string) (int, error) {
	var pending int
	if err := x.QueryRow(`SELECT COUNT(DISTINCT d.message_id)
		FROM deliveries d JOIN subscriptions s ON s.id=d.subscription_id
		WHERE s.agent=? AND d.acked_at IS NULL AND d.dlq=0`, agent).Scan(&pending); err != nil {
		return 0, err
	}
	return pending, nil
}

func (b *Bus) emitPublish(msg Message, delivered []string) {
	if b.hook != nil {
		b.hook(msg, delivered)
	}
}

func messageByIdempotency(x dbtx, key string) (Message, bool, error) {
	var idempotency sql.NullString
	m, err := scanMessage(x.QueryRow(`SELECT `+messageColumns+`, idempotency_key
		FROM messages WHERE idempotency_key = ?`, key), &idempotency)
	if err == sql.ErrNoRows {
		return Message{}, false, nil
	}
	if err != nil {
		return Message{}, false, err
	}
	m.IdempotencyKey = idempotency.String
	return m, true, nil
}

// nextSubscriptionID numbers a subscription within its timestamp, exactly as
// nextMessageID does. A bare timestamp collided whenever one agent gained two
// subscriptions inside the same clock tick — provisioning an agent's three
// chats is precisely that case.
func nextSubscriptionID(x dbtx, agent string, now time.Time) (string, error) {
	prefix := "sub-" + agent + "-" + now.UTC().Format("20060102150405") + "-"
	var maxSeq int64
	if err := x.QueryRow(`SELECT COALESCE(MAX(CAST(substr(id, ?) AS INTEGER)), 0)
		FROM subscriptions WHERE agent = ? AND substr(id, 1, ?) = ?`,
		len(prefix)+1, agent, len(prefix), prefix).Scan(&maxSeq); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%0*d", prefix, seqWidth, maxSeq+1), nil
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
		// Re-subscribing resumes a revoked row rather than adding a second one,
		// so rejoining a chat keeps the queue it already had.
		if _, err := b.db.Exec(`UPDATE subscriptions SET revoked_at='' WHERE id=?`, existing); err != nil {
			return Subscription{}, err
		}
		return b.getSubscription(existing)
	}
	if err != sql.ErrNoRows {
		return Subscription{}, err
	}
	id, err := nextSubscriptionID(b.db, agent, now)
	if err != nil {
		return Subscription{}, err
	}
	if _, err := b.db.Exec(`INSERT INTO subscriptions(id, agent, channel, matcher, type_filter, created_at)
		VALUES (?,?,?,?,?,?)`, id, agent, channel, matcher, tf, store.FormatTime(now)); err != nil {
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

// RevokeChannel stops future delivery of channel to agent without removing the
// subscription rows, so the deliveries they already own stay in that agent
// queue. This is how leaving a chat works: membership ends, outstanding work
// does not. Returns the number of rows revoked.
func (b *Bus) RevokeChannel(agent, channel string) (int, error) {
	res, err := b.db.Exec(`UPDATE subscriptions SET revoked_at=? WHERE agent=? AND channel=? AND revoked_at=''`,
		store.FormatTime(b.clock()), agent, channel)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
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
		FROM subscriptions WHERE agent=? AND revoked_at='' ORDER BY created_at, id`, agent)
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
	// Reading and accounting share one transaction: a message is handed out only
	// if its attempts were counted, so a failed UPDATE cannot bypass the DLQ cap.
	tx, err := b.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT `+messageColumnsM+`, d.attempts, d.subscription_id
		FROM deliveries d
		JOIN subscriptions s ON s.id = d.subscription_id
		JOIN messages m ON m.id = d.message_id
		WHERE s.agent = ? AND d.acked_at IS NULL AND d.dlq = 0
		ORDER BY m.ts, m.id`, agent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := store.FormatTime(b.clock())
	// returned holds ONLY message ids appended to out; marks holds delivery rows
	// for those same ids (and only those) to be accounted after the read.
	returned := map[string]bool{}
	var out []Message
	type mark struct {
		subID, msgID string
		attempts     int
		decodeErr    error
	}
	var marks []mark
	// broken holds messages whose stored row does not decode: their deliveries
	// go to the DLQ instead of stopping the whole queue.
	broken := map[string]error{}
	for rows.Next() {
		var subID string
		var attempts int
		m, err := scanMessage(rows, &attempts, &subID)
		if err != nil && !errors.Is(err, errMessageDecode) {
			return nil, err
		}
		if returned[m.ID] || broken[m.ID] != nil {
			// Another delivery row for a message already handled: account it.
			marks = append(marks, mark{subID: subID, msgID: m.ID, attempts: attempts, decodeErr: broken[m.ID]})
			continue
		}
		if len(out) >= limit {
			// Full, and this row starts a new (held-back) message. Because rows
			// are grouped by (ts, id), no returned message has rows left below.
			break
		}
		if err != nil {
			broken[m.ID] = err
			marks = append(marks, mark{subID: subID, msgID: m.ID, attempts: attempts, decodeErr: err})
			continue
		}
		out = append(out, m)
		returned[m.ID] = true
		marks = append(marks, mark{subID: subID, msgID: m.ID, attempts: attempts})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Close the cursor before updating on the same transaction: after an early
	// break the row iterator is still open.
	if err := rows.Close(); err != nil {
		return nil, err
	}
	// Account ONLY the returned messages' delivery rows.
	for _, mk := range marks {
		if mk.decodeErr != nil {
			_, err = tx.Exec(`UPDATE deliveries SET dlq=1, result='decode_error' WHERE subscription_id=? AND message_id=?`, mk.subID, mk.msgID)
		} else if mk.attempts+1 > maxAttempts {
			_, err = tx.Exec(`UPDATE deliveries SET dlq=1 WHERE subscription_id=? AND message_id=?`, mk.subID, mk.msgID)
		} else {
			_, err = tx.Exec(`UPDATE deliveries SET delivered_at=?, attempts=attempts+1
				WHERE subscription_id=? AND message_id=?`, now, mk.subID, mk.msgID)
		}
		if err != nil {
			return nil, fmt.Errorf("pending: account delivery of %s: %w", mk.msgID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	for id, decodeErr := range broken {
		b.emitAudit(agent, "message_dead_lettered", auditJSON(map[string]string{
			"message_id": id, "result": "decode_error", "error": decodeErr.Error(),
		}))
	}
	return out, nil
}

// PendingTotal is the number of unacked, non-DLQ deliveries across all
// subscribers — the channel queue-depth gauge for telemetry (spec §14).
func (b *Bus) PendingTotal() (int, error) {
	var n int
	err := b.db.QueryRow(pendingTotalSQL).Scan(&n)
	return n, err
}

// pendingTotalSQL counts through idx_deliveries_pending, so its cost follows
// the pending backlog rather than the delivery history.
const pendingTotalSQL = `SELECT COUNT(*) FROM deliveries WHERE acked_at IS NULL AND dlq = 0`

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
	now := store.FormatTime(b.clock())
	tx, err := b.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for start := 0; start < len(messageIDs); start += ackBatch {
		batch := messageIDs[start:min(start+ackBatch, len(messageIDs))]
		args := []any{now, agent}
		for _, id := range batch {
			args = append(args, id)
		}
		if _, err := tx.Exec(`UPDATE deliveries SET acked_at=?
			WHERE acked_at IS NULL AND subscription_id IN (SELECT id FROM subscriptions WHERE agent=?)
			  AND message_id IN (?`+strings.Repeat(",?", len(batch)-1)+`)`, args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ackBatch keeps one Ack statement well under SQLite's bound-variable limit.
const ackBatch = 500

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
	rows, err := b.db.Query(`SELECT `+messageColumns+` FROM messages
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
	rows, err := b.db.Query(`SELECT `+messageColumns+` FROM messages
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
	// A revoked subscription is skipped: its owner left the channel, but the
	// deliveries it already holds stay in its queue (migration 0047).
	rows, err := x.Query(`SELECT id, agent, channel, matcher, type_filter, created_at, params, watch, locked
		FROM subscriptions WHERE channel=? AND revoked_at=''`, channel)
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
		if err := json.Unmarshal([]byte(matcher), &s.Matcher); err != nil {
			return Subscription{}, fmt.Errorf("subscription %s: decode matcher: %w", s.ID, err)
		}
	}
	if tf.Valid && tf.String != "" {
		if err := json.Unmarshal([]byte(tf.String), &s.TypeFilter); err != nil {
			return Subscription{}, fmt.Errorf("subscription %s: decode type filter: %w", s.ID, err)
		}
	}
	if params.Valid && params.String != "" {
		s.Params = map[string]any{}
		if err := json.Unmarshal([]byte(params.String), &s.Params); err != nil {
			return Subscription{}, fmt.Errorf("subscription %s: decode params: %w", s.ID, err)
		}
	}
	s.Watch = watch.String
	s.Locked = locked != 0
	return s, nil
}

func scanMessages(rows *sql.Rows) ([]Message, error) {
	defer rows.Close()
	var out []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// errMessageDecode marks a stored message whose JSON columns do not decode.
var errMessageDecode = errors.New("decode stored message")

// scanMessage reads one messageColumns row, then the extra columns that follow
// it. A row whose subject or data does not decode returns the message with its
// id set and an error wrapping errMessageDecode.
func scanMessage(row scanner, extra ...any) (Message, error) {
	var m Message
	var subject string
	var text, data, pa, pi, pp, cid, irt, rt, dl sql.NullString
	dest := append([]any{&m.ID, &m.Channel, &m.TS, &m.Source, &m.Type, &subject, &text, &data,
		&pa, &pi, &pp, &m.Kind, &cid, &irt, &rt, &dl}, extra...)
	if err := row.Scan(dest...); err != nil {
		return Message{}, err
	}
	m.Text, m.ProducedByAgent, m.ProducedInIteration, m.ProducedByPlugin = text.String, pa.String, pi.String, pp.String
	m.CorrelationID, m.InReplyTo, m.ReplyTo, m.Deadline = cid.String, irt.String, rt.String, dl.String
	m.Subject = map[string]any{}
	if subject != "" {
		if err := json.Unmarshal([]byte(subject), &m.Subject); err != nil {
			return m, fmt.Errorf("message %s: subject: %w: %w", m.ID, errMessageDecode, err)
		}
	}
	if data.Valid && data.String != "" {
		m.Data = map[string]any{}
		if err := json.Unmarshal([]byte(data.String), &m.Data); err != nil {
			return m, fmt.Errorf("message %s: data: %w: %w", m.ID, errMessageDecode, err)
		}
	}
	return m, nil
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
