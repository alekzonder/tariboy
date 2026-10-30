package tasks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/workflowfile"
	"github.com/alekzonder/tariboy/internal/workflowimage"
)

// seedBindingImage records a manifest whose pool-owned statuses use pools and
// returns its digest. It installs a resolver on svc that knows the refs seeded
// so far.
func seedBindingImage(t *testing.T, svc *Service, name, version string, pools ...string) string {
	t.Helper()
	def := workflowfile.File{SchemaVersion: 1, Name: name, WorkflowVersion: version, InitialStatus: "done"}
	for _, p := range pools {
		def.Statuses = append(def.Statuses, workflowfile.Status{
			ID: "in-" + p, Owner: workflowfile.Owner{Kind: workflowfile.OwnerPool, Pool: p},
		})
	}
	def.Statuses = append(def.Statuses, workflowfile.Status{ID: "done", Terminal: true})
	sum := sha256.Sum256([]byte(name + "|" + version + "|" + strings.Join(pools, ",")))
	digest := hex.EncodeToString(sum[:])
	storeManifest(t, svc, workflowimage.Manifest{
		SchemaVersion: 1, Name: name, Version: version, Digest: digest, BuiltAt: "2026-10-02T00:00:00Z", Definition: def,
	})
	// A snapshot of the seeded images, newest first, so the resolver never
	// queries the database: like the image store, it is safe inside a task
	// transaction.
	type seeded struct{ digest, name, version string }
	var images []seeded
	rows, err := svc.db.Query(`SELECT digest, name, version FROM task_workflow_images ORDER BY built_at DESC, rowid DESC`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var img seeded
		if err := rows.Scan(&img.digest, &img.name, &img.version); err != nil {
			t.Fatal(err)
		}
		images = append(images, img)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	svc.SetWorkflowResolver(func(ref string) (string, error) {
		n, tag := ref, ""
		if i := strings.LastIndex(ref, ":"); i >= 0 {
			n, tag = ref[:i], ref[i+1:]
		}
		if tag == "latest" {
			tag = ""
		}
		for _, img := range images {
			if (img.name == n && (tag == "" || img.version == tag)) || img.digest == tag {
				return img.digest, nil
			}
		}
		return "", fmt.Errorf("%w: %s", workflowimage.ErrNotFound, ref)
	})
	return digest
}

func workflowFixture(t *testing.T) (*Service, Actor) {
	t.Helper()
	svc := newTestService(t)
	actor := CustomerActor("customer")
	if _, err := svc.CreateQueue(context.Background(), actor, CreateQueueInput{
		Prefix: "DEV", Name: "Development",
	}); err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"dev-1", "dev-2", "reviewer-1"} {
		seedAgent(t, svc, agent, "")
	}
	return svc, actor
}

