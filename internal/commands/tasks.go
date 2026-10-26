package commands

import (
	"context"
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
		taskActivity(),
		taskRoute("tasks.queue.list", "GET", "/api/task-queues", "List task queues",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				queues, err := control.ListQueues(ctx, actor)
				return TaskQueueList{Queues: queues, Count: len(queues)}, err
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
				var in tasks.ListRequest
				tasks.Decode(p, &in)
				return control.ListTasks(ctx, actor, in.Filter())
			}),
		taskRoute("tasks.create", "POST", "/api/tasks", "Create a native task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				var in tasks.CreateRequest
				tasks.Decode(p, &in)
				return control.CreateTask(ctx, actor, in.Input())
			}),
		taskRoute("tasks.get", "GET", "/api/tasks/{key}", "Inspect a native task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.GetTask(ctx, actor, p.Trimmed("key"))
			}),
		taskRoute("tasks.update", "PATCH", "/api/tasks/{key}", "Update a native task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				var in tasks.UpdateRequest
				tasks.Decode(p, &in)
				return control.UpdateTask(ctx, actor, in.Key, in.Input())
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
				var in tasks.MoveRequest
				tasks.Decode(p, &in)
				return control.MoveTask(ctx, actor, in.Key, in.Input())
			}),
		taskRoute("tasks.complete", "POST", "/api/tasks/{key}/complete", "Complete a task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				var in tasks.DoneRequest
				tasks.Decode(p, &in)
				return control.CompleteTask(ctx, actor, in.Key, in.Input())
			}),
		taskRoute("tasks.comments.list", "GET", "/api/tasks/{key}/comments", "List task comments",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				detail, err := control.GetTask(ctx, actor, p.Trimmed("key"))
				return TaskCommentList{Comments: detail.Comments, Count: len(detail.Comments)}, err
			}),
		taskRoute("tasks.comments.add", "POST", "/api/tasks/{key}/comments", "Add a task comment",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				var in tasks.CommentRequest
				tasks.Decode(p, &in)
				return control.AddComment(ctx, actor, in.Key, in.Input())
			}),
		taskRoute("tasks.relations.list", "GET", "/api/tasks/{key}/relations", "List task relations",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				detail, err := control.GetTask(ctx, actor, p.Trimmed("key"))
				return TaskRelationList{Relations: detail.Relations, Count: len(detail.Relations)}, err
			}),
		taskRoute("tasks.relations.add", "POST", "/api/tasks/{key}/relations", "Add a task relation",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				var in tasks.RelateRequest
				tasks.Decode(p, &in)
				return control.AddRelation(ctx, actor, in.Key, in.Input())
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
				return TaskRelationDeleted{Deleted: true, RelationID: in.RelationID}, nil
			}),
		taskRoute("tasks.events", "GET", "/api/tasks/{key}/events", "List task events",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				events, err := control.ListEvents(ctx, actor, p.Trimmed("key"),
					p.Int64("after"), int(p.Int64("limit")))
				return TaskEventList{Events: events, Count: len(events)}, err
			}),
		taskRoute("tasks.principals", "GET", "/api/task-principals", "List task principals",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.Principals(ctx, actor)
			}),
		taskRoute("tasks.queue.pool.set", "PATCH", "/api/task-queues/{queue}/pools/{pool}", "Bind agents to a logical workflow pool",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.RebindAgentPool(ctx, actor, p.Trimmed("queue"), p.Trimmed("pool"), p.Strings("agents"), p.Int64("revision"), p.Trimmed("idempotency_key"))
			}),
		taskRoute("tasks.queue.pool.list", "GET", "/api/task-queues/{queue}/pools", "List logical workflow pools",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				items, err := control.ListAgentPools(ctx, actor, p.Trimmed("queue"))
				return TaskAgentPoolList{Items: items, Count: len(items)}, err
			}),
		taskRoute("tasks.queue.pool.get", "GET", "/api/task-queues/{queue}/pools/{pool}", "Inspect a logical workflow pool",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.GetAgentPool(ctx, actor, p.Trimmed("queue"), p.Trimmed("pool"))
			}),
		taskRoute("tasks.queue.trigger.list", "GET", "/api/task-queues/{queue}/workflow-triggers", "List queue workflow triggers",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				items, err := control.ListQueueWorkflowTriggers(ctx, actor, p.Trimmed("queue"))
				return TaskTriggerList{Items: items, Count: len(items)}, err
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
				return TaskTriggerDeleted{Deleted: true, ID: id}, nil
			}),
		taskRoute("tasks.advance", "POST", "/api/tasks/{key}/advance", "Declare an outcome for a workflow task's current status",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				var in tasks.AdvanceRequest
				tasks.Decode(p, &in)
				return control.Advance(ctx, actor, in.Key, in.Input())
			}),
		taskRoute("tasks.artifacts.set", "PUT", "/api/tasks/{key}/artifacts/{name}", "Set a workflow task artifact",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				var in tasks.ArtifactSetRequest
				tasks.Decode(p, &in)
				return control.SetArtifact(ctx, actor, in.Key, in.Name, in.Text())
			}),
		taskRoute("tasks.artifacts.list", "GET", "/api/tasks/{key}/artifacts", "List the current artifacts of a workflow task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				artifacts, err := control.ListArtifacts(ctx, actor, p.Trimmed("key"))
				return TaskArtifactList{Artifacts: artifacts, Count: len(artifacts)}, err
			}),
		taskRoute("tasks.artifacts.get", "GET", "/api/tasks/{key}/artifacts/{name}", "Show a workflow task artifact and its history",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				artifact, history, err := control.GetArtifact(ctx, actor, p.Trimmed("key"), p.Trimmed("name"))
				return TaskArtifactHistory{Artifact: artifact, History: history}, err
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
				return TaskScriptRunList{Runs: runs, Count: len(runs)}, err
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
				return TaskRunLog{RunID: id, Text: text, Truncated: truncated}, err
			}),
		taskRoute("tasks.workflow.move", "POST", "/api/tasks/{key}/workflow/move", "Move a workflow task to another status",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				var in tasks.WorkflowMoveRequest
				tasks.Decode(p, &in)
				return control.MoveWorkflow(ctx, actor, in.Key, in.To, in.Reason)
			}),
		taskRoute("tasks.workflow.upgrade", "POST", "/api/tasks/{key}/workflow/upgrade", "Restart a workflow task at the version its queue is bound to",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				var in tasks.WorkflowUpgradeRequest
				tasks.Decode(p, &in)
				return control.UpgradeWorkflow(ctx, actor, in.Key, in.Reason)
			}),
		taskRoute("tasks.workflow.resume", "POST", "/api/tasks/{key}/workflow/resume", "Resolve the pause of a workflow task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				var in tasks.ResumeRequest
				tasks.Decode(p, &in)
				return control.ResumeWorkflow(ctx, actor, in.Key, in.Decision)
			}),
		taskRoute("tasks.workflow.reassign", "POST", "/api/tasks/{key}/workflow/reassign", "Hand the pool status of a workflow task to another member",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				var in tasks.ReassignRequest
				tasks.Decode(p, &in)
				return control.ReassignWorkflow(ctx, actor, in.Key, in.Agent)
			}),
		taskRoute("tasks.cancel", "POST", "/api/tasks/{key}/cancel", "Cancel a workflow task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				return control.CancelWorkflowTask(ctx, actor, p.Trimmed("key"))
			}),
		taskRoute("tasks.requeue", "POST", "/api/tasks/{key}/requeue", "Move a task to another queue as a new task",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				var in tasks.RequeueRequest
				tasks.Decode(p, &in)
				return control.RequeueTask(ctx, actor, in.Key, in.Input())
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
				return TaskQueueWorkflowCleared{Queue: queue, Cleared: true}, nil
			}),
		withMaxBody(taskRoute("tasks.queue.secret.set", "PUT", "/api/task-queues/{queue}/secrets/{key}", "Set a queue secret for workflow scripts",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				queue, key := p.Trimmed("queue"), p.Trimmed("key")
				info, err := control.SetQueueSecretInfo(ctx, actor, queue, key, p.String("value"))
				if err != nil {
					return nil, err
				}
				return TaskQueueSecretSet{Queue: strings.ToUpper(queue), Key: key, UpdatedAt: info.UpdatedAt}, nil
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
				return TaskQueueSecretList{Secrets: items, Count: len(items), GlobalEnv: global}, err
			}),
		taskRoute("tasks.queue.source.ls", "GET", "/api/task-queues/{queue}/sources", "List the workflow sources of a queue and their recent runs",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				items, err := control.ListQueueSources(ctx, actor, p.Trimmed("queue"))
				return TaskQueueSourceList{Sources: items, Count: len(items)}, err
			}),
		taskRoute("tasks.queue.source.log", "GET", "/api/task-queues/{queue}/source-runs/{id}/log", "Read the tail of a workflow source run's log",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				id := p.Int64("id")
				maxBytes, err := tasks.ParseMaxBytes(p["max_bytes"])
				if err != nil {
					return nil, err
				}
				text, truncated, err := control.QueueSourceRunLog(ctx, actor, p.Trimmed("queue"), id, maxBytes)
				return TaskRunLog{RunID: id, Text: text, Truncated: truncated}, err
			}),
		taskRoute("tasks.queue.secret.rm", "DELETE", "/api/task-queues/{queue}/secrets/{key}", "Remove a queue secret",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				queue, key := p.Trimmed("queue"), p.Trimmed("key")
				if err := control.RemoveQueueSecret(ctx, actor, queue, key); err != nil {
					return nil, err
				}
				return TaskQueueValueRemoved{Queue: strings.ToUpper(queue), Key: key, Removed: true}, nil
			}),
		withMaxBody(taskRoute("tasks.queue.env.set", "PUT", "/api/task-queues/{queue}/env/{key}", "Set a plain queue env var for workflow scripts",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				queue, key := p.Trimmed("queue"), p.Trimmed("key")
				item, err := control.SetQueueEnvInfo(ctx, actor, queue, key, p.String("value"))
				if err != nil {
					return nil, err
				}
				return TaskQueueEnvSet{Queue: strings.ToUpper(queue), Key: key, Value: item.Value, UpdatedAt: item.UpdatedAt}, nil
			}), maxQueueSecretBody),
		taskRoute("tasks.queue.env.ls", "GET", "/api/task-queues/{queue}/env", "List the plain env vars of a queue with their values",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				items, err := control.ListQueueEnv(ctx, actor, p.Trimmed("queue"))
				return TaskQueueEnvList{Env: items, Count: len(items)}, err
			}),
		taskRoute("tasks.queue.env.rm", "DELETE", "/api/task-queues/{queue}/env/{key}", "Remove a plain queue env var",
			func(ctx context.Context, control registry.TaskControl, actor tasks.Actor, p registry.Params) (any, error) {
				queue, key := p.Trimmed("queue"), p.Trimmed("key")
				if err := control.RemoveQueueEnv(ctx, actor, queue, key); err != nil {
					return nil, err
				}
				return TaskQueueValueRemoved{Queue: strings.ToUpper(queue), Key: key, Removed: true}, nil
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
	bodyArgs := taskWireArgs(method, route)
	if bodyArgs == nil {
		bodyArgs = taskHTTPArgs(path)
	}
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
	withResult, ok := taskResultTypes[path]
	if !ok {
		panic(path + ": no result type in taskResultTypes")
	}
	return withResult(registry.Command{
		Path: path, Summary: summary, CLIHidden: true,
		Args: args,
		HTTP: &registry.HTTPRoute{Method: method, Path: route},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			if c.Tasks == nil {
				return nil, api.UserError{Status: http.StatusServiceUnavailable,
					Code: "tasks_unavailable", Msg: "native Tasks service is unavailable"}
			}
			actor := tasks.CustomerActor(c.Tasks.CustomerLogin())
			return handler(p.Ctx, c.Tasks, actor, p.Params)
		},
	})
}

