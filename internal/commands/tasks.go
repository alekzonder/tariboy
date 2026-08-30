package commands

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/tasks"
)

func taskCommands() []registry.Command {
	return []registry.Command{
		taskRoute("tasks.queue.list", "GET", "/api/task-queues", "List task queues",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				queues, err := control.ListQueues(ctx, actor)
				return map[string]any{"queues": queues, "count": len(queues)}, err
			}),
		taskRoute("tasks.queue.create", "POST", "/api/task-queues", "Create a task queue",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.CreateQueue(ctx, actor, tasks.CreateQueueInput{
					Prefix:           stringParam(p, "prefix"),
					Name:             stringParam(p, "name"),
					Description:      stringParam(p, "description"),
					Owners:           stringSliceParam(p, "owners"),
					ResponsibleAgent: stringParam(p, "responsible_agent"),
				})
			}),
		taskRoute("tasks.queue.get", "GET", "/api/task-queues/{prefix}", "Inspect a task queue",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.GetQueue(ctx, actor, stringParam(p, "prefix"))
			}),
		taskRoute("tasks.queue.update", "PATCH", "/api/task-queues/{prefix}", "Update a task queue",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				var owners *[]string
				if _, ok := p["owners"]; ok {
					value := stringSliceParam(p, "owners")
					owners = &value
				}
				return control.UpdateQueue(ctx, actor, stringParam(p, "prefix"), tasks.UpdateQueueInput{
					Name:             optionalStringParam(p, "name"),
					Description:      optionalStringParam(p, "description"),
					Owners:           owners,
					ResponsibleAgent: optionalStringParam(p, "responsible_agent"),
					Revision:         int64Param(p, "revision"),
				})
			}),
		taskRoute("tasks.list", "GET", "/api/tasks", "List native tasks",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				filter := tasks.ListFilter{
					Queue:      stringParam(p, "queue"),
					Status:     stringParam(p, "status"),
					StatusView: stringParam(p, "status_view"),
					Assignee:   stringParam(p, "assignee"),
					Author:     stringParam(p, "author"),
					Group:      stringParam(p, "group"),
					Text:       stringParam(p, "text"),
					WaitingFor: stringParam(p, "waiting_for"),
					ScopeAgent: stringParam(p, "scope_agent"),
					Limit:      int(int64Param(p, "limit")),
					AfterKey:   stringParam(p, "after"),
				}
				if value, ok := boolParam(p, "blocked"); ok {
					filter.Blocked = &value
				}
				return control.ListTasks(ctx, actor, filter)
			}),
		taskRoute("tasks.create", "POST", "/api/tasks", "Create a native task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.CreateTask(ctx, actor, tasks.CreateTaskInput{
					Queue:          stringParam(p, "queue"),
					ParentKey:      stringParam(p, "parent_key"),
					Title:          stringParam(p, "title"),
					Description:    stringParam(p, "description"),
					Assignee:       stringParam(p, "assignee"),
					Group:          stringParam(p, "group"),
					Priority:       tasks.Priority(stringParam(p, "priority")),
					IdempotencyKey: stringParam(p, "idempotency_key"),
				})
			}),
		taskRoute("tasks.get", "GET", "/api/tasks/{key}", "Inspect a native task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.GetTask(ctx, actor, stringParam(p, "key"))
			}),
		taskRoute("tasks.update", "PATCH", "/api/tasks/{key}", "Update a native task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.UpdateTask(ctx, actor, stringParam(p, "key"), tasks.UpdateTaskInput{
					Title:             optionalStringParam(p, "title"),
					Description:       optionalStringParam(p, "description"),
					Status:            optionalStringParam(p, "status"),
					Assignee:          optionalStringParam(p, "assignee"),
					ManualBlockReason: optionalStringParam(p, "manual_block_reason"),
					Priority:          optionalPriorityParam(p, "priority"),
					Revision:          int64Param(p, "revision"),
				})
			}),
		taskRoute("tasks.claim", "POST", "/api/tasks/{key}/claim", "Claim a native task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.ClaimTask(ctx, actor, stringParam(p, "key"), int64Param(p, "revision"))
			}),
		taskRoute("tasks.move", "POST", "/api/tasks/{key}/move", "Move a task in its queue tree",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.MoveTask(ctx, actor, stringParam(p, "key"), tasks.MoveInput{
					ParentKey: stringParam(p, "parent_key"),
					BeforeKey: stringParam(p, "before_key"),
					Revision:  int64Param(p, "revision"),
				})
			}),
		taskRoute("tasks.complete", "POST", "/api/tasks/{key}/complete", "Complete a task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				completeAnyway, _ := boolParam(p, "complete_anyway")
				return control.CompleteTask(ctx, actor, stringParam(p, "key"), tasks.CompleteInput{
					Revision: int64Param(p, "revision"), CompleteAnyway: completeAnyway,
				})
			}),
		taskRoute("tasks.comments.list", "GET", "/api/tasks/{key}/comments", "List task comments",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				detail, err := control.GetTask(ctx, actor, stringParam(p, "key"))
				return map[string]any{"comments": detail.Comments, "count": len(detail.Comments)}, err
			}),
		taskRoute("tasks.comments.add", "POST", "/api/tasks/{key}/comments", "Add a task comment",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.AddComment(ctx, actor, stringParam(p, "key"), tasks.AddCommentInput{
					Body: stringParam(p, "body"), IdempotencyKey: stringParam(p, "idempotency_key"),
				})
			}),
		taskRoute("tasks.relations.list", "GET", "/api/tasks/{key}/relations", "List task relations",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				detail, err := control.GetTask(ctx, actor, stringParam(p, "key"))
				return map[string]any{"relations": detail.Relations, "count": len(detail.Relations)}, err
			}),
		taskRoute("tasks.relations.add", "POST", "/api/tasks/{key}/relations", "Add a task relation",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.AddRelation(ctx, actor, stringParam(p, "key"), tasks.RelationInput{
					TargetKey: stringParam(p, "target_key"), Type: stringParam(p, "type"),
					Revision: int64Param(p, "revision"), IdempotencyKey: stringParam(p, "idempotency_key"),
				})
			}),
		taskRoute("tasks.relations.delete", "DELETE", "/api/tasks/{key}/relations", "Delete a task relation",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				in := tasks.DeleteRelationInput{
					RelationID: int64Param(p, "relation_id"), Revision: int64Param(p, "revision"),
					IdempotencyKey: stringParam(p, "idempotency_key"),
				}
				if err := control.DeleteRelation(ctx, actor, stringParam(p, "key"), in); err != nil {
					return nil, err
				}
				return map[string]any{"deleted": true, "relation_id": in.RelationID}, nil
			}),
		taskRoute("tasks.events", "GET", "/api/tasks/{key}/events", "List task events",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				events, err := control.ListEvents(ctx, actor, stringParam(p, "key"),
					int64Param(p, "after"), int(int64Param(p, "limit")))
				return map[string]any{"events": events, "count": len(events)}, err
			}),
		taskRoute("tasks.principals", "GET", "/api/task-principals", "List task principals",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.Principals(ctx, actor)
			}),
		taskRoute("tasks.notifications.list", "GET", "/api/task-notifications", "List customer task notifications",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				include, _ := boolParam(p, "include_dismissed")
				list, err := control.ListNotifications(ctx, actor, include)
				return map[string]any{"notifications": list, "count": len(list)}, err
			}),
		taskRoute("tasks.notifications.read", "POST", "/api/task-notifications/{id}/read", "Mark a task notification read",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.MarkNotification(ctx, actor, stringParam(p, "id"), "read")
			}),
		taskRoute("tasks.notifications.dismiss", "POST", "/api/task-notifications/{id}/dismiss", "Dismiss a task notification",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.MarkNotification(ctx, actor, stringParam(p, "id"), "dismiss")
			}),
		taskRoute("tasks.workflows.create", "POST", "/api/workflows", "Create or update a workflow draft",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				var definition tasks.WorkflowDefinition
				if err := decodeTaskParam(p, "definition", &definition); err != nil {
					return nil, err
				}
				return control.CreateWorkflowDraft(ctx, actor, definition)
			}),
		taskRoute("tasks.workflows.versions", "GET", "/api/workflows/{name}/versions", "List workflow versions",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				items, err := control.ListWorkflowVersions(ctx, actor, stringParam(p, "name"))
				return map[string]any{"items": items, "count": len(items)}, err
			}),
		taskRoute("tasks.workflows.get", "GET", "/api/workflows/{name}/versions/{version}", "Inspect a workflow version",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.GetWorkflowVersion(ctx, actor, stringParam(p, "name"), int(int64Param(p, "version")))
			}),
		taskRoute("tasks.workflows.validate", "POST", "/api/workflows/{name}/versions/{version}/validate", "Validate a workflow version",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				items, err := control.ValidateWorkflowVersion(ctx, actor, stringParam(p, "name"), int(int64Param(p, "version")))
				return map[string]any{"items": items, "count": len(items), "valid": err == nil && len(items) == 0}, err
			}),
		taskRoute("tasks.workflows.publish", "POST", "/api/workflows/{name}/versions/{version}/publish", "Publish an immutable workflow version",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.PublishWorkflowVersion(ctx, actor, stringParam(p, "name"), int(int64Param(p, "version")))
			}),
		taskRoute("tasks.queue.workflow.set", "PUT", "/api/task-queues/{queue}/workflow", "Activate a published workflow for a queue",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.ActivateQueueWorkflow(ctx, actor, stringParam(p, "queue"), int64Param(p, "workflow_version_id"), int64Param(p, "revision"), stringParam(p, "idempotency_key"))
			}),
		taskRoute("tasks.queue.workflow.get", "GET", "/api/task-queues/{queue}/workflow", "Inspect a queue workflow binding",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.GetQueueWorkflow(ctx, actor, stringParam(p, "queue"))
			}),
		taskRoute("tasks.queue.pool.set", "PATCH", "/api/task-queues/{queue}/pools/{pool}", "Bind agents to a logical workflow pool",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.RebindAgentPool(ctx, actor, stringParam(p, "queue"), stringParam(p, "pool"), stringSliceParam(p, "agents"), int64Param(p, "revision"), stringParam(p, "idempotency_key"))
			}),
		taskRoute("tasks.queue.pool.list", "GET", "/api/task-queues/{queue}/pools", "List logical workflow pools",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				items, err := control.ListAgentPools(ctx, actor, stringParam(p, "queue"))
				return map[string]any{"items": items, "count": len(items)}, err
			}),
		taskRoute("tasks.queue.pool.get", "GET", "/api/task-queues/{queue}/pools/{pool}", "Inspect a logical workflow pool",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.GetAgentPool(ctx, actor, stringParam(p, "queue"), stringParam(p, "pool"))
			}),
		taskRoute("tasks.queue.trigger.list", "GET", "/api/task-queues/{queue}/workflow-triggers", "List queue workflow triggers",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				items, err := control.ListQueueWorkflowTriggers(ctx, actor, stringParam(p, "queue"))
				return map[string]any{"items": items, "count": len(items)}, err
			}),
		taskRoute("tasks.queue.trigger.create", "POST", "/api/task-queues/{queue}/workflow-triggers", "Create a queue workflow trigger",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.CreateQueueWorkflowTrigger(ctx, actor, stringParam(p, "queue"), tasks.CreateQueueWorkflowTriggerInput{Pattern: stringParam(p, "pattern"), CorrelationKey: stringParam(p, "correlation_key"), Action: stringParam(p, "action")})
			}),
		taskRoute("tasks.queue.trigger.delete", "DELETE", "/api/task-queues/{queue}/workflow-triggers/{id}", "Delete a queue workflow trigger",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				id := int64Param(p, "id")
				if err := control.DeleteQueueWorkflowTrigger(ctx, actor, stringParam(p, "queue"), id); err != nil {
					return nil, err
				}
				return map[string]any{"deleted": true, "id": id}, nil
			}),
		taskRoute("tasks.workflow.get", "GET", "/api/tasks/{key}/workflow", "Inspect workflow state for a task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.GetWorkflowExecution(ctx, actor, stringParam(p, "key"))
			}),
		taskRoute("tasks.workflow.packets", "GET", "/api/tasks/{key}/work-packets", "List workflow work packets",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				items, err := control.ListWorkPackets(ctx, actor, stringParam(p, "key"))
				return map[string]any{"items": items, "count": len(items)}, err
			}),
		taskRoute("tasks.workflow.assignments", "GET", "/api/tasks/{key}/assignments", "List workflow assignments",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				items, err := control.ListWorkflowAssignments(ctx, actor, stringParam(p, "key"))
				return map[string]any{"items": items, "count": len(items)}, err
			}),
		taskRoute("tasks.workflow.artifacts", "GET", "/api/tasks/{key}/artifacts", "List workflow artifacts",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				items, err := control.ListArtifacts(ctx, actor, stringParam(p, "key"), stringParam(p, "assignment_id"))
				return map[string]any{"items": items, "count": len(items)}, err
			}),
		taskRoute("tasks.workflow.artifact.get", "GET", "/api/tasks/{key}/artifacts/{id}", "Inspect a workflow artifact",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.GetArtifact(ctx, actor, stringParam(p, "key"), stringParam(p, "assignment_id"), int64Param(p, "id"))
			}),
		taskRoute("tasks.workflow.questions", "GET", "/api/tasks/{key}/questions", "List workflow questions",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				items, err := control.ListWorkflowQuestions(ctx, actor, stringParam(p, "key"), stringParam(p, "assignment_id"))
				return map[string]any{"items": items, "count": len(items)}, err
			}),
		taskRoute("tasks.workflow.question.get", "GET", "/api/tasks/{key}/questions/{id}", "Inspect a workflow question",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.GetWorkflowQuestion(ctx, actor, stringParam(p, "key"), int64Param(p, "id"))
			}),
		taskRoute("tasks.workflow.subscriptions", "GET", "/api/tasks/{key}/subscriptions", "List workflow subscriptions",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				items, err := control.ListTaskWorkflowSubscriptions(ctx, actor, stringParam(p, "key"), stringParam(p, "assignment_id"))
				return map[string]any{"items": items, "count": len(items)}, err
			}),
		taskRoute("tasks.workflow.events", "GET", "/api/tasks/{key}/workflow-events", "Replay workflow events",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				all, err := control.ListEvents(ctx, actor, stringParam(p, "key"), int64Param(p, "after"), int(int64Param(p, "limit")))
				items := make([]tasks.Event, 0, len(all))
				for _, event := range all {
					if strings.HasPrefix(event.Kind, "workflow.") {
						items = append(items, event)
					}
				}
				return map[string]any{"items": items, "count": len(items)}, err
			}),
	}
}

