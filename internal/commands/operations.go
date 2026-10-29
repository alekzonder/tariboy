package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/operations"
	"github.com/alekzonder/tariboy/internal/registry"
)

const (
	defaultOperationsPage = 50
	maxOperationsPage     = 200
	// operationPollInterval is how often the CLI reads a running operation's log.
	operationPollInterval = 300 * time.Millisecond
)

var errOperationsUnavailable = api.UserError{Code: "operations_unavailable", Msg: "background operations are unavailable", Status: http.StatusServiceUnavailable}

// operationStartArgs are the arguments of every command that starts an
// operation.
func operationStartArgs() []registry.Arg {
	return []registry.Arg{
		{Name: "wait", Flag: "wait", Type: registry.Bool, Help: "answer only when the operation ends; a failed build answers with its error"},
		{Name: "initiator", Flag: "initiator", Type: registry.String, Help: "who started the operation, as the operations list shows it: ui, cli, compose or auto"},
		{Name: "detach", Flag: "detach", Type: registry.Bool, Help: "CLI: print only the operation id and do not follow its log"},
	}
}

// startOperation runs run as a background operation and answers with the
// started operation, or with the finished one when the request sets wait.
func startOperation(c *registry.Ctx, p registry.Request, spec operations.Spec, run operations.RunFunc) (any, error) {
	if c.Operations == nil {
		return nil, errOperationsUnavailable
	}
	h, err := c.Operations.Start(spec, func(ctx context.Context, log *operations.Log) (any, error) {
		result, err := run(ctx, log)
		if err != nil {
			return nil, operationFailure(err)
		}
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	if !p.Bool("wait") {
		return OperationStarted{Operation: operationFrom(h.Record)}, nil
	}
	if _, err := h.Wait(p.Ctx); err != nil {
		var failure operations.Failure
		if errors.As(err, &failure) && failure.Cause != nil {
			return nil, failure.Cause
		}
		return nil, err
	}
	rec, _, err := c.Operations.Store().Get(h.Record.ID)
	if err != nil {
		return nil, err
	}
	return OperationStarted{Operation: operationFrom(rec)}, nil
}

// operationFailure keeps the API code of a build error for the operation.
func operationFailure(err error) error {
	var ue api.UserError
	if errors.As(err, &ue) {
		return operations.Failure{Code: ue.Code, Message: ue.Msg, Cause: ue}
	}
	return operations.Failure{Code: "build_failed", Message: err.Error(), Cause: err}
}

func operationFrom(rec operations.Record) Operation {
	op := Operation{
		ID: rec.ID, Kind: rec.Kind, Title: rec.Title, Status: rec.Status, Initiator: rec.Initiator, Params: rec.Params,
		ErrorCode: rec.ErrorCode, ErrorMessage: rec.ErrorMessage, CreatedAt: rec.CreatedAt, FinishedAt: rec.FinishedAt,
	}
	if len(rec.Result) == 0 {
		return op
	}
	switch rec.Kind {
	case "image.build":
		var result ImageBuildResult
		if json.Unmarshal(rec.Result, &result) == nil {
			op.ImageResult = &result
		}
	case "workflow.build":
		var result WorkflowImageBuildResult
		if json.Unmarshal(rec.Result, &result) == nil {
			op.WorkflowResult = &result
		}
	}
	return op
}

func operationStore(c *registry.Ctx) (*operations.Store, error) {
	if c.Operations == nil {
		return nil, errOperationsUnavailable
	}
	return c.Operations.Store(), nil
}

func operationID(p registry.Params) (int64, error) {
	id, err := strconv.ParseInt(p.String("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, api.UserError{Code: "bad_request", Msg: "operation id must be a positive number", Status: http.StatusBadRequest}
	}
	return id, nil
}

func operationNotFound(id int64) error {
	return api.UserError{Code: "not_found", Msg: fmt.Sprintf("operation %d not found", id), Status: http.StatusNotFound}
}

func operationLs() registry.Command {
	return registry.WithResult[OperationList](registry.Command{
		Path:    "operation.ls",
		Summary: "List background operations, newest first",
		Args: []registry.Arg{
			{Name: "page", Flag: "page", Type: registry.Int, Default: 1, Help: "page number, from 1"},
			{Name: "limit", Flag: "limit", Type: registry.Int, Default: defaultOperationsPage, Help: "operations per page, 1 to 200"},
		},
		HTTP:  &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/operations"},
		Local: operationLsLocal,
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			s, err := operationStore(c)
			if err != nil {
				return nil, err
			}
			page, limit := p.IntOr("page", 1), p.IntOr("limit", defaultOperationsPage)
			if page < 1 || limit < 1 || limit > maxOperationsPage {
				return nil, api.UserError{Code: "bad_request", Msg: "page must be at least 1 and limit 1 to 200", Status: http.StatusBadRequest}
			}
			records, total, err := s.List(page, limit)
			if err != nil {
				return nil, err
			}
			ops := make([]Operation, 0, len(records))
			for _, rec := range records {
				ops = append(ops, operationFrom(rec))
			}
			return OperationList{Operations: ops, Total: total, Page: page, Limit: limit}, nil
		},
	})
}

func operationShow() registry.Command {
	return registry.WithResult[Operation](registry.Command{
		Path:    "operation.show",
		Summary: "Show a background operation",
		Args:    []registry.Arg{{Name: "id", Type: registry.String, Required: true, Help: "operation id"}},
		HTTP:    &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/operations/{id}"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			s, err := operationStore(c)
			if err != nil {
				return nil, err
			}
			id, err := operationID(p.Params)
			if err != nil {
				return nil, err
			}
			rec, ok, err := s.Get(id)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, operationNotFound(id)
			}
			return operationFrom(rec), nil
		},
	})
}

