package tasks

import (
	"context"
	"strings"
	"testing"
)

func artifactWorkflowTask(t *testing.T) (*Service, Task, Assignment) {
	t.Helper()
	definition := WorkflowDefinition{
		Name: "artifact-contract", Version: 1, InitialStatus: "work",
		Statuses: []WorkflowStatus{
			{
				ID: "work",
				Requirements: []WorkflowRequirement{{
					ID: "implementation", Pool: "developers", Dispatch: DispatchClaimOne,
					Inputs:   []string{"requirements"},
					Produces: []string{"notes", "data", "patch", "revision", "link"},
					Outcomes: []string{"completed"},
				}},
				Transitions: []WorkflowTransition{{When: "implementation.completed", To: "done"}},
			},
			{ID: "done", Terminal: true},
		},
	}
	svc, _, task := runtimeWorkflowTask(t, definition, map[string][]string{
		"developers": {"dev-a"},
	})
	work, err := svc.NextWork(context.Background(), AgentActor("dev-a"), "DEV", 1)
	if err != nil || len(work) != 1 {
		t.Fatalf("next work = %#v, err=%v", work, err)
	}
	claimed, err := svc.ClaimAssignment(context.Background(), AgentActor("dev-a"), assignmentID(work[0]), ClaimAssignmentInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: work[0].Revision, IdempotencyKey: "claim-artifacts",
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, task, claimed
}

func TestAddArtifactValidatesBaseTypesAndOutputPolicy(t *testing.T) {
	svc, task, assignment := artifactWorkflowTask(t)
	ctx := context.Background()
	actor := AgentActor("dev-a")

	tests := []struct {
		name, artifactType, content, wantCode string
	}{
		{"notes", ArtifactMarkdown, "implementation notes", ""},
		{"notes", ArtifactMarkdown, strings.Repeat("x", maxArtifactMarkdownBytes+1), "artifact_too_large"},
		{"data", ArtifactJSON, `{"ok":true}`, ""},
		{"data", ArtifactJSON, `[1,2]`, "invalid_artifact_content"},
		{"patch", ArtifactFile, "changes/implementation.patch", ""},
		{"patch", ArtifactFile, "../../etc/passwd", "invalid_artifact_content"},
		{"revision", ArtifactCommit, `{"repository":"https://example.test/repo.git","ref":"abc123"}`, ""},
		{"revision", ArtifactCommit, `{"repository":"","ref":"abc123"}`, "invalid_artifact_content"},
		{"link", ArtifactURL, "https://example.test/pr/1", ""},
		{"link", ArtifactURL, "/relative/pr/1", "invalid_artifact_content"},
		{"secret", ArtifactMarkdown, "must not be accepted", "artifact_not_allowed"},
		{"notes", "binary", "opaque", "invalid_artifact_type"},
	}
	for i, tt := range tests {
		t.Run(tt.name+"/"+tt.artifactType+"/"+tt.wantCode, func(t *testing.T) {
			_, err := svc.AddArtifact(ctx, actor, assignmentID(assignment), AddArtifactInput{
				TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision,
				Name: tt.name, Type: tt.artifactType, Content: tt.content,
				IdempotencyKey: "artifact-validation-" + string(rune('a'+i)),
			})
			if ErrorCode(err) != tt.wantCode {
				t.Fatalf("AddArtifact error = %v (%q); want code %q", err, ErrorCode(err), tt.wantCode)
			}
		})
	}
}

func TestAddArtifactIsIdempotentAndListable(t *testing.T) {
	svc, task, assignment := artifactWorkflowTask(t)
	in := AddArtifactInput{
		TaskRevision: task.WorkflowRevision, AssignmentRevision: assignment.Revision,
		Name: "notes", Type: ArtifactMarkdown, Content: "first",
		Metadata: map[string]any{"format": "brief"}, IdempotencyKey: "same-artifact",
	}
	first, err := svc.AddArtifact(context.Background(), AgentActor("dev-a"), assignmentID(assignment), in)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := svc.AddArtifact(context.Background(), AgentActor("dev-a"), assignmentID(assignment), AddArtifactInput{
		TaskRevision: -1, AssignmentRevision: -1, Name: "changed", Type: ArtifactJSON,
		Content: `{"ignored":true}`, IdempotencyKey: in.IdempotencyKey,
	})
	if err != nil || replayed.ID != first.ID || replayed.Content != first.Content {
		t.Fatalf("replay = %#v, err=%v; want %#v", replayed, err, first)
	}
	got, err := svc.GetArtifact(context.Background(), CustomerActor("customer"), task.Key, "", first.ID)
	if err != nil || got.ID != first.ID || got.Metadata["format"] != "brief" {
		t.Fatalf("GetArtifact = %#v, err=%v", got, err)
	}
	list, err := svc.ListArtifacts(context.Background(), CustomerActor("customer"), task.Key, "")
	if err != nil || len(list) != 1 || list[0].ID != first.ID {
		t.Fatalf("ListArtifacts = %#v, err=%v", list, err)
	}
}
