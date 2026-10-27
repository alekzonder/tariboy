package tasks

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// The request structs of the native task commands. Their tags are described
// on Spec; the Validate methods hold the command-line rules that span fields.

// KeyRequest names one task.
type KeyRequest struct {
	Key string `json:"key" pos:"0" label:"task key" required:"cli"`
}

// ListRequest lists tasks: list and mine on the command line and the agent
// socket, and GET /api/tasks.
type ListRequest struct {
	Queue      string   `json:"queue" flag:"queue" help:"Queue prefix"`
	Status     []string `json:"status" flag:"status" help:"Comma-separated statuses; a workflow status or a category"`
	Category   []string `json:"category" flag:"category" help:"Comma-separated categories"`
	Priority   []string `json:"priority" flag:"priority" help:"Comma-separated priorities"`
	Assignee   string   `json:"assignee" flag:"assignee" help:"Task assignee"`
	Author     string   `json:"author" flag:"author" help:"Author principal"`
	Customer   string   `json:"customer" flag:"customer" help:"Customer principal"`
	Group      string   `json:"group" flag:"group" help:"Task group"`
	Parent     string   `json:"parent" flag:"parent" help:"Parent task key whose direct children to list"`
	Subtree    string   `json:"subtree" flag:"subtree" help:"Task key whose descendants to list"`
	WaitingFor string   `json:"waiting_for" flag:"waiting-for" help:"Principal a pending answer waits for"`
	Blocked    *bool    `json:"blocked" flag:"blocked" help:"true lists only blocked tasks, false only unblocked ones"`
	Text       string   `json:"text" flag:"text" help:"Search text in the key, title, and description"`
	View       string   `json:"status_view" flag:"view" help:"active, closed, or all"`
	Sort       string   `json:"sort" flag:"sort" help:"tree, updated, created, or priority"`
	Limit      int      `json:"limit" flag:"limit" min:"1" max:"500" help:"Maximum tasks in a page"`
	After      string   `json:"after" flag:"after" help:"Task key to continue after"`
	ScopeAgent string   `json:"scope_agent" help:"Agent whose visible tasks to list"`
	// The tools socket scrubs author and customer from every body, so an
	// agent's filters arrive under these keys.
	ByAuthor   string `json:"by_author" http:"-"`
	ByCustomer string `json:"by_customer" http:"-"`
}

// Validate checks the list values before any call; without --limit a page
// holds 50 tasks.
func (r *ListRequest) Validate() error {
	if r.Parent != "" && r.Subtree != "" {
		return errors.New("--parent cannot be combined with --subtree")
	}
	if err := oneOf("--sort", []string{r.Sort}, "tree", "updated", "created", "priority"); err != nil {
		return err
	}
	if err := oneOf("--view", []string{r.View}, "active", "closed", "all"); err != nil {
		return err
	}
	if err := oneOf("--priority", r.Priority, "P0", "P1", "P2", "P3"); err != nil {
		return err
	}
	if err := oneOf("--category", r.Category, StatusOpen, StatusInProgress, StatusWaitCustomer, StatusDone, StatusCancelled); err != nil {
		return err
	}
	if r.Limit == 0 {
		r.Limit = 50
	}
	return nil
}

// oneOf refuses a non-empty value outside allowed.
func oneOf(flag string, values []string, allowed ...string) error {
	for _, value := range values {
		if value != "" && !slices.Contains(allowed, value) {
			return fmt.Errorf("%s must be %s, or %s, got %q", flag, strings.Join(allowed[:len(allowed)-1], ", "), allowed[len(allowed)-1], value)
		}
	}
	return nil
}

// Filter is the ListTasks filter of an operator request.
func (r *ListRequest) Filter() ListFilter {
	filter := ListFilter{
		Queue: r.Queue, Statuses: r.Status, Categories: r.Category, StatusView: r.View,
		Assignee: r.Assignee, Author: r.Author, Customer: r.Customer, Group: r.Group,
		Text: r.Text, WaitingFor: r.WaitingFor, ScopeAgent: r.ScopeAgent,
		ParentKey: r.Parent, SubtreeKey: r.Subtree, Sort: r.Sort, Blocked: r.Blocked,
		Limit: r.Limit, AfterKey: r.After,
	}
	for _, priority := range r.Priority {
		filter.Priorities = append(filter.Priorities, Priority(priority))
	}
	return filter
}