func operationLog() registry.Command {
	return registry.WithResult[OperationLog](registry.Command{
		Path:    "operation.log",
		Summary: "Read the log of a background operation",
		Args: []registry.Arg{
			{Name: "id", Type: registry.String, Required: true, Help: "operation id"},
			{Name: "offset", Flag: "offset", Type: registry.Int, Help: "byte offset to read from"},
			{Name: "follow", Flag: "follow", Short: "f", Type: registry.Bool, Help: "CLI: keep printing the log until the operation ends"},
		},
		HTTP:  &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/operations/{id}/log"},
		Local: operationLogLocal,
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			s, err := operationStore(c)
			if err != nil {
				return nil, err
			}
			id, err := operationID(p.Params)
			if err != nil {
				return nil, err
			}
			// The status is read before the log: a finished operation has
			// closed its log, so this read sees all of it.
			rec, ok, err := s.Get(id)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, operationNotFound(id)
			}
			out := OperationLog{ID: id, Finished: rec.Status != operations.StatusRunning}
			chunk, err := s.ReadLog(id, int64(p.IntOr("offset", 0)))
			if errors.Is(err, os.ErrNotExist) {
				return out, nil
			}
			if err != nil {
				return nil, err
			}
			out.Offset, out.Next, out.Size, out.Content, out.Available = chunk.Offset, chunk.Next, chunk.Size, chunk.Content, true
			return out, nil
		},
	})
}

// operationCaller is the daemon API as the CLI-local operation verbs use it.
type operationCaller interface {
	Call(method, route string, body any) (json.RawMessage, error)
}

func operationCall[T any](call operationCaller, method, route string, body any) (T, error) {
	var out T
	raw, err := call.Call(method, route, body)
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal(raw, &out)
}

// followOperation prints the log of operation id from its start to out until
// the operation ends, and returns the finished operation. With follow false
// it prints the log written so far and returns.
func followOperation(ctx context.Context, call operationCaller, id int64, out io.Writer, follow bool) (Operation, error) {
	offset := int64(0)
	for {
		chunk, err := operationCall[OperationLog](call, http.MethodGet, fmt.Sprintf("/api/operations/%d/log", id), map[string]string{"offset": strconv.FormatInt(offset, 10)})
		if err != nil {
			return Operation{}, err
		}
		if _, err := io.WriteString(out, chunk.Content); err != nil {
			return Operation{}, err
		}
		offset = max(offset, chunk.Next)
		if chunk.Available && chunk.Next < chunk.Size {
			continue
		}
		if chunk.Finished || !follow {
			return operationCall[Operation](call, http.MethodGet, fmt.Sprintf("/api/operations/%d", id), nil)
		}
		select {
		case <-ctx.Done():
			return Operation{}, ctx.Err()
		case <-time.After(operationPollInterval):
		}
	}
}

