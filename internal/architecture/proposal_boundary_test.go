package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var proposalImportAllowlist = map[string]struct{}{
	"bytes":                              {},
	"encoding/json":                      {},
	"errors":                             {},
	"fmt":                                {},
	"io":                                 {},
	"regexp":                             {},
	"slices":                             {},
	"sort":                               {},
	"strings":                            {},
	ModulePath + "/internal/engineering": {},
}

func proposalImportAllowed(path string) bool {
	_, ok := proposalImportAllowlist[path]
	return ok
}

func proposalImportSpecAllowed(spec *ast.ImportSpec) bool {
	return spec.Name == nil && proposalImportAllowed(strings.Trim(spec.Path.Value, "\""))
}

func forbiddenProposalCallName(expr ast.Expr) (string, bool) {
	switch fun := expr.(type) {
	case *ast.Ident:
		if fun.Name == "print" || fun.Name == "println" {
			return fun.Name, true
		}
	case *ast.SelectorExpr:
		pkg, ok := fun.X.(*ast.Ident)
		if !ok || pkg.Name != "fmt" {
			return "", false
		}
		for _, prefix := range []string{"Print", "Fprint", "Scan", "Fscan"} {
			if strings.HasPrefix(fun.Sel.Name, prefix) {
				return "fmt." + fun.Sel.Name, true
			}
		}
	}
	return "", false
}

// TestProposalPackageIsPure pins AD-034's authority boundary mechanically:
// production proposal code may depend only on the exact set of pure value-
// function imports used by the reviewed implementation. In particular, adding
// another standard-library authority or non-determinism package fails here
// just as adding an external or internal dependency does.
func TestProposalPackageIsPure(t *testing.T) {
	dir := filepath.Join(ModuleRoot(), "internal", "proposal")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			imp := strings.Trim(spec.Path.Value, "\"")
			if !proposalImportSpecAllowed(spec) {
				if spec.Name != nil {
					t.Errorf("%s imports %q as %q; internal/proposal forbids aliased, dot and blank imports", entry.Name(), imp, spec.Name.Name)
					continue
				}
				t.Errorf("%s imports %q; internal/proposal imports must remain inside the exact reviewed allowlist", entry.Name(), imp)
			}
		}

		forbiddenNames := map[string]bool{
			"Repository": true, "Repositories": true, "UnitOfWork": true,
			"Clock": true, "Provider": true, "HTTPClient": true,
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if name, forbidden := forbiddenProposalCallName(call.Fun); forbidden {
					t.Errorf("%s calls forbidden I/O function %q", entry.Name(), name)
				}
			}
			id, ok := n.(*ast.Ident)
			if ok && forbiddenNames[id.Name] {
				t.Errorf("%s declares or uses forbidden authority identifier %q", entry.Name(), id.Name)
			}
			return true
		})
	}
}

func TestProposalCallGuardRejectsIO(t *testing.T) {
	for _, expression := range []string{
		`print("x")`,
		`println("x")`,
		`fmt.Print("x")`,
		`fmt.Printf("%s", "x")`,
		`fmt.Println("x")`,
		`fmt.Fprint(writer, "x")`,
		`fmt.Fprintf(writer, "%s", "x")`,
		`fmt.Fprintln(writer, "x")`,
		`fmt.Scan(&value)`,
		`fmt.Scanf("%s", &value)`,
		`fmt.Scanln(&value)`,
		`fmt.Fscan(reader, &value)`,
		`fmt.Fscanf(reader, "%s", &value)`,
		`fmt.Fscanln(reader, &value)`,
	} {
		expr, err := parser.ParseExpr(expression)
		if err != nil {
			t.Fatalf("parse %q: %v", expression, err)
		}
		call, ok := expr.(*ast.CallExpr)
		if !ok {
			t.Fatalf("%q did not parse as a call", expression)
		}
		if _, forbidden := forbiddenProposalCallName(call.Fun); !forbidden {
			t.Errorf("I/O call %q unexpectedly allowed", expression)
		}
	}

	for _, expression := range []string{
		`fmt.Errorf("invalid: %s", value)`,
		`fmt.Sprintf("%s", value)`,
		`fmt.Sscan(value, &target)`,
		`strings.TrimSpace(value)`,
	} {
		expr, err := parser.ParseExpr(expression)
		if err != nil {
			t.Fatalf("parse %q: %v", expression, err)
		}
		call := expr.(*ast.CallExpr)
		if name, forbidden := forbiddenProposalCallName(call.Fun); forbidden {
			t.Errorf("pure value call %q rejected as %q", expression, name)
		}
	}
}

