package engineering

import (
	"errors"
	"strings"
	"testing"
)

func mustContent(t *testing.T) CapabilitySpecificationContent {
	t.Helper()
	c, err := NewCapabilitySpecificationContent(1, "Homework after a lesson", "No follow-up work today.")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestContentValidation(t *testing.T) {
	tests := []struct {
		name    string
		build   func() error
		wantErr error
	}{
		{"empty title", func() error {
			_, err := NewCapabilitySpecificationContent(1, "", "problem")
			return err
		}, ErrInvalidContent},
		{"blank title", func() error {
			_, err := NewCapabilitySpecificationContent(1, "   ", "problem")
			return err
		}, ErrInvalidContent},
		{"empty problem statement", func() error {
			_, err := NewCapabilitySpecificationContent(1, "title", "")
			return err
		}, ErrInvalidContent},
		{"unsupported schema version 0", func() error {
			_, err := NewCapabilitySpecificationContent(0, "title", "problem")
			return err
		}, ErrUnsupportedSchemaVersion},
		{"unsupported schema version 2", func() error {
			_, err := NewCapabilitySpecificationContent(2, "title", "problem")
			return err
		}, ErrUnsupportedSchemaVersion},
		{"blank list element", func() error {
			c := mustContent(t)
			_, err := c.WithFunctionalBehaviours([]string{"ok", "   "})
			return err
		}, ErrInvalidContent},
		{"duplicate acceptance criterion key", func() error {
			c := mustContent(t)
			ac1, _ := NewAcceptanceCriterion("AC-1", "first")
			ac2, _ := NewAcceptanceCriterion("AC-1", "second")
			_, err := c.WithAcceptanceCriteria([]AcceptanceCriterion{ac1, ac2})
			return err
		}, ErrDuplicateAcceptanceCriterionKey},
		{"invalid acceptance criterion key", func() error {
			_, err := NewAcceptanceCriterion("AC 1!", "text")
			return err
		}, ErrInvalidContent},
		{"empty acceptance criterion text", func() error {
			_, err := NewAcceptanceCriterion("AC-1", "   ")
			return err
		}, ErrInvalidContent},
		{"title too large", func() error {
			_, err := NewCapabilitySpecificationContent(1, strings.Repeat("x", maxStringBytes+1), "problem")
			return err
		}, ErrContentTooLarge},
		{"too many list elements", func() error {
			c := mustContent(t)
			items := make([]string, maxListItems+1)
			for i := range items {
				items[i] = "item"
			}
			_, err := c.WithDependencies(items)
			return err
		}, ErrContentTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.build()
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want wrapping %v", err, tc.wantErr)
			}
		})
	}
}