// buildLocal is the CLI side of a build command: it starts the operation,
// prints its log to stderr until it ends and then prints the build result as
// the command did before builds became operations.
func buildLocal(route string) registry.HandlerFunc {
	return func(c *registry.Ctx, p registry.Request) (any, error) {
		return runBuildLocal(c, p, client.New(c.Socket), route)
	}
}

func runBuildLocal(c *registry.Ctx, p registry.Request, call operationCaller, route string) (any, error) {
	body := map[string]any{}
	for key, value := range p.Params {
		if key != "detach" && key != "wait" {
			body[key] = value
		}
	}
	body["initiator"] = "cli"
	started, err := operationCall[OperationStarted](call, http.MethodPost, route, body)
	if err != nil {
		return nil, err
	}
	id := started.Operation.ID
	if p.Bool("detach") {
		if c.JSON {
			return nil, printJSON(c.Out, started)
		}
		fmt.Fprintf(c.Out, "operation: %d\n", id)
		return nil, nil
	}
	fmt.Fprintf(c.ErrOut, "operation %d: %s\n", id, started.Operation.Title)
	op, err := followOperation(p.Ctx, call, id, c.ErrOut, true)
	if err != nil {
		if p.Ctx.Err() != nil {
			fmt.Fprintf(c.ErrOut, "stopped following operation %d; the build goes on in the daemon (tariboy operation log %d -f)\n", id, id)
			return nil, registry.ExitCode(1)
		}
		return nil, err
	}
	if op.Status != operations.StatusSucceeded {
		return nil, api.UserError{Code: op.ErrorCode, Msg: op.ErrorMessage}
	}
	var result any = op.ImageResult
	if op.WorkflowResult != nil {
		result = op.WorkflowResult
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	if c.JSON {
		fmt.Fprintln(c.Out, string(raw))
	} else {
		registry.PrintHuman(raw, c.Out)
	}
	return nil, nil
}

func printJSON(out io.Writer, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, string(raw))
	return nil
}

func operationLogLocal(c *registry.Ctx, p registry.Request) (any, error) {
	call := client.New(c.Socket)
	id, err := operationID(p.Params)
	if err != nil {
		return nil, err
	}
	if c.JSON {
		chunk, err := operationCall[OperationLog](call, http.MethodGet, fmt.Sprintf("/api/operations/%d/log", id), map[string]string{"offset": strconv.Itoa(p.IntOr("offset", 0))})
		if err != nil {
			return nil, err
		}
		return nil, printJSON(c.Out, chunk)
	}
	if _, err := followOperation(p.Ctx, call, id, c.Out, p.Bool("follow")); err != nil && p.Ctx.Err() == nil {
		return nil, err
	}
	return nil, nil
}

func operationLsLocal(c *registry.Ctx, p registry.Request) (any, error) {
	query := map[string]string{"page": strconv.Itoa(p.IntOr("page", 1)), "limit": strconv.Itoa(p.IntOr("limit", defaultOperationsPage))}
	list, err := operationCall[OperationList](client.New(c.Socket), http.MethodGet, "/api/operations", query)
	if err != nil {
		return nil, err
	}
	if c.JSON {
		return nil, printJSON(c.Out, list)
	}
	w := tabwriter.NewWriter(c.Out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tCREATED\tINITIATOR\tTITLE")
	for _, op := range list.Operations {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", op.ID, op.Status, op.CreatedAt, op.Initiator, op.Title)
	}
	if err := w.Flush(); err != nil {
		return nil, err
	}
	fmt.Fprintf(c.Out, "page %d, %d per page, %d operations in total\n", list.Page, list.Limit, list.Total)
	return nil, nil
}
