package tasks

import "testing"

func validWorkflowDefinition() WorkflowDefinition {
	return WorkflowDefinition{
		Name:          "development",
		Version:       1,
		InitialStatus: "implement",
		Statuses: []WorkflowStatus{
			{
				ID: "implement",
				Requirements: []WorkflowRequirement{{
					ID: "implementation", Pool: "developers", Dispatch: DispatchClaimOne,
					Inputs: []string{"requirements"}, Produces: []string{"implementation"},
					Outcomes: []string{"completed"},
				}},
				Transitions: []WorkflowTransition{{When: "implementation.completed", To: "verify"}},
			},
			{
				ID: "verify", Join: "require_all",
				Requirements: []WorkflowRequirement{{
					ID: "qa", Pool: "reviewers", Dispatch: DispatchRequireAll,
					Inputs: []string{"implementation"}, Produces: []string{"test_report"},
					Outcomes: []string{"passed", "failed"},
				}},
				Transitions: []WorkflowTransition{
					{When: "(qa.all(passed) && artifact.implementation.exists)", To: "done"},
					{When: "qa.any(failed) || (task.priority == \"P0\")", To: "failed"},
				},
			},
			{ID: "done", Terminal: true},
			{ID: "failed", Terminal: true},
		},
		Questions: WorkflowQuestionPolicy{RouteTo: "developers"},
		Permissions: WorkflowPermissions{Channels: WorkflowChannelPermissions{
			Subscribe: []string{"logs:${task.artifacts.implementation}", "metrics:*", "group:dev:inbox"},
		}},
	}
}

func assertValidationCode(t *testing.T, got []WorkflowValidationError, code string) {
	t.Helper()
	for _, validationErr := range got {
		if validationErr.Code == code {
			return
		}
	}
	t.Fatalf("validation errors = %#v; want code %q", got, code)
}

func TestValidateWorkflowAcceptsSupportedGraphAndGuards(t *testing.T) {
	if got := ValidateWorkflow(validWorkflowDefinition()); len(got) != 0 {
		t.Fatalf("validation errors = %#v; want none", got)
	}
}

func TestCanonicalWorkflowDefinitionMatchesPersistenceNormalization(t *testing.T) {
	definition := validWorkflowDefinition()
	definition.Name = "  development  "
	definition.InitialStatus = " implement "
	definition.Statuses[0].Requirements[0].Pool = " developers "
	got := CanonicalWorkflowDefinition(definition)
	if got.Name != "development" || got.InitialStatus != "implement" || got.Statuses[0].Requirements[0].Pool != "developers" {
		t.Fatalf("canonical definition = %#v", got)
	}
}