type taskHandler func(context.Context, registry.TaskControl, tasks.Actor, registry.Params) (any, error)

func taskRoute(path, method, route, summary string, handler taskHandler) registry.Command {
	return registry.Command{
		Path: path, Summary: summary, CLIHidden: path != "tasks.queue.create",
		Args: taskHTTPArgs(path), ResultSchema: taskHTTPResultSchema(path),
		Schemas: taskOpenAPISchemas(),
		HTTP:    &registry.HTTPRoute{Method: method, Path: route},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			if c.Tasks == nil {
				return nil, api.UserError{Status: http.StatusServiceUnavailable,
					Code: "tasks_unavailable", Msg: "native Tasks service is unavailable"}
			}
			actor := tasks.CustomerActor(c.Tasks.CustomerLogin())
			result, err := handler(registry.RequestContext(p), c.Tasks, actor, p)
			if err == nil {
				return result, nil
			}
			var domain *tasks.Error
			if errors.As(err, &domain) {
				return nil, api.UserError{
					Status: domain.Status, Code: domain.Code, Msg: domain.Msg, Data: domain.Data,
				}
			}
			return nil, err
		},
	}
}

func taskHTTPArgs(path string) []registry.Arg {
	switch path {
	case "tasks.queue.create":
		return []registry.Arg{
			{Name: "prefix", Required: true, Help: "Queue key prefix"},
			{Name: "name", Required: true, Help: "Queue name"},
			{Name: "description", Help: "Queue description"},
			{Name: "owners", Help: "Comma-separated owner agents"},
			{Name: "responsible_agent", Flag: "responsible-agent", Help: "Responsible agent"},
		}
	case "tasks.workflows.create":
		return []registry.Arg{{Name: "definition", Required: true, Help: "Versioned workflow definition", Schema: schemaRef("WorkflowDefinition")}}
	case "tasks.queue.workflow.set":
		return workflowMutationArgs(registry.Arg{Name: "workflow_version_id", Type: registry.Int, Required: true, Help: "Published workflow version id"})
	case "tasks.queue.pool.set":
		return workflowMutationArgs(registry.Arg{Name: "agents", Required: true, Help: "Explicit agent names", Schema: map[string]any{"type": "array", "items": map[string]any{"type": "string"}}})
	case "tasks.queue.trigger.create":
		return []registry.Arg{{Name: "pattern", Required: true, Help: "Allowed channel pattern"}, {Name: "correlation_key", Help: "Correlation selector"}, {Name: "action", Required: true, Help: "Declared workflow trigger action"}}
	case "tasks.workflows.get", "tasks.workflows.validate", "tasks.workflows.publish":
		return []registry.Arg{{Name: "version", Type: registry.Int, Required: true, Help: "Workflow version"}}
	case "tasks.queue.trigger.delete", "tasks.workflow.artifact.get", "tasks.workflow.question.get":
		return []registry.Arg{{Name: "id", Type: registry.Int, Required: true, Help: "Resource id"}}
	case "tasks.workflow.artifacts", "tasks.workflow.questions":
		return []registry.Arg{{Name: "assignment_id", Help: "Optional assignment scope"}}
	case "tasks.workflow.subscriptions":
		return []registry.Arg{{Name: "assignment_id", Required: true, Help: "Assignment id"}}
	case "tasks.workflow.events":
		return []registry.Arg{{Name: "after", Type: registry.Int, Help: "Resume after sequence"}, {Name: "limit", Type: registry.Int, Help: "Maximum events"}}
	default:
		return nil
	}
}

