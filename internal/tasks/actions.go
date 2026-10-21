package tasks

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// AgentAction runs one action of the agent tools socket. The body is decoded
// into the action's request from the Specs table; an operator-only or unknown
// action is refused.
func (s *TaskService) AgentAction(ctx context.Context, actor Actor, action string, body map[string]any) (any, error) {
	if actor.IsCustomer || !strings.HasPrefix(actor.Principal, "agent:") {
		return nil, domainError(http.StatusForbidden, "forbidden", "agent identity required")
	}
	spec, ok := SpecFor(action)
	if !ok || spec.agent == nil {
		return nil, domainError(http.StatusBadRequest, "invalid_action",
			fmt.Sprintf("unknown task action %q", action))
	}
	// Decode reads a malformed max_bytes as absent; the socket refuses it, as
	// the HTTP route does.
	if action == "workflow_run_log" {
		if _, err := ParseMaxBytes(body["max_bytes"]); err != nil {
			return nil, err
		}
	}
	request := spec.New()
	Decode(body, request)
	return spec.agent(ctx, s, actor, request)
}

func (s *TaskService) agentList(ctx context.Context, actor Actor, in *ListRequest) (any, error) {
	// An agent lists only what it sees itself, so scope_agent is never read.
	filter := in.Filter()
	filter.Author, filter.Customer, filter.ScopeAgent = in.ByAuthor, in.ByCustomer, ""
	if filter.WaitingFor == "me" {
		filter.WaitingFor = actor.Principal
	}
	return s.ListTasks(ctx, actor, filter)
}

func (s *TaskService) agentReady(ctx context.Context, actor Actor, in *ReadyRequest) (any, error) {
	filter := ReadyFilter{Queue: in.Queue, Limit: in.Limit}
	if in.Claim {
		return s.ClaimReady(ctx, actor, filter, in.IdempotencyKey)
	}
	return s.Ready(ctx, actor, filter)
}

func (s *TaskService) agentShow(ctx context.Context, actor Actor, in *KeyRequest) (any, error) {
	return s.GetTask(ctx, actor, in.Key)
}

func (s *TaskService) agentCreate(ctx context.Context, actor Actor, in *CreateRequest) (any, error) {
	return s.CreateTask(ctx, actor, in.Input())
}

func (s *TaskService) agentUpdate(ctx context.Context, actor Actor, in *UpdateRequest) (any, error) {
	revision, err := s.actionRevision(ctx, actor, in.Key, in.Revision)
	if err != nil {
		return nil, err
	}
	update := in.Input()
	update.Revision = revision
	return s.UpdateTask(ctx, actor, in.Key, update)
}

func (s *TaskService) agentAssign(ctx context.Context, actor Actor, in *AssignRequest) (any, error) {
	revision, err := s.actionRevision(ctx, actor, in.Key, in.Revision)
	if err != nil {
		return nil, err
	}
	return s.UpdateTask(ctx, actor, in.Key, UpdateTaskInput{Assignee: &in.Assignee, Revision: revision})
}

func (s *TaskService) agentComment(ctx context.Context, actor Actor, in *CommentRequest) (any, error) {
	return s.AddComment(ctx, actor, in.Key, in.Input())
}

// agentAsk posts the question as a comment that mentions the asked principal;
// a bare name is an agent.
func (s *TaskService) agentAsk(ctx context.Context, actor Actor, in *AskRequest) (any, error) {
	principal := in.Principal
	if principal != "" && !strings.Contains(principal, ":") {
		principal = agentPrincipal(principal)
	}
	if principal == "" {
		return nil, domainError(http.StatusBadRequest, "missing_principal", "question principal is required")
	}
	return s.AddComment(ctx, actor, in.Key, AddCommentInput{
		Body: "@" + principal + "\n\n" + in.Body, IdempotencyKey: in.IdempotencyKey,
	})
}

func (s *TaskService) agentMove(ctx context.Context, actor Actor, in *MoveRequest) (any, error) {
	revision, err := s.actionRevision(ctx, actor, in.Key, in.Revision)
	if err != nil {
		return nil, err
	}
	move := in.Input()
	move.Revision = revision
	return s.MoveTask(ctx, actor, in.Key, move)
}