func TestValidateWorkflowRejectsMalformedDefinitions(t *testing.T) {
	tests := []struct {
		name string
		code string
		edit func(*WorkflowDefinition)
	}{
		{
			name: "missing status id", code: "missing_status_id",
			edit: func(def *WorkflowDefinition) { def.Statuses[1].ID = "" },
		},
		{
			name: "duplicate status id", code: "duplicate_status_id",
			edit: func(def *WorkflowDefinition) { def.Statuses[1].ID = "implement" },
		},
		{
			name: "missing initial status", code: "missing_initial_status",
			edit: func(def *WorkflowDefinition) { def.InitialStatus = "" },
		},
		{
			name: "multiple initial statuses", code: "multiple_initial_status",
			edit: func(def *WorkflowDefinition) { def.Statuses[1].ID = "implement" },
		},
		{
			name: "no reachable terminal", code: "no_reachable_terminal",
			edit: func(def *WorkflowDefinition) {
				def.Statuses[1].Transitions = []WorkflowTransition{{When: "qa.all(passed)", To: "implement"}}
			},
		},
		{
			name: "unknown pool", code: "unknown_pool",
			edit: func(def *WorkflowDefinition) { def.Questions.RouteTo = "managers" },
		},
		{
			name: "unknown artifact", code: "unknown_artifact",
			edit: func(def *WorkflowDefinition) {
				def.Statuses[1].Transitions[0].When = "artifact.missing.exists"
			},
		},
		{
			name: "unknown transition status", code: "unknown_transition_status",
			edit: func(def *WorkflowDefinition) { def.Statuses[0].Transitions[0].To = "missing" },
		},
		{
			name: "invalid dispatch", code: "invalid_dispatch",
			edit: func(def *WorkflowDefinition) { def.Statuses[0].Requirements[0].Dispatch = "broadcast" },
		},
		{
			name: "reserved question requirement", code: "reserved_requirement_id",
			edit: func(def *WorkflowDefinition) { def.Statuses[0].Requirements[0].ID = "__question:forged" },
		},
		{
			name: "reserved observation requirement", code: "reserved_requirement_id",
			edit: func(def *WorkflowDefinition) { def.Statuses[0].Requirements[0].ID = "__observation:forged" },
		},
		{
			name: "invalid join", code: "invalid_join",
			edit: func(def *WorkflowDefinition) { def.Statuses[1].Join = "require_any" },
		},
		{
			name: "invalid observation reaction", code: "invalid_observation_reaction",
			edit: func(def *WorkflowDefinition) { def.Observations.AllowedReactions = []string{"run_script"} },
		},
		{
			name: "invalid late observation reaction", code: "invalid_late_observation_reaction",
			edit: func(def *WorkflowDefinition) { def.Observations.OnLateEvent = "wake_current" },
		},
		{
			name: "empty nonterminal status", code: "empty_nonterminal_status",
			edit: func(def *WorkflowDefinition) { def.Statuses[0].Requirements = nil },
		},
		{
			name: "terminal status requirements", code: "terminal_status_requirements",
			edit: func(def *WorkflowDefinition) {
				def.Statuses[2].Requirements = []WorkflowRequirement{{
					ID: "terminal-work", Pool: "developers", Dispatch: DispatchClaimOne,
					Outcomes: []string{"completed"},
				}}
			},
		},
		{
			name: "terminal status transitions", code: "terminal_status_transitions",
			edit: func(def *WorkflowDefinition) {
				def.Statuses[2].Transitions = []WorkflowTransition{{To: "failed"}}
			},
		},
		{
			name: "terminal initial status", code: "initial_status_terminal",
			edit: func(def *WorkflowDefinition) { def.InitialStatus = "done" },
		},
		{
			name: "unsupported retry backoff", code: "unsupported_retry_backoff",
			edit: func(def *WorkflowDefinition) { def.Retries.Backoff = "exponential" },
		},
		{
			name: "unsupported timeout action", code: "unsupported_timeout_action",
			edit: func(def *WorkflowDefinition) { def.Timeouts.OnTimeout = "escalate" },
		},
		{
			name: "invalid outcome", code: "invalid_outcome",
			edit: func(def *WorkflowDefinition) { def.Statuses[0].Requirements[0].Outcomes = []string{""} },
		},
		{
			name: "unreachable status", code: "unreachable_status",
			edit: func(def *WorkflowDefinition) {
				def.Statuses = append(def.Statuses, WorkflowStatus{ID: "orphan", Terminal: true})
			},
		},
		{
			name: "ambiguous unconditional transitions", code: "ambiguous_unconditional_transitions",
			edit: func(def *WorkflowDefinition) {
				def.Statuses[0].Transitions = []WorkflowTransition{{To: "done"}, {To: "failed"}}
			},
		},
		{
			name: "mixed unconditional and conditional transitions", code: "ambiguous_unconditional_transitions",
			edit: func(def *WorkflowDefinition) {
				def.Statuses[0].Transitions = append(def.Statuses[0].Transitions, WorkflowTransition{To: "failed"})
			},
		},
		{
			name: "invalid channel pattern", code: "invalid_channel_pattern",
			edit: func(def *WorkflowDefinition) {
				def.Permissions.Channels.Subscribe = []string{"Chat:Room"}
			},
		},
		{
			name: "unsupported guard token", code: "unsupported_guard_token",
			edit: func(def *WorkflowDefinition) {
				def.Statuses[0].Transitions[0].When = "shell(\"rm -rf /\")"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := validWorkflowDefinition()
			tt.edit(&def)
			assertValidationCode(t, ValidateWorkflow(def), tt.code)
		})
	}
}

func TestValidateWorkflowAcceptsImplementedImmediateRetryPolicy(t *testing.T) {
	definition := validWorkflowDefinition()
	definition.Retries.Backoff = "immediate"
	definition.Timeouts.OnTimeout = "retry"
	if got := ValidateWorkflow(definition); len(got) != 0 {
		t.Fatalf("implemented runtime policy validation errors = %#v; want none", got)
	}
}

func TestValidateWorkflowRejectsUnknownDestination(t *testing.T) {
	def := validWorkflowDefinition()
	def.Statuses[0].Transitions[0].To = "missing"
	got := ValidateWorkflow(def)
	assertValidationCode(t, got, "unknown_transition_status")
}
