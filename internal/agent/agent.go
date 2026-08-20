// Package agent owns the agents/iterations/secrets tables and their types.
package agent

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// ValidateCwd checks a fully-resolved agent working directory. Empty is allowed
// (the agent falls back to its workdir). A non-empty value must be an absolute
// path to an existing directory; expansion of $CWD/$HOME/~ is the caller's job.
// It returns a plain error; callers in the command layer wrap it as a bad_cwd
// UserError (this package cannot import internal/api without an import cycle).
func ValidateCwd(path string) error {
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("cwd must be an absolute path: %s", path)
	}
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		return fmt.Errorf("cwd is not an existing directory: %s", path)
	}
	return nil
}

var ErrNotFound = errors.New("not found")

// ErrTimeoutNotExtendable means an iteration is not the live, unexpired,
// extendable timeout snapshot. Callers map it to a conflict rather than
// treating it as a database failure.
var ErrTimeoutNotExtendable = errors.New("iteration timeout is not extendable")

// ErrNoIterationTimeout means the iteration deliberately has no positive soft
// timeout snapshot. It is distinct from an old/null row, which is not safely
// extendable either.
var ErrNoIterationTimeout = errors.New("iteration has no soft timeout")

// IdleStopPrefix is the leading token of the reason string SetIdleStopped
// records in status_message when the idle-autostop threshold trips. It is the
// single source of that literal: the loop composes the reason from it,
// StartResetIdle matches on it, and HaltReason tests for it — so an
// agent-authored status line is never mistaken for a halt reason.
const IdleStopPrefix = "idle_limit"

type Agent struct {
	Name        string
	ImageRef    string
	ImageDigest string
	CreatedAt   string
	ErrorReason string // non-empty when the loop is halted abnormally
	Cwd         string
	HarnessType string
	Model       string
	Effort      string
	Interactive bool
	LoopEnabled bool
	// Enabled is the master on/off switch for the whole agent, sitting above
	// LoopEnabled. When false the agent is fully inert (no loop iterations,
	// channels, interactive session, or boot reconcile) regardless of
	// LoopEnabled. Owned by Update (like LoopEnabled).
	Enabled          bool
	IntervalS        int
	TimeoutS         int
	HardTimeoutS     int
	OnTimeout        string // restart | stop
	OnError          string // restart | stop
	UserPrompt       string
	Env              map[string]string
	Plugins          []string
	MessagesBatch    int
	MessagesMaxQueue int
	Group            string
	// StatusMessage is the agent-authored "what I'm doing now" line, written via
	// Store.SetStatus (daemon is the single writer, like ErrorReason). Distinct
	// from the computed live state. StatusUpdated is its RFC3339-ish timestamp.
	StatusMessage string
	StatusUpdated string
	// Alias is a user-set friendly display name; Notes is user scratch text.
	// Both owned by SetAlias/SetNotes (Update never touches them).
	Alias string
	Notes string
	// Color is a user-set accent hex ("#4f8cff") used to tint the agent header.
	// Owned by SetColor (Update never touches it, mirroring Alias/Notes).
	Color string
	// MaxIdleIterations is the idle-autostop threshold: the loop stops after this
	// many consecutive self-declared idle iterations. 0 means never auto-stop on
	// idle. The setter/reconcile arrive in later idle-autostop tasks; for now the
	// column is loaded read-only.
	MaxIdleIterations int
}

type Iteration struct {
	ID        string
	Agent     string
	Trigger   string // interval | manual
	Status    string // running | done | no_i_am_done | harness_error | timeout | killed
	StartedAt string
	EndedAt   string
	ExitCode  *int
	DoneFlag  bool
	// Productive is false only when the agent self-declared this iteration idle
	// via `i-am-done --idle`; a plain done and a no_i_am_done pass leave it true.
	// Owned by SetIterationDone alongside DoneFlag.
	Productive bool
	PromptPath string
	CPUMs      *int
	MemPeakKB  *int
	// Timeout fields are nullable for rows written before timeout snapshots were
	// introduced. A positive TimeoutPeriodS always has both deadlines.
	TimeoutPeriodS       *int
	TimeoutDeadline      *string
	HardTimeoutDeadline  *string
	TimeoutExtensions    int
	TimeoutTriggeredAt   *string
	ImageRef             string
	ImageDigest          string
	PromptTemplateSHA256 string
}

