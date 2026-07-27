package domain

import (
	"reflect"
	"strings"
	"testing"
)

// TestFeatureCardHasNoDerivedState is the FF-012 §1 reflection test asserting
// FeatureCard carries no current-revision, readiness, or lifecycle field, and
// no collection of requirements, decisions, or claims (FF-002 §2).
func TestFeatureCardHasNoDerivedState(t *testing.T) {
	typ := reflect.TypeOf(FeatureCard{})
	forbidden := []string{
		"revision", "readiness", "lifecycle", "requirement",
		"decision", "claim", "status", "current",
	}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		lower := strings.ToLower(f.Name)
		for _, bad := range forbidden {
			if strings.Contains(lower, bad) {
				t.Errorf("FeatureCard field %q looks like derived engineering state (matches %q)", f.Name, bad)
			}
		}
		switch f.Type.Kind() {
		case reflect.Slice, reflect.Map, reflect.Array:
			t.Errorf("FeatureCard field %q is a collection; FeatureCard must hold no requirement/decision/claim collections", f.Name)
		}
	}
}

// TestDomainHasNoSetters asserts that Project and FeatureCard expose no Set*
// method and no pointer-receiver method. Every modifier follows the PEOS-style
// copy-return convention (With*), never in-place mutation.
func TestDomainHasNoSetters(t *testing.T) {
	for _, v := range []any{Project{}, FeatureCard{}} {
		valType := reflect.TypeOf(v)
		ptrType := reflect.PointerTo(valType)

		if ptrType.NumMethod() != valType.NumMethod() {
			t.Errorf("%s has one or more pointer-receiver methods: value type has %d exported methods, pointer type has %d",
				valType.Name(), valType.NumMethod(), ptrType.NumMethod())
		}
		for i := 0; i < valType.NumMethod(); i++ {
			name := valType.Method(i).Name
			if strings.HasPrefix(name, "Set") {
				t.Errorf("%s.%s looks like a setter; use a copy-return With* method instead", valType.Name(), name)
			}
		}
	}
}
