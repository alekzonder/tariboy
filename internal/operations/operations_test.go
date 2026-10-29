package operations

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/events"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	base := t.TempDir()
	st := storetest.OpenAt(t, filepath.Join(base, "state.db"))
	return &Store{DB: st.DB, Dir: filepath.Join(base, "operations")}
}

func TestStoreListsNewestFirstWithPages(t *testing.T) {
	s := newStore(t)
	for i := range 5 {
		if _, err := s.Create(Spec{Kind: "image.build", Title: fmt.Sprintf("op %d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	page, total, err := s.List(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 || len(page) != 2 || page[0].Title != "op 4" || page[1].Title != "op 3" {
		t.Fatalf("page 1 = %+v total %d", page, total)
	}
	page, _, err = s.List(3, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || page[0].Title != "op 0" {
		t.Fatalf("page 3 = %+v", page)
	}
}

func TestStoreRecordsParamsResultAndError(t *testing.T) {
	s := newStore(t)
	rec, err := s.Create(Spec{Kind: "image.build", Title: "image build demo", Initiator: "cli", Params: map[string]string{"name": "demo"}})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != StatusRunning || rec.Params["name"] != "demo" || rec.Initiator != "cli" || rec.CreatedAt == "" {
		t.Fatalf("created = %+v", rec)
	}
	if err := s.Finish(rec.ID, StatusFailed, nil, "bad_imagefile", "broken yaml"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Get(rec.ID)
	if err != nil || !ok {
		t.Fatalf("get: %v %v", ok, err)
	}
	if got.Status != StatusFailed || got.ErrorCode != "bad_imagefile" || got.ErrorMessage != "broken yaml" || got.FinishedAt == "" {
		t.Fatalf("finished = %+v", got)
	}
	if _, ok, err := s.Get(rec.ID + 100); err != nil || ok {
		t.Fatalf("missing get = %v %v", ok, err)
	}
}

func TestStoreReadsLogFromOffset(t *testing.T) {
	s := newStore(t)
	rec, err := s.Create(Spec{Kind: "image.build", Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	l, err := s.openLog(rec.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	l.Printf("first")
	l.Printf("second %d", 2)
	l.close()
	chunk, err := s.ReadLog(rec.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if chunk.Content != "first\nsecond 2\n" || chunk.Next != int64(len(chunk.Content)) {
		t.Fatalf("chunk = %+v", chunk)
	}
	tail, err := s.ReadLog(rec.ID, 6)
	if err != nil {
		t.Fatal(err)
	}
	if tail.Content != "second 2\n" {
		t.Fatalf("tail = %q", tail.Content)
	}
	if _, err := s.ReadLog(rec.ID+1, 0); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing log err = %v", err)
	}
}

func TestLogStopsAtLimit(t *testing.T) {
	s := newStore(t)
	rec, _ := s.Create(Spec{Kind: "image.build", Title: "t"})
	l, err := s.openLog(rec.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	l.limit = 10
	l.Printf("0123456789")
	l.Printf("dropped")
	l.Printf("dropped too")
	l.close()
	chunk, _ := s.ReadLog(rec.ID, 0)
	if chunk.Content != "0123456789\n"+truncatedLine+"\n" {
		t.Fatalf("content = %q", chunk.Content)
	}
}

func TestLogWriterSplitsLines(t *testing.T) {
	s := newStore(t)
	rec, _ := s.Create(Spec{Kind: "image.build", Title: "t"})
	l, err := s.openLog(rec.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	w := l.Writer("npx: ")
	fmt.Fprint(w, "a\nb")
	fmt.Fprint(w, "c\n")
	fmt.Fprint(w, "tail")
	w.Close()
	l.close()
	chunk, _ := s.ReadLog(rec.ID, 0)
	if chunk.Content != "npx: a\nnpx: bc\nnpx: tail\n" {
		t.Fatalf("content = %q", chunk.Content)
	}
}

func TestMarkInterrupted(t *testing.T) {
	s := newStore(t)
	running, _ := s.Create(Spec{Kind: "image.build", Title: "running"})
	done, _ := s.Create(Spec{Kind: "image.build", Title: "done"})
	if err := s.Finish(done.ID, StatusSucceeded, map[string]string{"digest": "x"}, "", ""); err != nil {
		t.Fatal(err)
	}
	n, err := s.MarkInterrupted()
	if err != nil || n != 1 {
		t.Fatalf("marked %d, %v", n, err)
	}
	got, _, _ := s.Get(running.ID)
	if got.Status != StatusInterrupted || got.FinishedAt == "" {
		t.Fatalf("running after restart = %+v", got)
	}
	got, _, _ = s.Get(done.ID)
	if got.Status != StatusSucceeded || string(got.Result) != `{"digest":"x"}` {
		t.Fatalf("done after restart = %+v", got)
	}
}

type recorder struct {
	mu     sync.Mutex
	events []events.Event
}

func (r *recorder) emit(e events.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recorder) statuses() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, e := range r.events {
		out = append(out, e.Data["status"].(string))
	}
	return out
}

func TestRunnerRecordsSuccess(t *testing.T) {
	s := newStore(t)
	rec := &recorder{}
	r := NewRunner(context.Background(), s, rec.emit, nil)
	h, err := r.Start(Spec{Kind: "image.build", Title: "ok"}, func(ctx context.Context, l *Log) (any, error) {
		l.Printf("building")
		return map[string]string{"digest": "abc"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if h.Record.Status != StatusRunning {
		t.Fatalf("started = %+v", h.Record)
	}
	result, err := h.Wait(context.Background())
	if err != nil || result.(map[string]string)["digest"] != "abc" {
		t.Fatalf("wait = %v %v", result, err)
	}
	r.Shutdown()
	got, _, _ := s.Get(h.Record.ID)
	if got.Status != StatusSucceeded || string(got.Result) != `{"digest":"abc"}` {
		t.Fatalf("record = %+v", got)
	}
	chunk, _ := s.ReadLog(h.Record.ID, 0)
	if !strings.Contains(chunk.Content, "building\n") || !strings.Contains(chunk.Content, "succeeded") {
		t.Fatalf("log = %q", chunk.Content)
	}
	statuses := rec.statuses()
	if len(statuses) < 2 || statuses[0] != StatusRunning || statuses[len(statuses)-1] != StatusSucceeded {
		t.Fatalf("events = %v", statuses)
	}
}

func TestRunnerRecordsFailureCode(t *testing.T) {
	s := newStore(t)
	r := NewRunner(context.Background(), s, nil, nil)
	cause := Failure{Code: "bad_imagefile", Message: "line 3: bad key"}
	h, err := r.Start(Spec{Kind: "image.build", Title: "bad"}, func(ctx context.Context, l *Log) (any, error) {
		return nil, cause
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Wait(context.Background()); !errors.Is(err, cause) {
		t.Fatalf("wait err = %v", err)
	}
	r.Shutdown()
	got, _, _ := s.Get(h.Record.ID)
	if got.Status != StatusFailed || got.ErrorCode != "bad_imagefile" || got.ErrorMessage != "line 3: bad key" {
		t.Fatalf("record = %+v", got)
	}
	chunk, _ := s.ReadLog(h.Record.ID, 0)
	if !strings.Contains(chunk.Content, "bad_imagefile: line 3: bad key") {
		t.Fatalf("log = %q", chunk.Content)
	}
}

func TestRunnerRecordsPanic(t *testing.T) {
	s := newStore(t)
	r := NewRunner(context.Background(), s, nil, nil)
	h, err := r.Start(Spec{Kind: "image.build", Title: "panic"}, func(ctx context.Context, l *Log) (any, error) {
		panic("boom")
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Wait(context.Background()); err == nil {
		t.Fatal("wait: no error")
	}
	r.Shutdown()
	got, _, _ := s.Get(h.Record.ID)
	if got.Status != StatusFailed || got.ErrorCode != "internal" || !strings.Contains(got.ErrorMessage, "boom") {
		t.Fatalf("record = %+v", got)
	}
}

func TestRunnerShutdownInterruptsRunning(t *testing.T) {
	s := newStore(t)
	r := NewRunner(context.Background(), s, nil, nil)
	started := make(chan struct{})
	h, err := r.Start(Spec{Kind: "image.build", Title: "slow"}, func(ctx context.Context, l *Log) (any, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	r.Shutdown()
	got, _, _ := s.Get(h.Record.ID)
	if got.Status != StatusInterrupted {
		t.Fatalf("record = %+v", got)
	}
	if _, err := r.Start(Spec{Kind: "image.build", Title: "late"}, nil); err == nil {
		t.Fatal("start after shutdown succeeded")
	}
}

func TestWaitReturnsOnCallerCancel(t *testing.T) {
	s := newStore(t)
	r := NewRunner(context.Background(), s, nil, nil)
	release := make(chan struct{})
	h, _ := r.Start(Spec{Kind: "image.build", Title: "slow"}, func(ctx context.Context, l *Log) (any, error) {
		<-release
		return nil, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := h.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait err = %v", err)
	}
	close(release)
	r.Shutdown()
	got, _, _ := s.Get(h.Record.ID)
	if got.Status != StatusSucceeded {
		t.Fatalf("a cancelled wait stopped the operation: %+v", got)
	}
}
