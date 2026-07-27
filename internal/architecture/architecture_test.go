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

// TestDomainDoesNotImportPEOS (FF-012 §12).
func TestDomainDoesNotImportPEOS(t *testing.T) {
	assertNoTransitivePEOS(t, ModulePath+"/internal/domain")
}

// TestEngineeringDoesNotImportPEOS (FF-012 §12).
func TestEngineeringDoesNotImportPEOS(t *testing.T) {
	assertNoTransitivePEOS(t, ModulePath+"/internal/engineering")
}

// TestApplicationDoesNotImportPEOS (FF-012 §12).
func TestApplicationDoesNotImportPEOS(t *testing.T) {
	assertNoTransitivePEOS(t, ModulePath+"/internal/application")
}

// TestInfrastructureDoesNotImportPEOS (FF-012 §12).
func TestInfrastructureDoesNotImportPEOS(t *testing.T) {
	assertNoTransitivePEOS(t, ModulePath+"/internal/infrastructure/memory")
}

func assertNoTransitivePEOS(t *testing.T, importPath string) {
	t.Helper()
	all, err := InternalPackages()
	if err != nil {
		t.Fatal(err)
	}
	imports := TransitiveImports(importPath, all)
	if ImportsPEOS(imports) {
		t.Errorf("%s transitively imports PEOS; its import set is %v", importPath, sortedKeys(imports))
	}
}

// TestOnlyIntegrationPackageImportsPEOS (FF-012 §12): exactly one package
// in the module directly imports the PEOS SDK.
func TestOnlyIntegrationPackageImportsPEOS(t *testing.T) {
	all, err := InternalPackages()
	if err != nil {
		t.Fatal(err)
	}
	var importers []string
	for _, p := range all {
		for _, imp := range p.Imports {
			if strings.HasPrefix(imp, "github.com/aleka7sk/PEOS") {
				importers = append(importers, p.ImportPath)
				break
			}
		}
	}
	if len(importers) != 1 || importers[0] != ModulePath+"/internal/engineering/peos" {
		t.Errorf("packages directly importing PEOS = %v, want exactly [%s]", importers, ModulePath+"/internal/engineering/peos")
	}
}

// TestApplicationDoesNotImportIntegration (FF-012 §12): application
// consumes the EngineeringRecorder port it declares itself, never the
// engineering/peos package.
func TestApplicationDoesNotImportIntegration(t *testing.T) {
	all, err := InternalPackages()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range all {
		if p.ImportPath != ModulePath+"/internal/application" {
			continue
		}
		for _, imp := range p.Imports {
			if imp == ModulePath+"/internal/engineering/peos" {
				t.Error("internal/application must not import internal/engineering/peos")
			}
		}
	}
}

// TestEngineeringDoesNotImportItsChild (FF-012 §12).
func TestEngineeringDoesNotImportItsChild(t *testing.T) {
	all, err := InternalPackages()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range all {
		if p.ImportPath != ModulePath+"/internal/engineering" {
			continue
		}
		for _, imp := range p.Imports {
			if imp == ModulePath+"/internal/engineering/peos" {
				t.Error("internal/engineering must not import its own peos child package")
			}
		}
	}
}

