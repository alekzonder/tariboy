// Package operations runs long daemon actions, such as image and workflow
// builds, in the background. Each operation is a row of the operations table
// and a log file under <base-dir>/operations; a client starts one, gets its id
// at once and reads its status and log later.
package operations

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/alekzonder/tariboy/internal/store"
)

const (
	StatusRunning     = "running"
	StatusSucceeded   = "succeeded"
	StatusFailed      = "failed"
	StatusInterrupted = "interrupted"
)

// maxLogChunk bounds one ReadLog answer; a client reads on from Next.
const maxLogChunk = 1 << 20

// Spec describes an operation to start.
type Spec struct {
	Kind      string
	Title     string
	Initiator string
	Params    map[string]string
}

// Record is one stored operation.
type Record struct {
	ID           int64
	Kind         string
	Title        string
	Status       string
	Initiator    string
	Params       map[string]string
	Result       json.RawMessage // empty unless the operation succeeded
	ErrorCode    string
	ErrorMessage string
	CreatedAt    string
	FinishedAt   string
}

// LogChunk is the part of an operation log from Offset to Next.
type LogChunk struct {
	Offset  int64
	Next    int64
	Size    int64
	Content string
}

// Store keeps the operation rows in DB and their logs in Dir.
type Store struct {
	DB  *sql.DB
	Dir string
	Now func() time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// LogPath is the log file of operation id.
func (s *Store) LogPath(id int64) string {
	return filepath.Join(s.Dir, strconv.FormatInt(id, 10)+".log")
}

// Create stores a new running operation.
func (s *Store) Create(spec Spec) (Record, error) {
	params := spec.Params
	if params == nil {
		params = map[string]string{}
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return Record{}, err
	}
	created := store.FormatTime(s.now())
	res, err := s.DB.Exec(`INSERT INTO operations(kind, title, status, initiator, params, created_at) VALUES (?,?,?,?,?,?)`,
		spec.Kind, spec.Title, StatusRunning, spec.Initiator, string(raw), created)
	if err != nil {
		return Record{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Record{}, err
	}
	return Record{ID: id, Kind: spec.Kind, Title: spec.Title, Status: StatusRunning, Initiator: spec.Initiator, Params: params, CreatedAt: created}, nil
}

// Finish records the final status of a running operation. result is stored
// as JSON when it is not nil.
func (s *Store) Finish(id int64, status string, result any, code, message string) error {
	raw := ""
	if result != nil {
		b, err := json.Marshal(result)
		if err != nil {
			return err
		}
		raw = string(b)
	}
	_, err := s.DB.Exec(`UPDATE operations SET status=?, result=?, error_code=?, error_message=?, finished_at=? WHERE id=?`,
		status, raw, code, message, store.FormatTime(s.now()), id)
	return err
}

// MarkInterrupted finishes every running operation as interrupted. The daemon
// runs it at start: no build of a previous daemon process is still running.
func (s *Store) MarkInterrupted() (int64, error) {
	res, err := s.DB.Exec(`UPDATE operations SET status=?, finished_at=? WHERE status=?`,
		StatusInterrupted, store.FormatTime(s.now()), StatusRunning)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

const selectColumns = `SELECT id, kind, title, status, initiator, params, result, error_code, error_message, created_at, finished_at FROM operations`

// Get returns operation id; ok is false when it does not exist.
func (s *Store) Get(id int64) (Record, bool, error) {
	rec, err := scanRecord(s.DB.QueryRow(selectColumns+` WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, false, nil
	}
	return rec, err == nil, err
}

// List returns page (from 1) of limit operations, newest first, and the
// number of all operations.
func (s *Store) List(page, limit int) ([]Record, int, error) {
	var total int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM operations`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.Query(selectColumns+` ORDER BY id DESC LIMIT ? OFFSET ?`, limit, (max(page, 1)-1)*limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, rec)
	}
	return out, total, rows.Err()
}

func scanRecord(row interface{ Scan(...any) error }) (Record, error) {
	var rec Record
	var params, result string
	if err := row.Scan(&rec.ID, &rec.Kind, &rec.Title, &rec.Status, &rec.Initiator, &params, &result, &rec.ErrorCode, &rec.ErrorMessage, &rec.CreatedAt, &rec.FinishedAt); err != nil {
		return Record{}, err
	}
	rec.Params = map[string]string{}
	if err := json.Unmarshal([]byte(params), &rec.Params); err != nil {
		return Record{}, err
	}
	if result != "" {
		rec.Result = json.RawMessage(result)
	}
	return rec, nil
}

// ReadLog returns up to 1 MiB of the log of operation id from offset. The
// error wraps os.ErrNotExist when the log file is gone.
func (s *Store) ReadLog(id, offset int64) (LogChunk, error) {
	f, err := os.Open(s.LogPath(id))
	if err != nil {
		return LogChunk{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return LogChunk{}, err
	}
	offset = min(max(offset, 0), info.Size())
	buf := make([]byte, min(info.Size()-offset, maxLogChunk))
	n, err := f.ReadAt(buf, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return LogChunk{}, err
	}
	buf = buf[:n]
	// Stop before a rune the chunk cuts; the next read starts with it.
	if cut := len(buf); cut > 0 && offset+int64(cut) < info.Size() {
		for i := cut - 1; i >= 0 && i >= cut-utf8.UTFMax; i-- {
			if utf8.RuneStart(buf[i]) {
				if !utf8.FullRune(buf[i:]) {
					buf = buf[:i]
				}
				break
			}
		}
	}
	return LogChunk{Offset: offset, Next: offset + int64(len(buf)), Size: info.Size(), Content: string(buf)}, nil
}

// OpenLog opens the log file of operation id for a download.
func (s *Store) OpenLog(id int64) (*os.File, error) {
	return os.Open(s.LogPath(id))
}
