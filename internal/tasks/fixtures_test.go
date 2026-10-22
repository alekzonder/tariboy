package tasks

// Fixture builders and read-back helpers shared by the package's tests.
//
// Read helpers go through Service methods. The few helpers below that still
// hold SQL do so because the state they set or read belongs to another
// package (agents, bus messages, published images) or is an engine counter no
// API exposes; each says which. Keeping that SQL here means a schema change
// touches one file instead of every test.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/workflowfile"
	"github.com/alekzonder/tariboy/internal/workflowimage"
)

// operatorOf is the customer actor of svc, who may read every task.
func operatorOf(svc *Service) Actor { return CustomerActor(svc.CustomerLogin()) }

// --- Reads through the Service API ---

// workflowView returns the task's workflow view as the operator sees it.
func workflowView(t *testing.T, svc *Service, task Task) WorkflowView {
	t.Helper()
	view, err := svc.GetWorkflow(context.Background(), operatorOf(svc), task.Key)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

// openStatusVisit returns the task's open visit.
func openStatusVisit(t *testing.T, svc *Service, task Task) StatusVisit {
	t.Helper()
	for _, visit := range workflowView(t, svc, task).Visits {
		if visit.LeftAt == "" {
			return visit
		}
	}
	t.Fatalf("task %s has no open visit", task.Key)
	return StatusVisit{}
}

// lastLeftVisit returns the most recent visit of status that was left.
func lastLeftVisit(t *testing.T, svc *Service, task Task, status string) StatusVisit {
	t.Helper()
	visits := workflowView(t, svc, task).Visits
	for i := len(visits) - 1; i >= 0; i-- {
		if visits[i].Status == status && visits[i].LeftAt != "" {
			return visits[i]
		}
	}
	t.Fatalf("task %s never left %q", task.Key, status)
	return StatusVisit{}
}

// visitBySequence returns the task's visit with the given sequence.
func visitBySequence(t *testing.T, svc *Service, task Task, sequence int64) StatusVisit {
	t.Helper()
	for _, visit := range workflowView(t, svc, task).Visits {
		if visit.Sequence == sequence {
			return visit
		}
	}
	t.Fatalf("task %s has no visit %d", task.Key, sequence)
	return StatusVisit{}
}

// transitionRequest returns one transition request of task.
func transitionRequest(t *testing.T, svc *Service, task Task, id int64) TransitionRequest {
	t.Helper()
	request, err := svc.GetTransitionRequest(context.Background(), operatorOf(svc), task.Key, id)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func requestState(t *testing.T, svc *Service, task Task, id int64) string {
	t.Helper()
	return string(transitionRequest(t, svc, task, id).State)
}

// requestRow returns the request's state and result message.
func requestRow(t *testing.T, svc *Service, task Task, id int64) (string, string) {
	t.Helper()
	request := transitionRequest(t, svc, task, id)
	return string(request.State), request.ResultMessage
}

// runState returns the run's state, verdict, and whether a running run has a
// cancellation requested.
func runState(t *testing.T, svc *Service, task Task, id int64) (string, string, bool) {
	t.Helper()
	ctx := context.Background()
	run, err := svc.GetScriptRun(ctx, operatorOf(svc), task.Key, id)
	if err != nil {
		t.Fatal(err)
	}
	toKill, err := svc.CancelRequestedRuns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel := false
	for _, r := range toKill {
		cancel = cancel || r.ID == id
	}
	return string(run.State), string(run.Verdict), cancel
}

// taskEvents returns every event of task, oldest first.
func taskEvents(t *testing.T, svc *Service, task Task) []Event {
	t.Helper()
	var all []Event
	var after int64
	for {
		page, err := svc.ListEvents(context.Background(), operatorOf(svc), task.Key, after, 500)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, page...)
		if len(page) < 500 {
			return all
		}
		after = page[len(page)-1].Sequence
	}
}

// queueEvents returns the events of queue (any queue when empty), task events
// included, whose kind starts with prefix, oldest first.
func queueEvents(t *testing.T, svc *Service, queue, prefix string) []Event {
	t.Helper()
	var out []Event
	var after int64
	for {
		page, err := svc.ListEvents(context.Background(), operatorOf(svc), "", after, 500)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range page {
			if (queue == "" || event.Queue == queue) && strings.HasPrefix(event.Kind, prefix) {
				out = append(out, event)
			}
		}
		if len(page) < 500 {
			return out
		}
		after = page[len(page)-1].Sequence
	}
}

// eventPayload returns the payload of task's newest event of kind.
func eventPayload(t *testing.T, svc *Service, task Task, kind string) map[string]any {
	t.Helper()
	events := taskEvents(t, svc, task)
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == kind {
			return events[i].Payload
		}
	}
	t.Fatalf("event %s: none", kind)
	return nil
}