func mustRebindPool(t *testing.T, svc *Service, actor Actor, pool string, agents []string, revision int64) AgentPool {
	t.Helper()
	got, err := svc.RebindAgentPool(context.Background(), actor, "DEV", pool, agents, revision, "pool-"+pool)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestRebindAgentPoolIsRevisionedAuditedAndIdempotent(t *testing.T) {
	svc, actor := workflowFixture(t)
	if _, err := svc.RebindAgentPool(context.Background(), AgentActor("dev-1"), "DEV", "developers", []string{"dev-1"}, 0, "agent-rebind"); ErrorCode(err) != "forbidden" {
		t.Fatalf("agent rebind error = %v; want forbidden", err)
	}
	if _, err := svc.RebindAgentPool(context.Background(), actor, "DEV", "developers", []string{"missing"}, 0, "missing-agent"); ErrorCode(err) != "agent_not_found" {
		t.Fatalf("unknown agent error = %v; want agent_not_found", err)
	}
	developers := mustRebindPool(t, svc, actor, "developers", []string{"dev-1"}, 0)

	rebound, err := svc.RebindAgentPool(context.Background(), actor, "DEV", "developers", []string{"agent:dev-2", "dev-2"}, developers.Revision, "rebind-developers")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := svc.RebindAgentPool(context.Background(), actor, "DEV", "developers", []string{"dev-1"}, 999, "rebind-developers")
	if err != nil {
		t.Fatal(err)
	}
	if len(rebound.Agents) != 1 || rebound.Agents[0] != "dev-2" || replayed.Revision != rebound.Revision {
		t.Fatalf("rebound/replayed pool = %#v / %#v", rebound, replayed)
	}
	_, err = svc.RebindAgentPool(context.Background(), actor, "DEV", "developers", []string{"dev-1"}, developers.Revision, "stale-rebind")
	conflict, ok := err.(*Error)
	if !ok || conflict.Code != "revision_conflict" {
		t.Fatalf("stale rebind error = %#v; want revision_conflict", err)
	}
	if conflict.Data["current_revision"] != rebound.Revision || !reflect.DeepEqual(conflict.Data["current"], rebound) {
		t.Fatalf("stale rebind conflict data = %#v; want current pool %#v", conflict.Data, rebound)
	}
	emptied, err := svc.RebindAgentPool(context.Background(), actor, "DEV", "developers", nil, rebound.Revision, "empty-rebind")
	if err != nil || len(emptied.Agents) != 0 {
		t.Fatalf("empty rebind = %#v, %v; want an empty pool", emptied, err)
	}
	rebinds := queueEvents(t, svc, "DEV", "task.agent_pool_rebound")
	if len(rebinds) == 0 {
		t.Fatal("no task.agent_pool_rebound event")
	}
	eventRevision := rebinds[len(rebinds)-1].TaskRevision
	if eventRevision != emptied.Revision {
		t.Fatalf("pool audit revision = %d; want %d", eventRevision, emptied.Revision)
	}
}

func TestGetAndListAgentPools(t *testing.T) {
	svc, actor := workflowFixture(t)
	reviewers := mustRebindPool(t, svc, actor, "reviewers", []string{"reviewer-1"}, 0)
	developers := mustRebindPool(t, svc, actor, "developers", []string{"dev-2", "dev-1"}, 0)
	got, err := svc.GetAgentPool(context.Background(), actor, "dev", "developers")
	if err != nil || !reflect.DeepEqual(got, developers) {
		t.Fatalf("get pool = %#v, %v; want %#v", got, err, developers)
	}
	if _, err := svc.GetAgentPool(context.Background(), actor, "DEV", "missing"); ErrorCode(err) != "workflow_pool_not_found" {
		t.Fatalf("missing pool error = %v; want workflow_pool_not_found", err)
	}
	listed, err := svc.ListAgentPools(context.Background(), actor, "DEV")
	if err != nil || !reflect.DeepEqual(listed, []AgentPool{developers, reviewers}) {
		t.Fatalf("listed pools = %#v, %v", listed, err)
	}
	if _, err := svc.ListAgentPools(context.Background(), AgentActor("dev-1"), "DEV"); ErrorCode(err) != "forbidden" {
		t.Fatalf("agent list error = %v; want forbidden", err)
	}
}

func TestQueueWorkflowBindingRules(t *testing.T) {
	ctx := context.Background()
	svc, actor := workflowFixture(t)
	if _, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "flow", 0); ErrorCode(err) != "workflow_unavailable" {
		t.Fatalf("no resolver error = %v; want workflow_unavailable", err)
	}
	d1 := seedBindingImage(t, svc, "flow", "1", "developers", "reviewers")
	for _, call := range []func() error{
		func() error { _, err := svc.SetQueueWorkflow(ctx, AgentActor("dev-1"), "DEV", "flow", 0); return err },
		func() error { return svc.ClearQueueWorkflow(ctx, AgentActor("dev-1"), "DEV", 1) },
		func() error { _, err := svc.GetQueueWorkflow(ctx, AgentActor("dev-1"), "DEV"); return err },
	} {
		if err := call(); ErrorCode(err) != "forbidden" {
			t.Fatalf("agent error = %v; want forbidden", err)
		}
	}
	if _, err := svc.GetQueueWorkflow(ctx, actor, "DEV"); ErrorCode(err) != "queue_workflow_not_found" {
		t.Fatalf("unbound get error = %v; want queue_workflow_not_found", err)
	}
	if _, err := svc.SetQueueWorkflow(ctx, actor, "NOPE", "flow", 0); ErrorCode(err) != "queue_not_found" {
		t.Fatalf("unknown queue error = %v", err)
	}
	if _, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "missing", 0); ErrorCode(err) != "not_found" {
		t.Fatalf("unknown ref error = %v; want not_found", err)
	}
	// Both pools are missing.
	_, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "flow:1", 0)
	var domain *Error
	if !errors.As(err, &domain) || domain.Code != "workflow_pool_empty" || domain.Status != http.StatusConflict ||
		!reflect.DeepEqual(domain.Data["pools"], []string{"developers", "reviewers"}) {
		t.Fatalf("missing pools error = %#v", err)
	}
	// One pool exists but is empty, the other has a member.
	mustRebindPool(t, svc, actor, "developers", nil, 0)
	mustRebindPool(t, svc, actor, "reviewers", []string{"reviewer-1"}, 0)
	_, err = svc.SetQueueWorkflow(ctx, actor, "DEV", "flow:1", 0)
	if !errors.As(err, &domain) || domain.Code != "workflow_pool_empty" || !reflect.DeepEqual(domain.Data["pools"], []string{"developers"}) {
		t.Fatalf("empty pool error = %#v", err)
	}
	developers, err := svc.GetAgentPool(ctx, actor, "DEV", "developers")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RebindAgentPool(ctx, actor, "DEV", "developers", []string{"dev-1"}, developers.Revision, "fill-developers"); err != nil {
		t.Fatal(err)
	}

	bound, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "flow:1", 0)
	if err != nil || bound.Digest != d1 || bound.Name != "flow" || bound.Version != "1" || bound.Revision != 1 || bound.Queue != "DEV" {
		t.Fatalf("bind = %#v, %v", bound, err)
	}
	got, err := svc.GetQueueWorkflow(ctx, actor, "dev")
	if err != nil || got != bound {
		t.Fatalf("get = %#v, %v; want %#v", got, err, bound)
	}
	// The same ref is a no-op that keeps the revision.
	same, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "flow:1", 1)
	if err != nil || same != bound {
		t.Fatalf("rebind same digest = %#v, %v", same, err)
	}
	d2 := seedBindingImage(t, svc, "flow", "2", "developers")
	// A stale revision, or 0 once bound, conflicts and reports the current one
	// when the ref names another digest.
	for _, stale := range []int64{0, 7} {
		_, err = svc.SetQueueWorkflow(ctx, actor, "DEV", "flow:2", stale)
		if !errors.As(err, &domain) || domain.Code != "revision_conflict" || domain.Status != http.StatusConflict || domain.Data["current_revision"] != int64(1) {
			t.Fatalf("stale revision %d error = %#v", stale, err)
		}
	}
	next, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "flow:2", 1)
	if err != nil || next.Digest != d2 || next.Revision != 2 || next.Version != "2" {
		t.Fatalf("bind v2 = %#v, %v", next, err)
	}
	var kinds []string
	for _, event := range queueEvents(t, svc, "DEV", "queue.workflow_") {
		kinds = append(kinds, event.Kind)
	}
	if !reflect.DeepEqual(kinds, []string{"queue.workflow_bound", "queue.workflow_bound"}) {
		t.Fatalf("events = %v; want one per real change", kinds)
	}

	// A task in flight keeps its pinned digest across a clear.
	inFlight := insertPinnedTask(t, svc, d2, "in-developers")
	if err := svc.ClearQueueWorkflow(ctx, actor, "DEV", 1); ErrorCode(err) != "revision_conflict" {
		t.Fatalf("stale clear error = %v", err)
	}
	if err := svc.ClearQueueWorkflow(ctx, actor, "DEV", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetQueueWorkflow(ctx, actor, "DEV"); ErrorCode(err) != "queue_workflow_not_found" {
		t.Fatalf("get after clear = %v", err)
	}
	if err := svc.ClearQueueWorkflow(ctx, actor, "DEV", 2); ErrorCode(err) != "queue_workflow_not_found" {
		t.Fatalf("clear unbound = %v", err)
	}
	if pinned := storedTask(t, svc, inFlight.Key).WorkflowDigest; pinned != d2 {
		t.Fatalf("pinned = %q; want %q", pinned, d2)
	}
	if events := queueEvents(t, svc, "DEV", ""); len(events) == 0 || events[len(events)-1].Kind != "queue.workflow_cleared" {
		t.Fatalf("events = %#v; want queue.workflow_cleared last", events)
	}
}