// TestNoPEOSTypeIsCopied (FF-012 §12): no FeatureForge struct reproduces
// the field set of a listed PEOS type. Approximated by asserting no
// FeatureForge type declaration outside engineering/peos uses any of these
// exact exported type names -- which would only be possible by importing
// PEOS (already forbidden) or by literally duplicating the name, both of
// which this test would catch via TestNoShadowStructNames below. This test
// additionally scans for structurally suspicious field-name clusters that
// would suggest a shadow struct (e.g. a type with fields named exactly
// ArtifactID, RevisionID, Origin, Provenance, Integrity together).
func TestNoPEOSTypeIsCopied(t *testing.T) {
	suspiciousFieldSets := [][]string{
		{"ArtifactID", "RevisionID", "Origin", "Provenance", "Integrity"},
		{"ClaimType", "Subject", "Scope", "Outcome", "Criteria"},
	}
	err := walkGoFiles(filepath.Join(ModuleRoot(), "internal"), func(path string, file *ast.File) error {
		if strings.Contains(filepath.ToSlash(path), "/internal/engineering/peos/") {
			return nil // the one package allowed to name PEOS-shaped fields
		}
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok || st.Fields == nil {
				return true
			}
			var names []string
			for _, f := range st.Fields.List {
				for _, name := range f.Names {
					names = append(names, name.Name)
				}
			}
			for _, set := range suspiciousFieldSets {
				if containsAll(names, set) {
					t.Errorf("%s: type %s's fields %v resemble a copied PEOS type", path, ts.Name.Name, set)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestNoShadowStructNames (FF-012 §12).
func TestNoShadowStructNames(t *testing.T) {
	forbidden := map[string]bool{
		"Artifact": true, "ArtifactRevision": true, "Claim": true, "Requirement": true,
		"Decision": true, "ExecutionRecord": true, "StateAssignment": true,
		"Provenance": true, "Representation": true,
	}
	err := walkGoFiles(filepath.Join(ModuleRoot(), "internal"), func(path string, file *ast.File) error {
		if strings.Contains(filepath.ToSlash(path), "/internal/engineering/peos/") {
			return nil
		}
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			if forbidden[ts.Name.Name] {
				t.Errorf("%s: type named %q shadows a PEOS type name", path, ts.Name.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestNoForbiddenPackageNames (FF-012 §12).
func TestNoForbiddenPackageNames(t *testing.T) {
	forbidden := map[string]bool{
		"workflow": true, "engine": true, "framework": true, "shared": true,
		"common": true, "util": true, "pkg": true, "integration": true, "core": true,
	}
	all, err := InternalPackages()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range all {
		name := filepath.Base(p.Dir)
		if forbidden[name] {
			t.Errorf("package directory %q uses a forbidden name", p.Dir)
		}
	}
}

// TestNoOperationalScenarioEntity (FF-012 §12): no type, field, or package
// is named for a scenario concept -- those are content, never code.
func TestNoOperationalScenarioEntity(t *testing.T) {
	forbidden := []string{"teacher", "student", "lesson", "homework", "attachment", "notification"}
	err := walkGoFiles(filepath.Join(ModuleRoot(), "internal"), func(path string, file *ast.File) error {
		ast.Inspect(file, func(n ast.Node) bool {
			var name string
			switch decl := n.(type) {
			case *ast.TypeSpec:
				name = decl.Name.Name
			case *ast.Field:
				for _, id := range decl.Names {
					checkForbiddenSubstring(t, path, id.Name, forbidden)
				}
				return true
			default:
				return true
			}
			checkForbiddenSubstring(t, path, name, forbidden)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func checkForbiddenSubstring(t *testing.T, path, name string, forbidden []string) {
	t.Helper()
	lower := strings.ToLower(name)
	for _, bad := range forbidden {
		if strings.Contains(lower, bad) {
			t.Errorf("%s: identifier %q names an operational scenario concept (%q); scenario content must never become code", path, name, bad)
		}
	}
}

// TestNoDerivedStateOnFeatureCard (FF-012 §12).
func TestNoDerivedStateOnFeatureCard(t *testing.T) {
	forbidden := []string{"revision", "readiness", "lifecycle", "requirement", "decision", "claim", "status", "current"}
	path := filepath.Join(ModuleRoot(), "internal", "domain", "featurecard.go")
	err := parseAndInspect(path, func(file *ast.File) {
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || ts.Name.Name != "FeatureCard" {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok || st.Fields == nil {
				return true
			}
			for _, f := range st.Fields.List {
				for _, id := range f.Names {
					checkForbiddenSubstring(t, path, id.Name, forbidden)
				}
			}
			return true
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestOperationalTypesDoNotEmbedEnvelopes (FF-012 §12).
func TestOperationalTypesDoNotEmbedEnvelopes(t *testing.T) {
	envelopeTypes := map[string]bool{"ArtifactEnvelope": true, "RevisionEnvelope": true, "RecordEnvelope": true}
	path := filepath.Join(ModuleRoot(), "internal", "domain")
	err := walkGoFiles(path, func(file string, f *ast.File) error {
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || (ts.Name.Name != "Project" && ts.Name.Name != "FeatureCard") {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok || st.Fields == nil {
				return true
			}
			for _, field := range st.Fields.List {
				if sel, ok := field.Type.(*ast.SelectorExpr); ok {
					if envelopeTypes[sel.Sel.Name] {
						t.Errorf("%s: %s embeds envelope type %s", file, ts.Name.Name, sel.Sel.Name)
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestNoHTTPDatabaseUIOrAIPackage (FF-012 §12).
func TestNoHTTPDatabaseUIOrAIPackage(t *testing.T) {
	forbiddenImports := []string{"net/http", "database/sql", "html/template", "text/template"}
	err := walkGoFiles(filepath.Join(ModuleRoot(), "internal"), func(path string, file *ast.File) error {
		for _, imp := range file.Imports {
			value := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range forbiddenImports {
				if value == bad || strings.Contains(value, "sql/driver") {
					t.Errorf("%s imports %q, which M.3 must not use", path, value)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	all, err := InternalPackages()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range all {
		rel := strings.TrimPrefix(p.ImportPath, ModulePath+"/internal/")
		if strings.HasPrefix(rel, "http") || strings.HasPrefix(rel, "ui") || strings.HasPrefix(rel, "ai") ||
			strings.HasPrefix(rel, "postgres") || strings.HasPrefix(rel, "sql") {
			t.Errorf("forbidden package present for M.3: %s", p.ImportPath)
		}
	}
}

// TestNoTimeNowOutsideClock (FF-012 §12).
func TestNoTimeNowOutsideClock(t *testing.T) {
	allowedFile := filepath.Join(ModuleRoot(), "internal", "application", "clock.go")
	err := walkGoFiles(filepath.Join(ModuleRoot(), "internal"), func(path string, file *ast.File) error {
		if path == allowedFile || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Now" {
				return true
			}
			if pkgIdent, ok := sel.X.(*ast.Ident); ok && pkgIdent.Name == "time" {
				t.Errorf("%s calls time.Now() outside the production Clock implementation", path)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestGoModHasOnlyPEOSRequirement (FF-012 §12).
func TestGoModHasOnlyPEOSRequirement(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(ModuleRoot(), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "replace ") {
		t.Error("go.mod must not contain a replace directive")
	}
	requireCount := strings.Count(content, "github.com/aleka7sk/PEOS")
	if requireCount == 0 {
		t.Error("go.mod must require github.com/aleka7sk/PEOS")
	}
	if !strings.Contains(content, "github.com/aleka7sk/PEOS v1.0.0") {
		t.Error("go.mod must pin exactly v1.0.0")
	}
}

// --- shared helpers ---

func walkGoFiles(root string, fn func(path string, file *ast.File) error) error {
	fset := token.NewFileSet()
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if perr != nil {
			return perr
		}
		return fn(path, file)
	})
}

func parseAndInspect(path string, fn func(file *ast.File)) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return err
	}
	fn(file)
	return nil
}

func containsAll(haystack, needles []string) bool {
	set := map[string]bool{}
	for _, h := range haystack {
		set[h] = true
	}
	for _, n := range needles {
		if !set[n] {
			return false
		}
	}
	return true
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
