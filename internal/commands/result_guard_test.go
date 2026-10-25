package commands

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommandsReturnNoRawMaps keeps command results typed: a function with
// the handler result (any, error) must not return a map[string]any literal,
// make(map[string]any), or a variable it filled from one, so every result has
// a named struct the OpenAPI schema derives from.
func TestCommandsReturnNoRawMaps(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, pos := range rawMapReturns(parsed) {
			t.Errorf("%s returns a map[string]any result: return a named struct declared with registry.WithResult", fset.Position(pos))
		}
	}
}

// rawMapReturns returns the positions of raw map results in f.
func rawMapReturns(f *ast.File) []token.Pos {
	var out []token.Pos
	ast.Inspect(f, func(n ast.Node) bool {
		var typ *ast.FuncType
		var body *ast.BlockStmt
		switch fn := n.(type) {
		case *ast.FuncDecl:
			typ, body = fn.Type, fn.Body
		case *ast.FuncLit:
			typ, body = fn.Type, fn.Body
		}
		if body != nil && returnsResult(typ) {
			out = append(out, rawMapReturnsIn(body)...)
		}
		return true
	})
	return out
}

// returnsResult reports whether a function returns (any, error), the shape
// of a command handler result. Helpers that decode JSON into
// (map[string]any, error) are not results.
func returnsResult(typ *ast.FuncType) bool {
	if typ.Results == nil || len(typ.Results.List) != 2 {
		return false
	}
	if second, ok := typ.Results.List[1].Type.(*ast.Ident); !ok || second.Name != "error" {
		return false
	}
	ident, ok := typ.Results.List[0].Type.(*ast.Ident)
	return ok && ident.Name == "any"
}

// rawMapReturnsIn inspects one function body, skipping nested function
// literals, which rawMapReturns visits on their own.
func rawMapReturnsIn(body *ast.BlockStmt) []token.Pos {
	filled := map[string]bool{}
	var returns []*ast.ReturnStmt
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok && i < len(node.Rhs) && isRawMapValue(node.Rhs[i]) {
					filled[ident.Name] = true
				}
			}
		case *ast.ValueSpec:
			for i, name := range node.Names {
				if (node.Type != nil && isRawMap(node.Type)) || (i < len(node.Values) && isRawMapValue(node.Values[i])) {
					filled[name.Name] = true
				}
			}
		case *ast.ReturnStmt:
			returns = append(returns, node)
		}
		return true
	})
	var out []token.Pos
	for _, ret := range returns {
		if len(ret.Results) != 2 {
			continue
		}
		value := ret.Results[0]
		if ident, ok := value.(*ast.Ident); (ok && filled[ident.Name]) || isRawMapValue(value) {
			out = append(out, value.Pos())
		}
	}
	return out
}

// isRawMapValue reports a map[string]any composite literal or
// make(map[string]any, ...).
func isRawMapValue(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.CompositeLit:
		return v.Type != nil && isRawMap(v.Type)
	case *ast.CallExpr:
		fn, ok := v.Fun.(*ast.Ident)
		return ok && fn.Name == "make" && len(v.Args) > 0 && isRawMap(v.Args[0])
	}
	return false
}

func isRawMap(expr ast.Expr) bool {
	m, ok := expr.(*ast.MapType)
	if !ok {
		return false
	}
	key, keyOK := m.Key.(*ast.Ident)
	value, valueOK := m.Value.(*ast.Ident)
	if valueOK && value.Name == "any" {
		return keyOK && key.Name == "string"
	}
	iface, ifaceOK := m.Value.(*ast.InterfaceType)
	return keyOK && key.Name == "string" && ifaceOK && len(iface.Methods.List) == 0
}

// Catches a guard that misses a raw map result or flags a typed one.
func TestRawMapReturnsFindsMapResults(t *testing.T) {
	t.Parallel()
	src := `package p
type out struct{}
func literal() (any, error) { return map[string]any{"a": 1}, nil }
func made() (any, error) { return make(map[string]any), nil }
func filled() (any, error) { res := map[string]any{}; res["a"] = 1; return res, nil }
func declared() (any, error) { var res map[string]any; return res, nil }
func nested() (any, error) { f := func() (any, error) { return map[string]any{}, nil }; return f() }
func typed() (any, error) { return out{}, nil }
func helper() map[string]any { return map[string]any{} }
func schema() (map[string]any, string) { return map[string]any{}, "" }
func decode() (map[string]any, error) { var out map[string]any; return out, nil }
`
	f, err := parser.ParseFile(token.NewFileSet(), "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(rawMapReturns(f)); got != 5 {
		t.Fatalf("found %d raw map returns, want 5", got)
	}
}