func TestRebindAgentPoolKeepsBoundPoolsNonEmpty(t *testing.T) {
	ctx := context.Background()
	svc, actor := workflowFixture(t)
	seedBindingImage(t, svc, "flow", "1", "developers")
	developers := mustRebindPool(t, svc, actor, "developers", []string{"dev-1"}, 0)
	other := mustRebindPool(t, svc, actor, "other", []string{"dev-2"}, 0)
	if _, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "flow", 0); err != nil {
		t.Fatal(err)
	}
	_, err := svc.RebindAgentPool(ctx, actor, "DEV", "developers", nil, developers.Revision, "empty-bound")
	var domain *Error
	if !errors.As(err, &domain) || domain.Code != "workflow_pool_empty" || domain.Status != http.StatusConflict {
		t.Fatalf("empty bound pool error = %#v", err)
	}
	if got, err := svc.GetAgentPool(ctx, actor, "DEV", "developers"); err != nil || !reflect.DeepEqual(got, developers) {
		t.Fatalf("pool after refused rebind = %#v, %v", got, err)
	}
	if _, err := svc.RebindAgentPool(ctx, actor, "DEV", "developers", []string{"dev-2"}, developers.Revision, "swap"); err != nil {
		t.Fatalf("non-empty rebind: %v", err)
	}
	if _, err := svc.RebindAgentPool(ctx, actor, "DEV", "other", nil, other.Revision, "empty-unused"); err != nil {
		t.Fatalf("empty rebind of an unused pool: %v", err)
	}
}

