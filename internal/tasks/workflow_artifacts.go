package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"
)

const maxArtifactMarkdownBytes = 64 * 1024

type AddArtifactInput struct {
	TaskRevision       int64          `json:"task_revision"`
	AssignmentRevision int64          `json:"assignment_revision"`
	Name               string         `json:"name"`
	Type               string         `json:"type"`
	Content            string         `json:"content"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	IdempotencyKey     string         `json:"idempotency_key"`
}

type artifactReadScope struct {
	assignmentID int64
	inputs       map[string]bool
	outputs      map[string]bool
}

// AddArtifact stores a validated output of the caller's active assignment.
// Artifact content is data only: file values are references and are never read.
func (s *Service) AddArtifact(ctx context.Context, actor Actor, assignmentID string, in AddArtifactInput) (Artifact, error) {
	if err := requireWorkflowAgent(actor); err != nil {
		return Artifact{}, err
	}
	if err := requireIdempotencyKey(in.IdempotencyKey); err != nil {
		return Artifact{}, err
	}
	id, err := parseAssignmentID(assignmentID)
	if err != nil {
		return Artifact{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Artifact{}, err
	}
	defer tx.Rollback()
	if replay, ok, err := readTaskIdempotency[Artifact](ctx, tx, actor.Principal, "add_workflow_artifact", in.IdempotencyKey); err != nil {
		return Artifact{}, err
	} else if ok {
		return replay, nil
	}
	current, err := assignmentContextByID(ctx, tx, id)
	if err != nil {
		return Artifact{}, err
	}
	if err := requireOwnedActiveLease(actor, current, s.clock().UTC()); err != nil {
		return Artifact{}, err
	}
	if err := requireArtifactRevisions(ctx, tx, actor, current, in.TaskRevision, in.AssignmentRevision); err != nil {
		return Artifact{}, err
	}
	requirement, ok := workflowRequirementByID(current.Workflow.Definition, current.Task.WorkflowStatus, current.RequirementID)
	if !ok {
		return Artifact{}, domainError(http.StatusConflict, "workflow_invalid", "assignment requirement is missing from the pinned workflow")
	}
	name := strings.TrimSpace(in.Name)
	if !containsString(requirement.Produces, name) {
		return Artifact{}, domainError(http.StatusBadRequest, "artifact_not_allowed", "artifact name is not an allowed assignment output")
	}
	typ := strings.TrimSpace(in.Type)
	content := in.Content
	if err := validateArtifactContent(typ, content); err != nil {
		return Artifact{}, err
	}
	metadata := in.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	rawMetadata, err := json.Marshal(metadata)
	if err != nil {
		return Artifact{}, domainError(http.StatusBadRequest, "invalid_artifact_metadata", "artifact metadata must be JSON-compatible")
	}
	now := s.now()
	var revision int64
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(revision), 0) + 1 FROM task_artifacts
		WHERE task_id = ? AND name = ?`, current.Task.ID, name).Scan(&revision); err != nil {
		return Artifact{}, err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO task_artifacts(task_id, assignment_id, name, type, content, metadata,
		                           revision, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, current.Task.ID, id, name, typ,
		content, string(rawMetadata), revision, actor.Principal, now, now)
	if err != nil {
		return Artifact{}, err
	}
	artifactID, err := result.LastInsertId()
	if err != nil {
		return Artifact{}, err
	}
	artifact := Artifact{ID: artifactID, TaskKey: current.Task.Key, AssignmentID: id,
		Name: name, Type: typ, Content: content, Metadata: metadata, Revision: revision,
		CreatedBy: actor.Principal, CreatedAt: now, UpdatedAt: now}
	current.Task.Access = "write"
	if _, err := appendEventTx(ctx, tx, current.Task, "workflow.artifact_added", actor,
		map[string]any{"artifact_id": artifact.ID, "assignment_id": id, "name": name, "type": typ}, now); err != nil {
		return Artifact{}, err
	}
	if err := writeTaskIdempotency(ctx, tx, actor.Principal, "add_workflow_artifact", in.IdempotencyKey, artifact, now); err != nil {
		return Artifact{}, err
	}
	if err := tx.Commit(); err != nil {
		return Artifact{}, err
	}
	s.signal()
	return artifact, nil
}

