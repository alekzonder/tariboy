package store

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsUniqueViolation(t *testing.T) {
	t.Parallel()
	s := open(t)
	if _, err := s.DB.Exec(`CREATE TABLE uniq_probe(id INTEGER PRIMARY KEY, name TEXT UNIQUE, note TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO uniq_probe(id, name) VALUES (1, 'a')`); err != nil {
		t.Fatal(err)
	}
	_, dupName := s.DB.Exec(`INSERT INTO uniq_probe(id, name) VALUES (2, 'a')`)
	_, dupKey := s.DB.Exec(`INSERT INTO uniq_probe(id, name) VALUES (1, 'b')`)
	_, notNull := s.DB.Exec(`INSERT INTO uniq_probe(id, name, note) VALUES (3, 'c', NULL)`)
	for name, tc := range map[string]struct {
		err  error
		want bool
	}{
		"unique column":    {dupName, true},
		"primary key":      {dupKey, true},
		"wrapped":          {fmt.Errorf("insert: %w", dupName), true},
		"other constraint": {notNull, false},
		"plain error":      {errors.New("UNIQUE constraint failed"), false},
		"nil":              {nil, false},
	} {
		if IsUniqueViolation(tc.err) != tc.want {
			t.Errorf("%s: IsUniqueViolation(%v) = %v, want %v", name, tc.err, !tc.want, tc.want)
		}
	}
}