func TestProposalImportAllowlistRejectsAuthorityPackages(t *testing.T) {
	for _, imp := range []string{
		"context",
		"crypto/rand",
		"database/sql",
		"math/rand/v2",
		"net/http",
		"os/exec",
		"plugin",
		"runtime",
		"syscall",
		"unsafe",
		"C",
		ModulePath + "/internal/application",
		"github.com/provider/sdk",
	} {
		if proposalImportAllowed(imp) {
			t.Errorf("authority or non-determinism import %q unexpectedly allowed", imp)
		}
	}

	for imp := range proposalImportAllowlist {
		if !proposalImportAllowed(imp) {
			t.Errorf("reviewed import %q unexpectedly rejected", imp)
		}
	}
}

func TestProposalImportAllowlistRejectsAliases(t *testing.T) {
	for _, declaration := range []string{
		`package proposal; import f "fmt"`,
		`package proposal; import . "fmt"`,
		`package proposal; import _ "fmt"`,
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "alias.go", declaration, 0)
		if err != nil {
			t.Fatalf("parse %q: %v", declaration, err)
		}
		if len(file.Imports) != 1 {
			t.Fatalf("%q imports = %d, want 1", declaration, len(file.Imports))
		}
		if proposalImportSpecAllowed(file.Imports[0]) {
			t.Errorf("aliased import in %q unexpectedly allowed", declaration)
		}
	}

	file, err := parser.ParseFile(token.NewFileSet(), "plain.go", `package proposal; import "fmt"`, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !proposalImportSpecAllowed(file.Imports[0]) {
		t.Error("plain reviewed fmt import unexpectedly rejected")
	}
}

// TestGeneratorHasOneValueInput makes the deliberately narrow M.6 seam
// exact: Generate receives one ContextPack value and returns Proposal,error.
// Adding context.Context, callbacks or optional dependencies changes the AST
// shape and fails this test.
func TestGeneratorHasOneValueInput(t *testing.T) {
	dir := filepath.Join(ModuleRoot(), "internal", "proposal")
	found := false
	err := walkGoFiles(dir, func(path string, file *ast.File) error {
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, raw := range gen.Specs {
				spec, ok := raw.(*ast.TypeSpec)
				if !ok || spec.Name.Name != "Generator" {
					continue
				}
				found = true
				iface, ok := spec.Type.(*ast.InterfaceType)
				if !ok || iface.Methods == nil || len(iface.Methods.List) != 1 {
					t.Errorf("Generator must be an interface with exactly one method")
					continue
				}
				method := iface.Methods.List[0]
				if len(method.Names) != 1 || method.Names[0].Name != "Generate" {
					t.Errorf("Generator's only method must be Generate")
					continue
				}
				fn, ok := method.Type.(*ast.FuncType)
				if !ok || fn.Params == nil || len(fn.Params.List) != 1 || !identType(fn.Params.List[0].Type, "ContextPack") {
					t.Errorf("Generator.Generate must receive exactly one ContextPack value")
					continue
				}
				if fn.Results == nil || len(fn.Results.List) != 2 || !identType(fn.Results.List[0].Type, "Proposal") || !identType(fn.Results.List[1].Type, "error") {
					t.Errorf("Generator.Generate must return exactly (Proposal, error)")
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("internal/proposal does not declare Generator")
	}
}

func identType(expr ast.Expr, want string) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == want
}