type ImageAssignment struct {
	Ref    string `json:"ref"`
	Digest string `json:"digest"`
	Error  string `json:"error"`
}

func (s *Store) SetPendingImage(name, ref, digest string) error {
	res, err := s.db.Exec(`UPDATE agents SET pending_image_ref=?,pending_image_digest=?,pending_image_error='' WHERE name=?`, ref, digest, name)
	if err != nil {
		return err
	}
	return affected(res)
}
func (s *Store) SetPendingImageError(name, message string) error {
	res, err := s.db.Exec(`UPDATE agents SET pending_image_error=? WHERE name=?`, message, name)
	if err != nil {
		return err
	}
	return affected(res)
}
func (s *Store) SetPendingImageErrorIf(name, ref, digest, message string) error {
	res, err := s.db.Exec(`UPDATE agents SET pending_image_error=? WHERE name=? AND pending_image_ref=? AND pending_image_digest=?`, message, name, ref, digest)
	if err != nil {
		return err
	}
	return affected(res)
}
func (s *Store) ClearPendingImage(name string) error {
	res, err := s.db.Exec(`UPDATE agents SET pending_image_ref='',pending_image_digest='',pending_image_error='' WHERE name=?`, name)
	if err != nil {
		return err
	}
	return affected(res)
}
func (s *Store) PendingImage(name string) (ImageAssignment, error) {
	var out ImageAssignment
	err := s.db.QueryRow(`SELECT pending_image_ref,pending_image_digest,pending_image_error FROM agents WHERE name=?`, name).Scan(&out.Ref, &out.Digest, &out.Error)
	if err == sql.ErrNoRows {
		return ImageAssignment{}, ErrNotFound
	}
	return out, err
}
func (s *Store) PromotePendingImage(name string) error {
	current, err := s.Get(name)
	if err != nil {
		return err
	}
	pending, err := s.PendingImage(name)
	if err != nil {
		return err
	}
	return s.PromotePendingImageWithPlugins(name, pending.Ref, pending.Digest, current.Plugins)
}

func (s *Store) PromotePendingImageWithPlugins(name, expectedRef, expectedDigest string, plugins []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if expectedRef == "" {
		return errors.New("no pending image")
	}
	encoded, err := json.Marshal(plugins)
	if err != nil {
		return err
	}
	if plugins == nil {
		encoded = []byte("[]")
	}
	res, err := tx.Exec(`UPDATE agents SET image_ref=?,image_digest=?,plugins=?,pending_image_ref='',pending_image_digest='',pending_image_error='' WHERE name=? AND pending_image_ref=? AND pending_image_digest=?`, expectedRef, expectedDigest, string(encoded), name, expectedRef, expectedDigest)
	if err != nil {
		return err
	}
	if err := affected(res); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) SnapshotIterationImage(iterationID, ref, digest, templateSHA string) error {
	res, err := s.db.Exec(`UPDATE iterations SET image_ref=?,image_digest=?,prompt_template_sha256=? WHERE id=?`, ref, digest, templateSHA, iterationID)
	if err != nil {
		return err
	}
	return affected(res)
}

type Store struct{ db *sql.DB }

