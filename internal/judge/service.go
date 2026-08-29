package judge

// The service is the trust boundary for judge work.  Agent names and iteration
// ids arrive from the authenticated agent endpoint, never from action bodies.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/bus"
	"github.com/alekzonder/tariboy/internal/groups"
	"github.com/alekzonder/tariboy/internal/improvement"
)

type proposalCreator interface {
	CreateProposal(context.Context, improvement.CreateProposalRequest) (improvement.Proposal, error)
}

type ServiceConfig struct {
	Store        *Store
	Agents       *agent.Store
	Groups       *groups.Store
	Bus          *bus.Bus
	Evidence     *EvidenceReader
	Audit        func(agent, typ, iteration string, data map[string]any)
	Enqueue      func(string)
	Improvements proposalCreator
	Automation   *AutomationService
}
type Service struct {
	store        *Store
	agents       *agent.Store
	groups       *groups.Store
	bus          *bus.Bus
	evidence     *EvidenceReader
	audit        func(string, string, string, map[string]any)
	enqueue      func(string)
	improvements proposalCreator
	automation   *AutomationService
}

func NewService(c ServiceConfig) *Service {
	return &Service{store: c.Store, agents: c.Agents, groups: c.Groups, bus: c.Bus, evidence: c.Evidence, audit: c.Audit, enqueue: c.Enqueue, improvements: c.Improvements, automation: c.Automation}
}