// A pool an older version uses stays non-empty while a task pinned to that
// version is open, even after the queue is rebound to a version without it.
func TestRebindAgentPoolKeepsPoolsOfPinnedVersionsNonEmpty(t *testing.T) {
	ctx := context.Background()
	svc, actor := workflowFixture(t)
	d1 := seedBindingImage(t, svc, "flow", "1", "developers", "reviewers")
	seedBindingImage(t, svc, "flow", "2", "developers")
	mustRebindPool(t, svc, actor, "developers", []string{"dev-1"}, 0)
	reviewers := mustRebindPool(t, svc, actor, "reviewers", []string{"reviewer-1"}, 0)
	if _, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "flow:1", 0); err != nil {
		t.Fatal(err)
	}
	pinned := insertPinnedTask(t, svc, d1, "in-reviewers")
	if _, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "flow:2", 1); err != nil {
		t.Fatal(err)
	}
	_, err := svc.RebindAgentPool(ctx, actor, "DEV", "reviewers", nil, reviewers.Revision, "empty-pinned")
	var domain *Error
	if !errors.As(err, &domain) || domain.Code != "workflow_pool_empty" || domain.Status != http.StatusConflict ||
		!reflect.DeepEqual(domain.Data["pools"], []string{"reviewers"}) {
		t.Fatalf("empty pool of a pinned version error = %#v", err)
	}
	forceTask(t, svc, pinned, `status = 'done'`)
	if _, err := svc.RebindAgentPool(ctx, actor, "DEV", "reviewers", nil, reviewers.Revision, "empty-closed"); err != nil {
		t.Fatalf("empty rebind once the pinned task closed: %v", err)
	}
}

// Binding the digest already bound is a no-op even with the revision the
// original bind used, so a retry of a successful bind succeeds.
func TestSetQueueWorkflowRetryOfTheSameDigestIsANoOp(t *testing.T) {
	ctx := context.Background()
	svc, actor := workflowFixture(t)
	seedBindingImage(t, svc, "flow", "1", "developers")
	mustRebindPool(t, svc, actor, "developers", []string{"dev-1"}, 0)
	bound, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "flow:1", 0)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := svc.SetQueueWorkflow(ctx, actor, "DEV", "flow:1", 0)
	if err != nil || retried != bound {
		t.Fatalf("retry = %#v, %v; want %#v", retried, err, bound)
	}
	if n := countRows(t, svc, `SELECT COUNT(*) FROM task_events WHERE queue_prefix = 'DEV' AND kind = 'queue.workflow_bound'`); n != 1 {
		t.Fatalf("bound events = %d", n)
	}
}