func validateArtifactContent(typ, content string) error {
	switch typ {
	case ArtifactMarkdown:
		if !utf8.ValidString(content) {
			return domainError(http.StatusBadRequest, "invalid_artifact_content", "markdown artifact must be valid UTF-8")
		}
		if len(content) > maxArtifactMarkdownBytes {
			return domainError(http.StatusRequestEntityTooLarge, "artifact_too_large", "markdown artifact exceeds the size limit")
		}
	case ArtifactJSON:
		var object map[string]any
		if err := json.Unmarshal([]byte(content), &object); err != nil || object == nil {
			return domainError(http.StatusBadRequest, "invalid_artifact_content", "JSON artifact must be an object")
		}
	case ArtifactFile:
		reference := strings.TrimSpace(content)
		clean := path.Clean(reference)
		if reference == "" || strings.Contains(reference, "\\") || strings.HasPrefix(reference, "/") || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != reference {
			return domainError(http.StatusBadRequest, "invalid_artifact_content", "file artifact must be a clean relative reference")
		}
	case ArtifactCommit:
		var ref struct {
			Repository string `json:"repository"`
			Ref        string `json:"ref"`
		}
		if err := json.Unmarshal([]byte(content), &ref); err != nil || strings.TrimSpace(ref.Repository) == "" || strings.TrimSpace(ref.Ref) == "" || len(ref.Repository) > 512 || len(ref.Ref) > 256 || strings.ContainsAny(ref.Repository+ref.Ref, "\r\n\x00") {
			return domainError(http.StatusBadRequest, "invalid_artifact_content", "commit artifact must contain bounded repository and ref strings")
		}
	case ArtifactURL:
		parsed, err := url.Parse(strings.TrimSpace(content))
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
			return domainError(http.StatusBadRequest, "invalid_artifact_content", "URL artifact must be an absolute http or https URL without credentials")
		}
	default:
		return domainError(http.StatusBadRequest, "invalid_artifact_type", "artifact type is not supported")
	}
	return nil
}

func (s *Service) GetArtifact(ctx context.Context, actor Actor, taskKey, assignmentID string, artifactID int64) (Artifact, error) {
	if err := validateActor(actor); err != nil {
		return Artifact{}, err
	}
	task, err := taskByKey(s.db, strings.TrimSpace(taskKey))
	if err != nil {
		return Artifact{}, err
	}
	scope, err := s.artifactReadScope(ctx, actor, task, assignmentID)
	if err != nil {
		return Artifact{}, err
	}
	artifact, err := artifactByID(ctx, s.db, artifactID)
	if err != nil || artifact.TaskKey != task.Key {
		if err == nil || ErrorCode(err) == "artifact_not_found" {
			return Artifact{}, domainError(http.StatusNotFound, "artifact_not_found", "artifact not found")
		}
		return Artifact{}, err
	}
	if !actor.IsCustomer {
		visible, err := artifactVisibleInScope(ctx, s.db, task.ID, artifact, scope)
		if err != nil {
			return Artifact{}, err
		}
		if !visible {
			return Artifact{}, domainError(http.StatusNotFound, "artifact_not_found", "artifact not found")
		}
		artifact.Metadata = nil
	}
	return artifact, nil
}

