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

// TestProposalPackageIsPure pins AD-034's authority boundary mechanically:
// production proposal code may depend only on the standard library and the
// PEOS-free engineering package.  In particular, giving a deterministic
// generator a conveniently named repository, clock or transport helper is a
// build failure rather than a review convention.
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
			if imp == ModulePath+"/internal/engineering" {
				continue
			}
			// Standard-library import paths have no dotted first path
			// segment; every external module path does.
			first, _, _ := strings.Cut(imp, "/")
			if strings.Contains(first, ".") || strings.HasPrefix(imp, ModulePath+"/") {
				t.Errorf("%s imports %q; internal/proposal may import only stdlib and internal/engineering", entry.Name(), imp)
			}
			switch imp {
			case "context", "database/sql", "net", "net/http", "os", "time", "math/rand", "crypto/rand":
				t.Errorf("%s imports forbidden authority/non-determinism package %q", entry.Name(), imp)
			}
		}

		forbiddenNames := map[string]bool{
			"Repository": true, "Repositories": true, "UnitOfWork": true,
			"Clock": true, "Provider": true, "HTTPClient": true,
		}
		ast.Inspect(file, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if ok && forbiddenNames[id.Name] {
				t.Errorf("%s declares or uses forbidden authority identifier %q", entry.Name(), id.Name)
			}
			return true
		})
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
