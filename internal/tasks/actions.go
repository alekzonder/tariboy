package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ActiveWorkflowPermissions is the daemon-side authorization projection for
// the authenticated agent's current iteration. An empty result preserves the
// legacy tool surface; Managed=true means direct tools are deny-by-default.
type ActiveWorkflowPermissionSet struct {
	Managed         bool     `json:"managed"`
	AssignmentID    int64    `json:"assignment_id,omitempty"`
	Tools           []string `json:"tools,omitempty"`
	ChannelPatterns []string `json:"channel_patterns,omitempty"`
}

func (s *Service) ActiveWorkflowPermissions(ctx context.Context, agent, iteration string) (ActiveWorkflowPermissionSet, error) {
	if strings.TrimSpace(iteration) == "" {
		return ActiveWorkflowPermissionSet{}, nil
	}
	principal := agentPrincipal(strings.TrimPrefix(strings.TrimSpace(agent), "agent:"))
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.lease_expires_at, w.definition FROM task_assignments a
		JOIN task_requirement_executions re ON re.id=a.requirement_execution_id
		JOIN task_status_executions se ON se.id=re.status_execution_id
		JOIN task_workflow_versions w ON w.id=se.workflow_version_id
		WHERE a.state='leased' AND a.lease_owner=? AND a.lease_iteration=?
		  AND re.state='pending' AND (se.state='active' OR re.requirement_id GLOB '__question:*')
		ORDER BY a.updated_at DESC, a.id DESC`, principal, strings.TrimSpace(iteration))
	if err != nil {
		return ActiveWorkflowPermissionSet{}, err
	}
	defer rows.Close()
	result := ActiveWorkflowPermissionSet{}
	for rows.Next() {
		var id int64
		var leaseExpires, raw string
		if err := rows.Scan(&id, &leaseExpires, &raw); err != nil {
			return ActiveWorkflowPermissionSet{}, err
		}
		deadline, err := time.Parse(time.RFC3339Nano, leaseExpires)
		if err != nil {
			return ActiveWorkflowPermissionSet{}, err
		}
		if !deadline.After(s.clock().UTC()) {
			continue
		}
		var def WorkflowDefinition
		if err := json.Unmarshal([]byte(raw), &def); err != nil {
			return ActiveWorkflowPermissionSet{}, err
		}
		if !result.Managed {
			result = ActiveWorkflowPermissionSet{Managed: true, AssignmentID: id, Tools: append([]string(nil), def.Permissions.Tools...), ChannelPatterns: append([]string(nil), def.Permissions.Channels.Subscribe...)}
			continue
		}
		result.AssignmentID = 0
		result.Tools = intersectWorkflowPermissions(result.Tools, def.Permissions.Tools)
		result.ChannelPatterns = intersectWorkflowPermissions(result.ChannelPatterns, def.Permissions.Channels.Subscribe)
	}
	return result, rows.Err()
}

func intersectWorkflowPermissions(left, right []string) []string {
	out := make([]string, 0, len(left))
	for _, value := range left {
		if containsString(right, value) {
			out = append(out, value)
		}
	}
	return out
}

func (s *Service) AgentAction(ctx context.Context, actor Actor, action string, body map[string]any) (any, error) {
	if actor.IsCustomer || !strings.HasPrefix(actor.Principal, "agent:") {
		return nil, domainError(http.StatusForbidden, "forbidden", "agent identity required")
	}
	switch action {
	case "work_next":
		key := actionString(body, "idempotency_key")
		if err := requireIdempotencyKey(key); err != nil {
			return nil, err
		}
		iterationID := actionString(body, "iteration_id")
		if iterationID == "" {
			return nil, domainError(http.StatusConflict, "no_iteration", "workflow work can only be claimed from an active iteration")
		}
		if replay, ok, err := s.readClaimReplay(ctx, actor, key); err != nil {
			return nil, err
		} else if ok && replay.Packet != nil {
			return *replay.Packet, replay.err()
		}
		available, err := s.NextWork(ctx, actor, actionString(body, "queue"), 1)
		if err != nil || len(available) == 0 {
			return available, err
		}
		current, err := assignmentContextByID(ctx, s.db, available[0].ID)
		if err != nil {
			return nil, err
		}
		id := strconv.FormatInt(available[0].ID, 10)
		claimed, err := s.ClaimAssignment(ctx, actor, id, ClaimAssignmentInput{TaskRevision: current.Task.WorkflowRevision, AssignmentRevision: available[0].Revision, IdempotencyKey: key, IterationID: iterationID})
		if err != nil {
			return nil, err
		}
		if replay, ok, err := s.readClaimReplay(ctx, actor, key); err != nil {
			return nil, err
		} else if ok && replay.Packet != nil {
			return *replay.Packet, replay.err()
		}
		return s.GetWorkPacket(ctx, actor, strconv.FormatInt(claimed.ID, 10))
	case "work_show":
		return s.GetWorkPacket(ctx, actor, actionString(body, "assignment_id"))
	case "questions":
		packet, err := s.GetWorkPacket(ctx, actor, actionString(body, "assignment_id"))
		if err != nil {
			return nil, err
		}
		return packet.Questions, nil
	case "work_complete":
		return s.CompleteAssignment(ctx, actor, actionString(body, "assignment_id"), CompleteAssignmentInput{
			TaskRevision: actionInt64(body, "task_revision"), AssignmentRevision: actionInt64(body, "assignment_revision"),
			Outcome: actionString(body, "outcome"), IdempotencyKey: actionString(body, "idempotency_key"),
		})
	case "work_release":
		return s.ReleaseAssignment(ctx, actor, actionString(body, "assignment_id"), ReleaseAssignmentInput{
			TaskRevision: actionInt64(body, "task_revision"), AssignmentRevision: actionInt64(body, "assignment_revision"),
			IdempotencyKey: actionString(body, "idempotency_key"),
		})
	case "artifact_add":
		return s.AddArtifact(ctx, actor, actionString(body, "assignment_id"), AddArtifactInput{
			TaskRevision: actionInt64(body, "task_revision"), AssignmentRevision: actionInt64(body, "assignment_revision"),
			Name: actionString(body, "name"), Type: actionString(body, "type"), Content: actionStringRaw(body, "content"),
			Metadata: actionMap(body, "metadata"), IdempotencyKey: actionString(body, "idempotency_key"),
		})
	case "artifact_show":
		return s.GetArtifact(ctx, actor, actionString(body, "task_key"), actionString(body, "assignment_id"), actionInt64(body, "artifact_id"))
	case "workflow_ask":
		return s.AskWorkflowQuestion(ctx, actor, actionString(body, "assignment_id"), AskWorkflowQuestionInput{
			TaskRevision: actionInt64(body, "task_revision"), AssignmentRevision: actionInt64(body, "assignment_revision"),
			Question: actionString(body, "question"), Context: actionString(body, "context"), BlockingScope: actionString(body, "blocking_scope"),
			Anchor: actionString(body, "anchor"), Options: actionStrings(body, "options"), SuggestedAnswer: actionString(body, "suggested_answer"),
			ArtifactAttachments: actionInt64s(body, "artifact_attachments"), IdempotencyKey: actionString(body, "idempotency_key"),
		})
	case "workflow_answer":
		return s.AnswerWorkflowQuestion(ctx, actor, actionString(body, "question_id"), AnswerWorkflowQuestionInput{
			TaskRevision: actionInt64(body, "task_revision"), AssignmentRevision: actionInt64(body, "assignment_revision"),
			Answer: actionString(body, "answer"), IdempotencyKey: actionString(body, "idempotency_key"),
		})
	case "observe_subscribe":
		return s.CreateWorkflowSubscription(ctx, actor, actionString(body, "assignment_id"), CreateWorkflowSubscriptionInput{
			TaskRevision: actionInt64(body, "task_revision"), AssignmentRevision: actionInt64(body, "assignment_revision"),
			Pattern: actionString(body, "pattern"), CorrelationKey: actionString(body, "correlation_key"), Reaction: actionString(body, "reaction"),
			IdempotencyKey: actionString(body, "idempotency_key"),
		})
	case "observe_list":
		return s.ListWorkflowSubscriptions(ctx, actor, actionString(body, "assignment_id"))
	case "observe_cancel":
		return s.CancelWorkflowSubscription(ctx, actor, actionString(body, "assignment_id"), actionInt64(body, "subscription_id"), CancelWorkflowSubscriptionInput{
			TaskRevision: actionInt64(body, "task_revision"), AssignmentRevision: actionInt64(body, "assignment_revision"),
			IdempotencyKey: actionString(body, "idempotency_key"),
		})
	case "mine":
		waiting := actionString(body, "waiting_for")
		if waiting == "me" {
			waiting = actor.Principal
		}
		return s.ListTasks(ctx, actor, ListFilter{
			Queue: actionString(body, "queue"), Status: actionString(body, "status"),
			Assignee: actionString(body, "assignee"), WaitingFor: waiting,
			Text: actionString(body, "text"),
		})
	case "ready":
		filter := ReadyFilter{Queue: actionString(body, "queue"), Limit: actionInt(body, "limit")}
		if actionBool(body, "claim") {
			return s.ClaimReady(ctx, actor, filter, actionString(body, "idempotency_key"))
		}
		return s.Ready(ctx, actor, filter)
	case "show":
		return s.GetTask(ctx, actor, actionString(body, "key"))
	case "create":
		return s.CreateTask(ctx, actor, CreateTaskInput{
			Queue: actionString(body, "queue"), ParentKey: actionString(body, "parent_key"),
			Title: actionString(body, "title"), Description: actionString(body, "description"),
			Assignee: actionString(body, "assignee"), Group: actionString(body, "group"),
			Priority:       Priority(actionString(body, "priority")),
			IdempotencyKey: actionString(body, "idempotency_key"),
		})
	case "update":
		key := actionString(body, "key")
		revision, err := s.actionRevision(ctx, actor, key, body)
		if err != nil {
			return nil, err
		}
		return s.UpdateTask(ctx, actor, key, UpdateTaskInput{
			Title: actionOptionalString(body, "title"), Description: actionOptionalString(body, "description"),
			Status: actionOptionalString(body, "status"), PullRequest: actionOptionalString(body, "pull_request"), Assignee: actionOptionalString(body, "assignee"),
			ManualBlockReason: actionOptionalString(body, "manual_block_reason"), Priority: actionOptionalPriority(body, "priority"), Revision: revision,
		})
	case "assign":
		key := actionString(body, "key")
		revision, err := s.actionRevision(ctx, actor, key, body)
		if err != nil {
			return nil, err
		}
		assignee := actionString(body, "assignee")
		return s.UpdateTask(ctx, actor, key, UpdateTaskInput{Assignee: &assignee, Revision: revision})
	case "comment":
		return s.AddComment(ctx, actor, actionString(body, "key"), AddCommentInput{
			Body: actionString(body, "body"), IdempotencyKey: actionString(body, "idempotency_key"),
		})
	case "ask":
		principal := actionString(body, "principal")
		if principal != "" && !strings.Contains(principal, ":") {
			principal = agentPrincipal(principal)
		}
		if principal == "" {
			return nil, domainError(http.StatusBadRequest, "missing_principal", "question principal is required")
		}
		return s.AddComment(ctx, actor, actionString(body, "key"), AddCommentInput{
			Body:           "@" + principal + " " + actionString(body, "body"),
			IdempotencyKey: actionString(body, "idempotency_key"),
		})
	case "move":
		key := actionString(body, "key")
		revision, err := s.actionRevision(ctx, actor, key, body)
		if err != nil {
			return nil, err
		}
		return s.MoveTask(ctx, actor, key, MoveInput{
			ParentKey: actionString(body, "parent_key"),
			BeforeKey: actionString(body, "before_key"),
			Revision:  revision,
		})
	case "block":
		blocked := actionString(body, "key")
		blocker := actionString(body, "blocker_key")
		revision, err := s.actionRevision(ctx, actor, blocker, body)
		if err != nil {
			return nil, err
		}
		return s.AddRelation(ctx, actor, blocker, RelationInput{
			TargetKey: blocked, Type: "blocks", Revision: revision,
			IdempotencyKey: actionString(body, "idempotency_key"),
		})
	case "relate":
		key := actionString(body, "key")
		revision, err := s.actionRevision(ctx, actor, key, body)
		if err != nil {
			return nil, err
		}
		return s.AddRelation(ctx, actor, key, RelationInput{
			TargetKey: actionString(body, "target_key"), Type: "related", Revision: revision,
			IdempotencyKey: actionString(body, "idempotency_key"),
		})
	case "done":
		key := actionString(body, "key")
		revision, err := s.actionRevision(ctx, actor, key, body)
		if err != nil {
			return nil, err
		}
		return s.CompleteTask(ctx, actor, key, CompleteInput{
			Revision: revision, CompleteAnyway: actionBool(body, "complete_anyway"),
		})
	default:
		return nil, domainError(http.StatusBadRequest, "invalid_action",
			fmt.Sprintf("unknown task action %q", action))
	}
}

func (s *Service) readClaimReplay(ctx context.Context, actor Actor, key string) (assignmentMutationReplay, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return assignmentMutationReplay{}, false, err
	}
	defer tx.Rollback()
	return readAssignmentReplay(ctx, tx, actor, "claim_assignment", key)
}

func (s *Service) actionRevision(ctx context.Context, actor Actor, key string, body map[string]any) (int64, error) {
	if revision := int64(actionInt(body, "revision")); revision > 0 {
		return revision, nil
	}
	detail, err := s.GetTask(ctx, actor, key)
	return detail.Task.Revision, err
}

func actionString(body map[string]any, key string) string {
	value, _ := body[key].(string)
	return strings.TrimSpace(value)
}

func actionStringRaw(body map[string]any, key string) string {
	value, _ := body[key].(string)
	return value
}
func actionInt64(body map[string]any, key string) int64 { return int64(actionInt(body, key)) }
func actionMap(body map[string]any, key string) map[string]any {
	value, _ := body[key].(map[string]any)
	return value
}
func actionStrings(body map[string]any, key string) []string {
	if value, ok := body[key].([]string); ok {
		return value
	}
	raw, _ := body[key].([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if value, ok := item.(string); ok {
			out = append(out, value)
		}
	}
	return out
}
func actionInt64s(body map[string]any, key string) []int64 {
	raw, _ := body[key].([]any)
	out := make([]int64, 0, len(raw))
	for _, item := range raw {
		out = append(out, int64(actionInt(map[string]any{"v": item}, "v")))
	}
	return out
}

func actionOptionalString(body map[string]any, key string) *string {
	value, exists := body[key]
	if !exists {
		return nil
	}
	text, ok := value.(string)
	if !ok {
		return nil
	}
	return &text
}

func actionOptionalPriority(body map[string]any, key string) *Priority {
	value, exists := body[key]
	if !exists {
		return nil
	}
	text, ok := value.(string)
	if !ok {
		return nil
	}
	priority := Priority(strings.TrimSpace(text))
	return &priority
}

func actionBool(body map[string]any, key string) bool {
	value, _ := body[key].(bool)
	return value
}

func actionInt(body map[string]any, key string) int {
	switch value := body[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case string:
		parsed, _ := strconv.Atoi(value)
		return parsed
	default:
		return 0
	}
}
