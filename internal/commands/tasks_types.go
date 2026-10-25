package commands

import "github.com/alekzonder/tariboy/internal/tasks"

// Response types of the tasks.go commands that wrap the native Tasks values.
// Each command declares its type with registry.WithResult through
// taskResultTypes, which derives the OpenAPI result schema from it.

// TaskQueueList is the queues the reader may see (tasks.queue.list).
type TaskQueueList struct {
	Count  int           `json:"count"`
	Queues []tasks.Queue `json:"queues"`
}

// TaskCommentList is the comments of one task (tasks.comments.list).
type TaskCommentList struct {
	Comments []tasks.Comment `json:"comments"`
	Count    int             `json:"count"`
}

// TaskRelationList is the relations of one task (tasks.relations.list).
type TaskRelationList struct {
	Count     int              `json:"count"`
	Relations []tasks.Relation `json:"relations"`
}

// TaskRelationDeleted confirms a deleted task relation (tasks.relations.delete).
type TaskRelationDeleted struct {
	Deleted    bool  `json:"deleted"`
	RelationID int64 `json:"relation_id"`
}

// TaskEventList is a page of one task's events (tasks.events).
type TaskEventList struct {
	Count  int           `json:"count"`
	Events []tasks.Event `json:"events"`
}

// TaskNotificationList is the customer's task notifications
// (tasks.notifications.list).
type TaskNotificationList struct {
	Count         int                  `json:"count"`
	Notifications []tasks.Notification `json:"notifications"`
}

// TaskAgentPoolList is the logical workflow pools of a queue
// (tasks.queue.pool.list).
type TaskAgentPoolList struct {
	Count int               `json:"count"`
	Items []tasks.AgentPool `json:"items"`
}

// TaskTriggerList is the workflow triggers of a queue
// (tasks.queue.trigger.list).
type TaskTriggerList struct {
	Count int                          `json:"count"`
	Items []tasks.QueueWorkflowTrigger `json:"items"`
}

// TaskTriggerDeleted confirms a deleted queue workflow trigger
// (tasks.queue.trigger.delete).
type TaskTriggerDeleted struct {
	Deleted bool  `json:"deleted"`
	ID      int64 `json:"id"`
}

// TaskArtifactList is the current artifacts of a workflow task
// (tasks.artifacts.list).
type TaskArtifactList struct {
	Artifacts []tasks.Artifact `json:"artifacts"`
	Count     int              `json:"count"`
}

// TaskArtifactHistory is one artifact of a workflow task and its earlier
// versions (tasks.artifacts.get).
type TaskArtifactHistory struct {
	Artifact tasks.Artifact   `json:"artifact"`
	History  []tasks.Artifact `json:"history"`
}

// TaskScriptRunList is the script runs of a workflow task
// (tasks.workflow.runs.list).
type TaskScriptRunList struct {
	Count int               `json:"count"`
	Runs  []tasks.ScriptRun `json:"runs"`
}

// TaskRunLog is the tail of a script or source run's log
// (tasks.workflow.runs.log, tasks.queue.source.log).
type TaskRunLog struct {
	RunID     int64  `json:"run_id"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}

// TaskQueueWorkflowCleared confirms a removed queue workflow binding
// (tasks.queue.workflow.clear).
type TaskQueueWorkflowCleared struct {
	Cleared bool   `json:"cleared"`
	Queue   string `json:"queue"`
}

// TaskQueueSecretSet confirms a stored queue secret without its value
// (tasks.queue.secret.set).
type TaskQueueSecretSet struct {
	Key       string `json:"key"`
	Queue     string `json:"queue"`
	UpdatedAt string `json:"updated_at"`
}

// TaskQueueSecretList is the secret keys of a queue and the names of the
// required secrets the global environment holds (tasks.queue.secret.ls).
type TaskQueueSecretList struct {
	Count     int                     `json:"count"`
	GlobalEnv []string                `json:"global_env"`
	Secrets   []tasks.QueueSecretInfo `json:"secrets"`
}

// TaskQueueValueRemoved confirms a removed queue secret or env var
// (tasks.queue.secret.rm, tasks.queue.env.rm).
type TaskQueueValueRemoved struct {
	Key     string `json:"key"`
	Queue   string `json:"queue"`
	Removed bool   `json:"removed"`
}

// TaskQueueEnvSet is a stored plain queue env var (tasks.queue.env.set).
type TaskQueueEnvSet struct {
	Key       string `json:"key"`
	Queue     string `json:"queue"`
	UpdatedAt string `json:"updated_at"`
	Value     string `json:"value"`
}

// TaskQueueEnvList is the plain env vars of a queue (tasks.queue.env.ls).
type TaskQueueEnvList struct {
	Count int                 `json:"count"`
	Env   []tasks.QueueEnvVar `json:"env"`
}

// TaskQueueSourceList is the workflow sources of a queue
// (tasks.queue.source.ls).
type TaskQueueSourceList struct {
	Count   int                 `json:"count"`
	Sources []tasks.QueueSource `json:"sources"`
}