func countEvents(t *testing.T, svc *Service, task Task, kind string) int {
	t.Helper()
	n := 0
	for _, event := range taskEvents(t, svc, task) {
		if event.Kind == kind {
			n++
		}
	}
	return n
}

// queueTasks returns every task of queue the operator sees, oldest first.
func queueTasks(t *testing.T, svc *Service, queue string) []Task {
	t.Helper()
	var all []Task
	after := ""
	for {
		page, err := svc.ListTasks(context.Background(), operatorOf(svc), ListFilter{Queue: queue, Sort: "created", AfterKey: after})
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, page.Tasks...)
		if page.NextCursor == "" || len(page.Tasks) == 0 {
			return all
		}
		after = page.NextCursor
	}
}

// tasksMentioning returns the tasks of queue whose description contains text.
func tasksMentioning(t *testing.T, svc *Service, queue, text string) []Task {
	t.Helper()
	var out []Task
	for _, task := range queueTasks(t, svc, queue) {
		if strings.Contains(task.Description, text) {
			out = append(out, task)
		}
	}
	return out
}

// --- Fixtures that need SQL ---
//
// Agents, bus messages, and published workflow images are written by other
// packages; these helpers seed them the way those packages store them.

// seedAgent records an agent the way the agents registry does; group may be
// empty.
func seedAgent(t *testing.T, svc *Service, name, group string) {
	t.Helper()
	if _, err := svc.db.Exec(`INSERT INTO agents(name, image_ref, image_digest, "group") VALUES (?, 'basic:latest', 'digest', ?)`,
		name, group); err != nil {
		t.Fatal(err)
	}
}

// enableAgents makes every agent enabled with its Goal on, as the registry
// does when an operator turns them on.
func enableAgents(t *testing.T, svc *Service) {
	t.Helper()
	if _, err := svc.db.Exec(`UPDATE agents SET enabled = 1, goal_enabled = 1`); err != nil {
		t.Fatal(err)
	}
}

// setAgent applies SQL assignments to one agent's registry row.
func setAgent(t *testing.T, svc *Service, name, assignments string) {
	t.Helper()
	if _, err := svc.db.Exec(`UPDATE agents SET `+assignments+` WHERE name = ?`, name); err != nil {
		t.Fatal(err)
	}
}

// agentGoalTaskKey returns the agent's current Goal task key from the
// registry row the tasks package maintains.
func agentGoalTaskKey(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	var key string
	if err := db.QueryRow(`SELECT current_goal_task_key FROM agents WHERE name = ?`, name).Scan(&key); err != nil {
		t.Fatal(err)
	}
	return key
}

// storeManifest records m as a published workflow image, as the image store
// does after a build.
func storeManifest(t *testing.T, svc *Service, m workflowimage.Manifest) {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`INSERT INTO task_workflow_images(digest, name, version, manifest, built_at) VALUES (?, ?, ?, ?, ?)`,
		m.Digest, m.Name, m.Version, string(raw), m.BuiltAt); err != nil {
		t.Fatal(err)
	}
}

// resolveStoredImage resolves name[:version] or a digest to the newest stored
// image, as the image store's resolver does.
func resolveStoredImage(svc *Service, name, tag string) (string, error) {
	var digest string
	err := svc.db.QueryRow(`SELECT digest FROM task_workflow_images
		WHERE (name = ? AND (? = '' OR version = ?)) OR digest = ? ORDER BY built_at DESC, rowid DESC LIMIT 1`,
		name, tag, tag, tag).Scan(&digest)
	return digest, err
}

