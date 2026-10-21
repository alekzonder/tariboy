package migrate

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	_ "modernc.org/sqlite"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "m.db")+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func files(m map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, body := range m {
		fsys["migrations/"+name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

func applied(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM schema_migrations ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		out = append(out, name)
	}
	return out
}

func count(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRunAppliesInNameOrderOnce(t *testing.T) {
	db := openDB(t)
	fsys := files(map[string]string{
		"0002_row.sql":   `INSERT INTO t(v) VALUES ('x');`,
		"0001_table.sql": `CREATE TABLE t (v TEXT);`,
		"notes.txt":      `not a migration`,
	})
	for i := 0; i < 2; i++ {
		if err := Run(db, fsys, "migrations", nil); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if got := applied(t, db); strings.Join(got, ",") != "0001_table.sql,0002_row.sql" {
		t.Fatalf("applied = %v", got)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM t`); n != 1 {
		t.Fatalf("rows = %d, want 1 (second Run must do nothing)", n)
	}
}

func TestRunFailedMigrationRollsBack(t *testing.T) {
	db := openDB(t)
	fsys := files(map[string]string{
		"0001_bad.sql": `CREATE TABLE t (v TEXT); INSERT INTO missing VALUES (1);`,
	})
	err := Run(db, fsys, "migrations", nil)
	if err == nil || !strings.Contains(err.Error(), "migration 0001_bad.sql:") {
		t.Fatalf("err = %v, want wrapped migration error", err)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE name='t'`); n != 0 {
		t.Fatal("failed migration left its table")
	}
	if got := applied(t, db); len(got) != 0 {
		t.Fatalf("applied = %v", got)
	}
}

const parentChild = `CREATE TABLE parent (id INTEGER PRIMARY KEY);
CREATE TABLE child (parent_id INTEGER NOT NULL REFERENCES parent(id));
INSERT INTO parent(id) VALUES (1);
INSERT INTO child(parent_id) VALUES (1);`

const rebuildParent = `CREATE TABLE parent_new (id INTEGER PRIMARY KEY, note TEXT NOT NULL DEFAULT '');
INSERT INTO parent_new(id) SELECT id FROM parent;
DROP TABLE parent;
ALTER TABLE parent_new RENAME TO parent;`

func TestRunForeignKeysOffRebuildsReferencedTable(t *testing.T) {
	db := openDB(t)
	fsys := files(map[string]string{
		"0001_base.sql":    parentChild,
		"0002_rebuild.sql": "-- tariboy: foreign_keys=off\n" + rebuildParent,
	})
	if err := Run(db, fsys, "migrations", nil); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM pragma_table_info('parent') WHERE name='note'`); n != 1 {
		t.Fatal("rebuild did not apply")
	}
	if n := count(t, db, `PRAGMA foreign_keys`); n != 1 {
		t.Fatal("foreign keys not restored")
	}
}

func TestRunForeignKeysRequiresHeader(t *testing.T) {
	db := openDB(t)
	fsys := files(map[string]string{
		"0001_base.sql":    parentChild,
		"0002_rebuild.sql": rebuildParent,
	})
	if err := Run(db, fsys, "migrations", nil); err == nil {
		t.Fatal("rebuilding a referenced table with foreign keys on succeeded")
	}
}

func TestRunForeignKeysOffViolationRollsBack(t *testing.T) {
	db := openDB(t)
	fsys := files(map[string]string{
		"0001_base.sql": parentChild,
		"0002_orphan.sql": "-- tariboy: foreign_keys=off\n" +
			rebuildParent + "\nINSERT INTO child(parent_id) VALUES (99);",
	})
	err := Run(db, fsys, "migrations", nil)
	if err == nil || !strings.Contains(err.Error(), "migration 0002_orphan.sql foreign key check failed for table child") {
		t.Fatalf("err = %v, want foreign key check failure", err)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM child`); n != 1 {
		t.Fatalf("child rows = %d, want 1 after rollback", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM pragma_table_info('parent') WHERE name='note'`); n != 0 {
		t.Fatal("rebuild was committed despite the violation")
	}
	if got := applied(t, db); strings.Join(got, ",") != "0001_base.sql" {
		t.Fatalf("applied = %v", got)
	}
	if n := count(t, db, `PRAGMA foreign_keys`); n != 1 {
		t.Fatal("foreign keys not restored after failure")
	}
}

func TestRunSkipIfColumn(t *testing.T) {
	db := openDB(t)
	fsys := files(map[string]string{
		"0001_table.sql": `CREATE TABLE t (a TEXT);`,
		"0002_add_a.sql": "-- tariboy: skip_if_column=t.a\nALTER TABLE t ADD COLUMN a TEXT;",
		"0003_add_b.sql": "-- tariboy: skip_if_column=t.b\nALTER TABLE t ADD COLUMN b TEXT;",
	})
	if err := Run(db, fsys, "migrations", nil); err != nil {
		t.Fatal(err)
	}
	if got := applied(t, db); strings.Join(got, ",") != "0001_table.sql,0002_add_a.sql,0003_add_b.sql" {
		t.Fatalf("applied = %v", got)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM pragma_table_info('t') WHERE name='b'`); n != 1 {
		t.Fatal("body of a migration whose column is missing did not run")
	}
}

func TestRunStepSeesBodyInSameTransaction(t *testing.T) {
	db := openDB(t)
	fsys := files(map[string]string{
		"0001_step.sql": "-- tariboy: step=fill\n-- other comment\nCREATE TABLE s (v TEXT);",
	})
	steps := map[string]Step{"fill": func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO s(v) VALUES ('from step')`)
		return err
	}}
	if err := Run(db, fsys, "migrations", steps); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM s WHERE v='from step'`); n != 1 {
		t.Fatal("step row missing")
	}
}

func TestRunStepFailureRollsBackBody(t *testing.T) {
	db := openDB(t)
	fsys := files(map[string]string{
		"0001_step.sql": "-- tariboy: step=fail\nCREATE TABLE s (v TEXT);",
	})
	boom := errors.New("boom")
	steps := map[string]Step{"fail": func(tx *sql.Tx) error { return boom }}
	err := Run(db, fsys, "migrations", steps)
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "migration 0001_step.sql:") {
		t.Fatalf("err = %v, want wrapped step error", err)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE name='s'`); n != 0 {
		t.Fatal("step failure left the body's table")
	}
	if got := applied(t, db); len(got) != 0 {
		t.Fatalf("applied = %v", got)
	}
}

func TestRunRejectsBadHeaders(t *testing.T) {
	for name, body := range map[string]string{
		"unknown key":       "-- tariboy: frobnicate=yes\nCREATE TABLE t (v TEXT);",
		"missing step":      "-- tariboy: step=absent\nCREATE TABLE t (v TEXT);",
		"bad foreign keys":  "-- tariboy: foreign_keys=on\nCREATE TABLE t (v TEXT);",
		"bad column":        "-- tariboy: skip_if_column=nodot\nCREATE TABLE t (v TEXT);",
		"no key value pair": "-- tariboy: foreign_keys\nCREATE TABLE t (v TEXT);",
	} {
		t.Run(name, func(t *testing.T) {
			db := openDB(t)
			err := Run(db, files(map[string]string{"0001_x.sql": body}), "migrations", nil)
			if err == nil || !strings.Contains(err.Error(), "0001_x.sql") {
				t.Fatalf("err = %v, want header error naming the file", err)
			}
			if n := count(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE name='t'`); n != 0 {
				t.Fatal("body ran despite a bad header")
			}
			if got := applied(t, db); len(got) != 0 {
				t.Fatalf("applied = %v", got)
			}
		})
	}
}

func TestRunHeaderOnlyAtTop(t *testing.T) {
	db := openDB(t)
	fsys := files(map[string]string{
		"0001_x.sql": "CREATE TABLE t (v TEXT);\n-- tariboy: frobnicate=yes\n",
	})
	if err := Run(db, fsys, "migrations", nil); err != nil {
		t.Fatalf("a tariboy comment after the header is plain SQL comment: %v", err)
	}
}