// taskResultTypes declares the result type of every task route; taskRoute
// refuses a path the table lacks.
var taskResultTypes = map[string]func(registry.Command) registry.Command{
	"tasks.queue.list":              registry.WithResult[TaskQueueList],
	"tasks.queue.create":            registry.WithResult[tasks.Queue],
	"tasks.queue.get":               registry.WithResult[tasks.Queue],
	"tasks.queue.update":            registry.WithResult[tasks.Queue],
	"tasks.list":                    registry.WithResult[tasks.TaskPage],
	"tasks.create":                  registry.WithResult[tasks.Task],
	"tasks.get":                     registry.WithResult[tasks.TaskDetail],
	"tasks.update":                  registry.WithResult[tasks.Task],
	"tasks.export":                  registry.WithResult[tasks.TransferBundle],
	"tasks.import":                  registry.WithResult[tasks.Task],
	"tasks.claim":                   registry.WithResult[tasks.Task],
	"tasks.move":                    registry.WithResult[tasks.Task],
	"tasks.complete":                registry.WithResult[tasks.Task],
	"tasks.comments.list":           registry.WithResult[TaskCommentList],
	"tasks.comments.add":            registry.WithResult[tasks.CommentResult],
	"tasks.relations.list":          registry.WithResult[TaskRelationList],
	"tasks.relations.add":           registry.WithResult[tasks.Relation],
	"tasks.relations.delete":        registry.WithResult[TaskRelationDeleted],
	"tasks.events":                  registry.WithResult[TaskEventList],
	"tasks.principals":              registry.WithResult[tasks.PrincipalInfo],
	"tasks.queue.pool.set":          registry.WithResult[tasks.AgentPool],
	"tasks.queue.pool.list":         registry.WithResult[TaskAgentPoolList],
	"tasks.queue.pool.get":          registry.WithResult[tasks.AgentPool],
	"tasks.queue.trigger.list":      registry.WithResult[TaskTriggerList],
	"tasks.queue.trigger.create":    registry.WithResult[tasks.QueueWorkflowTrigger],
	"tasks.queue.trigger.delete":    registry.WithResult[TaskTriggerDeleted],
	"tasks.advance":                 registry.WithResult[tasks.TransitionRequest],
	"tasks.artifacts.set":           registry.WithResult[tasks.Artifact],
	"tasks.artifacts.list":          registry.WithResult[TaskArtifactList],
	"tasks.artifacts.get":           registry.WithResult[TaskArtifactHistory],
	"tasks.workflow.get":            registry.WithResult[tasks.WorkflowView],
	"tasks.workflow.requests.get":   registry.WithResult[tasks.TransitionRequest],
	"tasks.workflow.requests.retry": registry.WithResult[tasks.TransitionRequest],
	"tasks.workflow.runs.list":      registry.WithResult[TaskScriptRunList],
	"tasks.workflow.runs.get":       registry.WithResult[tasks.ScriptRun],
	"tasks.workflow.runs.log":       registry.WithResult[TaskRunLog],
	"tasks.workflow.move":           registry.WithResult[tasks.Task],
	"tasks.workflow.upgrade":        registry.WithResult[tasks.Task],
	"tasks.workflow.resume":         registry.WithResult[tasks.Task],
	"tasks.workflow.reassign":       registry.WithResult[tasks.Task],
	"tasks.cancel":                  registry.WithResult[tasks.Task],
	"tasks.requeue":                 registry.WithResult[tasks.Task],
	"tasks.queue.workflow.set":      registry.WithResult[tasks.QueueWorkflow],
	"tasks.queue.workflow.get":      registry.WithResult[tasks.QueueWorkflow],
	"tasks.queue.workflow.clear":    registry.WithResult[TaskQueueWorkflowCleared],
	"tasks.queue.secret.set":        registry.WithResult[TaskQueueSecretSet],
	"tasks.queue.secret.ls":         registry.WithResult[TaskQueueSecretList],
	"tasks.queue.source.ls":         registry.WithResult[TaskQueueSourceList],
	"tasks.queue.source.log":        registry.WithResult[TaskRunLog],
	"tasks.queue.secret.rm":         registry.WithResult[TaskQueueValueRemoved],
	"tasks.queue.env.set":           registry.WithResult[TaskQueueEnvSet],
	"tasks.queue.env.ls":            registry.WithResult[TaskQueueEnvList],
	"tasks.queue.env.rm":            registry.WithResult[TaskQueueValueRemoved],
}

