package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
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
	assertNoTransitivePEOS(t, ModulePath+"/internal/infrastructure/postgres")
	assertNoTransitivePEOS(t, ModulePath+"/internal/infrastructure/contracttest")
}

// TestTransportDoesNotImportPEOS (FF-018 §18 criterion 4, FF-007
// deliverable): no transport type references a PEOS type. Response DTOs
// read projected fields off engineering.RevisionEnvelope /
// engineering.RecordEnvelope (FF-018 §5.1), never a PEOS type -- and
// internal/engineering is itself PEOS-free, so the transitive check that
// already proves that for domain/engineering/application/infrastructure
// proves it here too.
func TestTransportDoesNotImportPEOS(t *testing.T) {
	assertNoTransitivePEOS(t, ModulePath+"/internal/transport/http")
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
// in the module directly imports the PEOS SDK. Covers cmd/ too (AD-023,
// FF-018 §13): cmd/featureforge imports internal/engineering/peos, the
// FeatureForge-owned wrapper, never the PEOS SDK path itself, so no
// allow-list entry is needed to keep this single-holder.
func TestOnlyIntegrationPackageImportsPEOS(t *testing.T) {
	all := allPackagesIncludingCmd(t)
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

// TestOnlyPostgresInfrastructureImportsDriver (AD-020): exactly one package
// in the module directly imports the PostgreSQL driver, so "PostgreSQL is a
// replaceable adapter" is structurally true rather than merely intended.
// Covers cmd/ too (AD-023, FF-018 §13): cmd/featureforge obtains its
// *pgxpool.Pool through postgres.Connect's return, carried only by :=
// type inference, so it holds no direct driver import and needs no
// allow-list entry to keep this single-holder (open question N1).
func TestOnlyPostgresInfrastructureImportsDriver(t *testing.T) {
	all := allPackagesIncludingCmd(t)
	var importers []string
	for _, p := range all {
		for _, imp := range p.Imports {
			if strings.HasPrefix(imp, DriverModulePath) {
				importers = append(importers, p.ImportPath)
				break
			}
		}
	}
	want := ModulePath + "/internal/infrastructure/postgres"
	if len(importers) != 1 || importers[0] != want {
		t.Errorf("packages directly importing the driver = %v, want exactly [%s]", importers, want)
	}
}

// TestDomainAndApplicationDoNotImportDriver (AD-020): the layers that carry
// domain meaning must not depend, even transitively, on a database driver.
func TestDomainAndApplicationDoNotImportDriver(t *testing.T) {
	all, err := InternalPackages()
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range []string{
		ModulePath + "/internal/domain",
		ModulePath + "/internal/engineering",
		ModulePath + "/internal/application",
	} {
		imports := TransitiveImports(pkg, all)
		if ImportsDriver(imports) {
			t.Errorf("%s transitively imports the PostgreSQL driver; its import set is %v", pkg, sortedKeys(imports))
		}
	}
}

// TestAdaptersDoNotImportEachOther (AD-020): neither persistence adapter may
// depend on the other. Both may import the shared contract suite, which is
// exactly why that suite lives in its own package.
func TestAdaptersDoNotImportEachOther(t *testing.T) {
	all, err := InternalPackages()
	if err != nil {
		t.Fatal(err)
	}
	memoryPkg := ModulePath + "/internal/infrastructure/memory"
	postgresPkg := ModulePath + "/internal/infrastructure/postgres"

	if TransitiveImports(memoryPkg, all)[postgresPkg] {
		t.Error("the in-memory adapter imports the PostgreSQL adapter")
	}
	if TransitiveImports(postgresPkg, all)[memoryPkg] {
		t.Error("the PostgreSQL adapter imports the in-memory adapter")
	}
}

// TestNoUpdateOrDeleteOnEngineeringTables (FF-007 M.4 exit criterion):
// engineering records are immutable, so no statement anywhere in the
// PostgreSQL adapter may UPDATE or DELETE one. Correcting a record means
// writing a new record that references it, never editing history.
func TestNoUpdateOrDeleteOnEngineeringTables(t *testing.T) {
	engineeringTables := []string{
		"artifact_envelopes", "revision_envelopes", "structured_content",
		"record_envelopes", "revision_order", "revision_acceptance", "requirement_criterion_traces",
		"projects", "feature_cards",
	}
	dir := filepath.Join(ModuleRoot(), "internal", "infrastructure", "postgres")

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if ext := filepath.Ext(path); ext != ".go" && ext != ".sql" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lower := strings.ToLower(string(content))
		for _, table := range engineeringTables {
			for _, verb := range []string{"update " + table, "delete from " + table} {
				if strings.Contains(lower, verb) {
					rel, _ := filepath.Rel(ModuleRoot(), path)
					t.Errorf("%s contains %q; engineering records are immutable", rel, verb)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
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

// allPackagesIncludingCmd returns InternalPackages() plus CmdPackages(), for
// guards that must also cover the composition root (AD-023, FF-018 §13).
func allPackagesIncludingCmd(t *testing.T) []PackageInfo {
	t.Helper()
	internal, err := InternalPackages()
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := CmdPackages()
	if err != nil {
		t.Fatal(err)
	}
	return append(internal, cmd...)
}

// TestOnlyTransportAndCommandImportNetHTTP (AD-023): net/http is permitted
// only in the HTTP transport package, the UI package (FF-021, which speaks
// to the API in-process over net/http per AD-028), and the cmd composition
// root that builds the server around both. Replaces the net/http half of
// the former TestNoHTTPDatabaseUIOrAIPackage with a named-holder test,
// mirroring TestOnlyIntegrationPackageImportsPEOS and
// TestOnlyPostgresInfrastructureImportsDriver.
func TestOnlyTransportAndCommandImportNetHTTP(t *testing.T) {
	all := allPackagesIncludingCmd(t)
	allowed := map[string]bool{
		ModulePath + "/internal/transport/http": true,
		ModulePath + "/internal/ui":             true,
		ModulePath + "/cmd/featureforge":        true,
	}
	for _, p := range all {
		for _, imp := range p.Imports {
			if imp == "net/http" && !allowed[p.ImportPath] {
				t.Errorf("%s imports net/http, which only internal/transport/http, internal/ui, and cmd/featureforge may import (AD-023)", p.ImportPath)
			}
		}
	}
}

// TestOnlyUIImportsHTMLTemplate (AD-023): html/template is reserved for the
// Phase B UI package, internal/ui (FF-021). Narrowed from "forbidden
// everywhere" now that the reserved holder exists; cmd/featureforge
// composes the UI but does not render templates itself, so it is not a
// permitted holder.
func TestOnlyUIImportsHTMLTemplate(t *testing.T) {
	all := allPackagesIncludingCmd(t)
	allowed := map[string]bool{ModulePath + "/internal/ui": true}
	for _, p := range all {
		for _, imp := range p.Imports {
			if imp == "html/template" && !allowed[p.ImportPath] {
				t.Errorf("%s imports html/template, which only internal/ui may import (AD-023)", p.ImportPath)
			}
		}
	}
}

// TestTextTemplateIsNeverImported (AD-023, FF-018 §24, M.5 publication
// remediation M-2): text/template has no permitted holder anywhere under
// internal/ or cmd/ -- unlike html/template above, which internal/ui alone
// may import. AD-023's own decision text already said
// "html/template/text/template remain forbidden everywhere in Phase A",
// but the four-test decomposition that replaced the original
// TestNoHTTPDatabaseUIOrAIPackage carried only the html/template half
// forward; this restores the other half as its own named, absolute guard,
// mirroring TestDatabaseSQLIsNeverImported's no-holder shape rather than
// folding it into TestOnlyUIImportsHTMLTemplate, so a failure here names
// the actual defect (an unwanted text/template import) rather than an
// ambiguous "some template package" message.
func TestTextTemplateIsNeverImported(t *testing.T) {
	all := allPackagesIncludingCmd(t)
	for _, p := range all {
		for _, imp := range p.Imports {
			if imp == "text/template" {
				t.Errorf("%s imports text/template, which no package may import (AD-023)", p.ImportPath)
			}
		}
	}
}

// TestUIDoesNotImportPEOS (AD-005, FF-021): internal/ui speaks to the
// engineering model only through the existing HTTP API, in-process
// (AD-028), never through PEOS directly.
func TestUIDoesNotImportPEOS(t *testing.T) {
	assertNoTransitivePEOS(t, ModulePath+"/internal/ui")
}

// TestUIDoesNotImportApplicationOrInfrastructure (FF-021 §2): internal/ui's
// AD-028 write path makes every other internal/ boundary structurally
// unreachable, not just test-enforced -- the UI holds no engineering rule
// and reaches no repository, so it should import nothing under internal/
// at all except itself.
func TestUIDoesNotImportApplicationOrInfrastructure(t *testing.T) {
	all, err := InternalPackages()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range all {
		if p.ImportPath != ModulePath+"/internal/ui" && !strings.HasPrefix(p.ImportPath, ModulePath+"/internal/ui/") {
			continue
		}
		for _, imp := range p.Imports {
			if strings.HasPrefix(imp, ModulePath+"/internal/") && imp != ModulePath+"/internal/ui" && !strings.HasPrefix(imp, ModulePath+"/internal/ui/") {
				t.Errorf("%s imports %s; internal/ui must import nothing else under internal/ (FF-021 §2, AD-028)", p.ImportPath, imp)
			}
		}
	}
}

// TestDatabaseSQLIsNeverImported (AD-020, AD-023): absolute -- no package,
// anywhere including cmd/, may import database/sql or a database/sql driver.
// AD-020 chose native pgx precisely to keep this prohibition absolute.
func TestDatabaseSQLIsNeverImported(t *testing.T) {
	all := allPackagesIncludingCmd(t)
	for _, p := range all {
		for _, imp := range p.Imports {
			if imp == "database/sql" || strings.Contains(imp, "sql/driver") {
				t.Errorf("%s imports %q; database/sql is never permitted (AD-020)", p.ImportPath, imp)
			}
		}
	}
}

// TestNoAIPackage (FF-012 §12): no package path segment is "ai", under
// internal/ or cmd/.
func TestNoAIPackage(t *testing.T) {
	all := allPackagesIncludingCmd(t)
	for _, p := range all {
		rel := strings.TrimPrefix(strings.TrimPrefix(p.ImportPath, ModulePath+"/internal/"), ModulePath+"/cmd/")
		if strings.HasPrefix(rel, "ai") {
			t.Errorf("forbidden package present: %s", p.ImportPath)
		}
	}
}

// TestNoTimeNowOutsideClock (FF-012 §12).
func TestNoTimeNowOutsideClock(t *testing.T) {
	allowedFiles := map[string]bool{
		filepath.Join(ModuleRoot(), "internal", "application", "clock.go"): true,
		// internal/transport/http/middleware.go measures request duration
		// for FF-018 §14.3's request-logging line -- HTTP observability,
		// not an engineering-record timestamp. application.Clock exists to
		// keep *those* deterministic and retry-safe under UnitOfWork.Do
		// (FF-010 §2); reusing it here would conflate two unrelated
		// concerns rather than serve the one it was built for. Narrowed,
		// not removed, the same way AD-023 narrowed the net/http
		// prohibition rather than deleting it -- this guard predates the
		// transport package and did not anticipate it.
		filepath.Join(ModuleRoot(), "internal", "transport", "http", "middleware.go"): true,
		// internal/ui/middleware.go measures request duration for the same
		// reason (FF-021 §2, mirroring internal/transport/http's own
		// middleware exactly): HTTP observability on the UI's own request
		// log line, not an engineering-record timestamp.
		filepath.Join(ModuleRoot(), "internal", "ui", "middleware.go"): true,
	}
	err := walkGoFiles(filepath.Join(ModuleRoot(), "internal"), func(path string, file *ast.File) error {
		if allowedFiles[path] || strings.HasSuffix(path, "_test.go") {
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

// TestDoCallbacksAreRetrySafe (AD-020): PostgreSQL's UnitOfWork runs every
// callback under SERIALIZABLE and re-runs the whole callback from scratch on
// a serialization failure. That makes retry-safety load-bearing: a callback
// must not read the clock again inside itself, because two attempts would
// then record two different times for one engineering act, and the act that
// eventually commits would carry a timestamp that never corresponded to when
// it was attempted.
//
// Every command already captures Clock.Now() once, before calling Do. This
// test pins that: no Clock.Now() call may appear lexically inside a function
// literal passed to a Do call.
func TestDoCallbacksAreRetrySafe(t *testing.T) {
	err := walkGoFiles(filepath.Join(ModuleRoot(), "internal"), func(path string, file *ast.File) error {
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Do" {
				return true
			}
			for _, arg := range call.Args {
				lit, ok := arg.(*ast.FuncLit)
				if !ok {
					continue
				}
				ast.Inspect(lit.Body, func(inner ast.Node) bool {
					innerCall, ok := inner.(*ast.CallExpr)
					if !ok {
						return true
					}
					innerSel, ok := innerCall.Fun.(*ast.SelectorExpr)
					if !ok || innerSel.Sel.Name != "Now" {
						return true
					}
					rel, _ := filepath.Rel(ModuleRoot(), path)
					t.Errorf("%s calls .Now() inside a UnitOfWork.Do callback; capture the time once before Do, "+
						"because the callback may be re-run after a serialization failure", rel)
					return true
				})
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestGoModHasOnlyApprovedRequirements (FF-012 §12, AD-020): PEOS stays
// pinned at v1.0.0 with no replace directive, and the only other DIRECT
// requirement is the one PostgreSQL driver M.4 is permitted to add.
// Indirect requirements are unconstrained -- they are whatever the driver
// itself pulls in -- but a new direct one is a dependency decision and must
// fail the build until it is recorded.
func TestGoModHasOnlyApprovedRequirements(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(ModuleRoot(), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "replace ") {
		t.Error("go.mod must not contain a replace directive")
	}
	if !strings.Contains(content, "github.com/aleka7sk/PEOS v1.0.0") {
		t.Error("go.mod must require github.com/aleka7sk/PEOS pinned at exactly v1.0.0")
	}

	allowedDirect := map[string]bool{
		"github.com/aleka7sk/PEOS": true,
		"github.com/jackc/pgx/v5":  true,
	}
	for _, module := range directRequirements(content) {
		if !allowedDirect[module] {
			t.Errorf("go.mod directly requires %s, which no accepted milestone permits; record the decision first", module)
		}
	}
}

// directRequirements returns the module paths go.mod requires directly --
// every require line not marked "// indirect".
func directRequirements(goMod string) []string {
	var out []string
	inBlock := false
	for _, line := range strings.Split(goMod, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "require (":
			inBlock = true
			continue
		case inBlock && trimmed == ")":
			inBlock = false
			continue
		}
		if strings.Contains(trimmed, "// indirect") || trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if inBlock {
			if fields := strings.Fields(trimmed); len(fields) >= 2 {
				out = append(out, fields[0])
			}
			continue
		}
		// A single-line "require path version" outside any block.
		if after, found := strings.CutPrefix(trimmed, "require "); found {
			if fields := strings.Fields(after); len(fields) >= 2 {
				out = append(out, fields[0])
			}
		}
	}
	return out
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