// editManifest rewrites a published image in place. Published images are
// immutable, so no API does this; tests use it to change the definition a
// task already pinned.
func editManifest(t *testing.T, svc *Service, digest string, edit func(*workflowimage.Manifest)) {
	t.Helper()
	m, err := loadManifestTx(context.Background(), svc.db, digest)
	if err != nil {
		t.Fatal(err)
	}
	edit(&m)
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`UPDATE task_workflow_images SET manifest = ? WHERE digest = ?`, string(raw), digest); err != nil {
		t.Fatal(err)
	}
}

// rewriteDefinition replaces the definition in the task's pinned manifest.
func rewriteDefinition(t *testing.T, svc *Service, task Task, def workflowfile.File) {
	t.Helper()
	editManifest(t, svc, task.WorkflowDigest, func(m *workflowimage.Manifest) { m.Definition = def })
}

// legacyMessage is a bus message row written with a chosen id and timestamp.
type legacyMessage struct {
	ID, Channel, TS, Source, Type, Text string
	// Sequenced also records the message's commit sequence, as the bus does
	// for every message it publishes; a row without one is what a database
	// written before sequences existed holds.
	Sequenced bool
}

// insertMessage writes msg the way the bus stores it, with the id and
// timestamp the bus would otherwise choose itself, and returns its commit
// sequence (0 when unsequenced). q may be a transaction.
func insertMessage(t *testing.T, q interface {
	Exec(string, ...any) (sql.Result, error)
}, msg legacyMessage) int64 {
	t.Helper()
	if msg.Source == "" {
		msg.Source = "plugin:test"
	}
	if msg.Type == "" {
		msg.Type = "event"
	}
	if _, err := q.Exec(`INSERT INTO channels(name,kind,created_at) VALUES(?,'chat',?) ON CONFLICT DO NOTHING`, msg.Channel, msg.TS); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Exec(`INSERT INTO messages(id,channel,ts,source,type,subject,text,data,kind) VALUES(?,?,?,?,?,'{}',?,'{}','event')`,
		msg.ID, msg.Channel, msg.TS, msg.Source, msg.Type, msg.Text); err != nil {
		t.Fatal(err)
	}
	if !msg.Sequenced {
		return 0
	}
	res, err := q.Exec(`INSERT INTO task_workflow_message_sequence(message_id) VALUES(?)`, msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return sequence
}

// messageSequence returns the commit sequence recorded for a message.
func messageSequence(t *testing.T, svc *Service, id string) int64 {
	t.Helper()
	var sequence int64
	if err := svc.db.QueryRow(`SELECT sequence FROM task_workflow_message_sequence WHERE message_id = ?`, id).Scan(&sequence); err != nil {
		t.Fatal(err)
	}
	return sequence
}

// ingressCursor returns the last message sequence workflow ingress consumed;
// no API exposes it.
func ingressCursor(t *testing.T, svc *Service) int64 {
	t.Helper()
	var cursor int64
	if err := svc.db.QueryRow(`SELECT last_message_sequence FROM task_workflow_ingress_state WHERE singleton=1`).Scan(&cursor); err != nil {
		t.Fatal(err)
	}
	return cursor
}

// --- Engine state no API exposes or sets ---
//
// The open visit's counters and watch schedule, and the holder rows, are
// internal engine state. Tests read them to prove what the engine did, and
// force them to reach a limit or a recovery case without replaying the
// history that would produce it.

// visitRow is the open visit's internal state.
type visitRow struct {
	Status                                     string
	IdleIterations, RejectedRequests, Failures int
	NextWatchAt, ResumedAt                     string
}

func openVisitRow(t *testing.T, svc *Service, task Task) visitRow {
	t.Helper()
	var v visitRow
	if err := svc.db.QueryRow(`
		SELECT status_id, idle_iterations, rejected_requests, script_failures, next_watch_at, resumed_at
		FROM task_status_visits WHERE task_id = ? AND left_at = ''`, task.ID).
		Scan(&v.Status, &v.IdleIterations, &v.RejectedRequests, &v.Failures, &v.NextWatchAt, &v.ResumedAt); err != nil {
		t.Fatal(err)
	}
	return v
}

// openVisit returns the open visit's status, rejected_requests,
// script_failures, and next_watch_at.
func openVisit(t *testing.T, svc *Service, task Task) (string, int, int, string) {
	t.Helper()
	v := openVisitRow(t, svc, task)
	return v.Status, v.RejectedRequests, v.Failures, v.NextWatchAt
}

// visitCounters returns idle_iterations, rejected_requests, and script_failures
// of the open visit.
func visitCounters(t *testing.T, svc *Service, task Task) [3]int {
	t.Helper()
	v := openVisitRow(t, svc, task)
	return [3]int{v.IdleIterations, v.RejectedRequests, v.Failures}
}

// resumedAt returns the open visit's resumed_at.
func resumedAt(t *testing.T, svc *Service, task Task) string {
	t.Helper()
	return openVisitRow(t, svc, task).ResumedAt
}

// scheduledVisits counts the task's visits with a watch scheduled.
func scheduledVisits(t *testing.T, svc *Service, task Task) int {
	t.Helper()
	return countRows(t, svc, `SELECT COUNT(*) FROM task_status_visits WHERE task_id = ? AND next_watch_at <> ''`, task.ID)
}

// forceOpenVisit applies SQL assignments to the open visit, e.g.
// "script_failures = 2".
func forceOpenVisit(t *testing.T, svc *Service, task Task, assignments string, args ...any) {
	t.Helper()
	if _, err := svc.db.Exec(`UPDATE task_status_visits SET `+assignments+` WHERE task_id = ? AND left_at = ''`,
		append(args, task.ID)...); err != nil {
		t.Fatal(err)
	}
}

// forceScriptFailures sets the open visit's script failure count.
func forceScriptFailures(t *testing.T, svc *Service, task Task, n int) {
	t.Helper()
	forceOpenVisit(t, svc, task, `script_failures = ?`, n)
}

// forceNextWatchAt sets when the open visit's watch is due.
func forceNextWatchAt(t *testing.T, svc *Service, task Task, at string) {
	t.Helper()
	forceOpenVisit(t, svc, task, `next_watch_at = ?`, at)
}

// holderRecord is a task's holder row for one pool.
type holderRecord struct {
	Agent            string
	Released         bool
	UnavailableSince string
	Found            bool
}

func holderFor(t *testing.T, svc *Service, task Task, pool string) holderRecord {
	t.Helper()
	var h holderRecord
	err := svc.db.QueryRow(`SELECT agent, released, unavailable_since FROM task_workflow_holders WHERE task_id = ? AND pool = ?`,
		task.ID, pool).Scan(&h.Agent, &h.Released, &h.UnavailableSince)
	if errors.Is(err, sql.ErrNoRows) {
		return holderRecord{}
	}
	if err != nil {
		t.Fatal(err)
	}
	h.Found = true
	return h
}

func holderOf(t *testing.T, svc *Service, task Task, pool string) string {
	t.Helper()
	return holderFor(t, svc, task, pool).Agent
}

func holderRow(t *testing.T, svc *Service, task Task, pool string) (string, bool) {
	t.Helper()
	h := holderFor(t, svc, task, pool)
	return h.Agent, h.Released
}

// unavailableSince returns the holder row's unavailable_since.
func unavailableSince(t *testing.T, svc *Service, task Task, pool string) string {
	t.Helper()
	h := holderFor(t, svc, task, pool)
	if !h.Found {
		t.Fatalf("task %s has no %s holder", task.Key, pool)
	}
	return h.UnavailableSince
}

// forceHolders applies SQL assignments to every holder row of the task, e.g.
// "released = 1"; the engine changes holders only through dispatch.
func forceHolders(t *testing.T, svc *Service, task Task, assignments string, args ...any) {
	t.Helper()
	if _, err := svc.db.Exec(`UPDATE task_workflow_holders SET `+assignments+` WHERE task_id = ?`, append(args, task.ID)...); err != nil {
		t.Fatal(err)
	}
}

// forceTask applies SQL assignments to the task row, e.g.
// "workflow_digest = 'missing'". It reaches states the API refuses on
// purpose: a lost manifest, a pause without its comment, an undeclared
// status, a hand-set assignee.
func forceTask(t *testing.T, svc *Service, task Task, assignments string, args ...any) {
	t.Helper()
	if _, err := svc.db.Exec(`UPDATE tasks SET `+assignments+` WHERE id = ?`, append(args, task.ID)...); err != nil {
		t.Fatal(err)
	}
}

// forceRun applies SQL assignments to a script run row, e.g.
// "cancel_requested = 1"; the worker and the engine own these columns.
func forceRun(t *testing.T, svc *Service, id int64, assignments string, args ...any) {
	t.Helper()
	if _, err := svc.db.Exec(`UPDATE task_script_runs SET `+assignments+` WHERE id = ?`, append(args, id)...); err != nil {
		t.Fatal(err)
	}
}

// setLogPath records a run's log path without claiming it.
func setLogPath(t *testing.T, svc *Service, id int64, path string) {
	t.Helper()
	forceRun(t, svc, id, `log_path = ?`, path)
}

// forceRequest applies SQL assignments to a transition request row, e.g.
// "state = 'pending'"; only the engine finishes requests.
func forceRequest(t *testing.T, svc *Service, id int64, assignments string, args ...any) {
	t.Helper()
	if _, err := svc.db.Exec(`UPDATE task_transition_requests SET `+assignments+` WHERE id = ?`, append(args, id)...); err != nil {
		t.Fatal(err)
	}
}

// deleteRequest removes a transition request row, as if it never existed.
func deleteRequest(t *testing.T, svc *Service, id int64) {
	t.Helper()
	if _, err := svc.db.Exec(`DELETE FROM task_transition_requests WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
}

// insertPendingRequest records a pending "ready" request of dev-1 on the open
// visit without running the transition's checks.
func insertPendingRequest(t *testing.T, svc *Service, task Task) int64 {
	t.Helper()
	result, err := svc.db.Exec(`
		INSERT INTO task_transition_requests(task_id, visit_id, outcome, actor, state, created_at)
		SELECT ?, id, 'ready', 'agent:dev-1', 'pending', '2026-10-02T00:00:00Z'
		FROM task_status_visits WHERE task_id = ? AND left_at = ''`, task.ID, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// deleteArtifacts removes every artifact of the task; artifacts are
// append-only through the API.
func deleteArtifacts(t *testing.T, svc *Service, task Task) {
	t.Helper()
	if _, err := svc.db.Exec(`DELETE FROM task_artifacts WHERE task_id = ?`, task.ID); err != nil {
		t.Fatal(err)
	}
}

// seedOpenWait records an open wait requester asked of expected on task,
// with the comment that asked; requester may be a principal no actor can act
// as, such as the workflow itself.
func seedOpenWait(t *testing.T, svc *Service, task Task, requester, expected string) {
	t.Helper()
	result, err := svc.db.Exec(`INSERT INTO task_comments(task_id, author, body, created_at, updated_at) VALUES (?, ?, 'q', 'n', 'n')`,
		task.ID, requester)
	if err != nil {
		t.Fatal(err)
	}
	commentID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`
		INSERT INTO task_waiting_for(task_id, expected_principal, requesting_principal, requesting_comment_id, requested_at, resolved_at)
		VALUES (?, ?, ?, ?, 'n', '')`, task.ID, expected, requester, commentID); err != nil {
		t.Fatal(err)
	}
}

// artifactsOf returns the task's current artifacts.
func artifactsOf(t *testing.T, svc *Service, task Task) []Artifact {
	t.Helper()
	artifacts, err := svc.ListArtifacts(context.Background(), operatorOf(svc), task.Key)
	if err != nil {
		t.Fatal(err)
	}
	return artifacts
}

// countRuns counts the task's runs with verdict and, unless state is empty,
// that state.
func countRuns(t *testing.T, svc *Service, task Task, verdict, state string) int {
	t.Helper()
	n := 0
	for _, run := range listRuns(t, svc, operatorOf(svc), task) {
		if string(run.Verdict) == verdict && (state == "" || string(run.State) == state) {
			n++
		}
	}
	return n
}

// lastRequestID returns the id of the task's newest transition request, 0
// when it has none.
func lastRequestID(t *testing.T, svc *Service, task Task) int64 {
	t.Helper()
	if last := workflowView(t, svc, task).LastRequest; last != nil {
		return last.ID
	}
	return 0
}

// clearMessages deletes every bus message, as retention does.
func clearMessages(t *testing.T, svc *Service) {
	t.Helper()
	if _, err := svc.db.Exec(`DELETE FROM messages`); err != nil {
		t.Fatal(err)
	}
}

// dropPoolMember removes an agent from every pool's member list directly,
// without RebindAgentPool, so only the engine's own checks notice.
func dropPoolMember(t *testing.T, svc *Service, agent string) {
	t.Helper()
	if _, err := svc.db.Exec(`DELETE FROM task_agent_pool_members WHERE agent = ?`, agent); err != nil {
		t.Fatal(err)
	}
}

// deleteAgent removes an agent's registry row, as the agents registry does.
func deleteAgent(t *testing.T, svc *Service, name string) {
	t.Helper()
	if _, err := svc.db.Exec(`DELETE FROM agents WHERE name = ?`, name); err != nil {
		t.Fatal(err)
	}
}

// taskDetail returns the task as the operator reads it.
func taskDetail(t *testing.T, svc *Service, task Task) TaskDetail {
	t.Helper()
	detail, err := svc.GetTask(context.Background(), operatorOf(svc), task.Key)
	if err != nil {
		t.Fatal(err)
	}
	return detail
}

// commentOf returns the comment with id from detail, or the zero comment.
func commentOf(detail TaskDetail, id int64) Comment {
	for _, comment := range detail.Comments {
		if comment.ID == id {
			return comment
		}
	}
	return Comment{}
}

// publishMessage publishes msg through the bus at the service clock.
func publishMessage(t *testing.T, svc *Service, msg bus.Message) bus.Message {
	t.Helper()
	published, err := workflowBus(svc).Publish(msg)
	if err != nil {
		t.Fatal(err)
	}
	return published
}

// forceAgentHolders applies SQL assignments to every holder row of agent.
func forceAgentHolders(t *testing.T, svc *Service, agent, assignments string) {
	t.Helper()
	if _, err := svc.db.Exec(`UPDATE task_workflow_holders SET `+assignments+` WHERE agent = ?`, agent); err != nil {
		t.Fatal(err)
	}
}

// eventJSON returns the event's payload as compact JSON with sorted keys.
func eventJSON(t *testing.T, event Event) string {
	t.Helper()
	raw, err := json.Marshal(event.Payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// forceHolder applies SQL assignments to the task's holder row for pool.
func forceHolder(t *testing.T, svc *Service, task Task, pool, assignments string) {
	t.Helper()
	if _, err := svc.db.Exec(`UPDATE task_workflow_holders SET `+assignments+` WHERE task_id = ? AND pool = ?`, task.ID, pool); err != nil {
		t.Fatal(err)
	}
}

// insertPinnedTask writes an open task in DEV pinned to digest and standing
// in workflowStatus, without entering the status: a task in flight on an
// older version, which CreateTask cannot make once the queue is rebound.
func insertPinnedTask(t *testing.T, svc *Service, digest, workflowStatus string) Task {
	t.Helper()
	if _, err := svc.db.Exec(`
		INSERT INTO tasks(task_key, queue_prefix, title, status, author, customer, created_at, updated_at, workflow_digest, workflow_status)
		VALUES ('DEV-1', 'DEV', 't', 'open', 'u', 'u', 'n', 'n', ?, ?)`, digest, workflowStatus); err != nil {
		t.Fatal(err)
	}
	return storedTask(t, svc, "DEV-1")
}

// storedSourceRunState reads a source run's state from storage; the API lists
// the runs of bound queues only, so a run of an unbound queue is out of its
// reach.
func storedSourceRunState(t *testing.T, svc *Service, id int64) string {
	t.Helper()
	var state string
	if err := svc.db.QueryRow(`SELECT state FROM task_queue_source_runs WHERE id = ?`, id).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}
