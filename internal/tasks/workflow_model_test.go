package tasks

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestWorkflowModelTypedJSONRoundTrip(t *testing.T) {
	definition := WorkflowDefinition{
		Name:          "development",
		Version:       2,
		InitialStatus: "implement",
		Statuses: []WorkflowStatus{{
			ID:           "implement",
			Instructions: "implement the approved plan",
			Requirements: []WorkflowRequirement{{
				ID:       "implementation",
				Pool:     "developers",
				Dispatch: DispatchClaimOne,
				Inputs:   []string{"requirements"},
				Produces: []string{"implementation"},
				Outcomes: []string{"completed"},
				Optional: true,
			}},
			Transitions: []WorkflowTransition{{When: "implementation.completed", To: "verify"}},
			Join:        "require_all",
			Terminal:    true,
		}},
		Budgets:  WorkflowBudgetPolicy{MaxCycles: 4, MaxAssignments: 12, OnExhausted: "failed"},
		Timeouts: WorkflowTimeoutPolicy{Assignment: "30m", Question: "15m", OnTimeout: "escalate"},
		Retries:  WorkflowRetryPolicy{MaxAttempts: 3, Backoff: "exponential", OnExhausted: "failed"},
		Questions: WorkflowQuestionPolicy{
			RouteTo: "managers", AllowedHolds: []string{HoldAssignment, HoldRequirement},
			MaxOpenPerAssignment: 2, Timeout: "30m",
		},
		Observations: WorkflowObservationPolicy{OnLateEvent: "record_only", AllowedReactions: []string{"wake_current"}},
		Permissions: WorkflowPermissions{
			Tools: []string{"tasks.artifacts.add", "git.status"},
			Channels: WorkflowChannelPermissions{
				Subscribe: []string{"logs:${task.artifacts.service_id}"},
				Reactions: []string{"record_only", "wake_current"},
			},
		},
	}
	tests := []struct {
		name string
		want any
		into func() any
	}{
		{
			name: "workflow definition",
			want: definition,
			into: func() any { return &WorkflowDefinition{} },
		},
		{
			name: "workflow version",
			want: WorkflowVersion{
				ID: 1, Name: "development", Version: 2, State: "published",
				Definition: definition, CreatedAt: "2026-08-07T00:00:00Z",
				UpdatedAt: "2026-08-07T00:01:00Z", PublishedAt: "2026-08-07T00:02:00Z",
			},
			into: func() any { return &WorkflowVersion{} },
		},
		{
			name: "workflow status",
			want: definition.Statuses[0],
			into: func() any { return &WorkflowStatus{} },
		},
		{
			name: "workflow requirement",
			want: definition.Statuses[0].Requirements[0],
			into: func() any { return &WorkflowRequirement{} },
		},
		{
			name: "workflow transition",
			want: WorkflowTransition{When: "review.approved", To: "done"},
			into: func() any { return &WorkflowTransition{} },
		},
		{
			name: "workflow budget policy",
			want: definition.Budgets,
			into: func() any { return &WorkflowBudgetPolicy{} },
		},
		{
			name: "workflow timeout policy",
			want: definition.Timeouts,
			into: func() any { return &WorkflowTimeoutPolicy{} },
		},
		{
			name: "workflow retry policy",
			want: definition.Retries,
			into: func() any { return &WorkflowRetryPolicy{} },
		},
		{
			name: "workflow question policy",
			want: definition.Questions,
			into: func() any { return &WorkflowQuestionPolicy{} },
		},
		{
			name: "workflow observation policy",
			want: definition.Observations,
			into: func() any { return &WorkflowObservationPolicy{} },
		},
		{
			name: "workflow permissions",
			want: definition.Permissions,
			into: func() any { return &WorkflowPermissions{} },
		},
		{
			name: "workflow channel permissions",
			want: definition.Permissions.Channels,
			into: func() any { return &WorkflowChannelPermissions{} },
		},
		{
			name: "queue workflow binding",
			want: QueueWorkflowBinding{Queue: "DEV", WorkflowVersionID: 1, WorkflowName: "development", WorkflowVersion: 2, Revision: 3, BoundBy: "user:operator", BoundAt: "2026-08-07T00:00:00Z"},
			into: func() any { return &QueueWorkflowBinding{} },
		},
		{
			name: "agent pool",
			want: AgentPool{ID: 1, Queue: "DEV", Name: "reviewers", Agents: []string{"reviewer-1"}, Revision: 2, CreatedAt: "2026-08-07T00:00:00Z", UpdatedAt: "2026-08-07T00:01:00Z"},
			into: func() any { return &AgentPool{} },
		},
		{
			name: "status execution",
			want: StatusExecution{ID: 1, TaskKey: "DEV-1", WorkflowVersionID: 2, Status: "verify", Sequence: 2, State: "transitioned", TransitionTo: "done", TaskRevision: 3, CreatedAt: "2026-08-07T00:00:00Z", CompletedAt: "2026-08-07T00:01:00Z"},
			into: func() any { return &StatusExecution{} },
		},
		{
			name: "requirement execution",
			want: RequirementExecution{ID: 1, StatusExecutionID: 2, RequirementID: "review", Pool: "reviewers", Dispatch: DispatchRequireAll, Optional: true, PoolSnapshot: []string{"reviewer-1"}, Inputs: []string{"implementation"}, Produces: []string{"review"}, Outcomes: []string{"approved"}, State: "completed", CreatedAt: "2026-08-07T00:00:00Z", CompletedAt: "2026-08-07T00:01:00Z"},
			into: func() any { return &RequirementExecution{} },
		},
		{
			name: "assignment",
			want: Assignment{ID: 1, RequirementExecutionID: 2, Agent: "reviewer-1", Attempt: 1, State: AssignmentLeased, LeaseOwner: "agent:reviewer-1", LeaseExpiresAt: "2026-08-07T00:30:00Z", Revision: 4, Outcome: "approved", CreatedAt: "2026-08-07T00:00:00Z", UpdatedAt: "2026-08-07T00:01:00Z", CompletedAt: "2026-08-07T00:02:00Z"},
			into: func() any { return &Assignment{} },
		},
		{
			name: "artifact",
			want: Artifact{ID: 1, TaskKey: "DEV-1", AssignmentID: 2, Name: "review", Type: ArtifactMarkdown, Content: "approved", Metadata: map[string]any{"commit": "abc"}, Revision: 3, CreatedBy: "agent:reviewer-1", CreatedAt: "2026-08-07T00:00:00Z", UpdatedAt: "2026-08-07T00:01:00Z"},
			into: func() any { return &Artifact{} },
		},
		{
			name: "workflow question",
			want: WorkflowQuestion{ID: 1, TaskKey: "DEV-1", AssignmentID: 2, RequirementExecutionID: 3, Question: "Which limit?", Context: "needed", BlockingScope: HoldAssignment, Anchor: "retry", Options: []string{"3", "5"}, SuggestedAnswer: "3", Answer: "3", AnsweredBy: "agent:manager-1", CreatedAt: "2026-08-07T00:00:00Z", AnsweredAt: "2026-08-07T00:01:00Z"},
			into: func() any { return &WorkflowQuestion{} },
		},
		{
			name: "workflow hold",
			want: WorkflowHold{ID: 1, TaskKey: "DEV-1", AssignmentID: 2, RequirementExecutionID: 3, QuestionID: 4, Scope: HoldAssignment, Reason: "awaiting answer", CreatedAt: "2026-08-07T00:00:00Z", ReleasedAt: "2026-08-07T00:01:00Z"},
			into: func() any { return &WorkflowHold{} },
		},
		{
			name: "workflow observation",
			want: WorkflowObservation{ID: 1, TaskKey: "DEV-1", SubscriptionID: 2, AssignmentID: 3, Kind: "observation.appended", Payload: map[string]any{"level": "warn"}, ObservedAt: "2026-08-07T00:00:00Z"},
			into: func() any { return &WorkflowObservation{} },
		},
		{
			name: "workflow subscription",
			want: WorkflowSubscription{ID: 1, TaskKey: "DEV-1", AssignmentID: 2, Pattern: "logs:service", CorrelationKey: "request-1", Reaction: "wake_current", State: "active", CreatedBy: "agent:developer-1", CreatedAt: "2026-08-07T00:00:00Z", CancelledAt: "2026-08-07T00:01:00Z"},
			into: func() any { return &WorkflowSubscription{} },
		},
		{
			name: "work packet",
			want: WorkPacket{TaskKey: "DEV-1", TaskRevision: 3, Goal: "ship workflow", Status: "verify", StatusInstructions: "review code", Assignment: Assignment{ID: 2}, Requirement: WorkflowRequirement{ID: "review"}, Inputs: []Artifact{{ID: 1, Name: "implementation"}}, AllowedOutcomes: []string{"approved"}, AllowedActions: []string{"complete", "release"}, AllowedTools: []string{"tasks.artifacts.add"}, AllowedChannelSubscriptions: []string{"logs:${task.artifacts.service_id}"}, Questions: []WorkflowQuestion{{ID: 1}}, Observations: []WorkflowObservation{{ID: 1}}, Subscriptions: []WorkflowSubscription{{ID: 1}}},
			into: func() any { return &WorkPacket{} },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.want)
			if err != nil {
				t.Fatal(err)
			}
			got := tt.into()
			if err := json.Unmarshal(data, got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(tt.want, reflect.ValueOf(got).Elem().Interface()) {
				t.Fatalf("typed JSON round trip = %#v; want %#v", got, tt.want)
			}
		})
	}
}

func TestWorkflowModelConstants(t *testing.T) {
	if DispatchClaimOne != "claim_one" || DispatchRequireAll != "require_all" {
		t.Fatalf("dispatch constants = %q, %q", DispatchClaimOne, DispatchRequireAll)
	}
	if AssignmentClaimable != "claimable" || AssignmentLeased != "leased" ||
		AssignmentCompleted != "completed" || AssignmentReleased != "released" ||
		AssignmentExpired != "expired" || AssignmentFailed != "failed" {
		t.Fatal("assignment state constants do not match the workflow contract")
	}
	if HoldNone != "none" || HoldAssignment != "assignment" || HoldRequirement != "requirement" {
		t.Fatal("hold scope constants do not match the workflow contract")
	}
	if ArtifactMarkdown != "markdown" || ArtifactJSON != "json" || ArtifactFile != "file" ||
		ArtifactCommit != "commit" || ArtifactURL != "url" {
		t.Fatal("artifact type constants do not match the workflow contract")
	}
}