// ReadyRequest lists or claims eligible flexible tasks.
type ReadyRequest struct {
	Queue          string `json:"queue" flag:"queue"`
	Limit          int    `json:"limit" flag:"limit"`
	IdempotencyKey string `json:"idempotency_key" flag:"idempotency-key"`
	Claim          bool   `json:"claim" flag:"claim"`
}

// CreateRequest creates a root or child task.
type CreateRequest struct {
	Queue          string `json:"queue" flag:"queue" help:"Queue prefix for a root task"`
	ParentKey      string `json:"parent_key" flag:"parent" help:"Parent task key"`
	Title          string `json:"title" flag:"title" required:"true" help:"Task title"`
	Description    string `json:"description" flag:"description" help:"Task description"`
	PullRequest    string `json:"pull_request" flag:"pull-request" help:"Canonical pull request URL"`
	Assignee       string `json:"assignee" flag:"assignee" help:"Task assignee"`
	Group          string `json:"group" flag:"group" help:"Task group"`
	Priority       string `json:"priority" flag:"priority" enum:"P0,P1,P2,P3" help:"Task priority"`
	IdempotencyKey string `json:"idempotency_key" flag:"idempotency-key" help:"Stable retry key"`
}

func (r *CreateRequest) Validate() error {
	if r.Queue == "" && r.ParentKey == "" {
		return errors.New("--queue or --parent is required")
	}
	return nil
}

func (r *CreateRequest) Input() CreateTaskInput {
	return CreateTaskInput{
		Queue: r.Queue, ParentKey: r.ParentKey, Title: r.Title, Description: r.Description,
		PullRequest: r.PullRequest, Assignee: r.Assignee, Group: r.Group,
		Priority: Priority(r.Priority), IdempotencyKey: r.IdempotencyKey,
	}
}

// UpdateRequest changes the supplied fields of a task. The operator client
// and the agent socket read the revision when it is not given.
type UpdateRequest struct {
	Key               string  `json:"key" pos:"0" label:"task key" required:"cli"`
	Title             *string `json:"title" flag:"title" raw:"true" help:"Task title"`
	Description       *string `json:"description" flag:"description" raw:"true" help:"Task description"`
	Status            *string `json:"status" flag:"status" raw:"true" enum:"open,in_progress,wait_customer,done,cancelled" help:"Task status"`
	PullRequest       *string `json:"pull_request" flag:"pull-request" raw:"true" clear:"true" help:"Canonical pull request URL (empty clears it)"`
	Assignee          *string `json:"assignee" flag:"assignee" raw:"true" help:"Task assignee"`
	ManualBlockReason *string `json:"manual_block_reason" flag:"manual-block-reason" raw:"true" clear:"true" help:"Manual block reason"`
	Priority          *string `json:"priority" flag:"priority" help:"Task priority"`
	Revision          int64   `json:"revision" flag:"revision" required:"http" help:"Expected current revision"`
}

func (r *UpdateRequest) Input() UpdateTaskInput {
	return UpdateTaskInput{
		Title: r.Title, Description: r.Description, Status: r.Status, PullRequest: r.PullRequest,
		Assignee: r.Assignee, ManualBlockReason: r.ManualBlockReason,
		Priority: (*Priority)(r.Priority), Revision: r.Revision,
	}
}

// AssignRequest assigns a flexible task to an agent.
type AssignRequest struct {
	Key      string `json:"key" pos:"0" label:"task key" required:"cli"`
	Assignee string `json:"assignee" pos:"1" label:"assignee" required:"cli"`
	Revision int64  `json:"revision" flag:"revision"`
}

// CommentRequest adds a comment, given as --body, as the words after the key,
// or read by the client from File or stdin when neither is given.
type CommentRequest struct {
	Key            string   `json:"key" pos:"0" label:"task key" required:"cli"`
	Body           string   `json:"body" flag:"body" help:"Markdown comment text"`
	Text           []string `json:"-" pos:"1+"`
	File           string   `json:"-" flag:"file"`
	IdempotencyKey string   `json:"idempotency_key" flag:"idempotency-key" help:"Stable retry key"`
}

func (r *CommentRequest) Validate() error {
	if r.File != "" && (r.Body != "" || len(r.Text) > 0) {
		return errors.New("pass TEXT, --body, or --file, not more than one")
	}
	if r.Body != "" && len(r.Text) > 0 {
		return fmt.Errorf("unexpected argument: %s", r.Text[0])
	}
	given := r.Body != "" || len(r.Text) > 0
	if r.Body == "" {
		r.Body = strings.TrimSpace(strings.Join(r.Text, " "))
	}
	r.Text = nil
	if given && r.Body == "" {
		return errors.New("comment text is required")
	}
	return nil
}

