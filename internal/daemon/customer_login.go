package daemon

import (
	"database/sql"
	"os"
	"os/user"
	"strings"

	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/tasks"
)

const customerLoginKey = "customer_login"

// resolveCustomerLogin returns the persisted customer login, adopting
// DefaultCustomerLogin on first run. Adoption carries existing $USER-derived
// state — tasks, customer notification state, and the customer channel with its
// message history and subscriptions — over to the new principal in one
// transaction, so an upgraded daemon does not orphan them.
func resolveCustomerLogin(st *store.Store) (string, error) {
	if value, ok, err := st.ConfigGet(customerLoginKey); err != nil {
		return "", err
	} else if login := strings.TrimSpace(value); ok && login != "" {
		return login, nil
	}
	login := tasks.DefaultCustomerLogin
	tx, err := st.DB.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	previous, err := previousCustomerPrincipals(tx, "user:"+login)
	if err != nil {
		return "", err
	}
	for _, principal := range previous {
		if err := adoptCustomerPrincipal(tx, principal, "user:"+login); err != nil {
			return "", err
		}
	}
	if _, err := tx.Exec(`INSERT INTO daemon_config(key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, customerLoginKey, login); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return login, nil
}

// previousCustomerPrincipals lists the customer principals this database was
// written with, plus the login this process would have derived from the
// account, so a server with no tasks still renames its customer channel.
func previousCustomerPrincipals(tx *sql.Tx, current string) ([]string, error) {
	seen := map[string]bool{current: true}
	var out []string
	add := func(principal string) {
		principal = strings.TrimSpace(principal)
		if principal == "" || seen[principal] {
			return
		}
		seen[principal] = true
		out = append(out, principal)
	}
	for _, query := range []string{
		`SELECT DISTINCT customer FROM tasks`,
		`SELECT DISTINCT customer_principal FROM task_notification_state`,
	} {
		rows, err := tx.Query(query)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var principal string
			if err := rows.Scan(&principal); err != nil {
				rows.Close()
				return nil, err
			}
			add(principal)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	add("user:" + accountCustomerLogin())
	return out, nil
}

// adoptCustomerPrincipal rewrites every reference to previous so it names
// current instead. The customer channel is renamed rather than duplicated; when
// the target channel already exists the stale row is dropped after its messages
// and subscriptions have moved. A subscription that would collide with an
// existing one on the new channel is left alone rather than failing startup —
// the agent already holds the subscription the move was for.
func adoptCustomerPrincipal(tx *sql.Tx, previous, current string) error {
	statements := []struct {
		query string
		args  []any
	}{
		{`UPDATE tasks SET customer = ? WHERE customer = ?`, []any{current, previous}},
		{`UPDATE task_notification_state SET customer_principal = ? WHERE customer_principal = ?`, []any{current, previous}},
		{`UPDATE task_waiting_for SET expected_principal = ? WHERE expected_principal = ?`, []any{current, previous}},
		{`UPDATE task_waiting_for SET requesting_principal = ? WHERE requesting_principal = ?`, []any{current, previous}},
		{`UPDATE tasks SET author = ? WHERE author = ?`, []any{current, previous}},
		{`UPDATE tasks SET assignee = ? WHERE assignee = ?`, []any{current, previous}},
		{`UPDATE task_comments SET author = ? WHERE author = ?`, []any{current, previous}},
		{`UPDATE task_events SET actor = ? WHERE actor = ?`, []any{current, previous}},
		{`UPDATE task_notification_outbox SET channel = ? WHERE channel = ? AND published_at = ''`, []any{current, previous}},
		{`UPDATE messages SET channel = ? WHERE channel = ?`, []any{current, previous}},
		{`UPDATE OR IGNORE subscriptions SET channel = ? WHERE channel = ?`, []any{current, previous}},
		{`INSERT OR IGNORE INTO channels(name, kind) SELECT ?, kind FROM channels WHERE name = ?`, []any{current, previous}},
		{`DELETE FROM channels WHERE name = ?`, []any{previous}},
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement.query, statement.args...); err != nil {
			return err
		}
	}
	return nil
}

// accountCustomerLogin is the $USER-derived login older daemons used as the
// customer principal. It now only names the state that first-run reconciliation
// has to carry over to DefaultCustomerLogin.
func accountCustomerLogin() string {
	if current, err := user.Current(); err == nil {
		if login := strings.TrimSpace(current.Username); login != "" {
			if i := strings.LastIndexAny(login, `\/`); i >= 0 {
				login = login[i+1:]
			}
			if login != "" {
				return login
			}
		}
	}
	for _, key := range []string{"LOGNAME", "USER"} {
		if login := strings.TrimSpace(os.Getenv(key)); login != "" {
			return login
		}
	}
	return "customer"
}