func (s *Service) AgentAction(ctx context.Context, callerAgent, callerIteration, action string, body map[string]any) (map[string]any, error) {
	if err := s.active(callerAgent, callerIteration); err != nil {
		return nil, err
	}
	switch action {
	case "automation.begin":
		if s.automation == nil {
			return nil, ErrCapabilityDisabled
		}
		cycle, err := s.automation.Begin(ctx, callerAgent, callerIteration, num(body, "config_revision"), str(body, "delivery_id"), num(body, "limit"))
		return map[string]any{"cycle": cycle}, err
	case "iterations.search":
		if err := s.lead(callerAgent, str(body, "judge_group")); err != nil {
			return nil, err
		}
		return s.search(body)
	case "run.create":
		group := str(body, "judge_group")
		if err := s.lead(callerAgent, group); err != nil {
			return nil, err
		}
		req := CreateRunRequest{OriginalRequest: str(body, "original_request"), Selector: selector(body["selector"]), JudgeGroup: group, LeadAgent: callerAgent, SummaryAgent: str(body, "summary_agent"), CreatorIteration: callerIteration, JudgeAgents: stringsList(body["judge_agents"]), JudgesPerIteration: num(body, "judges_per_iteration"), MaxAttempts: num(body, "max_attempts")}
		r, ts, e := s.store.CreateRun(ctx, req)
		if e != nil {
			return nil, e
		}
		s.record(callerAgent, "judge_run_created", callerIteration, map[string]any{"run_id": r.ID, "targets": len(ts)})
		if s.enqueue != nil {
			s.enqueue(r.ID)
		}
		return map[string]any{"run": r, "targets": ts}, nil
	case "run.inspect":
		r, e := s.store.GetRun(str(body, "run_id"))
		if e != nil {
			return nil, e
		}
		if e = s.ownLead(callerAgent, r); e != nil {
			return nil, e
		}
		ts, e := s.store.ListTargets(r.ID)
		return map[string]any{"run": r, "targets": ts}, e
	case "work.claim":
		r, e := s.claimRun(str(body, "run_id"))
		if e != nil {
			return nil, e
		}
		if !contains(r.JudgeAgents, callerAgent) {
			return nil, ErrUnauthorized
		}
		a, ok, e := s.store.Claim(ClaimRequest{RunID: r.ID, Agent: callerAgent, Iteration: callerIteration})
		if e != nil {
			return nil, e
		}
		if !ok {
			return map[string]any{"claimed": false}, nil
		}
		s.record(callerAgent, "judge_work_claimed", callerIteration, map[string]any{"run_id": r.ID, "assignment_id": a.ID, "target_id": a.TargetID})
		return map[string]any{"claimed": true, "assignment": a, "criteria": r.OriginalRequest}, nil
	case "evidence.search", "evidence.get":
		a, e := s.ownedAssignment(callerAgent, callerIteration, str(body, "assignment_id"))
		if e != nil {
			return nil, e
		}
		h, e := s.bundle(a.TargetID)
		if e != nil {
			return nil, e
		}
		if s.evidence == nil {
			return nil, ErrNotFound
		}
		if action == "evidence.search" {
			q := EvidenceQuery{Artifacts: stringsList(body["artifacts"]), Query: str(body, "query"), Cursor: str(body, "cursor"), Limit: num(body, "limit")}
			p, e := s.evidence.Search(h, q)
			s.evidenceAudit(callerAgent, callerIteration, a, h, q)
			// Return the immutable CAS identity with the page. A judge may cite
			// only evidence for its owned assignment, and needs this exact hash to
			// form a schema-valid citation without broad run-inspection access.
			return map[string]any{"page": p, "bundle_hash": h}, e
		}
		l := EvidenceLocator{Artifact: str(body, "artifact"), Locator: str(body, "locator")}
		v, e := s.evidence.Get(h, l)
		s.record(callerAgent, "judge_evidence_read", callerIteration, map[string]any{"run_id": a.RunID, "target_id": a.TargetID, "artifact": l.Artifact, "locator": l.Locator})
		return map[string]any{"evidence": v}, e
	case "analysis.submit":
		a, e := s.ownedAssignment(callerAgent, callerIteration, str(body, "assignment_id"))
		if e != nil {
			return nil, e
		}
		result, e := analysis(body["result"])
		if e != nil {
			return nil, e
		}
		h, e := s.bundle(a.TargetID)
		if e != nil {
			return nil, e
		}
		out, e := s.store.SubmitAnalysis(SubmitAnalysisRequest{AssignmentID: a.ID, Agent: callerAgent, Iteration: callerIteration, RawSubmission: str(body, "raw_submission"), Result: result, Resolve: CitationResolverFunc(func(c Citation) error {
			_, e := s.evidence.Get(h, EvidenceLocator{Artifact: c.Artifact, Locator: c.Locator})
			return e
		})})
		if e == nil {
			s.record(callerAgent, "judge_analysis_submitted", callerIteration, map[string]any{"run_id": a.RunID, "assignment_id": a.ID, "analysis_id": out.ID})
			if s.enqueue != nil {
				s.enqueue(a.RunID)
			}
		}
		return map[string]any{"analysis": out}, e
	case "summary.claim":
		r, e := s.store.GetRun(str(body, "run_id"))
		if e != nil {
			return nil, e
		}
		if r.SummaryAgent != callerAgent {
			return nil, ErrUnauthorized
		}
		out, e := s.store.ClaimSummary(r.ID, callerAgent, callerIteration)
		return map[string]any{"run": out}, e
	case "summary.inputs":
		r, e := s.store.GetRun(str(body, "run_id"))
		if e != nil {
			return nil, e
		}
		if r.SummaryAgent != callerAgent {
			return nil, ErrUnauthorized
		}
		if e = s.summaryClaimed(r, callerIteration); e != nil {
			return nil, e
		}
		return s.summaryInputs(r.ID)
	case "summary.submit":
		r, e := s.store.GetRun(str(body, "run_id"))
		if e != nil {
			return nil, e
		}
		if r.SummaryAgent != callerAgent {
			return nil, ErrUnauthorized
		}
		result, e := summary(body["result"])
		if e != nil {
			return nil, e
		}
		out, e := s.store.SubmitSummary(SubmitSummaryRequest{RunID: r.ID, Agent: callerAgent, Iteration: callerIteration, RawSubmission: str(body, "raw_submission"), Result: result})
		if e == nil {
			s.record(callerAgent, "judge_summary_submitted", callerIteration, map[string]any{"run_id": r.ID, "summary_id": out.ID})
			if s.automation != nil {
				e = s.automation.Finish(ctx, r.ID, out.Result.ExecutiveConclusion)
			}
		}
		return map[string]any{"summary": out}, e
	case "improvement.submit":
		r, e := s.store.GetRun(str(body, "run_id"))
		if e != nil {
			return nil, e
		}
		if r.SummaryAgent != callerAgent || s.improvements == nil {
			return nil, ErrUnauthorized
		}
		if e = s.summaryOwned(r, callerIteration); e != nil {
			return nil, e
		}
		raw, e := json.Marshal(body["result"])
		if e != nil {
			return nil, improvement.ErrInvalidProposal
		}
		var draft improvement.ProposalDraft
		if e = json.Unmarshal(raw, &draft); e != nil {
			return nil, improvement.ErrInvalidProposal
		}
		if e = s.validateImprovementScope(r.ID, draft); e != nil {
			return nil, e
		}
		proposal, e := s.improvements.CreateProposal(ctx, improvement.CreateProposalRequest{JudgeRunID: r.ID, CreatorAgent: callerAgent, CreatorIteration: callerIteration, Draft: draft})
		if e != nil {
			return nil, e
		}
		if s.automation != nil {
			if e = s.automation.RecordProposal(ctx, r.ID, proposal.ID, proposal.RevisionHash); e != nil {
				return nil, e
			}
		}
		s.record(callerAgent, "improvement_plan_requested", callerIteration, map[string]any{"run_id": r.ID, "proposal_id": proposal.ID, "revision_hash": proposal.RevisionHash})
		if s.bus != nil {
			_, _ = s.bus.Publish(bus.Message{Channel: bus.InboxChannel(r.LeadAgent), Type: "improvement.plan.approval_requested", Source: "system:judge", Text: "improvement plan approval requested", Data: map[string]any{"run_id": r.ID, "proposal_id": proposal.ID, "revision_hash": proposal.RevisionHash}})
		}
		return map[string]any{"proposal": proposal}, nil
	case "run.cancel":
		r, e := s.store.GetRun(str(body, "run_id"))
		if e != nil {
			return nil, e
		}
		if e = s.ownLead(callerAgent, r); e != nil {
			return nil, e
		}
		e = s.OperatorCancel(r.ID)
		s.record(callerAgent, "judge_run_cancelled", callerIteration, map[string]any{"run_id": r.ID})
		return map[string]any{"cancelled": e == nil}, e
	case "work.retry":
		r, e := s.store.GetRun(str(body, "run_id"))
		if e != nil {
			return nil, e
		}
		if e = s.ownLead(callerAgent, r); e != nil {
			return nil, e
		}
		e = s.OperatorRetry(r.ID)
		if e == nil && s.enqueue != nil {
			s.enqueue(r.ID)
		}
		s.record(callerAgent, "judge_work_retried", callerIteration, map[string]any{"run_id": r.ID})
		return map[string]any{"retried": e == nil}, e
	default:
		return nil, ErrInvalidAction
	}
}

