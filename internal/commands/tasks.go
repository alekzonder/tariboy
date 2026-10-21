package commands

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/tasks"
)

// TaskOperatorCommands returns a copy of the native Tasks operator commands.
// Other CLIs can reuse their canonical HTTP schemas without maintaining a second
// task command registry.
func TaskOperatorCommands() []registry.Command {
	return []registry.Command{
		taskRoute("tasks.queue.list", "GET", "/api/task-queues", "List task queues",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				queues, err := control.ListQueues(ctx, actor)
				return map[string]any{"queues": queues, "count": len(queues)}, err
			}),
		taskRoute("tasks.queue.create", "POST", "/api/task-queues", "Create a task queue",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.CreateQueue(ctx, actor, tasks.CreateQueueInput{
					Prefix:           p.Trimmed("prefix"),
					Name:             p.Trimmed("name"),
					Description:      p.Trimmed("description"),
					Owners:           p.Strings("owners"),
					ResponsibleAgent: p.Trimmed("responsible_agent"),
				})
			}),
		taskRoute("tasks.queue.get", "GET", "/api/task-queues/{prefix}", "Inspect a task queue",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.GetQueue(ctx, actor, p.Trimmed("prefix"))
			}),
		taskRoute("tasks.queue.update", "PATCH", "/api/task-queues/{prefix}", "Update a task queue",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				var owners *[]string
				if p.Has("owners") {
					value := p.Strings("owners")
					owners = &value
				}
				return control.UpdateQueue(ctx, actor, p.Trimmed("prefix"), tasks.UpdateQueueInput{
					Name:             p.OptString("name"),
					Description:      p.OptString("description"),
					Owners:           owners,
					ResponsibleAgent: p.OptString("responsible_agent"),
					Revision:         p.Int64("revision"),
				})
			}),
		taskRoute("tasks.list", "GET", "/api/tasks", "List native tasks",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				filter := tasks.ListFilter{
					Queue:      p.Trimmed("queue"),
					Statuses:   p.Strings("status"),
					Categories: p.Strings("category"),
					StatusView: p.Trimmed("status_view"),
					Assignee:   p.Trimmed("assignee"),
					Author:     p.Trimmed("author"),
					Customer:   p.Trimmed("customer"),
					Group:      p.Trimmed("group"),
					Text:       p.Trimmed("text"),
					WaitingFor: p.Trimmed("waiting_for"),
					ScopeAgent: p.Trimmed("scope_agent"),
					ParentKey:  p.Trimmed("parent"),
					SubtreeKey: p.Trimmed("subtree"),
					Sort:       p.Trimmed("sort"),
					Limit:      int(p.Int64("limit")),
					AfterKey:   p.Trimmed("after"),
				}
				for _, priority := range p.Strings("priority") {
					filter.Priorities = append(filter.Priorities, tasks.Priority(priority))
				}
				if value, ok := p.OptBool("blocked"); ok {
					filter.Blocked = &value
				}
				return control.ListTasks(ctx, actor, filter)
			}),
		taskRoute("tasks.create", "POST", "/api/tasks", "Create a native task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.CreateTask(ctx, actor, tasks.CreateTaskInput{
					Queue:          p.Trimmed("queue"),
					ParentKey:      p.Trimmed("parent_key"),
					Title:          p.Trimmed("title"),
					Description:    p.Trimmed("description"),
					PullRequest:    p.Trimmed("pull_request"),
					Assignee:       p.Trimmed("assignee"),
					Group:          p.Trimmed("group"),
					Priority:       tasks.Priority(p.Trimmed("priority")),
					IdempotencyKey: p.Trimmed("idempotency_key"),
				})
			}),
		taskRoute("tasks.get", "GET", "/api/tasks/{key}", "Inspect a native task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.GetTask(ctx, actor, p.Trimmed("key"))
			}),
		taskRoute("tasks.update", "PATCH", "/api/tasks/{key}", "Update a native task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.UpdateTask(ctx, actor, p.Trimmed("key"), tasks.UpdateTaskInput{
					Title:             p.OptString("title"),
					Description:       p.OptString("description"),
					Status:            p.OptString("status"),
					PullRequest:       p.OptString("pull_request"),
					Assignee:          p.OptString("assignee"),
					ManualBlockReason: p.OptString("manual_block_reason"),
					Priority:          (*tasks.Priority)(p.OptString("priority")),
					Revision:          p.Int64("revision"),
				})
			}),
		taskRoute("tasks.export", "GET", "/api/tasks/{key}/export", "Export a task tree for transfer to another daemon",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.ExportTask(ctx, actor, p.Trimmed("key"))
			}),
		taskRoute("tasks.import", "POST", "/api/tasks/import", "Import a task tree exported from another daemon",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				var bundle tasks.TransferBundle
				if err := p.Decode("bundle", &bundle); err != nil {
					return nil, api.UserError{Code: "invalid_request", Msg: "bundle has an invalid schema"}
				}
				return control.ImportTask(ctx, actor, bundle)
			}),
		taskRoute("tasks.claim", "POST", "/api/tasks/{key}/claim", "Claim a native task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.ClaimTask(ctx, actor, p.Trimmed("key"), p.Int64("revision"))
			}),
		taskRoute("tasks.move", "POST", "/api/tasks/{key}/move", "Move a task in its queue tree",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.MoveTask(ctx, actor, p.Trimmed("key"), tasks.MoveInput{
					ParentKey: p.Trimmed("parent_key"),
					BeforeKey: p.Trimmed("before_key"),
					Revision:  p.Int64("revision"),
				})
			}),
		taskRoute("tasks.complete", "POST", "/api/tasks/{key}/complete", "Complete a task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				completeAnyway, _ := p.OptBool("complete_anyway")
				return control.CompleteTask(ctx, actor, p.Trimmed("key"), tasks.CompleteInput{
					Revision: p.Int64("revision"), CompleteAnyway: completeAnyway,
				})
			}),
		taskRoute("tasks.comments.list", "GET", "/api/tasks/{key}/comments", "List task comments",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				detail, err := control.GetTask(ctx, actor, p.Trimmed("key"))
				return map[string]any{"comments": detail.Comments, "count": len(detail.Comments)}, err
			}),
		taskRoute("tasks.comments.add", "POST", "/api/tasks/{key}/comments", "Add a task comment",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.AddComment(ctx, actor, p.Trimmed("key"), tasks.AddCommentInput{
					Body: p.Trimmed("body"), IdempotencyKey: p.Trimmed("idempotency_key"),
				})
			}),
		taskRoute("tasks.relations.list", "GET", "/api/tasks/{key}/relations", "List task relations",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				detail, err := control.GetTask(ctx, actor, p.Trimmed("key"))
				return map[string]any{"relations": detail.Relations, "count": len(detail.Relations)}, err
			}),
		taskRoute("tasks.relations.add", "POST", "/api/tasks/{key}/relations", "Add a task relation",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.AddRelation(ctx, actor, p.Trimmed("key"), tasks.RelationInput{
					TargetKey: p.Trimmed("target_key"), Type: p.Trimmed("type"),
					Revision: p.Int64("revision"), IdempotencyKey: p.Trimmed("idempotency_key"),
				})
			}),
		taskRoute("tasks.relations.delete", "DELETE", "/api/tasks/{key}/relations", "Delete a task relation",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				in := tasks.DeleteRelationInput{
					RelationID: p.Int64("relation_id"), Revision: p.Int64("revision"),
					IdempotencyKey: p.Trimmed("idempotency_key"),
				}
				if err := control.DeleteRelation(ctx, actor, p.Trimmed("key"), in); err != nil {
					return nil, err
				}
				return map[string]any{"deleted": true, "relation_id": in.RelationID}, nil
			}),
		taskRoute("tasks.events", "GET", "/api/tasks/{key}/events", "List task events",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				events, err := control.ListEvents(ctx, actor, p.Trimmed("key"),
					p.Int64("after"), int(p.Int64("limit")))
				return map[string]any{"events": events, "count": len(events)}, err
			}),
		taskRoute("tasks.principals", "GET", "/api/task-principals", "List task principals",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.Principals(ctx, actor)
			}),
		taskRoute("tasks.notifications.list", "GET", "/api/task-notifications", "List customer task notifications",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				include, _ := p.OptBool("include_dismissed")
				list, err := control.ListNotifications(ctx, actor, include)
				return map[string]any{"notifications": list, "count": len(list)}, err
			}),
		taskRoute("tasks.notifications.read", "POST", "/api/task-notifications/{id}/read", "Mark a task notification read",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.MarkNotification(ctx, actor, p.Trimmed("id"), "read")
			}),
		taskRoute("tasks.notifications.dismiss", "POST", "/api/task-notifications/{id}/dismiss", "Dismiss a task notification",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.MarkNotification(ctx, actor, p.Trimmed("id"), "dismiss")
			}),
		taskRoute("tasks.queue.pool.set", "PATCH", "/api/task-queues/{queue}/pools/{pool}", "Bind agents to a logical workflow pool",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.RebindAgentPool(ctx, actor, p.Trimmed("queue"), p.Trimmed("pool"), p.Strings("agents"), p.Int64("revision"), p.Trimmed("idempotency_key"))
			}),
		taskRoute("tasks.queue.pool.list", "GET", "/api/task-queues/{queue}/pools", "List logical workflow pools",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				items, err := control.ListAgentPools(ctx, actor, p.Trimmed("queue"))
				return map[string]any{"items": items, "count": len(items)}, err
			}),
		taskRoute("tasks.queue.pool.get", "GET", "/api/task-queues/{queue}/pools/{pool}", "Inspect a logical workflow pool",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.GetAgentPool(ctx, actor, p.Trimmed("queue"), p.Trimmed("pool"))
			}),
		taskRoute("tasks.queue.trigger.list", "GET", "/api/task-queues/{queue}/workflow-triggers", "List queue workflow triggers",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				items, err := control.ListQueueWorkflowTriggers(ctx, actor, p.Trimmed("queue"))
				return map[string]any{"items": items, "count": len(items)}, err
			}),
		taskRoute("tasks.queue.trigger.create", "POST", "/api/task-queues/{queue}/workflow-triggers", "Create a queue workflow trigger",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.CreateQueueWorkflowTrigger(ctx, actor, p.Trimmed("queue"), tasks.CreateQueueWorkflowTriggerInput{Pattern: p.Trimmed("pattern"), CorrelationKey: p.Trimmed("correlation_key"), Action: p.Trimmed("action")})
			}),
		taskRoute("tasks.queue.trigger.delete", "DELETE", "/api/task-queues/{queue}/workflow-triggers/{id}", "Delete a queue workflow trigger",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				id := p.Int64("id")
				if err := control.DeleteQueueWorkflowTrigger(ctx, actor, p.Trimmed("queue"), id); err != nil {
					return nil, err
				}
				return map[string]any{"deleted": true, "id": id}, nil
			}),
		taskRoute("tasks.advance", "POST", "/api/tasks/{key}/advance", "Declare an outcome for a workflow task's current status",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.Advance(ctx, actor, p.Trimmed("key"), tasks.AdvanceInput{
					Outcome: p.Trimmed("outcome"), Message: p.String("message"), From: p.Trimmed("from"),
				})
			}),
		taskRoute("tasks.artifacts.set", "PUT", "/api/tasks/{key}/artifacts/{name}", "Set a workflow task artifact",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.SetArtifact(ctx, actor, p.Trimmed("key"), p.Trimmed("name"), p.String("value"))
			}),
		taskRoute("tasks.artifacts.list", "GET", "/api/tasks/{key}/artifacts", "List the current artifacts of a workflow task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				artifacts, err := control.ListArtifacts(ctx, actor, p.Trimmed("key"))
				return map[string]any{"artifacts": artifacts, "count": len(artifacts)}, err
			}),
		taskRoute("tasks.artifacts.get", "GET", "/api/tasks/{key}/artifacts/{name}", "Show a workflow task artifact and its history",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				artifact, history, err := control.GetArtifact(ctx, actor, p.Trimmed("key"), p.Trimmed("name"))
				return map[string]any{"artifact": artifact, "history": history}, err
			}),
		taskRoute("tasks.workflow.get", "GET", "/api/tasks/{key}/workflow", "Show where a workflow task is and what it may do next",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.GetWorkflow(ctx, actor, p.Trimmed("key"))
			}),
		taskRoute("tasks.workflow.requests.get", "GET", "/api/tasks/{key}/workflow/requests/{id}", "Show one transition request of a workflow task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.GetTransitionRequest(ctx, actor, p.Trimmed("key"), p.Int64("id"))
			}),
		taskRoute("tasks.workflow.requests.retry", "POST", "/api/tasks/{key}/workflow/requests/{id}/retry", "Run the checks of a failed transition request again",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.RetryTransitionRequest(ctx, actor, p.Trimmed("key"), p.Int64("id"))
			}),
		taskRoute("tasks.workflow.runs.list", "GET", "/api/tasks/{key}/workflow/runs", "List the script runs of a workflow task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				runs, err := control.ListScriptRuns(ctx, actor, p.Trimmed("key"))
				return map[string]any{"runs": runs, "count": len(runs)}, err
			}),
		taskRoute("tasks.workflow.runs.get", "GET", "/api/tasks/{key}/workflow/runs/{id}", "Show one script run of a workflow task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.GetScriptRun(ctx, actor, p.Trimmed("key"), p.Int64("id"))
			}),
		taskRoute("tasks.workflow.runs.log", "GET", "/api/tasks/{key}/workflow/runs/{id}/log", "Read the tail of a script run's log",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				id := p.Int64("id")
				maxBytes, err := tasks.ParseMaxBytes(p["max_bytes"])
				if err != nil {
					return nil, err
				}
				text, truncated, err := control.ScriptRunLog(ctx, actor, p.Trimmed("key"), id, maxBytes)
				return map[string]any{"run_id": id, "text": text, "truncated": truncated}, err
			}),
		taskRoute("tasks.workflow.move", "POST", "/api/tasks/{key}/workflow/move", "Move a workflow task to another status",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.MoveWorkflow(ctx, actor, p.Trimmed("key"), p.Trimmed("to"), p.Trimmed("reason"))
			}),
		taskRoute("tasks.workflow.resume", "POST", "/api/tasks/{key}/workflow/resume", "Resolve the pause of a workflow task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.ResumeWorkflow(ctx, actor, p.Trimmed("key"), p.Trimmed("decision"))
			}),
		taskRoute("tasks.cancel", "POST", "/api/tasks/{key}/cancel", "Cancel a workflow task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.CancelWorkflowTask(ctx, actor, p.Trimmed("key"))
			}),
		taskRoute("tasks.queue.workflow.set", "PUT", "/api/task-queues/{queue}/workflow", "Bind a workflow image to a queue",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.SetQueueWorkflow(ctx, actor, p.Trimmed("queue"), p.Trimmed("ref"), p.Int64("revision"))
			}),
		taskRoute("tasks.queue.workflow.get", "GET", "/api/task-queues/{queue}/workflow", "Show the workflow image a queue is bound to",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.GetQueueWorkflow(ctx, actor, p.Trimmed("queue"))
			}),
		taskRoute("tasks.queue.workflow.clear", "DELETE", "/api/task-queues/{queue}/workflow", "Unbind the workflow image from a queue",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				queue := p.Trimmed("queue")
				if err := control.ClearQueueWorkflow(ctx, actor, queue, p.Int64("revision")); err != nil {
					return nil, err
				}
				return map[string]any{"queue": queue, "cleared": true}, nil
			}),
		withMaxBody(taskRoute("tasks.queue.secret.set", "PUT", "/api/task-queues/{queue}/secrets/{key}", "Set a queue secret for workflow scripts",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				queue, key := p.Trimmed("queue"), p.Trimmed("key")
				info, err := control.SetQueueSecretInfo(ctx, actor, queue, key, p.String("value"))
				if err != nil {
					return nil, err
				}
				return map[string]any{"queue": strings.ToUpper(queue), "key": key, "updated_at": info.UpdatedAt}, nil
			}), maxQueueSecretBody),
		taskRoute("tasks.queue.secret.ls", "GET", "/api/task-queues/{queue}/secrets", "List the keys of a queue's secrets and the required ones the global env holds",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				queue := p.Trimmed("queue")
				items, err := control.ListQueueSecrets(ctx, actor, queue)
				if err != nil {
					return nil, err
				}
				// Names only: a value of the global environment never leaves the daemon.
				global, err := control.QueueSecretsInGlobalEnv(ctx, actor, queue)
				return map[string]any{"secrets": items, "count": len(items), "global_env": global}, err
			}),
		taskRoute("tasks.queue.source.ls", "GET", "/api/task-queues/{queue}/sources", "List the workflow sources of a queue and their recent runs",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				items, err := control.ListQueueSources(ctx, actor, p.Trimmed("queue"))
				return map[string]any{"sources": items, "count": len(items)}, err
			}),
		taskRoute("tasks.queue.source.log", "GET", "/api/task-queues/{queue}/source-runs/{id}/log", "Read the tail of a workflow source run's log",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				id := p.Int64("id")
				maxBytes, err := tasks.ParseMaxBytes(p["max_bytes"])
				if err != nil {
					return nil, err
				}
				text, truncated, err := control.QueueSourceRunLog(ctx, actor, p.Trimmed("queue"), id, maxBytes)
				return map[string]any{"run_id": id, "text": text, "truncated": truncated}, err
			}),
		taskRoute("tasks.queue.secret.rm", "DELETE", "/api/task-queues/{queue}/secrets/{key}", "Remove a queue secret",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				queue, key := p.Trimmed("queue"), p.Trimmed("key")
				if err := control.RemoveQueueSecret(ctx, actor, queue, key); err != nil {
					return nil, err
				}
				return map[string]any{"queue": strings.ToUpper(queue), "key": key, "removed": true}, nil
			}),
		withMaxBody(taskRoute("tasks.queue.env.set", "PUT", "/api/task-queues/{queue}/env/{key}", "Set a plain queue env var for workflow scripts",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				queue, key := p.Trimmed("queue"), p.Trimmed("key")
				item, err := control.SetQueueEnvInfo(ctx, actor, queue, key, p.String("value"))
				if err != nil {
					return nil, err
				}
				return map[string]any{"queue": strings.ToUpper(queue), "key": key, "value": item.Value, "updated_at": item.UpdatedAt}, nil
			}), maxQueueSecretBody),
		taskRoute("tasks.queue.env.ls", "GET", "/api/task-queues/{queue}/env", "List the plain env vars of a queue with their values",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				items, err := control.ListQueueEnv(ctx, actor, p.Trimmed("queue"))
				return map[string]any{"env": items, "count": len(items)}, err
			}),
		taskRoute("tasks.queue.env.rm", "DELETE", "/api/task-queues/{queue}/env/{key}", "Remove a plain queue env var",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				queue, key := p.Trimmed("queue"), p.Trimmed("key")
				if err := control.RemoveQueueEnv(ctx, actor, queue, key); err != nil {
					return nil, err
				}
				return map[string]any{"queue": strings.ToUpper(queue), "key": key, "removed": true}, nil
			}),
	}
}