func (r *CommentRequest) Input() AddCommentInput {
	return AddCommentInput{Body: r.Body, IdempotencyKey: r.IdempotencyKey}
}

// AskRequest asks a principal a question as a comment that mentions it. The
// client reads the question from File or stdin when no words follow the
// principal.
type AskRequest struct {
	Key            string   `json:"key" pos:"0" label:"task key" required:"cli"`
	Principal      string   `json:"principal" pos:"1" label:"principal" required:"cli"`
	Question       []string `json:"-" pos:"2+"`
	File           string   `json:"-" flag:"file"`
	Body           string   `json:"body"`
	IdempotencyKey string   `json:"idempotency_key" flag:"idempotency-key"`
}

func (r *AskRequest) Validate() error {
	if r.File != "" && len(r.Question) > 0 {
		return errors.New("pass the question or --file, not both")
	}
	r.Body, r.Question = strings.Join(r.Question, " "), nil
	return nil
}

// MoveRequest reparents, reorders, or detaches a task. --to-root sends an
// empty parent.
type MoveRequest struct {
	Key       string  `json:"key" pos:"0" label:"task key" required:"cli"`
	ParentKey *string `json:"parent_key" flag:"parent" help:"New parent task key; empty detaches the task"`
	BeforeKey string  `json:"before_key" flag:"before" help:"Sibling task key to insert before"`
	ToRoot    bool    `json:"-" flag:"to-root"`
	Revision  int64   `json:"revision" flag:"revision" help:"Expected current revision"`
}

func (r *MoveRequest) Validate() error {
	if !r.ToRoot {
		if r.ParentKey == nil && r.BeforeKey == "" {
			return errors.New("pass --parent, --before, or --to-root to detach")
		}
		return nil
	}
	if r.ParentKey != nil || r.BeforeKey != "" {
		return errors.New("--to-root cannot be combined with --parent or --before")
	}
	root := ""
	r.ParentKey = &root
	return nil
}

func (r *MoveRequest) Input() MoveInput {
	in := MoveInput{BeforeKey: r.BeforeKey, Revision: r.Revision}
	if r.ParentKey != nil {
		in.ParentKey = *r.ParentKey
	}
	return in
}

// RelateRequest links two related tasks. Over HTTP it adds a relation of any
// type; the operator client sends related, and the agent action ignores it.
type RelateRequest struct {
	Key            string `json:"key" pos:"0" label:"task key" required:"cli"`
	TargetKey      string `json:"target_key" pos:"1" label:"related task key" required:"cli" help:"Other task of the relation"`
	Type           string `json:"type" help:"Relation type: blocks or related"`
	Revision       int64  `json:"revision" flag:"revision" help:"Expected current revision"`
	IdempotencyKey string `json:"idempotency_key" flag:"idempotency-key" help:"Stable retry key"`
}

func (r *RelateRequest) Input() RelationInput {
	return RelationInput{TargetKey: r.TargetKey, Type: r.Type, Revision: r.Revision, IdempotencyKey: r.IdempotencyKey}
}

// BlockRequest makes the task Key wait for BlockerKey. The relation and its
// revision belong to the blocker.
type BlockRequest struct {
	Key            string `json:"key" pos:"0" label:"task key" required:"cli"`
	BlockerKey     string `json:"blocker_key" flag:"by" required:"cli"`
	Revision       int64  `json:"revision" flag:"revision"`
	IdempotencyKey string `json:"idempotency_key" flag:"idempotency-key"`
}

// DoneRequest completes a flexible task.
type DoneRequest struct {
	Key            string `json:"key" pos:"0" label:"task key" required:"cli"`
	Revision       int64  `json:"revision" flag:"revision" help:"Expected current revision"`
	CompleteAnyway bool   `json:"complete_anyway" flag:"complete-anyway" help:"Complete despite active descendants"`
}

func (r *DoneRequest) Input() CompleteInput {
	return CompleteInput{Revision: r.Revision, CompleteAnyway: r.CompleteAnyway}
}

