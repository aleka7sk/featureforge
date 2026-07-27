package domain

import "errors"

var (
	ErrProjectIDRequired      = errors.New("domain: project id is required")
	ErrProjectNameRequired    = errors.New("domain: project name is required")
	ErrFeatureIDRequired      = errors.New("domain: feature card id is required")
	ErrInvalidFeatureTitle    = errors.New("domain: feature card title is invalid")
	ErrFeatureProjectRequired = errors.New("domain: feature card requires a project id")
)
