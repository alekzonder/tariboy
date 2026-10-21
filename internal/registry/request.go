package registry

import (
	"context"
	"fmt"
	"maps"
	"strings"
)

// Request is one command call: the transport context and the bound arguments.
// The HTTP router passes the request context; a local call passes Background.
type Request struct {
	Ctx context.Context
	Params
}

// ParamError is an argument that is missing or has the wrong type. Every
// transport answers it with 400 and its Code.
type ParamError struct {
	Name string
	Code string // missing_param or invalid_param
	Msg  string
}

func (e *ParamError) Error() string { return e.Msg }

// Bind checks raw against the command's declared arguments and returns a copy
// in which every declared value has its Go type: string, int, bool, []int64,
// map[string]any, or []string for a repeatable string. An argument with an
// OpenAPI Schema keeps the shape it was sent in, and undeclared keys pass
// through unchanged. Bind is the one place where arguments are validated.
func Bind(cmd Command, raw map[string]any) (Params, error) {
	p := Params{}
	maps.Copy(p, raw)
	for _, arg := range cmd.Args {
		value := p[arg.Name]
		if value == nil || isEmptyScalarText(arg, value) {
			delete(p, arg.Name)
			if arg.Required {
				return nil, &ParamError{Name: arg.Name, Code: "missing_param",
					Msg: "missing required argument: " + arg.Name}
			}
			continue
		}
		if arg.Schema != nil {
			continue
		}
		bound, ok := bindValue(arg, value)
		if !ok {
			return nil, &ParamError{Name: arg.Name, Code: "invalid_param",
				Msg: fmt.Sprintf("argument %s must be %s", arg.Name, typeDescription(arg))}
		}
		p[arg.Name] = bound
	}
	return p, nil
}

// Call binds raw and runs the command's handler with ctx.
func Call(c *Ctx, ctx context.Context, cmd Command, raw map[string]any) (any, error) {
	return run(c, ctx, cmd, cmd.Handler, raw)
}

// CallLocal binds raw and runs the command's Local handler, in the CLI process.
func CallLocal(c *Ctx, ctx context.Context, cmd Command, raw map[string]any) (any, error) {
	return run(c, ctx, cmd, cmd.Local, raw)
}

func run(c *Ctx, ctx context.Context, cmd Command, h HandlerFunc, raw map[string]any) (any, error) {
	if h == nil {
		return nil, fmt.Errorf("command %s cannot run here", cmd.Path)
	}
	p, err := Bind(cmd, raw)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return h(c, Request{Ctx: ctx, Params: p})
}

// isEmptyScalarText treats an empty query value such as ?limit= as an omitted
// number or boolean.
func isEmptyScalarText(arg Arg, value any) bool {
	s, ok := value.(string)
	return ok && (arg.Type == Int || arg.Type == Bool) && strings.TrimSpace(s) == ""
}

func bindValue(arg Arg, value any) (any, bool) {
	switch arg.Type {
	case Int:
		n, ok := wholeNumber(value)
		if !ok || n != int64(int(n)) {
			return nil, false
		}
		return int(n), true
	case Bool:
		return boolValue(value)
	case IntegerList:
		return integerList(value)
	case JSONObject:
		m, ok := value.(map[string]any)
		return m, ok
	default:
		if s, ok := value.(string); ok {
			return s, true
		}
		if arg.Repeatable {
			return stringList(value)
		}
		return nil, false
	}
}

func integerList(value any) ([]int64, bool) {
	var items []any
	switch v := value.(type) {
	case []int64:
		return v, true
	case []any:
		items = v
	case string:
		if strings.TrimSpace(v) == "" {
			return []int64{}, true
		}
		for _, part := range strings.Split(v, ",") {
			items = append(items, part)
		}
	default:
		return nil, false
	}
	out := make([]int64, 0, len(items))
	for _, item := range items {
		n, ok := wholeNumber(item)
		if !ok {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

func stringList(value any) ([]string, bool) {
	switch v := value.(type) {
	case []string:
		return v, true
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

func typeDescription(arg Arg) string {
	switch arg.Type {
	case Int:
		return "a whole number"
	case Bool:
		return "true or false"
	case IntegerList:
		return "a list of whole numbers"
	case JSONObject:
		return "a JSON object"
	default:
		if arg.Repeatable {
			return "a string or a list of strings"
		}
		return "a string"
	}
}