// AdvanceRequest declares an outcome for a workflow task. NoWait is a client
// option and never part of the payload.
type AdvanceRequest struct {
	Key     string `json:"key" pos:"0" label:"task key" required:"cli"`
	Outcome string `json:"outcome" flag:"outcome" required:"true" help:"Outcome to declare"`
	Message string `json:"message" flag:"message" raw:"true" help:"Message recorded with the transition"`
	From    string `json:"from" flag:"from" help:"Status the caller believes the task is in; a different current status is refused with status_changed"`
	NoWait  bool   `json:"-" flag:"no-wait"`
}

func (r *AdvanceRequest) Input() AdvanceInput {
	return AdvanceInput{Outcome: r.Outcome, Message: r.Message, From: r.From}
}

// ArtifactSetRequest stores an artifact value. Without VALUE the client
// reads it from File or stdin.
type ArtifactSetRequest struct {
	Key   string  `json:"key" pos:"0" label:"task key" required:"cli"`
	Name  string  `json:"name" pos:"1" label:"artifact name" required:"cli"`
	Value *string `json:"value" pos:"2" raw:"true" help:"Artifact value, stored as given"`
	File  string  `json:"-" flag:"file"`
}

func (r *ArtifactSetRequest) Validate() error {
	if r.Value != nil && r.File != "" {
		return errors.New("pass VALUE or --file, not both")
	}
	return nil
}

// Text is the value to store; an absent value is empty.
func (r *ArtifactSetRequest) Text() string {
	if r.Value == nil {
		return ""
	}
	return *r.Value
}

// ArtifactRequest names one artifact of a task.
type ArtifactRequest struct {
	Key  string `json:"key" pos:"0" label:"task key" required:"cli"`
	Name string `json:"name" pos:"1" label:"artifact name" required:"cli"`
}

// RunLogRequest reads the tail of a script run's log.
type RunLogRequest struct {
	Key      string `json:"key" pos:"0" label:"task key" required:"cli"`
	ID       int64  `json:"id" pos:"1" label:"run id" required:"cli" min:"1"`
	MaxBytes int    `json:"max_bytes" flag:"max-bytes" min:"1" help:"Largest log tail to return in bytes (default 65536, maximum 1048576)"`
}

// RequeueRequest moves a task to another queue as a new task.
type RequeueRequest struct {
	Key      string `json:"key" pos:"0" label:"task key" required:"cli"`
	Queue    string `json:"queue" flag:"queue" required:"true" help:"Target queue prefix"`
	Revision int64  `json:"revision" flag:"revision" help:"Expected current revision"`
}

func (r *RequeueRequest) Input() RequeueInput {
	return RequeueInput{Queue: r.Queue, Revision: r.Revision}
}

// RequestRef names one transition request of a task.
type RequestRef struct {
	Key string `json:"key"`
	ID  int64  `json:"id"`
}

// WorkflowMoveRequest moves a workflow task to another status.
type WorkflowMoveRequest struct {
	Key    string `json:"key" pos:"0" label:"task key" required:"cli"`
	To     string `json:"to" flag:"to" required:"true" help:"Target status id"`
	Reason string `json:"reason" flag:"reason" required:"true" help:"Why the task is moved"`
}

// WorkflowUpgradeRequest restarts a workflow task at the version its queue is
// bound to.
type WorkflowUpgradeRequest struct {
	Key    string `json:"key" pos:"0" label:"task key" required:"cli"`
	Reason string `json:"reason" flag:"reason" help:"Why the task is upgraded"`
}

// ResumeRequest resolves the pause of a workflow task.
type ResumeRequest struct {
	Key      string `json:"key" pos:"0" label:"task key" required:"cli"`
	Decision string `json:"decision" flag:"decision" required:"http" enum:"continue,release" help:"continue resumes the task in its current status with counters reset; release dispatches the task to another pool member"`
}

func (r *ResumeRequest) Validate() error {
	if r.Decision != ResumeContinue && r.Decision != ResumeRelease {
		return errors.New("--decision must be continue or release")
	}
	return nil
}

// ReassignRequest hands the pool status of a workflow task to another member.
type ReassignRequest struct {
	Key   string `json:"key" pos:"0" label:"task key" required:"cli"`
	Agent string `json:"agent" flag:"agent" required:"true" help:"Pool member that takes over the current status"`
}

// RetryRequest retries a failed transition request; without ID the client
// names the task's latest one.
type RetryRequest struct {
	Key string `json:"key" pos:"0" label:"task key" required:"cli"`
	ID  int64  `json:"id" pos:"1" label:"request id" min:"1"`
}
