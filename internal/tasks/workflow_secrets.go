package tasks

import (
	"context"
	"database/sql"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

// MaxQueueSecretBytes is the largest queue secret value.
const MaxQueueSecretBytes = 64 << 10

var queueSecretKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validateQueueSecret checks a key and value. Its errors never contain the
// value.
func validateQueueSecret(key, value string) error {
	if !queueSecretKeyPattern.MatchString(key) {
		return domainError(http.StatusBadRequest, "invalid_secret_key",
			"a secret key starts with a letter or underscore and holds letters, digits, and underscores")
	}
	if strings.HasPrefix(key, "TARIBOY_") {
		return domainError(http.StatusBadRequest, "invalid_secret_key",
			"a secret key must not start with TARIBOY_: it would shadow a protocol variable")
	}
	if value == "" || !utf8.ValidString(value) {
		return domainError(http.StatusBadRequest, "invalid_secret", "a secret value is non-empty UTF-8 text")
	}
	if len(value) > MaxQueueSecretBytes {
		return domainError(http.StatusRequestEntityTooLarge, "secret_too_large", "a secret value is at most 64 KiB")
	}
	return nil
}

func queueSecretsMissingError(keys []string) error {
	return &Error{
		Status: http.StatusConflict, Code: "workflow_secret_missing",
		Msg:  "workflow secrets have no value: " + strings.Join(keys, ", "),
		Data: map[string]any{"secrets": keys},
	}
}

// SetGlobalEnv sets the environment that workflow scripts running as the
// queue start from. A required secret it holds with a non-empty value needs no
// queue secret.
func (s *Service) SetGlobalEnv(env func() ([]string, error)) { s.globalEnv = env }

// globalEnvNames returns the sorted names among names that the global
// environment holds with a non-empty value. It never returns a value.
func (s *Service) globalEnvNames(names []string) ([]string, error) {
	found := []string{}
	if s.globalEnv == nil || len(names) == 0 {
		return found, nil
	}
	env, err := s.globalEnv()
	if err != nil {
		return nil, err
	}
	for _, kv := range env {
		name, value, _ := strings.Cut(kv, "=")
		if value != "" && slices.Contains(names, name) && !slices.Contains(found, name) {
			found = append(found, name)
		}
	}
	sort.Strings(found)
	return found, nil
}

// missingQueueSecrets returns the sorted names among names that neither the
// queue nor the global environment has a value for.
func (s *Service) missingQueueSecrets(ctx context.Context, q queryer, queue string, names []string) ([]string, error) {
	missing := []string{}
	for _, name := range names {
		var exists int
		err := q.QueryRowContext(ctx, `
			SELECT 1 FROM task_queue_secrets WHERE queue_prefix = ? AND key = ?`, queue, name).Scan(&exists)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			return nil, err
		}
		missing = append(missing, name)
	}
	global, err := s.globalEnvNames(missing)
	if err != nil {
		return nil, err
	}
	missing = slices.DeleteFunc(missing, func(name string) bool { return slices.Contains(global, name) })
	sort.Strings(missing)
	return missing, nil
}

// SetQueueSecret stores the value of a queue secret, replacing an earlier one.
// The value is written to the database only: not to an event, a log, or an
// error.
func (s *Service) SetQueueSecret(ctx context.Context, actor Actor, queue, key, value string) error {
	_, err := s.setQueueSecret(ctx, actor, queue, key, value)
	return err
}

// SetQueueSecretInfo is SetQueueSecret returning the key and the stored time,
// read in the transaction that stored the value. It never carries the value.
func (s *Service) SetQueueSecretInfo(ctx context.Context, actor Actor, queue, key, value string) (QueueSecretInfo, error) {
	updatedAt, err := s.setQueueSecret(ctx, actor, queue, key, value)
	if err != nil {
		return QueueSecretInfo{}, err
	}
	return QueueSecretInfo{Key: key, UpdatedAt: updatedAt}, nil
}