// taskWireArgs derives the arguments of a native task route from its request
// in the tasks.Specs table, without the path parameters taskRoute adds. It
// returns nil for a route the table does not own.
// stringOrStrings is the schema of a list the CLI sends comma-separated and an
// HTTP client may send as a JSON array.
var stringOrStrings = map[string]any{"oneOf": []any{
	map[string]any{"type": "string"},
	map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
}}

func taskWireArgs(method, route string) []registry.Arg {
	spec, ok := tasks.SpecForRoute(method, route)
	if !ok {
		return nil
	}
	args := []registry.Arg{}
	for _, field := range tasks.Fields(spec.Request) {
		if !field.HTTP || strings.Contains(route, "{"+field.Name+"}") {
			continue
		}
		arg := registry.Arg{Name: field.Name, Required: field.HTTPRequired, Help: field.Help}
		switch field.Kind {
		case tasks.KindInt, tasks.KindInt64:
			arg.Type = registry.Int
		case tasks.KindBool, tasks.KindOptBool:
			arg.Type = registry.Bool
		case tasks.KindStrings:
			arg.Schema = stringOrStrings
		}
		if field.Enum != nil {
			arg.Schema = map[string]any{"type": "string", "enum": field.Enum}
		}
		args = append(args, arg)
	}
	return args
}

