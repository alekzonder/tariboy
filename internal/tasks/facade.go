package tasks

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alekzonder/tariboy/internal/store"
)

// Service is the facade the daemon and its API use while the task code is
// split by concern. It embeds the four services, so their methods keep their
// names on Service; each service reaches another only through core's fields.
type Service struct {
	*core
	*TaskService
	*WorkflowEngine
	*RunService
	*Admin
}

// TaskService owns tasks as such: create, read, list, update, move, the
// tree, comments, relations, transfer, and activity.
type TaskService struct{ *core }

// WorkflowEngine owns workflow tasks: statuses and transitions, pauses,
// dispatch and holder checks, bindings and pools, artifacts, and triggers. It
// holds the service's in-memory workflow state.
type WorkflowEngine struct {
	*core
	holderFailuresMu                         sync.Mutex
	holderFailures                           map[int64]string // task id -> logged manifest failure of CheckHolders
	workflowIngressEnabled                   atomic.Bool
	workflowIngressAfterTargetCount          func()
	workflowActivationAfterWriterReservation func()
}

// RunService owns workflow script runs: checks, watches, sources, and their
// logs.
type RunService struct{ *core }

// Admin owns the customer's administration: queues, queue secrets and env,
// principals, the event feed, and notifications.
type Admin struct{ *core }

// core is what every service shares: the database, the clock, the signals,
// and a reference to each service for the calls that cross them.
type core struct {
	db               *sql.DB
	read             *sql.DB
	customer         string
	clock            func() time.Time
	hub              *Hub
	goalSignal       func()
	workflowResolver WorkflowResolver
	runBaseDir       string
	globalEnv        func() ([]string, error)
	log              *slog.Logger

	taskSvc *TaskService
	engine  *WorkflowEngine
	runs    *RunService
	admin   *Admin
}

func NewService(db *sql.DB, customer string, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	c := &core{db: db, customer: strings.TrimPrefix(strings.TrimSpace(customer), "user:"), clock: clock}
	c.taskSvc, c.engine, c.runs, c.admin = &TaskService{c}, &WorkflowEngine{core: c}, &RunService{c}, &Admin{c}
	s := &Service{core: c, TaskService: c.taskSvc, WorkflowEngine: c.engine, RunService: c.runs, Admin: c.admin}
	s.refreshWorkflowIngressEnabled(context.Background())
	return s
}

// SetReader makes ListTasks, which the UI polls, read through a read-only
// connection pool instead of the writer connection.
func (s *core) SetReader(db *sql.DB) { s.read = db }

func (s *core) reader() *sql.DB {
	if s.read != nil {
		return s.read
	}
	return s.db
}

func (s *core) CustomerLogin() string { return s.customer }

func (s *core) SetHub(hub *Hub) { s.hub = hub }

// SetRunBaseDir sets the daemon base directory under which workflow script runs
// keep their files, so a run's log is read only from where the worker wrote it.
func (s *core) SetRunBaseDir(dir string) { s.runBaseDir = dir }

func (s *core) SetGoalSignal(signal func()) { s.goalSignal = signal }

// SetLogger sets where the service logs what it skips instead of failing a
// whole call, such as a pending script run whose job cannot be built.
func (s *core) SetLogger(log *slog.Logger) { s.log = log }

func (s *core) logger() *slog.Logger {
	if s.log == nil {
		return slog.Default()
	}
	return s.log
}

func (s *core) signal() {
	if s.hub != nil {
		s.hub.Nudge()
	}
	if s.goalSignal != nil {
		s.goalSignal()
	}
}

func (s *core) now() string {
	return store.FormatTime(s.clock())
}
