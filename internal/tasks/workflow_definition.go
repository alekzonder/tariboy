package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
)

const workflowVersionSelect = `
SELECT id, name, version, state, definition, created_at, updated_at, published_at
FROM task_workflow_versions`

func (s *Service) requireWorkflowAdmin(actor Actor) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	if !actor.IsCustomer || actor.Principal != userPrincipal(s.customer) {
		return domainError(http.StatusForbidden, "forbidden", "only the daemon customer can administer workflows")
	}
	return nil
}

// CreateWorkflowDraft creates or replaces the draft identified by its stable
// name and version. Published rows are never changed.
func (s *Service) CreateWorkflowDraft(ctx context.Context, actor Actor, definition WorkflowDefinition) (WorkflowVersion, error) {
	if err := s.requireWorkflowAdmin(actor); err != nil {
		return WorkflowVersion{}, err
	}
	definition = normalizeWorkflowDefinition(definition)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkflowVersion{}, err
	}
	defer tx.Rollback()

	var id int64
	var state string
	err = tx.QueryRowContext(ctx, `
		SELECT id, state FROM task_workflow_versions WHERE name = ? AND version = ?`,
		definition.Name, definition.Version).Scan(&id, &state)
	if err != nil && err != sql.ErrNoRows {
		return WorkflowVersion{}, err
	}
	if err == nil && state != "draft" {
		return WorkflowVersion{}, workflowImmutableError()
	}
	validationErrors := ValidateWorkflow(definition)
	if len(validationErrors) != 0 {
		return WorkflowVersion{}, &Error{
			Status: http.StatusBadRequest, Code: "workflow_invalid", Msg: "workflow definition is invalid",
			Data: map[string]any{"validation_errors": validationErrors},
		}
	}
	canonical, err := json.Marshal(definition)
	if err != nil {
		return WorkflowVersion{}, err
	}
	now := s.now()
	if state == "draft" {
		result, err := tx.ExecContext(ctx, `
			UPDATE task_workflow_versions SET definition = ?, updated_at = ?
			WHERE id = ? AND state = 'draft'`, string(canonical), now, id)
		if err != nil {
			return WorkflowVersion{}, err
		}
		if affected, err := result.RowsAffected(); err != nil {
			return WorkflowVersion{}, err
		} else if affected != 1 {
			return WorkflowVersion{}, workflowImmutableError()
		}
	} else {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO task_workflow_versions(name, version, definition, state, created_at, updated_at)
			VALUES (?, ?, ?, 'draft', ?, ?)`,
			definition.Name, definition.Version, string(canonical), now, now)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return WorkflowVersion{}, workflowImmutableError()
			}
			return WorkflowVersion{}, err
		}
		id, err = result.LastInsertId()
		if err != nil {
			return WorkflowVersion{}, err
		}
	}

	version, err := workflowVersionByID(ctx, tx, id)
	if err != nil {
		return WorkflowVersion{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkflowVersion{}, err
	}
	return version, nil
}

func (s *Service) ValidateWorkflowVersion(ctx context.Context, actor Actor, name string, version int) ([]WorkflowValidationError, error) {
	if err := s.requireWorkflowAdmin(actor); err != nil {
		return nil, err
	}
	workflow, err := workflowVersionByName(ctx, s.db, strings.TrimSpace(name), version)
	if err != nil {
		return nil, err
	}
	return ValidateWorkflow(workflow.Definition), nil
}

func (s *Service) PublishWorkflowVersion(ctx context.Context, actor Actor, name string, version int) (WorkflowVersion, error) {
	if err := s.requireWorkflowAdmin(actor); err != nil {
		return WorkflowVersion{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkflowVersion{}, err
	}
	defer tx.Rollback()
	workflow, err := workflowVersionByName(ctx, tx, strings.TrimSpace(name), version)
	if err != nil {
		return WorkflowVersion{}, err
	}
	if workflow.State != "draft" {
		return WorkflowVersion{}, workflowImmutableError()
	}
	workflow.Definition = normalizeWorkflowDefinition(workflow.Definition)
	validationErrors := ValidateWorkflow(workflow.Definition)
	if len(validationErrors) != 0 {
		return WorkflowVersion{}, &Error{
			Status: http.StatusBadRequest, Code: "workflow_invalid", Msg: "workflow definition is invalid",
			Data: map[string]any{"validation_errors": validationErrors},
		}
	}
	canonical, err := json.Marshal(workflow.Definition)
	if err != nil {
		return WorkflowVersion{}, err
	}
	now := s.now()
	result, err := tx.ExecContext(ctx, `
		UPDATE task_workflow_versions
		SET definition = ?, state = 'published', updated_at = ?, published_at = ?
		WHERE id = ? AND state = 'draft'`, string(canonical), now, now, workflow.ID)
	if err != nil {
		return WorkflowVersion{}, err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return WorkflowVersion{}, err
	} else if affected != 1 {
		return WorkflowVersion{}, workflowImmutableError()
	}
	published, err := workflowVersionByID(ctx, tx, workflow.ID)
	if err != nil {
		return WorkflowVersion{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkflowVersion{}, err
	}
	return published, nil
}

func (s *Service) ListWorkflowVersions(ctx context.Context, actor Actor, name string) ([]WorkflowVersion, error) {
	if err := s.requireWorkflowAdmin(actor); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, workflowVersionSelect+`
		WHERE name = ? ORDER BY version`, strings.TrimSpace(name))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	versions := make([]WorkflowVersion, 0)
	for rows.Next() {
		workflow, err := scanWorkflowVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, workflow)
	}
	return versions, rows.Err()
}

func (s *Service) GetWorkflowVersion(ctx context.Context, actor Actor, name string, version int) (WorkflowVersion, error) {
	if err := s.requireWorkflowAdmin(actor); err != nil {
		return WorkflowVersion{}, err
	}
	return workflowVersionByName(ctx, s.db, strings.TrimSpace(name), version)
}

func workflowVersionByName(ctx context.Context, q queryer, name string, version int) (WorkflowVersion, error) {
	workflow, err := scanWorkflowVersion(q.QueryRowContext(ctx, workflowVersionSelect+`
		WHERE name = ? AND version = ?`, name, version))
	if err == sql.ErrNoRows {
		return WorkflowVersion{}, workflowVersionNotFound(name, version)
	}
	return workflow, err
}

func workflowVersionByID(ctx context.Context, q queryer, id int64) (WorkflowVersion, error) {
	workflow, err := scanWorkflowVersion(q.QueryRowContext(ctx, workflowVersionSelect+` WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return WorkflowVersion{}, workflowVersionNotFound("", 0)
	}
	return workflow, err
}

func scanWorkflowVersion(row rowScanner) (WorkflowVersion, error) {
	var workflow WorkflowVersion
	var definition string
	err := row.Scan(
		&workflow.ID, &workflow.Name, &workflow.Version, &workflow.State, &definition,
		&workflow.CreatedAt, &workflow.UpdatedAt, &workflow.PublishedAt,
	)
	if err != nil {
		return WorkflowVersion{}, err
	}
	if err := json.Unmarshal([]byte(definition), &workflow.Definition); err != nil {
		return WorkflowVersion{}, err
	}
	// Legacy immutable rows may predate collection canonicalization and contain
	// JSON nulls. Normalize the detached read model only; never rewrite stored
	// definition bytes from a read path.
	workflow.Definition = normalizeWorkflowDefinition(workflow.Definition)
	return workflow, nil
}

func workflowImmutableError() error {
	return domainError(http.StatusConflict, "workflow_immutable", "published workflow versions are immutable")
}

func workflowVersionNotFound(name string, version int) error {
	return &Error{
		Status: http.StatusNotFound, Code: "workflow_version_not_found",
		Msg: "workflow version not found", Data: map[string]any{"name": name, "version": version},
	}
}