func NewStore(s *store.Store) *Store { return &Store{db: s.DB} }

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Store) Create(a Agent) error {
	// Defense in depth: reject a traversing/malformed name here so no code path
	// can persist (or, via the caller's provisioning, materialise on disk) an
	// agent whose name escapes AgentsDir. Callers validate earlier too.
	if !ValidName(a.Name) {
		return fmt.Errorf("%w %q: must match ^[a-z0-9][a-z0-9_-]*$", ErrInvalidName, a.Name)
	}
	env, err := json.Marshal(a.Env)
	if err != nil {
		return err
	}
	if a.Env == nil {
		env = []byte("{}")
	}
	plugins, err := json.Marshal(a.Plugins)
	if err != nil {
		return err
	}
	if a.Plugins == nil {
		plugins = []byte("[]")
	}
	if a.MessagesBatch == 0 {
		a.MessagesBatch = 10
	}
	if a.MessagesMaxQueue == 0 {
		a.MessagesMaxQueue = 1000
	}
	_, err = s.db.Exec(`INSERT INTO agents
		(name, image_ref, image_digest, error_reason, cwd, harness_type, model, effort,
		 interactive, loop_enabled, enabled, interval_s, timeout_s, hard_timeout_s,
		 on_timeout, on_error, user_prompt, env, plugins, messages_batch, messages_max_queue, "group", alias, notes, color)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.Name, a.ImageRef, a.ImageDigest, a.ErrorReason, a.Cwd, a.HarnessType, a.Model, a.Effort,
		b2i(a.Interactive), b2i(a.LoopEnabled), b2i(a.Enabled), a.IntervalS, a.TimeoutS, a.HardTimeoutS,
		a.OnTimeout, a.OnError, a.UserPrompt, string(env), string(plugins),
		a.MessagesBatch, a.MessagesMaxQueue, a.Group, a.Alias, a.Notes, a.Color)
	return err
}

func (s *Store) Update(a Agent) error {
	env, _ := json.Marshal(a.Env)
	if a.Env == nil {
		env = []byte("{}")
	}
	plugins, _ := json.Marshal(a.Plugins)
	if a.Plugins == nil {
		plugins = []byte("[]")
	}
	res, err := s.db.Exec(`UPDATE agents SET
		cwd=?, harness_type=?, model=?, effort=?,
		interactive=?, loop_enabled=?, enabled=?, interval_s=?, timeout_s=?, hard_timeout_s=?,
		on_timeout=?, on_error=?, user_prompt=?, env=?, plugins=?,
		messages_batch=?, messages_max_queue=?, max_idle_iterations=? WHERE name=?`,
		a.Cwd, a.HarnessType, a.Model, a.Effort,
		b2i(a.Interactive), b2i(a.LoopEnabled), b2i(a.Enabled), a.IntervalS, a.TimeoutS, a.HardTimeoutS,
		a.OnTimeout, a.OnError, a.UserPrompt, string(env), string(plugins),
		a.MessagesBatch, a.MessagesMaxQueue, a.MaxIdleIterations, a.Name)
	if err != nil {
		return err
	}
	return affected(res)
}

func (s *Store) SetImageIdentity(name, ref, digest string) error {
	res, err := s.db.Exec(`UPDATE agents SET image_ref=?,image_digest=? WHERE name=?`, ref, digest, name)
	if err != nil {
		return err
	}
	return affected(res)
}

// HaltReason reports why the loop is off, derived purely from the two columns
// that already record it — no database access and no I/O. A stop policy
// (on_error/on_timeout) halt lands in ErrorReason via SetError; an idle-autostop
// lands in StatusMessage via SetIdleStopped, which deliberately leaves
// ErrorReason alone, so both can be populated at once. An error halt wins,
// being the more urgent explanation. A StatusMessage without the idle prefix is
// an agent-authored status line and is never a halt reason.
func (a *Agent) HaltReason() (kind, reason string) {
	switch {
	case a.ErrorReason != "":
		return "error", a.ErrorReason
	case strings.HasPrefix(a.StatusMessage, IdleStopPrefix):
		return IdleStopPrefix, a.StatusMessage
	default:
		return "", ""
	}
}

// SetError halts an agent's loop with a reason: it sets error_reason and clears
// loop_enabled in one statement. error_reason is owned solely by
// SetError/ClearError (Update never touches it, mirroring SetGroup).
func (s *Store) SetError(name, reason string) error {
	res, err := s.db.Exec(`UPDATE agents SET error_reason=?, loop_enabled=0 WHERE name=?`, reason, name)
	if err != nil {
		return err
	}
	return affected(res)
}

// SetIdleStopped cleanly halts an agent's loop after the idle-autostop threshold
// is reached: it clears both loop_enabled and the master enabled switch, and
// records a non-error reason in status_message (with its timestamp) in one
// statement. It deliberately does NOT touch error_reason, so the agent
// surfaces as derived state=stopped (a gentle stop, like Manager.Stop) rather
// than error. Mirrors SetError's single-write shape but for a clean, expected
// halt.
func (s *Store) SetIdleStopped(name, reason, updated string) error {
	res, err := s.db.Exec(`UPDATE agents SET loop_enabled=0, enabled=0, status_message=?, status_updated=? WHERE name=?`,
		reason, updated, name)
	if err != nil {
		return err
	}
	return affected(res)
}

// ClearError clears an agent's halt reason (called on the next successful start).
func (s *Store) ClearError(name string) error {
	res, err := s.db.Exec(`UPDATE agents SET error_reason='' WHERE name=?`, name)
	if err != nil {
		return err
	}
	return affected(res)
}

// StartResetIdle establishes the idle-autostop restart boundary and clears any
// stale idle-stop status residue. Called on Start/restart alongside ClearError.
// It (1) moves idle_reset_rowid to the newest iteration rowid so IdleStreak counts
// only iterations after this Start — granting a fresh max_idle_iterations budget
// instead of re-tripping on the pre-restart streak — and (2) clears status_message
// only when it still holds the idle_limit reason SetIdleStopped wrote, so a real
// agent-authored status is never clobbered. Both happen in one statement.
func (s *Store) StartResetIdle(name string) error {
	res, err := s.db.Exec(`UPDATE agents SET
		idle_reset_rowid = COALESCE((SELECT MAX(rowid) FROM iterations WHERE agent=?), 0),
		status_message = CASE WHEN status_message LIKE ? THEN '' ELSE status_message END
		WHERE name=?`, name, IdleStopPrefix+"%", name)
	if err != nil {
		return err
	}
	return affected(res)
}

// SetStatus records the agent-authored status message ("what I'm doing now") and
// its update timestamp in one statement. status_message/status_updated are owned
// solely by SetStatus (Update never touches them, mirroring SetError/SetGroup),
// so an agent's status line survives an unrelated config Update.
func (s *Store) SetStatus(name, message, updated string) error {
	res, err := s.db.Exec(`UPDATE agents SET status_message=?, status_updated=? WHERE name=?`,
		message, updated, name)
	if err != nil {
		return err
	}
	return affected(res)
}

// SetAlias sets (or clears with "") an agent's display alias. Owned solely by
// this setter — Update never touches alias, mirroring SetGroup/SetStatus.
func (s *Store) SetAlias(name, alias string) error {
	res, err := s.db.Exec(`UPDATE agents SET alias=? WHERE name=?`, alias, name)
	if err != nil {
		return err
	}
	return affected(res)
}

// SetNotes sets (or clears with "") an agent's free-form notes. Owned solely by
// this setter — Update never touches notes.
func (s *Store) SetNotes(name, notes string) error {
	res, err := s.db.Exec(`UPDATE agents SET notes=? WHERE name=?`, notes, name)
	if err != nil {
		return err
	}
	return affected(res)
}

// SetColor sets (or clears with "") an agent's accent color. Owned solely by
// this setter — Update never touches color, mirroring SetAlias/SetNotes.
func (s *Store) SetColor(name, color string) error {
	res, err := s.db.Exec(`UPDATE agents SET color=? WHERE name=?`, color, name)
	if err != nil {
		return err
	}
	return affected(res)
}

func (s *Store) Get(name string) (Agent, error) {
	row := s.db.QueryRow(`SELECT name, image_ref, image_digest, created_at, error_reason, cwd,
		harness_type, model, effort, interactive, loop_enabled, enabled, interval_s, timeout_s,
		hard_timeout_s, on_timeout, on_error, user_prompt, env, plugins,
		messages_batch, messages_max_queue, "group", status_message, status_updated, alias, notes, color, max_idle_iterations
		FROM agents WHERE name=?`, name)
	return scanAgent(row)
}

func (s *Store) List() ([]Agent, error) {
	rows, err := s.db.Query(`SELECT name, image_ref, image_digest, created_at, error_reason, cwd,
		harness_type, model, effort, interactive, loop_enabled, enabled, interval_s, timeout_s,
		hard_timeout_s, on_timeout, on_error, user_prompt, env, plugins,
		messages_batch, messages_max_queue, "group", status_message, status_updated, alias, notes, color, max_idle_iterations
		FROM agents ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Delete(name string) error {
	if _, err := s.db.Exec(`DELETE FROM iterations WHERE agent=?`, name); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM secrets WHERE agent=?`, name); err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM agents WHERE name=?`, name)
	if err != nil {
		return err
	}
	return affected(res)
}

// PurgeAgentData deletes every agent-keyed row Delete leaves behind, so a real
// (purge) remove leaves nothing orphaned. Delete only clears iterations/secrets/
// agents; these side-tables key by the agent name (subscriptions/schedules/
// scripts/script_result_outbox/ai_requests/retention_policies/eval_results,
// plus subscription deliveries) or by the scope "agent:<name>"
// (budgets/proxy_rules). All live in
// the one SQLite DB. The deletes share one transaction so script completion
// cannot enqueue a result between clearing scripts and clearing their outbox,
// and any statement failure rolls the entire purge back. It is a no-op-safe
// superset: rows that never existed simply match nothing.
//
// eval_results carries a direct agent column (populated at both runner insert
// sites), so it is keyed on agent here rather than via an iteration subquery —
// which sidesteps any ordering dependency on Delete's DELETE FROM iterations.
func (s *Store) PurgeAgentData(name string) error {
	scope := "agent:" + name
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmts := []struct {
		sql  string
		args []any
	}{
		// deliveries reference subscriptions by id, so clear them first.
		{`DELETE FROM deliveries WHERE subscription_id IN (SELECT id FROM subscriptions WHERE agent=?)`, []any{name}},
		{`DELETE FROM subscriptions WHERE agent=?`, []any{name}},
		{`DELETE FROM schedules WHERE agent=?`, []any{name}},
		{`DELETE FROM scripts WHERE agent=?`, []any{name}},
		{`DELETE FROM script_result_outbox WHERE agent=?`, []any{name}},
		{`DELETE FROM ai_requests WHERE agent=?`, []any{name}},
		{`DELETE FROM retention_policies WHERE agent=?`, []any{name}},
		{`DELETE FROM eval_results WHERE agent=?`, []any{name}},
		{`DELETE FROM budgets WHERE scope=?`, []any{scope}},
		{`DELETE FROM proxy_rules WHERE scope=?`, []any{scope}},
	}
	for _, st := range stmts {
		if _, err := tx.Exec(st.sql, st.args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Exists(name string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM agents WHERE name=?`, name).Scan(&n)
	return n > 0, err
}

// SetGroup sets (or clears, with "") an agent's group membership. It is the
// only writer of the agents."group" column besides Create, so env-only Update
// calls during reconcile never clobber membership.
func (s *Store) SetGroup(name, group string) error {
	res, err := s.db.Exec(`UPDATE agents SET "group"=? WHERE name=?`, group, name)
	if err != nil {
		return err
	}
	return affected(res)
}

// ListByGroup returns the members of a group, ordered by name.
func (s *Store) ListByGroup(group string) ([]Agent, error) {
	rows, err := s.db.Query(`SELECT name, image_ref, image_digest, created_at, error_reason, cwd,
		harness_type, model, effort, interactive, loop_enabled, enabled, interval_s, timeout_s,
		hard_timeout_s, on_timeout, on_error, user_prompt, env, plugins,
		messages_batch, messages_max_queue, "group", status_message, status_updated, alias, notes, color, max_idle_iterations
		FROM agents WHERE "group"=? ORDER BY name`, group)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) NextIterationID(agentName string, now time.Time) (string, error) {
	ts := now.UTC().Format("20060102150405")
	prefix := agentName + "-" + ts + "-"
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM iterations WHERE id LIKE ?`, prefix+"%").Scan(&n); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%d", prefix, n+1), nil
}

func (s *Store) CreateIteration(it Iteration) error {
	// productive is intentionally omitted: it relies on the column DEFAULT 1, so a
	// fresh iteration (and one that ends no_i_am_done) stays productive until an
	// explicit `i-am-done --idle` flips it via SetIterationDone.
	_, err := s.db.Exec(`INSERT INTO iterations
		(id, agent, trigger, status, started_at, ended_at, exit_code, done_flag, prompt_path, cpu_ms, mem_peak_kb,
		 image_ref, image_digest, prompt_template_sha256)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		it.ID, it.Agent, it.Trigger, it.Status, it.StartedAt, it.EndedAt,
		it.ExitCode, b2i(it.DoneFlag), it.PromptPath, it.CPUMs, it.MemPeakKB,
		it.ImageRef, it.ImageDigest, it.PromptTemplateSHA256)
	return err
}

// InitializeIterationTimeout persists the timeout snapshot from the one clock
// sample made immediately before shim spawn. A zero soft period has no soft
// deadline; a non-positive hard period has no explicit hard deadline.
func (s *Store) InitializeIterationTimeout(id string, periodS, hardPeriodS int, now time.Time) error {
	var softDeadline, hardDeadline any
	if periodS > 0 {
		softDeadline = now.Add(time.Duration(periodS) * time.Second).UTC().Format(time.RFC3339Nano)
	}
	if hardPeriodS > 0 {
		hardDeadline = now.Add(time.Duration(hardPeriodS) * time.Second).UTC().Format(time.RFC3339Nano)
	}
	res, err := s.db.Exec(`UPDATE iterations SET timeout_period_s=?, timeout_deadline=?,
		hard_timeout_deadline=?, timeout_extensions=0, timeout_triggered_at=NULL WHERE id=?`,
		periodS, softDeadline, hardDeadline, id)
	if err != nil {
		return err
	}
	return affected(res)
}

// ExtendIterationTimeout atomically validates and advances the persisted
// timeout snapshot. Keeping the read, expiry check, and write in one SQLite
// transaction makes concurrent extensions additive and lets a terminal update
// win if it commits first.
func (s *Store) ExtendIterationTimeout(agentName, id string, now time.Time) (Iteration, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Iteration{}, err
	}
	defer tx.Rollback()
	row := tx.QueryRow(`SELECT id, agent, trigger, status, started_at, ended_at,
		exit_code, done_flag, productive, prompt_path, cpu_ms, mem_peak_kb,
			timeout_period_s, timeout_deadline, hard_timeout_deadline,
			timeout_extensions, timeout_triggered_at, image_ref, image_digest, prompt_template_sha256
		FROM iterations WHERE agent=? AND id=?`, agentName, id)
	it, err := scanIteration(row)
	if err != nil {
		return Iteration{}, err
	}
	if it.Status != "running" || it.TimeoutTriggeredAt != nil {
		return Iteration{}, ErrTimeoutNotExtendable
	}
	if it.TimeoutPeriodS == nil {
		return Iteration{}, ErrTimeoutNotExtendable
	}
	if *it.TimeoutPeriodS <= 0 {
		return Iteration{}, ErrNoIterationTimeout
	}
	if it.TimeoutDeadline == nil || it.HardTimeoutDeadline == nil {
		return Iteration{}, ErrTimeoutNotExtendable
	}
	soft, err := time.Parse(time.RFC3339Nano, *it.TimeoutDeadline)
	if err != nil {
		return Iteration{}, fmt.Errorf("parse timeout deadline: %w", err)
	}
	hard, err := time.Parse(time.RFC3339Nano, *it.HardTimeoutDeadline)
	if err != nil {
		return Iteration{}, fmt.Errorf("parse hard timeout deadline: %w", err)
	}
	if !now.Before(soft) || !now.Before(hard) {
		return Iteration{}, ErrTimeoutNotExtendable
	}
	period := time.Duration(*it.TimeoutPeriodS) * time.Second
	newSoft := soft.Add(period).UTC().Format(time.RFC3339Nano)
	newHard := hard.Add(period).UTC().Format(time.RFC3339Nano)
	res, err := tx.Exec(`UPDATE iterations SET timeout_deadline=?, hard_timeout_deadline=?,
		timeout_extensions=timeout_extensions+1
		WHERE id=? AND agent=? AND status='running' AND timeout_triggered_at IS NULL`,
		newSoft, newHard, id, agentName)
	if err != nil {
		return Iteration{}, err
	}
	if err := affected(res); err != nil {
		return Iteration{}, ErrTimeoutNotExtendable
	}
	if err := tx.Commit(); err != nil {
		return Iteration{}, err
	}
	it.TimeoutDeadline = &newSoft
	it.HardTimeoutDeadline = &newHard
	it.TimeoutExtensions++
	return it, nil
}

// MarkIterationTimeoutTriggered records the durable start of soft-timeout
// enforcement. The expected deadline makes the update a CAS: an extension
// that commits after the observer's read wins and the stale observer must not
// kill the iteration. It also only succeeds for a still-running iteration.
func (s *Store) MarkIterationTimeoutTriggered(agentName, id, expectedDeadline, at string) (bool, error) {
	res, err := s.db.Exec(`UPDATE iterations SET timeout_triggered_at=?
		WHERE id=? AND agent=? AND status='running' AND timeout_triggered_at IS NULL
		AND timeout_deadline=?`, at, id, agentName, expectedDeadline)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// UpdateIteration updates the mutable result fields of an iteration. It does
// not touch done_flag, which is owned exclusively by SetIterationDone, nor
// prompt_path, which is owned exclusively by SetIterationPromptPath. Passing
// an Iteration with those fields unset (as engine.go does) must never clobber
// values written by those setters.
func (s *Store) UpdateIteration(it Iteration) error {
	res, err := s.db.Exec(`UPDATE iterations SET status=?, ended_at=?, exit_code=?,
		cpu_ms=?, mem_peak_kb=? WHERE id=?`,
		it.Status, it.EndedAt, it.ExitCode, it.CPUMs, it.MemPeakKB, it.ID)
	if err != nil {
		return err
	}
	return affected(res)
}

// SetIterationDone flips done_flag and records whether the iteration was
// productive. productive=false is written only when the agent self-declared the
// pass idle (`i-am-done --idle`); a plain done passes productive=true.
func (s *Store) SetIterationDone(id string, productive bool) error {
	res, err := s.db.Exec(`UPDATE iterations SET done_flag=1, productive=? WHERE id=?`, b2i(productive), id)
	if err != nil {
		return err
	}
	return affected(res)
}

func (s *Store) SetIterationPromptPath(id, path string) error {
	res, err := s.db.Exec(`UPDATE iterations SET prompt_path=? WHERE id=?`, path, id)
	if err != nil {
		return err
	}
	return affected(res)
}

func (s *Store) GetIteration(agentName, id string) (Iteration, error) {
	row := s.db.QueryRow(`SELECT id, agent, trigger, status, started_at, ended_at,
		exit_code, done_flag, productive, prompt_path, cpu_ms, mem_peak_kb,
		timeout_period_s, timeout_deadline, hard_timeout_deadline,
		timeout_extensions, timeout_triggered_at, image_ref, image_digest, prompt_template_sha256
		FROM iterations WHERE agent=? AND id=?`, agentName, id)
	return scanIteration(row)
}

func (s *Store) ListIterations(agentName string) ([]Iteration, error) {
	rows, err := s.db.Query(`SELECT id, agent, trigger, status, started_at, ended_at,
		exit_code, done_flag, productive, prompt_path, cpu_ms, mem_peak_kb,
		timeout_period_s, timeout_deadline, hard_timeout_deadline,
		timeout_extensions, timeout_triggered_at, image_ref, image_digest, prompt_template_sha256
		FROM iterations WHERE agent=? ORDER BY started_at, id`, agentName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Iteration
	for rows.Next() {
		it, err := scanIteration(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// IdleStreak returns how many of the most-recent iterations the agent
// self-declared idle (productive=0), counted newest-first and stopping at the
// first productive iteration. Abnormal outcomes (timeout/harness_error/
// no_i_am_done) default to productive=1 (Task 1), so they break the streak
// rather than extend it. The count is bounded to iterations after the last
// Start/restart (agents.idle_reset_rowid, written by StartResetIdle): a restart
// grants a fresh idle budget rather than re-tripping on the historical streak.
// Derived purely from persisted rows, so it is restart-safe: a daemon restart
// mid-streak recomputes the same count (the boundary is persisted too).
func (s *Store) IdleStreak(agentName string) (int, error) {
	// rowid DESC is true insertion order (newest-first), robust against
	// same-second started_at ties and the lexical ordering of numeric id
	// suffixes; iterations is a rowid table, so rowid is monotonic and stable.
	// The rowid > boundary predicate scopes the streak to iterations recorded
	// since this agent's last Start (see StartResetIdle).
	rows, err := s.db.Query(`SELECT i.productive FROM iterations i
		WHERE i.agent=?
		  AND i.rowid > (SELECT idle_reset_rowid FROM agents WHERE name=?)
		ORDER BY i.rowid DESC`, agentName, agentName)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	streak := 0
	for rows.Next() {
		var productive int
		if err := rows.Scan(&productive); err != nil {
			return 0, err
		}
		if productive != 0 {
			break
		}
		streak++
	}
	return streak, rows.Err()
}

func (s *Store) SecretSet(agentName, key, value string) error {
	_, err := s.db.Exec(`INSERT INTO secrets(agent, key, value) VALUES (?,?,?)
		ON CONFLICT(agent, key) DO UPDATE SET value=excluded.value`, agentName, key, value)
	return err
}

func (s *Store) SecretKeys(agentName string) ([]string, error) {
	rows, err := s.db.Query(`SELECT key FROM secrets WHERE agent=? ORDER BY key`, agentName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) SecretMap(agentName string) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM secrets WHERE agent=?`, agentName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (s *Store) SecretRemove(agentName, key string) error {
	res, err := s.db.Exec(`DELETE FROM secrets WHERE agent=? AND key=?`, agentName, key)
	if err != nil {
		return err
	}
	return affected(res)
}

type scanner interface{ Scan(dest ...any) error }

func scanAgent(row scanner) (Agent, error) {
	var a Agent
	var interactive, loopEnabled, enabled int
	var env, plugins string
	err := row.Scan(&a.Name, &a.ImageRef, &a.ImageDigest, &a.CreatedAt, &a.ErrorReason, &a.Cwd,
		&a.HarnessType, &a.Model, &a.Effort, &interactive, &loopEnabled, &enabled, &a.IntervalS,
		&a.TimeoutS, &a.HardTimeoutS, &a.OnTimeout, &a.OnError, &a.UserPrompt, &env, &plugins,
		&a.MessagesBatch, &a.MessagesMaxQueue, &a.Group, &a.StatusMessage, &a.StatusUpdated, &a.Alias, &a.Notes, &a.Color,
		&a.MaxIdleIterations)
	if err == sql.ErrNoRows {
		return Agent{}, ErrNotFound
	}
	if err != nil {
		return Agent{}, err
	}
	a.Interactive = interactive != 0
	a.LoopEnabled = loopEnabled != 0
	a.Enabled = enabled != 0
	if err := json.Unmarshal([]byte(env), &a.Env); err != nil {
		return Agent{}, fmt.Errorf("decode env of %s: %w", a.Name, err)
	}
	if err := json.Unmarshal([]byte(plugins), &a.Plugins); err != nil {
		return Agent{}, fmt.Errorf("decode plugins of %s: %w", a.Name, err)
	}
	return a, nil
}

func scanIteration(row scanner) (Iteration, error) {
	var it Iteration
	var done, productive int
	err := row.Scan(&it.ID, &it.Agent, &it.Trigger, &it.Status, &it.StartedAt, &it.EndedAt,
		&it.ExitCode, &done, &productive, &it.PromptPath, &it.CPUMs, &it.MemPeakKB,
		&it.TimeoutPeriodS, &it.TimeoutDeadline, &it.HardTimeoutDeadline,
		&it.TimeoutExtensions, &it.TimeoutTriggeredAt, &it.ImageRef, &it.ImageDigest, &it.PromptTemplateSHA256)
	if err == sql.ErrNoRows {
		return Iteration{}, ErrNotFound
	}
	if err != nil {
		return Iteration{}, err
	}
	it.DoneFlag = done != 0
	it.Productive = productive != 0
	return it, nil
}

func affected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