func taskHTTPResultSchema(path string) map[string]any {
	switch path {
	case "tasks.workflows.create", "tasks.workflows.get", "tasks.workflows.publish":
		return schemaRef("WorkflowVersion")
	case "tasks.workflows.versions":
		return listSchema("WorkflowVersion")
	case "tasks.workflows.validate":
		return schemaRef("WorkflowValidationResult")
	case "tasks.queue.workflow.set", "tasks.queue.workflow.get":
		return schemaRef("QueueWorkflowBinding")
	case "tasks.queue.pool.set", "tasks.queue.pool.get":
		return schemaRef("AgentPool")
	case "tasks.queue.pool.list":
		return listSchema("AgentPool")
	case "tasks.queue.trigger.create":
		return schemaRef("QueueWorkflowTrigger")
	case "tasks.queue.trigger.list":
		return listSchema("QueueWorkflowTrigger")
	case "tasks.workflow.get":
		return schemaRef("WorkflowExecutionView")
	case "tasks.workflow.packets":
		return listSchema("WorkPacket")
	case "tasks.workflow.assignments":
		return listSchema("Assignment")
	case "tasks.workflow.artifacts":
		return listSchema("Artifact")
	case "tasks.workflow.artifact.get":
		return schemaRef("Artifact")
	case "tasks.workflow.questions":
		return listSchema("WorkflowQuestion")
	case "tasks.workflow.question.get":
		return schemaRef("WorkflowQuestion")
	case "tasks.workflow.subscriptions":
		return listSchema("WorkflowSubscription")
	case "tasks.workflow.events":
		return listSchema("TaskEvent")
	default:
		return map[string]any{"type": "object"}
	}
}