func (s *Service) validateImprovementScope(runID string, draft improvement.ProposalDraft) error {
	subjects, err := s.store.ListSubjects(runID)
	if err != nil {
		return err
	}
	allowedSubjects := map[string]bool{}
	for _, subject := range subjects {
		allowedSubjects[subject.ID] = true
	}
	for _, id := range draft.SubjectIDs {
		if !allowedSubjects[id] {
			return improvement.ErrInvalidProposal
		}
	}
	targets, err := s.store.ListTargets(runID)
	if err != nil {
		return err
	}
	bundles := map[string]bool{}
	for _, target := range targets {
		bundles[target.BundleHash] = true
	}
	for _, finding := range draft.Findings {
		for _, citation := range finding.Evidence {
			if !bundles[citation.BundleHash] {
				return improvement.ErrInvalidProposal
			}
		}
	}
	return improvement.ValidateDraft(draft)
}
func (s *Service) OperatorList(f ListFilter) ([]Run, error) { return s.store.ListRuns(f) }
func (s *Service) OperatorInspect(id string) (map[string]any, error) {
	r, e := s.store.GetRun(id)
	if e != nil {
		return nil, e
	}
	ts, e := s.store.ListTargets(id)
	if e != nil {
		return nil, e
	}
	subjects, e := s.store.ListSubjects(id)
	if e != nil {
		return nil, e
	}
	analyses, e := s.store.ListAnalyses(id)
	if e != nil {
		return nil, e
	}
	summaries, e := s.store.ListSummaries(id)
	if e != nil {
		return nil, e
	}
	return map[string]any{"run": r, "subjects": subjects, "targets": ts, "analyses": analyses, "summaries": summaries}, nil
}

