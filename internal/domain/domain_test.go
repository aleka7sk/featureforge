package domain

import (
	"errors"
	"testing"
	"time"
)

func fixedTime() time.Time {
	return time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
}

func TestNewProject(t *testing.T) {
	id, err := NewProjectID("PRJ-1")
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewProject(id, "Belcanto Pilot", fixedTime())
	if err != nil {
		t.Fatal(err)
	}
	if p.ID() != id {
		t.Errorf("ID() = %v, want %v", p.ID(), id)
	}
	if p.Name() != "Belcanto Pilot" {
		t.Errorf("Name() = %q, want %q", p.Name(), "Belcanto Pilot")
	}
	if !p.CreatedAt().Equal(fixedTime()) {
		t.Errorf("CreatedAt() = %v, want %v", p.CreatedAt(), fixedTime())
	}
}

func TestNewProjectRejectsEmptyID(t *testing.T) {
	_, err := NewProject(ProjectID{}, "Name", fixedTime())
	if !errors.Is(err, ErrProjectIDRequired) {
		t.Errorf("err = %v, want ErrProjectIDRequired", err)
	}
}

func TestNewProjectRejectsEmptyName(t *testing.T) {
	id, err := NewProjectID("PRJ-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "   ", "\t\n"} {
		_, err := NewProject(id, name, fixedTime())
		if !errors.Is(err, ErrProjectNameRequired) {
			t.Errorf("name %q: err = %v, want ErrProjectNameRequired", name, err)
		}
	}
}

func TestNewFeatureCard(t *testing.T) {
	pid, _ := NewProjectID("PRJ-1")
	fid, _ := NewFeatureCardID("FC-1")
	c, err := NewFeatureCard(fid, pid, "Homework after a lesson", "intent", fixedTime())
	if err != nil {
		t.Fatal(err)
	}
	if c.ID() != fid {
		t.Errorf("ID() = %v, want %v", c.ID(), fid)
	}
	if c.ProjectID() != pid {
		t.Errorf("ProjectID() = %v, want %v", c.ProjectID(), pid)
	}
	if c.Title() != "Homework after a lesson" {
		t.Errorf("Title() = %q", c.Title())
	}
	if _, ok := c.CapabilityArtifactID(); ok {
		t.Error("new feature card must have no capability link")
	}
}

func TestNewFeatureCardRejectsMissingProject(t *testing.T) {
	fid, _ := NewFeatureCardID("FC-1")
	_, err := NewFeatureCard(fid, ProjectID{}, "Title", "", fixedTime())
	if !errors.Is(err, ErrFeatureProjectRequired) {
		t.Errorf("err = %v, want ErrFeatureProjectRequired", err)
	}
}

func TestNewFeatureCardRejectsBlankTitle(t *testing.T) {
	pid, _ := NewProjectID("PRJ-1")
	fid, _ := NewFeatureCardID("FC-1")
	for _, title := range []string{"", "   ", "\t"} {
		_, err := NewFeatureCard(fid, pid, title, "", fixedTime())
		if !errors.Is(err, ErrInvalidFeatureTitle) {
			t.Errorf("title %q: err = %v, want ErrInvalidFeatureTitle", title, err)
		}
	}
}

func TestFeatureCardWithCapabilityArtifactID(t *testing.T) {
	pid, _ := NewProjectID("PRJ-1")
	fid, _ := NewFeatureCardID("FC-1")
	c, err := NewFeatureCard(fid, pid, "Title", "", fixedTime())
	if err != nil {
		t.Fatal(err)
	}
	linked, err := c.WithCapabilityArtifactID("CAP-1")
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := linked.CapabilityArtifactID(); !ok || got != "CAP-1" {
		t.Errorf("CapabilityArtifactID() = (%q, %v), want (CAP-1, true)", got, ok)
	}
	// The original value must be unaffected -- copy-return, never mutation.
	if _, ok := c.CapabilityArtifactID(); ok {
		t.Error("WithCapabilityArtifactID must not mutate the receiver")
	}
}

func TestFeatureCardSameEstablishmentExcludesCapabilityLink(t *testing.T) {
	pid, _ := NewProjectID("PRJ-1")
	fid, _ := NewFeatureCardID("FC-1")
	base, err := NewFeatureCard(fid, pid, "Title", "Description", fixedTime())
	if err != nil {
		t.Fatal(err)
	}
	linked, err := base.WithCapabilityArtifactID("CAP-1")
	if err != nil {
		t.Fatal(err)
	}
	otherLink, err := base.WithCapabilityArtifactID("CAP-2")
	if err != nil {
		t.Fatal(err)
	}

	for name, candidate := range map[string]FeatureCard{
		"same base":      base,
		"linked":         linked,
		"different link": otherLink,
	} {
		t.Run(name, func(t *testing.T) {
			if !base.SameEstablishment(candidate) || !candidate.SameEstablishment(base) {
				t.Error("capability link changed establishment equality")
			}
		})
	}

	otherPID, _ := NewProjectID("PRJ-2")
	otherFID, _ := NewFeatureCardID("FC-2")
	mustCard := func(id FeatureCardID, projectID ProjectID, title, description string, createdAt time.Time) FeatureCard {
		t.Helper()
		card, err := NewFeatureCard(id, projectID, title, description, createdAt)
		if err != nil {
			t.Fatal(err)
		}
		return card
	}
	for name, candidate := range map[string]FeatureCard{
		"id":          mustCard(otherFID, pid, "Title", "Description", fixedTime()),
		"project":     mustCard(fid, otherPID, "Title", "Description", fixedTime()),
		"title":       mustCard(fid, pid, "Different", "Description", fixedTime()),
		"description": mustCard(fid, pid, "Title", "Different", fixedTime()),
		"created at":  mustCard(fid, pid, "Title", "Description", fixedTime().Add(time.Second)),
	} {
		t.Run("different "+name, func(t *testing.T) {
			if base.SameEstablishment(candidate) {
				t.Errorf("different %s must not have the same establishment", name)
			}
		})
	}
}
