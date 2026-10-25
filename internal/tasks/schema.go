package tasks

import (
	"github.com/alekzonder/tariboy/internal/workflowfile"
	"github.com/invopop/jsonschema"
)

// The JSONSchemaExtend methods below complete the OpenAPI schemas the command
// registry derives from these types: the closed value sets of string fields.
// A field shared by two types with different value sets, such as a run's
// state, is set per type rather than on its named string type.

// setEnum sets the allowed values of a string property of s.
func setEnum[T ~string](s *jsonschema.Schema, property string, values ...T) {
	prop, ok := s.Properties.Get(property)
	if !ok {
		panic("schema has no property " + property)
	}
	prop.Enum = make([]any, len(values))
	for i, value := range values {
		prop.Enum[i] = string(value)
	}
}

// categories are the stored statuses every task reports as its category.
var categories = []string{StatusOpen, StatusInProgress, StatusWaitCustomer, StatusDone, StatusCancelled}

// JSONSchemaExtend lists the categories and what a task may wait on.
func (Task) JSONSchemaExtend(s *jsonschema.Schema) {
	setEnum(s, "category", categories...)
	setEnum(s, "waiting_on", WaitingOnCustomer, WaitingOnScript, WaitingOnPause)
}

// JSONSchemaExtend lists the attention kinds, the most urgent first.
func (WorkflowAttention) JSONSchemaExtend(s *jsonschema.Schema) {
	setEnum(s, "kind", AttentionPaused, AttentionFailed, AttentionRejected, AttentionScriptFailed, AttentionIdle)
}

// JSONSchemaExtend lists the run modes of a check.
func (CheckView) JSONSchemaExtend(s *jsonschema.Schema) {
	setEnum(s, "run_as", workflowfile.RunAsQueue, workflowfile.RunAsAgent)
}

// JSONSchemaExtend lists the states of a transition request.
func (TransitionRequest) JSONSchemaExtend(s *jsonschema.Schema) {
	setEnum(s, "state", RequestStatePending, RequestStateApplied, RequestStateRejected, RequestStateFailed, "cancelled")
}

// JSONSchemaExtend lists the categories of the task's status.
func (WorkflowView) JSONSchemaExtend(s *jsonschema.Schema) {
	setEnum(s, "category", categories...)
}

// JSONSchemaExtend lists the kinds, run modes, states and verdicts of a
// workflow script run.
func (ScriptRun) JSONSchemaExtend(s *jsonschema.Schema) {
	setEnum(s, "kind", RunKindCheck, RunKindWatch)
	setEnum(s, "run_as", workflowfile.RunAsQueue, workflowfile.RunAsAgent)
	setEnum(s, "state", RunStatePending, RunStateRunning, RunStateFinished, RunStateInterrupted, RunStateCancelled)
	setEnum(s, "verdict", VerdictPass, VerdictReject, VerdictOutcome, VerdictQuiet, VerdictFailure)
}

// JSONSchemaExtend lists the states and verdicts of a source run, which is
// never pending.
func (SourceRun) JSONSchemaExtend(s *jsonschema.Schema) {
	setEnum(s, "state", RunStateRunning, RunStateFinished, RunStateInterrupted, RunStateCancelled)
	setEnum(s, "verdict", VerdictItems, VerdictQuiet, VerdictFailure)
}

// JSONSchemaExtend marks the payload as a free-form object: its keys depend
// on the event kind.
func (Event) JSONSchemaExtend(s *jsonschema.Schema) {
	payload, ok := s.Properties.Get("payload")
	if !ok {
		panic("schema has no property payload")
	}
	payload.AdditionalProperties = jsonschema.TrueSchema
}