func fullyPopulatedContent(t *testing.T) CapabilitySpecificationContent {
	t.Helper()
	c := mustContent(t)
	var err error
	if c, err = c.WithUserOutcome("A student can see the homework."); err != nil {
		t.Fatal(err)
	}
	if c, err = c.WithFunctionalBehaviours([]string{"attach", "publish"}); err != nil {
		t.Fatal(err)
	}
	if c, err = c.WithConstraints([]string{"visible only to the student"}); err != nil {
		t.Fatal(err)
	}
	ac1, _ := NewAcceptanceCriterion("AC-1", "Published homework is visible to the student.")
	ac2, _ := NewAcceptanceCriterion("AC-2", "Homework is not visible to unrelated users.")
	if c, err = c.WithAcceptanceCriteria([]AcceptanceCriterion{ac1, ac2}); err != nil {
		t.Fatal(err)
	}
	if c, err = c.WithDependencies([]string{"lesson completion state"}); err != nil {
		t.Fatal(err)
	}
	if c, err = c.WithOpenQuestions([]string{"audio attachment?"}); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCanonicalJSONFieldOrder(t *testing.T) {
	c := fullyPopulatedContent(t)
	data, err := c.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema_version":1,"title":"Homework after a lesson","problem_statement":"No follow-up work today.","user_outcome":"A student can see the homework.","functional_behaviours":["attach","publish"],"constraints":["visible only to the student"],"acceptance_criteria":[{"key":"AC-1","text":"Published homework is visible to the student."},{"key":"AC-2","text":"Homework is not visible to unrelated users."}],"dependencies":["lesson completion state"],"open_questions":["audio attachment?"]}`
	if string(data) != want {
		t.Errorf("canonical JSON mismatch:\ngot:  %s\nwant: %s", data, want)
	}
}

func TestCanonicalJSONStable(t *testing.T) {
	c := fullyPopulatedContent(t)
	first, err := c.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	for i := range 1000 {
		got, err := c.CanonicalJSON()
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(first) {
			t.Fatalf("iteration %d: output diverged", i)
		}
	}
}

func TestCanonicalJSONEmptyListsAreArrays(t *testing.T) {
	c := mustContent(t)
	data, err := c.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		`"functional_behaviours":[]`,
		`"constraints":[]`,
		`"acceptance_criteria":[]`,
		`"dependencies":[]`,
		`"open_questions":[]`,
	} {
		if !strings.Contains(string(data), field) {
			t.Errorf("expected %s in %s", field, data)
		}
	}
	if strings.Contains(string(data), "null") {
		t.Errorf("canonical JSON must never contain null: %s", data)
	}
}

func TestCanonicalJSONDoesNotEscapeHTML(t *testing.T) {
	c := mustContent(t)
	c, err := c.WithUserOutcome("A <student> & their \"homework\"")
	if err != nil {
		t.Fatal(err)
	}
	data, err := c.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "<student> & their") {
		t.Errorf("HTML characters were escaped: %s", data)
	}
}

func TestDigestStability(t *testing.T) {
	c := fullyPopulatedContent(t)
	d1, err := c.Digest()
	if err != nil {
		t.Fatal(err)
	}
	d2, err := c.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if !d1.Equal(d2) {
		t.Errorf("digest not stable: %v vs %v", d1, d2)
	}
}

func TestDigestSensitivity(t *testing.T) {
	c1 := mustContent(t)
	c1, err := c1.WithDependencies([]string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	c2 := mustContent(t)
	c2, err = c2.WithDependencies([]string{"b", "a"})
	if err != nil {
		t.Fatal(err)
	}
	d1, _ := c1.Digest()
	d2, _ := c2.Digest()
	if d1.Equal(d2) {
		t.Error("digests must differ when list order differs")
	}
}

func TestDigestSensitiveToSchemaVersion(t *testing.T) {
	c1, err := NewCapabilitySpecificationContent(1, "title", "problem")
	if err != nil {
		t.Fatal(err)
	}
	d1, _ := c1.Digest()
	// schema_version 2 is currently unsupported, so this asserts the digest
	// computation itself is schema-version-sensitive by encoding directly.
	data, _ := c1.CanonicalJSON()
	if !strings.Contains(string(data), `"schema_version":1`) {
		t.Fatalf("expected schema_version 1 in canonical JSON: %s", data)
	}
	alteredDigest := ComputeDigest([]byte(strings.Replace(string(data), `"schema_version":1`, `"schema_version":2`, 1)))
	if d1.Equal(alteredDigest) {
		t.Error("digest must change when schema_version changes")
	}
}

func TestContentEqualityIsCanonicalBytes(t *testing.T) {
	c1 := fullyPopulatedContent(t)
	c2 := fullyPopulatedContent(t)
	if !c1.Equal(c2) {
		t.Error("independently built equal contents must be Equal")
	}
}

func TestContentRoundTrip(t *testing.T) {
	c := fullyPopulatedContent(t)
	data1, err := c.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCapabilitySpecificationContent(data1)
	if err != nil {
		t.Fatal(err)
	}
	data2, err := parsed.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(data1) != string(data2) {
		t.Errorf("round trip not byte-identical:\ngot:  %s\nwant: %s", data2, data1)
	}
}