func taskHTTPArgs(path string) []registry.Arg {
	switch path {
	case "tasks.queue.update":
		return []registry.Arg{
			{Name: "name", Help: "Queue name"},
			{Name: "description", Help: "Queue description"},
			{Name: "owners", Help: "Owner agents, comma-separated or a list", Schema: stringOrStrings},
			{Name: "responsible_agent", Help: "Responsible agent"},
			{Name: "revision", Type: registry.Int, Required: true, Help: "Expected current revision"},
		}
	case "tasks.queue.create":
		return []registry.Arg{
			{Name: "prefix", Required: true, Help: "Queue key prefix"},
			{Name: "name", Required: true, Help: "Queue name"},
			{Name: "description", Help: "Queue description"},
			{Name: "owners", Help: "Owner agents, comma-separated or a list", Schema: stringOrStrings},
			{Name: "responsible_agent", Flag: "responsible-agent", Help: "Responsible agent"},
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
	case "tasks.queue.source.log":
		return []registry.Arg{{Name: "max_bytes", Type: registry.Int, Help: "Largest log tail to return in bytes (default 65536, maximum 1048576)"}}
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

func poolMutationArgs(primary registry.Arg) []registry.Arg {
	return []registry.Arg{primary, {Name: "revision", Type: registry.Int, Required: true, Help: "Expected current revision (zero for a new pool)"}, {Name: "idempotency_key", Required: true, Help: "Stable retry key"}}
}
