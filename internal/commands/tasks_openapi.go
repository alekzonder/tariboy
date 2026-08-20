package commands

func schemaRef(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}
func arrayOf(name string) map[string]any {
	return map[string]any{"type": "array", "items": schemaRef(name)}
}
func stringArray() map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
}
func objectSchema(required []string, properties map[string]any) map[string]any {
	s := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}
func listSchema(item string) map[string]any {
	return objectSchema([]string{"items", "count"}, map[string]any{"items": arrayOf(item), "count": map[string]any{"type": "integer"}})
}

func taskOpenAPISchemas() map[string]map[string]any {
	str := map[string]any{"type": "string"}
	integer := map[string]any{"type": "integer"}
	boolean := map[string]any{"type": "boolean"}
	free := map[string]any{"type": "object", "additionalProperties": true}
	return map[string]map[string]any{
		"WorkflowDefinition": objectSchema([]string{"name", "version", "initial_status", "statuses"}, map[string]any{
			"name": str, "version": integer, "initial_status": str, "statuses": arrayOf("WorkflowStatus"),
			"budgets": schemaRef("WorkflowBudgetPolicy"), "timeouts": schemaRef("WorkflowTimeoutPolicy"), "retries": schemaRef("WorkflowRetryPolicy"), "questions": schemaRef("WorkflowQuestionPolicy"), "observations": schemaRef("WorkflowObservationPolicy"), "permissions": schemaRef("WorkflowPermissions"),
		}),
		"WorkflowStatus":            objectSchema([]string{"id", "requirements", "transitions"}, map[string]any{"id": str, "instructions": str, "requirements": arrayOf("WorkflowRequirement"), "transitions": arrayOf("WorkflowTransition"), "join": str, "terminal": boolean}),
		"WorkflowRequirement":       objectSchema([]string{"id", "pool", "dispatch", "inputs", "produces", "outcomes"}, map[string]any{"id": str, "pool": str, "dispatch": str, "inputs": stringArray(), "produces": stringArray(), "outcomes": stringArray(), "optional": boolean}),
		"WorkflowTransition":        objectSchema([]string{"when", "to"}, map[string]any{"when": str, "to": str}),
		"WorkflowBudgetPolicy":      objectSchema(nil, map[string]any{"max_cycles": integer, "max_assignments": integer, "on_exhausted": str}),
		"WorkflowTimeoutPolicy":     objectSchema(nil, map[string]any{"assignment": str, "question": str, "on_timeout": str}),
		"WorkflowRetryPolicy":       objectSchema(nil, map[string]any{"max_attempts": integer, "backoff": str, "on_exhausted": str}),
		"WorkflowQuestionPolicy":    objectSchema(nil, map[string]any{"route_to": str, "allowed_holds": stringArray(), "max_open_per_assignment": integer, "timeout": str}),
		"WorkflowObservationPolicy": objectSchema(nil, map[string]any{"on_late_event": str, "allowed_reactions": stringArray()}),
		"WorkflowPermissions":       objectSchema(nil, map[string]any{"tools": stringArray(), "channels": objectSchema(nil, map[string]any{"subscribe": stringArray(), "reactions": stringArray()})}),
		"WorkflowVersion":           objectSchema([]string{"id", "name", "version", "state", "definition", "created_at", "updated_at"}, map[string]any{"id": integer, "name": str, "version": integer, "state": str, "definition": schemaRef("WorkflowDefinition"), "created_at": str, "updated_at": str, "published_at": str}),
		"WorkflowValidationError":   objectSchema([]string{"code", "message"}, map[string]any{"path": str, "code": str, "message": str}),
		"WorkflowValidationResult":  objectSchema([]string{"items", "count", "valid"}, map[string]any{"items": arrayOf("WorkflowValidationError"), "count": integer, "valid": boolean}),
		"QueueWorkflowBinding":      objectSchema([]string{"queue", "workflow_version_id", "revision", "bound_by", "bound_at"}, map[string]any{"queue": str, "workflow_version_id": integer, "workflow_name": str, "workflow_version": integer, "revision": integer, "bound_by": str, "bound_at": str}),
		"AgentPool":                 objectSchema([]string{"id", "queue", "name", "agents", "revision", "created_at", "updated_at"}, map[string]any{"id": integer, "queue": str, "name": str, "agents": stringArray(), "revision": integer, "created_at": str, "updated_at": str}),
		"Task":                      objectSchema([]string{"key", "queue", "title", "status", "revision"}, map[string]any{"key": str, "queue": str, "title": str, "description": str, "status": str, "revision": integer, "workflow_version_id": integer, "workflow_status": str, "workflow_revision": integer}),
		"StatusExecution":           objectSchema([]string{"id", "task_key", "workflow_version_id", "status", "sequence", "state", "task_revision", "created_at"}, map[string]any{"id": integer, "task_key": str, "workflow_version_id": integer, "status": str, "sequence": integer, "state": str, "transition_to": str, "task_revision": integer, "created_at": str, "completed_at": str}),
		"RequirementExecution":      objectSchema([]string{"id", "status_execution_id", "requirement_id", "pool", "dispatch", "optional", "pool_snapshot", "inputs", "produces", "outcomes", "state", "created_at"}, map[string]any{"id": integer, "status_execution_id": integer, "requirement_id": str, "pool": str, "dispatch": str, "optional": boolean, "pool_snapshot": stringArray(), "inputs": stringArray(), "produces": stringArray(), "outcomes": stringArray(), "state": str, "created_at": str, "completed_at": str}),
		"Assignment":                objectSchema([]string{"id", "requirement_execution_id", "attempt", "state", "revision", "created_at", "updated_at"}, map[string]any{"id": integer, "requirement_execution_id": integer, "agent": str, "attempt": integer, "state": str, "lease_owner": str, "lease_expires_at": str, "revision": integer, "outcome": str, "created_at": str, "updated_at": str, "completed_at": str}),
		"Artifact":                  objectSchema([]string{"id", "task_key", "name", "type", "revision", "created_by", "created_at", "updated_at"}, map[string]any{"id": integer, "task_key": str, "assignment_id": integer, "name": str, "type": str, "content": str, "metadata": free, "revision": integer, "created_by": str, "created_at": str, "updated_at": str}),
		"WorkflowQuestion":          objectSchema([]string{"id", "task_key", "question", "context", "blocking_scope", "state", "created_at"}, map[string]any{"id": integer, "task_key": str, "assignment_id": integer, "requirement_execution_id": integer, "question": str, "context": str, "blocking_scope": str, "anchor": str, "options": stringArray(), "suggested_answer": str, "artifact_attachments": map[string]any{"type": "array", "items": integer}, "state": str, "deadline_at": str, "answer": str, "answered_by": str, "created_at": str, "answered_at": str}),
		"WorkflowHold":              objectSchema([]string{"id", "task_key", "scope", "created_at"}, map[string]any{"id": integer, "task_key": str, "assignment_id": integer, "requirement_execution_id": integer, "question_id": integer, "scope": str, "reason": str, "created_at": str, "released_at": str}),
		"WorkflowObservation":       objectSchema([]string{"id", "task_key", "kind", "observed_at"}, map[string]any{"id": integer, "task_key": str, "subscription_id": integer, "assignment_id": integer, "kind": str, "payload": free, "observed_at": str}),
		"WorkflowSubscription":      objectSchema([]string{"id", "task_key", "pattern", "reaction", "state", "created_by", "created_at"}, map[string]any{"id": integer, "task_key": str, "assignment_id": integer, "pattern": str, "correlation_key": str, "reaction": str, "state": str, "created_by": str, "created_at": str, "cancelled_at": str}),
		"QueueWorkflowTrigger":      objectSchema([]string{"id", "queue", "pattern", "action", "enabled", "created_by", "created_at", "updated_at"}, map[string]any{"id": integer, "queue": str, "pattern": str, "correlation_key": str, "action": str, "enabled": boolean, "created_by": str, "created_at": str, "updated_at": str}),
		"WorkPacket":                objectSchema([]string{"task_key", "task_revision", "assignment", "requirement"}, map[string]any{"task_key": str, "task_revision": integer, "goal": str, "status": str, "status_instructions": str, "assignment": schemaRef("Assignment"), "requirement": schemaRef("WorkflowRequirement"), "inputs": arrayOf("Artifact"), "allowed_outcomes": stringArray(), "allowed_actions": stringArray(), "allowed_tools": stringArray(), "allowed_channel_subscriptions": stringArray(), "questions": arrayOf("WorkflowQuestion"), "holds": arrayOf("WorkflowHold"), "observations": arrayOf("WorkflowObservation"), "subscriptions": arrayOf("WorkflowSubscription")}),
		"WorkflowExecutionView":     objectSchema([]string{"task", "workflow", "status_executions", "requirement_executions", "assignments", "holds", "observations"}, map[string]any{"task": schemaRef("Task"), "workflow": schemaRef("WorkflowVersion"), "status_executions": arrayOf("StatusExecution"), "requirement_executions": arrayOf("RequirementExecution"), "assignments": arrayOf("Assignment"), "holds": arrayOf("WorkflowHold"), "observations": arrayOf("WorkflowObservation")}),
		"TaskEvent":                 objectSchema([]string{"sequence", "event_id", "queue", "kind", "actor", "task_revision", "payload", "created_at"}, map[string]any{"sequence": integer, "event_id": str, "task_key": str, "queue": str, "kind": str, "actor": str, "task_revision": integer, "payload": free, "created_at": str}),
	}
}