func workflowMutationArgs(primary registry.Arg) []registry.Arg {
	return []registry.Arg{primary, {Name: "revision", Type: registry.Int, Required: true, Help: "Expected current revision (zero for first binding)"}, {Name: "idempotency_key", Required: true, Help: "Stable retry key"}}
}

func stringParam(p registry.Params, key string) string {
	value, _ := p[key].(string)
	return strings.TrimSpace(value)
}

func optionalStringParam(p registry.Params, key string) *string {
	value, ok := p[key]
	if !ok || value == nil {
		return nil
	}
	text, ok := value.(string)
	if !ok {
		return nil
	}
	return &text
}

func optionalPriorityParam(p registry.Params, key string) *tasks.Priority {
	value := optionalStringParam(p, key)
	if value == nil {
		return nil
	}
	priority := tasks.Priority(*value)
	return &priority
}

func stringSliceParam(p registry.Params, key string) []string {
	switch values := p[key].(type) {
	case []string:
		return values
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return out
	case string:
		if strings.TrimSpace(values) == "" {
			return nil
		}
		return strings.Split(values, ",")
	default:
		return nil
	}
}

func int64Param(p registry.Params, key string) int64 {
	switch value := p[key].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	case string:
		n, _ := strconv.ParseInt(value, 10, 64)
		return n
	default:
		return 0
	}
}

func boolParam(p registry.Params, key string) (bool, bool) {
	value, exists := p[key]
	if !exists {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(typed)
		return parsed, err == nil
	default:
		return false, false
	}
}

func decodeTaskParam(p registry.Params, key string, out any) error {
	value, ok := p[key]
	if !ok {
		value = p
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return api.UserError{Status: http.StatusBadRequest, Code: "invalid_request", Msg: "request body must be valid JSON"}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return api.UserError{Status: http.StatusBadRequest, Code: "invalid_request", Msg: "request body has an invalid schema"}
	}
	return nil
}