func taskCommands() []registry.Command { return TaskOperatorCommands() }

// maxQueueSecretBody bounds the body of a queue secret PUT: a 64 KiB value
// with JSON escaping fits well within it.
const maxQueueSecretBody = 512 << 10

// withMaxBody sets the request body bound of an HTTP command.
// jsonEscapedBodyLimit is the body limit for a route that carries a text file
// of up to maxFile bytes as one JSON string: an escape such as \u0000 takes six
// bytes, plus room for the other fields.
func jsonEscapedBodyLimit(maxFile int64) int64 { return 6*maxFile + 64<<10 }

func withMaxBody(cmd registry.Command, limit int64) registry.Command {
	route := *cmd.HTTP
	route.MaxBodyBytes = limit
	cmd.HTTP = &route
	return cmd
}

type taskHandler func(context.Context, registry.TaskControl, tasks.Actor, registry.Params) (any, error)

func taskRoute(path, method, route, summary string, handler taskHandler) registry.Command {
	bodyArgs := taskHTTPArgs(path)
	var args []registry.Arg
	for _, segment := range strings.Split(route, "/") {
		if !strings.HasPrefix(segment, "{") || !strings.HasSuffix(segment, "}") {
			continue
		}
		name := segment[1 : len(segment)-1]
		arg := registry.Arg{Name: name, Required: true, Help: "Resource " + name}
		for i, candidate := range bodyArgs {
			if candidate.Name == name {
				arg = candidate
				bodyArgs = append(bodyArgs[:i], bodyArgs[i+1:]...)
				break
			}
		}
		args = append(args, arg)
	}
	args = append(args, bodyArgs...)
	for i := range args {
		if args[i].Flag == "" {
			args[i].Flag = strings.ReplaceAll(args[i].Name, "_", "-")
		}
	}
	return registry.Command{
		Path: path, Summary: summary, CLIHidden: path != "tasks.queue.create",
		Args: args, ResultSchema: taskHTTPResultSchema(path),
		Schemas: taskOpenAPISchemas(),
		HTTP:    &registry.HTTPRoute{Method: method, Path: route},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			if c.Tasks == nil {
				return nil, api.UserError{Status: http.StatusServiceUnavailable,
					Code: "tasks_unavailable", Msg: "native Tasks service is unavailable"}
			}
			actor := tasks.CustomerActor(c.Tasks.CustomerLogin())
			result, err := handler(p.Ctx, c.Tasks, actor, p.Params)
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
	case "tasks.queue.update":
		return []registry.Arg{
			{Name: "name", Help: "Queue name"},
			{Name: "description", Help: "Queue description"},
			{Name: "owners", Help: "Comma-separated owner agents"},
			{Name: "responsible_agent", Help: "Responsible agent"},
			{Name: "revision", Type: registry.Int, Required: true, Help: "Expected current revision"},
		}
	case "tasks.notifications.list":
		return []registry.Arg{{Name: "include_dismissed", Type: registry.Bool, Help: "Include dismissed notifications"}}
	case "tasks.queue.create":
		return []registry.Arg{
			{Name: "prefix", Required: true, Help: "Queue key prefix"},
			{Name: "name", Required: true, Help: "Queue name"},
			{Name: "description", Help: "Queue description"},
			{Name: "owners", Help: "Comma-separated owner agents"},
			{Name: "responsible_agent", Flag: "responsible-agent", Help: "Responsible agent"},
		}
	case "tasks.create":
		return []registry.Arg{
			{Name: "queue", Help: "Queue prefix for a root task"},
			{Name: "parent_key", Flag: "parent-key", Help: "Parent task key"},
			{Name: "title", Required: true, Help: "Task title"},
			{Name: "description", Help: "Task description"},
			{Name: "pull_request", Flag: "pull-request", Help: "Canonical pull request URL"},
			{Name: "assignee", Help: "Task assignee"},
			{Name: "group", Help: "Task group"},
			{Name: "priority", Help: "Task priority", Schema: map[string]any{
				"type": "string", "enum": []string{string(tasks.PriorityP0), string(tasks.PriorityP1), string(tasks.PriorityP2), string(tasks.PriorityP3)},
			}},
			{Name: "idempotency_key", Flag: "idempotency-key", Help: "Stable retry key"},
		}
	case "tasks.update":
		return []registry.Arg{
			{Name: "title", Help: "Task title"},
			{Name: "description", Help: "Task description"},
			{Name: "status", Help: "Task status", Schema: map[string]any{
				"type": "string", "enum": []string{tasks.StatusOpen, tasks.StatusInProgress, tasks.StatusWaitCustomer, tasks.StatusDone, tasks.StatusCancelled},
			}},
			{Name: "pull_request", Flag: "pull-request", Help: "Canonical pull request URL (empty clears it)"},
			{Name: "assignee", Help: "Task assignee"},
			{Name: "manual_block_reason", Flag: "manual-block-reason", Help: "Manual block reason"},
			{Name: "priority", Help: "Task priority"},
			{Name: "revision", Type: registry.Int, Required: true, Help: "Expected current revision"},
		}
	case "tasks.import":
		return []registry.Arg{{Name: "bundle", Type: registry.JSONObject, Required: true,
			Help:   "Bundle returned by tasks.export on the source daemon",
			Schema: map[string]any{"type": "object"}}}
	case "tasks.queue.pool.set":
		return poolMutationArgs(registry.Arg{Name: "agents", Required: true, Help: "Explicit agent names", Schema: map[string]any{"type": "array", "items": map[string]any{"type": "string"}}})
	case "tasks.queue.trigger.create":
		return []registry.Arg{{Name: "pattern", Required: true, Help: "Allowed channel pattern"}, {Name: "correlation_key", Help: "Correlation selector"}, {Name: "action", Required: true, Help: "Declared workflow trigger action"}}
	case "tasks.queue.trigger.delete":
		return []registry.Arg{{Name: "id", Type: registry.Int, Required: true, Help: "Resource id"}}
	case "tasks.advance":
		return []registry.Arg{{Name: "outcome", Required: true, Help: "Outcome to declare"}, {Name: "message", Help: "Message recorded with the transition"}, {Name: "from", Help: "Status the caller believes the task is in; a different current status is refused with status_changed"}}
	case "tasks.artifacts.set":
		return []registry.Arg{{Name: "value", Help: "Artifact value, stored as given"}}
	case "tasks.workflow.runs.log", "tasks.queue.source.log":
		return []registry.Arg{{Name: "max_bytes", Type: registry.Int, Help: "Largest log tail to return in bytes (default 65536, maximum 1048576)"}}
	case "tasks.workflow.move":
		return []registry.Arg{{Name: "to", Required: true, Help: "Target status id"}, {Name: "reason", Required: true, Help: "Why the task is moved"}}
	case "tasks.workflow.resume":
		return []registry.Arg{{Name: "decision", Required: true, Help: "continue resumes the task in its current status with counters reset; release dispatches the task to another pool member", Schema: map[string]any{
			"type": "string", "enum": []string{tasks.ResumeContinue, tasks.ResumeRelease},
		}}}
	case "tasks.queue.workflow.set":
		return []registry.Arg{{Name: "ref", Required: true, Help: "Workflow image reference, name:tag"}, {Name: "revision", Type: registry.Int, Help: "Expected current binding revision (zero for a new binding)"}}
	case "tasks.queue.workflow.clear":
		return []registry.Arg{{Name: "revision", Type: registry.Int, Required: true, Help: "Expected current binding revision"}}
	case "tasks.queue.secret.set":
		return []registry.Arg{{Name: "value", Help: "Secret value, stored as given; the CLI reads stdin when --value is absent"}}
	case "tasks.queue.env.set":
		return []registry.Arg{{Name: "value", Help: "Env value, stored as given; the CLI also takes it as the third argument or reads stdin"}}
	case "tasks.events":
		return []registry.Arg{{Name: "after", Type: registry.Int, Help: "Resume after sequence"}, {Name: "limit", Type: registry.Int, Help: "Maximum events"}}
	default:
		return nil
	}
}