// setQueueSecret stores a queue secret and returns its stored updated_at.
func (s *Service) setQueueSecret(ctx context.Context, actor Actor, queue, key, value string) (string, error) {
	if err := s.requireWorkflowAdmin(actor); err != nil {
		return "", err
	}
	queue = strings.ToUpper(strings.TrimSpace(queue))
	if err := validateQueueSecret(key, value); err != nil {
		return "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	revision, err := queueSecretRevision(ctx, tx, queue)
	if err != nil {
		return "", err
	}
	now := s.now()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO task_queue_secrets(queue_prefix, key, value, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(queue_prefix, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		queue, key, value, now); err != nil {
		return "", err
	}
	var updatedAt string
	if err := tx.QueryRowContext(ctx, `SELECT updated_at FROM task_queue_secrets WHERE queue_prefix = ? AND key = ?`,
		queue, key).Scan(&updatedAt); err != nil {
		return "", err
	}
	if _, err := appendQueueEventTx(ctx, tx, Queue{Prefix: queue, Revision: revision},
		"queue.secret_set", actor, map[string]any{"key": key}, now); err != nil {
		return "", err
	}
	return updatedAt, tx.Commit()
}

// QueueSecretInfo names a queue secret and when it was last set. It never
// carries the value.
type QueueSecretInfo struct {
	Key       string `json:"key"`
	UpdatedAt string `json:"updated_at"`
}

// ListQueueSecrets returns the secrets of a queue sorted by key, without
// their values.
func (s *Service) ListQueueSecrets(ctx context.Context, actor Actor, queue string) ([]QueueSecretInfo, error) {
	if err := s.requireWorkflowAdmin(actor); err != nil {
		return nil, err
	}
	queue = strings.ToUpper(strings.TrimSpace(queue))
	if _, err := queueSecretRevision(ctx, s.db, queue); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT key, updated_at FROM task_queue_secrets WHERE queue_prefix = ? ORDER BY key`, queue)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []QueueSecretInfo{}
	for rows.Next() {
		var item QueueSecretInfo
		if err := rows.Scan(&item.Key, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ListQueueSecretKeys returns the sorted keys of a queue's secrets, never a
// value.
func (s *Service) ListQueueSecretKeys(ctx context.Context, actor Actor, queue string) ([]string, error) {
	items, err := s.ListQueueSecrets(ctx, actor, queue)
	if err != nil {
		return nil, err
	}
	keys := make([]string, len(items))
	for i, item := range items {
		keys[i] = item.Key
	}
	return keys, nil
}

// QueueSecretsInGlobalEnv returns the sorted secrets required by the workflow
// bound to the queue that the global environment holds, by name only.
func (s *Service) QueueSecretsInGlobalEnv(ctx context.Context, actor Actor, queue string) ([]string, error) {
	if err := s.requireWorkflowAdmin(actor); err != nil {
		return nil, err
	}
	queue = strings.ToUpper(strings.TrimSpace(queue))
	if _, err := queueSecretRevision(ctx, s.db, queue); err != nil {
		return nil, err
	}
	binding, bound, err := queueWorkflowTx(ctx, s.db, queue)
	if err != nil || !bound {
		return []string{}, err
	}
	manifest, err := loadManifestTx(ctx, s.db, binding.Digest)
	if err != nil {
		return nil, err
	}
	return s.globalEnvNames(manifest.Definition.RequiresSecrets)
}

// RemoveQueueSecret deletes a queue secret. It is refused while the workflow
// bound to the queue requires the key and the global environment lacks it.
// Tasks pinned to older workflow versions are not considered: a script that
// misses its secret fails.
func (s *Service) RemoveQueueSecret(ctx context.Context, actor Actor, queue, key string) error {
	if err := s.requireWorkflowAdmin(actor); err != nil {
		return err
	}
	queue = strings.ToUpper(strings.TrimSpace(queue))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	revision, err := queueSecretRevision(ctx, tx, queue)
	if err != nil {
		return err
	}
	binding, bound, err := queueWorkflowTx(ctx, tx, queue)
	if err != nil {
		return err
	}
	if bound {
		manifest, err := loadManifestTx(ctx, tx, binding.Digest)
		if err != nil {
			return err
		}
		if slices.Contains(manifest.Definition.RequiresSecrets, key) {
			if global, err := s.globalEnvNames([]string{key}); err != nil {
				return err
			} else if len(global) == 0 {
				return queueSecretsMissingError([]string{key})
			}
		}
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM task_queue_secrets WHERE queue_prefix = ? AND key = ?`, queue, key)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return domainError(http.StatusNotFound, "not_found", "queue secret not found")
	}
	if _, err := appendQueueEventTx(ctx, tx, Queue{Prefix: queue, Revision: revision},
		"queue.secret_removed", actor, map[string]any{"key": key}, s.now()); err != nil {
		return err
	}
	return tx.Commit()
}

// queueSecrets returns every secret value of a queue for a script run's
// environment. It takes no actor and no route or agent action reaches it.
func (s *Service) queueSecrets(ctx context.Context, queue string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM task_queue_secrets WHERE queue_prefix = ?`, queue)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, rows.Err()
}

// queueSecretRevision proves the queue exists and returns its revision for the
// audit event.
func queueSecretRevision(ctx context.Context, q queryer, queue string) (int64, error) {
	var revision int64
	err := q.QueryRowContext(ctx, `SELECT revision FROM task_queues WHERE prefix = ?`, queue).Scan(&revision)
	if err == sql.ErrNoRows {
		return 0, domainError(http.StatusNotFound, "not_found", "queue not found")
	}
	return revision, err
}
