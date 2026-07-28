package http

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestErrorMappingIsExhaustive (FF-018 §8.4) parses
// internal/application/errors.go with go/ast -- the technique
// internal/architecture already uses -- and asserts every exported Err*
// sentinel declared there has a row in errorMappings. Comparison is by the
// sentinel's own message text, extracted straight from the errors.New(...)
// literal, rather than a hand-maintained name-to-value lookup table that
// could itself drift out of sync. Adding a sentinel without classifying it
// fails the build.
func TestErrorMappingIsExhaustive(t *testing.T) {
	declared := parseApplicationSentinels(t)
	if len(declared) == 0 {
		t.Fatal("found no Err* sentinels in internal/application/errors.go; the parser is broken")
	}

	mappedByMessage := make(map[string]string, len(errorMappings)) // message -> code
	for _, m := range errorMappings {
		mappedByMessage[m.err.Error()] = m.code
	}
	if len(mappedByMessage) != len(errorMappings) {
		t.Errorf("errorMappings has %d rows but only %d distinct messages; a sentinel is mapped twice", len(errorMappings), len(mappedByMessage))
	}

	for name, message := range declared {
		if _, ok := mappedByMessage[message]; !ok {
			t.Errorf("application.%s (%q) has no entry in errorMappings (FF-018 §8.4)", name, message)
		}
	}
	if len(declared) != len(errorMappings) {
		t.Errorf("internal/application declares %d Err* sentinels, but errorMappings has %d rows -- one list is stale", len(declared), len(errorMappings))
	}
}

// parseApplicationSentinels returns every exported Err* sentinel declared
// in internal/application/errors.go, name to its errors.New(...) message
// text, read directly from the source so no runtime value resolution --
// and no hand-maintained lookup table -- is needed.
func parseApplicationSentinels(t *testing.T) map[string]string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	moduleRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	errorsGo := filepath.Join(moduleRoot, "internal", "application", "errors.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, errorsGo, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", errorsGo, err)
	}

	out := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, id := range vs.Names {
			if !strings.HasPrefix(id.Name, "Err") || i >= len(vs.Values) {
				continue
			}
			call, ok := vs.Values[i].(*ast.CallExpr)
			if !ok {
				continue
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "New" {
				continue
			}
			if len(call.Args) != 1 {
				t.Fatalf("%s: errors.New for %s does not take exactly one literal argument; parseApplicationSentinels cannot read it", errorsGo, id.Name)
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				t.Fatalf("%s: errors.New for %s is not a plain string literal; parseApplicationSentinels cannot read it", errorsGo, id.Name)
			}
			message, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("%s: unquoting message literal for %s: %v", errorsGo, id.Name, err)
			}
			out[id.Name] = message
		}
		return true
	})
	return out
}