func TestDeleteAgentPoolIsRevisionedAuditedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	svc, actor := workflowFixture(t)
	other := mustRebindPool(t, svc, actor, "other", []string{"dev-1", "dev-2"}, 0)
	if _, err := svc.DeleteAgentPool(ctx, AgentActor("dev-1"), "DEV", "other", other.Revision, "agent-delete"); ErrorCode(err) != "forbidden" {
		t.Fatalf("agent delete error = %v; want forbidden", err)
	}
	if _, err := svc.DeleteAgentPool(ctx, actor, "DEV", "missing", 1, "missing-delete"); ErrorCode(err) != "workflow_pool_not_found" {
		t.Fatalf("unknown pool error = %v; want workflow_pool_not_found", err)
	}
	_, err := svc.DeleteAgentPool(ctx, actor, "DEV", "other", other.Revision+1, "stale-delete")
	conflict, ok := err.(*Error)
	if !ok || conflict.Code != "revision_conflict" || conflict.Data["current_revision"] != other.Revision {
		t.Fatalf("stale delete error = %#v; want revision_conflict", err)
	}
	// A holder of the pool on a closed task is released: a new pool of the
	// same name does not inherit it.
	closed := insertPinnedTask(t, svc, seedBindingImage(t, svc, "flow", "1", "other"), "in-other")
	forceTask(t, svc, closed, `status = 'done'`)
	if _, err := svc.db.Exec(`INSERT INTO task_workflow_holders(task_id, pool, agent, dispatched_at) VALUES (?, 'other', 'dev-1', 'n')`, closed.ID); err != nil {
		t.Fatal(err)
	}
	deleted, err := svc.DeleteAgentPool(ctx, actor, "dev", " other ", other.Revision, "delete-other")
	if err != nil {
		t.Fatal(err)
	}
	if want := (AgentPoolDeleted{Deleted: true, Queue: "DEV", Name: "other"}); deleted != want {
		t.Fatalf("deleted = %#v; want %#v", deleted, want)
	}
	if _, err := svc.GetAgentPool(ctx, actor, "DEV", "other"); ErrorCode(err) != "workflow_pool_not_found" {
		t.Fatalf("deleted pool lookup = %v; want workflow_pool_not_found", err)
	}
	if n := countRows(t, svc, `SELECT COUNT(*) FROM task_agent_pool_members`); n != 0 {
		t.Fatalf("pool members left = %d", n)
	}
	if n := countRows(t, svc, `SELECT COUNT(*) FROM task_workflow_holders WHERE pool = 'other' AND released = 0`); n != 0 {
		t.Fatalf("unreleased holders of the deleted pool = %d", n)
	}
	replayed, err := svc.DeleteAgentPool(ctx, actor, "DEV", "other", 999, "delete-other")
	if err != nil || replayed != deleted {
		t.Fatalf("replayed delete = %#v, %v; want %#v", replayed, err, deleted)
	}
	if events := queueEvents(t, svc, "DEV", "task.agent_pool_deleted"); len(events) != 1 {
		t.Fatalf("task.agent_pool_deleted events = %d; want 1", len(events))
	}
	recreated, err := svc.RebindAgentPool(ctx, actor, "DEV", "other", []string{"dev-1"}, 0, "recreate-other")
	if err != nil || recreated.Revision != 1 || recreated.ID == other.ID {
		t.Fatalf("recreated pool = %#v, %v", recreated, err)
	}
}

func TestDeleteAgentPoolRefusesAPoolInUse(t *testing.T) {
	ctx := context.Background()
	svc, actor, task := requestFixture(t)
	refused := func(pool string) {
		t.Helper()
		current, err := svc.GetAgentPool(ctx, actor, "DEV", pool)
		if err != nil {
			t.Fatal(err)
		}
		_, err = svc.DeleteAgentPool(ctx, actor, "DEV", pool, current.Revision, "delete-"+pool)
		var domain *Error
		if !errors.As(err, &domain) || domain.Code != "workflow_pool_empty" || domain.Status != http.StatusConflict ||
			!reflect.DeepEqual(domain.Data["pools"], []string{pool}) {
			t.Fatalf("delete of pool %s in use = %#v", pool, err)
		}
		if got, err := svc.GetAgentPool(ctx, actor, "DEV", pool); err != nil || !reflect.DeepEqual(got, current) {
			t.Fatalf("pool %s after refused delete = %#v, %v", pool, got, err)
		}
	}
	refused("developers")
	mustRebindPool(t, svc, actor, "approvers", []string{"dev-2"}, 0)
	if _, err := svc.SetQueueEnvInfo(ctx, actor, "DEV", CustomerAgentPoolEnv, "approvers"); err != nil {
		t.Fatal(err)
	}
	refused("approvers")
	// The open visit keeps the pool it entered with after the env var goes.
	enter(t, svc, task.Key, "approval", "ask")
	if err := svc.RemoveQueueEnv(ctx, actor, "DEV", CustomerAgentPoolEnv); err != nil {
		t.Fatal(err)
	}
	refused("approvers")
}