func (s *TaskService) agentBlock(ctx context.Context, actor Actor, in *BlockRequest) (any, error) {
	revision, err := s.actionRevision(ctx, actor, in.BlockerKey, in.Revision)
	if err != nil {
		return nil, err
	}
	return s.AddRelation(ctx, actor, in.BlockerKey, RelationInput{
		TargetKey: in.Key, Type: "blocks", Revision: revision, IdempotencyKey: in.IdempotencyKey,
	})
}

func (s *TaskService) agentRelate(ctx context.Context, actor Actor, in *RelateRequest) (any, error) {
	revision, err := s.actionRevision(ctx, actor, in.Key, in.Revision)
	if err != nil {
		return nil, err
	}
	relation := in.Input()
	relation.Type, relation.Revision = "related", revision
	return s.AddRelation(ctx, actor, in.Key, relation)
}

func (s *TaskService) agentDone(ctx context.Context, actor Actor, in *DoneRequest) (any, error) {
	revision, err := s.actionRevision(ctx, actor, in.Key, in.Revision)
	if err != nil {
		return nil, err
	}
	complete := in.Input()
	complete.Revision = revision
	return s.CompleteTask(ctx, actor, in.Key, complete)
}

func (s *TaskService) agentAdvance(ctx context.Context, actor Actor, in *AdvanceRequest) (any, error) {
	return s.engine.Advance(ctx, actor, in.Key, in.Input())
}

func (s *TaskService) agentArtifactSet(ctx context.Context, actor Actor, in *ArtifactSetRequest) (any, error) {
	return s.engine.SetArtifact(ctx, actor, in.Key, in.Name, in.Text())
}

func (s *TaskService) agentArtifactList(ctx context.Context, actor Actor, in *KeyRequest) (any, error) {
	artifacts, err := s.engine.ListArtifacts(ctx, actor, in.Key)
	return map[string]any{"artifacts": artifacts, "count": len(artifacts)}, err
}

func (s *TaskService) agentArtifactShow(ctx context.Context, actor Actor, in *ArtifactRequest) (any, error) {
	artifact, history, err := s.engine.GetArtifact(ctx, actor, in.Key, in.Name)
	return map[string]any{"artifact": artifact, "history": history}, err
}

func (s *TaskService) agentWorkflowGet(ctx context.Context, actor Actor, in *KeyRequest) (any, error) {
	return s.engine.GetWorkflow(ctx, actor, in.Key)
}

func (s *TaskService) agentRequestGet(ctx context.Context, actor Actor, in *RequestRef) (any, error) {
	if err := positiveID(in.ID); err != nil {
		return nil, err
	}
	return s.engine.GetTransitionRequest(ctx, actor, in.Key, in.ID)
}

func (s *TaskService) agentRuns(ctx context.Context, actor Actor, in *KeyRequest) (any, error) {
	runs, err := s.runs.ListScriptRuns(ctx, actor, in.Key)
	return map[string]any{"runs": runs, "count": len(runs)}, err
}

func (s *TaskService) agentRunLog(ctx context.Context, actor Actor, in *RunLogRequest) (any, error) {
	if err := positiveID(in.ID); err != nil {
		return nil, err
	}
	maxBytes, err := ParseMaxBytes(in.MaxBytes)
	if err != nil {
		return nil, err
	}
	text, truncated, err := s.runs.ScriptRunLog(ctx, actor, in.Key, in.ID, maxBytes)
	return map[string]any{"run_id": in.ID, "text": text, "truncated": truncated}, err
}

// positiveID refuses a request or run id that is not a positive number.
func positiveID(id int64) error {
	if id <= 0 {
		return domainError(http.StatusBadRequest, "invalid_id", "a positive numeric id is required")
	}
	return nil
}

// actionRevision is the given revision, or the task's current one.
func (s *TaskService) actionRevision(ctx context.Context, actor Actor, key string, revision int64) (int64, error) {
	if revision > 0 {
		return revision, nil
	}
	detail, err := s.GetTask(ctx, actor, key)
	return detail.Task.Revision, err
}
