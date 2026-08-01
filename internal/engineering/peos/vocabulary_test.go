package peos

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aleka7sk/PEOS/peos/core"
)

// declaredVocabularyValues returns every FeatureForge-declared
// core.VocabularyValue in this package, for TestVocabularySetIsClosed.
func declaredVocabularyValues() []core.VocabularyValue {
	return []core.VocabularyValue{
		ArtifactTypeProductCapability.Value(),
		ArtifactTypeValidationEvidence.Value(),
		MediaTypeSpecificationContent,
		MediaTypeValidationReport,
		ContentAddressAlgorithm,
		ProvenanceMethodAIAssisted,
		ValidationMethodManualReview.Value(),
		ValidationMethodManualInspection.Value(),
		CapabilityScopeKind,
		LifecycleSubjectType,
	}
}

func TestAllVocabularyValuesUseFeatureForgeNamespace(t *testing.T) {
	for _, v := range declaredVocabularyValues() {
		if v.Namespace() != Namespace {
			t.Errorf("vocabulary value %q has namespace %q, want %q", v, v.Namespace(), Namespace)
		}
	}
}

func TestVocabularyLocalNamesNonEmpty(t *testing.T) {
	for _, v := range declaredVocabularyValues() {
		if v.Value() == "" {
			t.Errorf("vocabulary value %q has an empty local name", v)
		}
	}
}

func TestNoDuplicateVocabularyDeclarations(t *testing.T) {
	seen := make(map[string]bool)
	for _, v := range declaredVocabularyValues() {
		key := v.String()
		if seen[key] {
			t.Errorf("duplicate vocabulary declaration: %s", key)
		}
		seen[key] = true
	}
}

func TestVocabularySetIsClosed(t *testing.T) {
	want := map[string]bool{
		"featureforge:product-capability":    true,
		"featureforge:validation-evidence":   true,
		"featureforge:specification-content": true,
		"featureforge:validation-report":     true,
		"featureforge:sha256":                true,
		"featureforge:ai-assisted":           true,
		"featureforge:manual-review":         true,
		"featureforge:manual-inspection":     true,
		"featureforge:capability":            true,
		"featureforge:capability-lifecycle":  true,
	}
	got := make(map[string]bool)
	for _, v := range declaredVocabularyValues() {
		got[v.String()] = true
	}
	for k := range want {
		if !got[k] {
			t.Errorf("expected vocabulary value %s to be declared", k)
		}
	}
	for k := range got {
		if !want[k] {
			t.Errorf("undeclared-in-spec vocabulary value found: %s", k)
		}
	}
}

func TestClaimTypeIsNotExtended(t *testing.T) {
	// core.ClaimType is the one PEOS vocabulary family treated as closed
	// (AD-009). FeatureForge must never call core.NewClaimType with a
	// featureforge-namespace value; only core.ClaimTypeSatisfaction is used.
	assertNoForbiddenCall(t, []string{"NewClaimType"})
}

func TestNoRequirementSatisfactionOrFeatureSpecificationVocabulary(t *testing.T) {
	assertSourceDoesNotContain(t, []string{"requirement-satisfaction", "feature-specification"})
}

// assertSourceDoesNotContain parses every non-test .go file in this
// package's directory and fails if any string literal contains one of the
// forbidden substrings.
func assertSourceDoesNotContain(t *testing.T, forbidden []string) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			for _, bad := range forbidden {
				if strings.Contains(lit.Value, bad) {
					t.Errorf("%s: literal %s contains forbidden substring %q", name, lit.Value, bad)
				}
			}
			return true
		})
	}
}

// assertNoForbiddenCall fails if any of the named functions/methods is
// called with a literal argument containing "featureforge" anywhere in this
// package's non-test source, which would indicate an attempt to extend a
// closed PEOS vocabulary family.
func assertNoForbiddenCall(t *testing.T, names []string) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			for _, fn := range names {
				if sel.Sel.Name == fn {
					t.Errorf("%s: call to %s must never be made with a featureforge-namespace value", name, fn)
				}
			}
			return true
		})
	}
}