func (s *Service) ListArtifacts(ctx context.Context, actor Actor, taskKey, assignmentID string) ([]Artifact, error) {
	if err := validateActor(actor); err != nil {
		return nil, err
	}
	task, err := taskByKey(s.db, strings.TrimSpace(taskKey))
	if err != nil {
		return nil, err
	}
	scope, err := s.artifactReadScope(ctx, actor, task, assignmentID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM task_artifacts WHERE task_id = ? ORDER BY name, revision, id`, task.ID)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	artifacts := []Artifact{}
	for _, id := range ids {
		artifact, err := artifactByID(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		if !actor.IsCustomer {
			visible, err := artifactVisibleInScope(ctx, s.db, task.ID, artifact, scope)
			if err != nil {
				return nil, err
			}
			if !visible {
				continue
			}
			artifact.Metadata = nil
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func artifactByID(ctx context.Context, q queryer, id int64) (Artifact, error) {
	var artifact Artifact
	var assignmentID sql.NullInt64
	var rawMetadata string
	err := q.QueryRowContext(ctx, `
		SELECT a.id, t.task_key, a.assignment_id, a.name, a.type, a.content, a.metadata,
		       a.revision, a.created_by, a.created_at, a.updated_at
		FROM task_artifacts a JOIN tasks t ON t.id = a.task_id WHERE a.id = ?`, id).Scan(
		&artifact.ID, &artifact.TaskKey, &assignmentID, &artifact.Name, &artifact.Type,
		&artifact.Content, &rawMetadata, &artifact.Revision, &artifact.CreatedBy,
		&artifact.CreatedAt, &artifact.UpdatedAt)
	if err == sql.ErrNoRows {
		return Artifact{}, domainError(http.StatusNotFound, "artifact_not_found", "artifact not found")
	}
	if err != nil {
		return Artifact{}, err
	}
	artifact.AssignmentID = assignmentID.Int64
	if err := json.Unmarshal([]byte(rawMetadata), &artifact.Metadata); err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}

func workflowRequirementByID(def WorkflowDefinition, statusID, requirementID string) (WorkflowRequirement, bool) {
	status, ok := workflowStatusByID(def, statusID)
	if !ok {
		return WorkflowRequirement{}, false
	}
	for _, requirement := range status.Requirements {
		if requirement.ID == requirementID {
			return requirement, true
		}
	}
	return WorkflowRequirement{}, false
}

func requireArtifactRevisions(ctx context.Context, tx *sql.Tx, actor Actor, current assignmentContext, taskRevision, assignmentRevision int64) error {
	if err := requireAssignmentMutationRevisions(taskRevision, assignmentRevision); err == nil {
		if err = requireRuntimeRevisions(current, taskRevision, assignmentRevision); err == nil {
			return nil
		}
	}
	packet, packetErr := buildWorkPacket(ctx, tx, actor, current)
	data := map[string]any{"current_task_revision": current.Task.WorkflowRevision, "current_assignment_revision": current.Revision}
	if packetErr == nil {
		data["work_packet"] = packet
	}
	return &Error{Status: http.StatusConflict, Code: "revision_conflict", Msg: "workflow task or assignment revision is stale", Data: data}
}

func (s *Service) artifactReadScope(ctx context.Context, actor Actor, task Task, assignmentID string) (artifactReadScope, error) {
	if actor.IsCustomer {
		access, err := taskAccess(ctx, s.db, actor, task.ID)
		if err != nil {
			return artifactReadScope{}, err
		}
		if access == "" {
			return artifactReadScope{}, notFound(task.Key)
		}
		return artifactReadScope{}, nil
	}
	id, err := parseAssignmentID(assignmentID)
	if err != nil {
		return artifactReadScope{}, err
	}
	current, err := assignmentContextByID(ctx, s.db, id)
	if err != nil {
		return artifactReadScope{}, err
	}
	if current.Task.ID != task.ID {
		return artifactReadScope{}, domainError(http.StatusNotFound, "assignment_not_found", "assignment not found")
	}
	switch current.State {
	case AssignmentClaimable:
		if err := authorizeClaim(actor, current); err != nil {
			return artifactReadScope{}, err
		}
	case AssignmentLeased:
		if err := requireOwnedActiveLease(actor, current, s.clock().UTC()); err != nil {
			return artifactReadScope{}, err
		}
	default:
		return artifactReadScope{}, domainError(http.StatusConflict, "assignment_not_active", "assignment is not active")
	}
	requirement, ok := workflowRequirementByID(current.Workflow.Definition, current.Task.WorkflowStatus, current.RequirementID)
	if !ok {
		return artifactReadScope{}, domainError(http.StatusConflict, "workflow_invalid", "assignment requirement is missing from the pinned workflow")
	}
	scope := artifactReadScope{assignmentID: id, inputs: make(map[string]bool, len(requirement.Inputs)), outputs: make(map[string]bool, len(requirement.Produces))}
	for _, name := range requirement.Inputs {
		scope.inputs[name] = true
	}
	for _, name := range requirement.Produces {
		scope.outputs[name] = true
	}
	return scope, nil
}

func artifactVisibleInScope(ctx context.Context, q queryer, taskID int64, artifact Artifact, scope artifactReadScope) (bool, error) {
	if scope.outputs[artifact.Name] && artifact.AssignmentID == scope.assignmentID {
		return true, nil
	}
	if !scope.inputs[artifact.Name] {
		return false, nil
	}
	var latestID int64
	err := q.QueryRowContext(ctx, `
		SELECT id FROM task_artifacts
		WHERE task_id = ? AND name = ? ORDER BY revision DESC, id DESC LIMIT 1`, taskID, artifact.Name).Scan(&latestID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return latestID == artifact.ID, err
}