// OperatorEvidence reads one immutable evidence item by its stable locator.
// targetID is checked against the run before the CAS object is opened.
func (s *Service) OperatorEvidence(runID, targetID string, locator EvidenceLocator) (map[string]any, error) {
	if runID == "" || targetID == "" || s.evidence == nil {
		return nil, ErrNotFound
	}
	var hash string
	err := s.store.db.QueryRow(`SELECT bundle_hash FROM judge_targets WHERE id=? AND run_id=?`, targetID, runID).Scan(&hash)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.evidence.Get(hash, locator)
}
func (s *Service) OperatorCancel(id string) error {
	res, e := s.store.db.Exec(`UPDATE judge_assignments SET state='cancelled',lease_owner='',lease_expires_at='' WHERE run_id=? AND state IN ('pending','claimed')`, id)
	_ = res
	if e != nil {
		return e
	}
	_, e = s.store.db.Exec(`UPDATE judge_runs SET status='cancelled',updated_at=? WHERE id=?`, s.store.now().UTC().Format(timeFormat), id)
	return e
}
func (s *Service) OperatorRetry(id string) error { return s.store.Retry(id) }

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"

func (s *Service) active(a, i string) error {
	if s.agents == nil {
		return nil
	}
	ag, e := s.agents.Get(a)
	if e != nil {
		return ErrUnauthorized
	}
	if !contains(ag.Plugins, "llm-as-judge") {
		return ErrCapabilityDisabled
	}
	it, e := s.agents.GetIteration(a, i)
	if e != nil || it.Status != "running" {
		return ErrStaleIteration
	}
	return nil
}
func (s *Service) lead(a, g string) error {
	if s.groups == nil || g == "" {
		return ErrUnauthorized
	}
	x, ok, e := s.groups.Get(g)
	if e != nil {
		return e
	}
	if !ok || x.Lead != a {
		return ErrUnauthorized
	}
	return nil
}
func (s *Service) ownLead(a string, r Run) error { return s.lead(a, r.JudgeGroup) }
func (s *Service) claimRun(id string) (Run, error) {
	if id != "" {
		return s.store.GetRun(id)
	}
	rs, e := s.store.ListRuns(ListFilter{Statuses: []RunStatus{RunRunning}})
	if e != nil {
		return Run{}, e
	}
	for _, r := range rs {
		if r.Status == RunRunning {
			return r, nil
		}
	}
	return Run{}, ErrNoAssignment
}
func (s *Service) ownedAssignment(a, i, id string) (Assignment, error) {
	if id == "" {
		return Assignment{}, ErrUnauthorized
	}
	var x Assignment
	e := s.store.db.QueryRow(`SELECT id,run_id,target_id,replica_index,state,judge_agent,judge_iteration,lease_owner,lease_expires_at,attempt_count,last_error,analysis_id FROM judge_assignments WHERE id=?`, id).Scan(&x.ID, &x.RunID, &x.TargetID, &x.ReplicaIndex, &x.State, &x.JudgeAgent, &x.JudgeIteration, &x.LeaseOwner, &x.LeaseExpiresAt, &x.AttemptCount, &x.LastError, &x.AnalysisID)
	if e != nil {
		return Assignment{}, ErrNotFound
	}
	if x.State != "claimed" || x.JudgeAgent != a || x.JudgeIteration != i || x.LeaseExpiresAt <= s.store.now().UTC().Format(timeFormat) {
		return Assignment{}, ErrLeaseNotOwned
	}
	return x, nil
}
func (s *Service) bundle(t string) (string, error) {
	var h string
	e := s.store.db.QueryRow(`SELECT bundle_hash FROM judge_targets WHERE id=?`, t).Scan(&h)
	if e != nil {
		return "", ErrNotFound
	}
	return h, nil
}
func (s *Service) summaryClaimed(r Run, i string) error {
	var c string
	e := s.store.db.QueryRow(`SELECT last_error FROM judge_runs WHERE id=?`, r.ID).Scan(&c)
	if e != nil {
		return e
	}
	if c != "summary claimed by "+i {
		return ErrLeaseNotOwned
	}
	return nil
}