func taskHTTPResultSchema(path string) map[string]any {
	switch path {
	case "tasks.queue.pool.set", "tasks.queue.pool.get":
		return schemaRef("AgentPool")
	case "tasks.queue.pool.list":
		return listSchema("AgentPool")
	case "tasks.queue.trigger.create":
		return schemaRef("QueueWorkflowTrigger")
	case "tasks.queue.trigger.list":
		return listSchema("QueueWorkflowTrigger")
	case "tasks.advance", "tasks.workflow.requests.get", "tasks.workflow.requests.retry":
		return schemaRef("TransitionRequest")
	case "tasks.workflow.runs.list":
		return objectSchema([]string{"runs", "count"}, map[string]any{"runs": arrayOf("ScriptRun"), "count": map[string]any{"type": "integer"}})
	case "tasks.workflow.runs.get":
		return schemaRef("ScriptRun")
	case "tasks.queue.source.ls":
		return objectSchema([]string{"sources", "count"}, map[string]any{"sources": arrayOf("QueueSource"), "count": map[string]any{"type": "integer"}})
	case "tasks.workflow.runs.log", "tasks.queue.source.log":
		return objectSchema([]string{"run_id", "text", "truncated"}, map[string]any{"run_id": map[string]any{"type": "integer"}, "text": map[string]any{"type": "string"}, "truncated": map[string]any{"type": "boolean"}})
	case "tasks.artifacts.set":
		return schemaRef("Artifact")
	case "tasks.artifacts.list":
		return objectSchema([]string{"artifacts", "count"}, map[string]any{"artifacts": arrayOf("Artifact"), "count": map[string]any{"type": "integer"}})
	case "tasks.artifacts.get":
		return objectSchema([]string{"artifact", "history"}, map[string]any{"artifact": schemaRef("Artifact"), "history": arrayOf("Artifact")})
	case "tasks.workflow.get":
		return schemaRef("WorkflowView")
	case "tasks.workflow.move", "tasks.workflow.resume", "tasks.cancel":
		return schemaRef("Task")
	case "tasks.queue.workflow.set", "tasks.queue.workflow.get":
		return schemaRef("QueueWorkflow")
	case "tasks.queue.secret.set":
		return objectSchema([]string{"queue", "key", "updated_at"}, map[string]any{"queue": map[string]any{"type": "string"}, "key": map[string]any{"type": "string"}, "updated_at": map[string]any{"type": "string"}})
	case "tasks.queue.secret.ls":
		return objectSchema([]string{"secrets", "count"}, map[string]any{"secrets": arrayOf("QueueSecret"), "count": map[string]any{"type": "integer"}})
	case "tasks.queue.env.set":
		return objectSchema([]string{"queue", "key", "value", "updated_at"}, map[string]any{"queue": map[string]any{"type": "string"}, "key": map[string]any{"type": "string"}, "value": map[string]any{"type": "string"}, "updated_at": map[string]any{"type": "string"}})
	case "tasks.queue.env.ls":
		return objectSchema([]string{"env", "count"}, map[string]any{"env": arrayOf("QueueEnvVar"), "count": map[string]any{"type": "integer"}})
	case "tasks.queue.secret.rm", "tasks.queue.env.rm":
		return objectSchema([]string{"queue", "key", "removed"}, map[string]any{"queue": map[string]any{"type": "string"}, "key": map[string]any{"type": "string"}, "removed": map[string]any{"type": "boolean"}})
	case "tasks.queue.workflow.clear":
		return objectSchema([]string{"queue", "cleared"}, map[string]any{"queue": map[string]any{"type": "string"}, "cleared": map[string]any{"type": "boolean"}})
	default:
		return map[string]any{"type": "object"}
	}
}

func poolMutationArgs(primary registry.Arg) []registry.Arg {
	return []registry.Arg{primary, {Name: "revision", Type: registry.Int, Required: true, Help: "Expected current revision (zero for a new pool)"}, {Name: "idempotency_key", Required: true, Help: "Stable retry key"}}
}
