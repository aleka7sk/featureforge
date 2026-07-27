package domain

import (
	"strings"
	"time"
)

// Project is an operational naming container for feature cards. It carries no
// engineering semantics and is created once, not edited, during the canonical
// scenario (FF-002 §2).
type Project struct {
	id        ProjectID
	name      string
	createdAt time.Time
}

// NewProject validates its arguments and returns a Project. createdAt is
// supplied explicitly by the caller via the application Clock; this
// constructor never calls time.Now.
func NewProject(id ProjectID, name string, createdAt time.Time) (Project, error) {
	if id.IsZero() {
		return Project{}, ErrProjectIDRequired
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return Project{}, ErrProjectNameRequired
	}
	return Project{id: id, name: trimmed, createdAt: createdAt}, nil
}

// ID returns the project's identity.
func (p Project) ID() ProjectID { return p.id }

// Name returns the project's name.
func (p Project) Name() string { return p.name }

// CreatedAt returns when the project was created.
func (p Project) CreatedAt() time.Time { return p.createdAt }

// IsZero reports whether p is the zero value.
func (p Project) IsZero() bool { return p.id.IsZero() }