func (s *Service) summaryOwned(r Run, iteration string) error {
	if err := s.summaryClaimed(r, iteration); err == nil {
		return nil
	}
	var exists int
	if err := s.store.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM judge_summaries WHERE run_id=? AND summary_agent=? AND summary_iteration=?)`, r.ID, r.SummaryAgent, iteration).Scan(&exists); err != nil {
		return err
	}
	if exists != 1 {
		return ErrLeaseNotOwned
	}
	return nil
}
func (s *Service) record(a, t, i string, d map[string]any) {
	if s.audit != nil {
		s.audit(a, t, i, d)
	}
}
func (s *Service) evidenceAudit(a, i string, x Assignment, h string, q EvidenceQuery) {
	sum := sha256.Sum256([]byte(q.Query))
	s.record(a, "judge_evidence_read", i, map[string]any{"run_id": x.RunID, "target_id": x.TargetID, "bundle_hash": h, "cursor": q.Cursor, "query_sha256": hex.EncodeToString(sum[:])})
}
func (s *Service) search(b map[string]any) (map[string]any, error) {
	tx, e := s.store.db.BeginTx(context.Background(), nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	rows, e := s.store.selectIterations(context.Background(), tx, selector(b["selector"]))
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	out := make([]map[string]any, 0, len(rows))
	for _, x := range rows {
		out = append(out, map[string]any{"id": x.ID, "agent": x.Agent, "status": x.Status})
	}
	return map[string]any{"iterations": out}, nil
}
func (s *Service) summaryInputs(id string) (map[string]any, error) {
	ts, e := s.store.ListTargets(id)
	if e != nil {
		return nil, e
	}
	rows, e := s.store.db.Query(`SELECT id,target_id,judge_agent,judge_iteration,result_json,created_at FROM judge_analyses WHERE run_id=? ORDER BY created_at,id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	as := []map[string]any{}
	for rows.Next() {
		var id, t, a, i, r, c string
		if e = rows.Scan(&id, &t, &a, &i, &r, &c); e != nil {
			return nil, e
		}
		as = append(as, map[string]any{"id": id, "target_id": t, "judge_agent": a, "judge_iteration": i, "result_json": r, "created_at": c})
	}
	return map[string]any{"targets": ts, "analyses": as}, rows.Err()
}
func str(b map[string]any, k string) string { v, _ := b[k].(string); return v }
func num(b map[string]any, k string) int {
	switch v := b[k].(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}
func stringsList(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		o := []string{}
		for _, v := range x {
			if z, ok := v.(string); ok {
				o = append(o, z)
			}
		}
		return o
	case string:
		if x == "" {
			return nil
		}
		return strings.Split(x, ",")
	}
	return nil
}
func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func selector(v any) Selector {
	if x, ok := v.(Selector); ok {
		return x
	}
	m, ok := v.(map[string]any)
	if !ok {
		return Selector{}
	}
	return Selector{ExplicitIDs: stringsList(m["iteration_ids"]), Agents: stringsList(m["agents"]), Group: str(m, "group"), Since: str(m, "since"), Until: str(m, "until"), Statuses: stringsList(m["statuses"]), Order: str(m, "order"), Limit: num(m, "limit")}
}
func analysis(v any) (AnalysisResult, error) {
	x, ok := v.(AnalysisResult)
	if ok {
		return x, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return AnalysisResult{}, fmt.Errorf("%w: result", ErrInvalidAction)
	}
	// Tool actions arrive as untyped JSON maps. Decode the complete v1 payload
	// so persisted operator results retain findings, citations, and follow-up
	// fields instead of silently keeping only the scalar summary fields.
	raw, err := json.Marshal(m)
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("%w: result: %v", ErrInvalidAction, err)
	}
	var out AnalysisResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return AnalysisResult{}, fmt.Errorf("%w: result: %v", ErrInvalidAction, err)
	}
	return out, nil
}
func summary(v any) (SummaryResult, error) {
	x, ok := v.(SummaryResult)
	if ok {
		return x, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return SummaryResult{}, fmt.Errorf("%w: result", ErrInvalidAction)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return SummaryResult{}, fmt.Errorf("%w: result: %v", ErrInvalidAction, err)
	}
	var out SummaryResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return SummaryResult{}, fmt.Errorf("%w: result: %v", ErrInvalidAction, err)
	}
	return out, nil
}
func float(m map[string]any, k string) float64 { v, _ := m[k].(float64); return v }
func mapInt(v any) map[string]int {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	o := map[string]int{}
	for k, v := range m {
		if n, ok := v.(float64); ok {
			o[k] = int(n)
		}
	}
	return o
}
