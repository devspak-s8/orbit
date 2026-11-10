// Package modules implements the module registry and lifecycle
// (discover, install, enable, disable, uninstall) from PRD 7.
package modules

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Manifest is the module declaration format (PRD 7.1).
type Manifest struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	APIVersion   string   `json:"apiVersion"`
	Description  string   `json:"description"`
	Permissions  []string `json:"permissions"`
	Dependencies []string `json:"dependencies"`
	// Events lists the event types this module may emit.
	Events []string `json:"events,omitempty"`
	// Entrypoints declares the frontend bundle and backend package.
	Entrypoints struct {
		Frontend string `json:"frontend,omitempty"`
		Backend  string `json:"backend,omitempty"`
	} `json:"entrypoints"`
}

// KnownPermissions is the permission vocabulary for the alpha. Unknown
// permissions fail validation rather than being silently granted.
var KnownPermissions = map[string]bool{
	"workspace.read": true,
	"events.write":   true,
}

var (
	idPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9-_.]*$`)
	versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	eventPattern   = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)+$`)
)

// Validate checks a manifest against the platform rules, returning a
// combined error listing every violation.
func (m *Manifest) Validate() error {
	var problems []string

	if !idPattern.MatchString(m.ID) {
		problems = append(problems, "id must match [a-z0-9][a-z0-9-_.]*")
	}
	if strings.TrimSpace(m.Name) == "" {
		problems = append(problems, "name is required")
	}
	if !versionPattern.MatchString(m.Version) {
		problems = append(problems, "version must be semantic (x.y.z)")
	}
	if m.APIVersion != "1" {
		problems = append(problems, "apiVersion must be \"1\"")
	}
	for _, p := range m.Permissions {
		if !KnownPermissions[p] {
			problems = append(problems, fmt.Sprintf("unknown permission %q", p))
		}
	}
	for _, d := range m.Dependencies {
		if d != "orbit.core" {
			problems = append(problems, fmt.Sprintf("unknown dependency %q (only orbit.core exists)", d))
		}
	}
	for _, e := range m.Events {
		if !eventPattern.MatchString(e) {
			problems = append(problems, fmt.Sprintf("invalid event type %q", e))
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("manifest %q invalid: %s", m.ID, strings.Join(problems, "; "))
	}
	return nil
}

var (
	// ErrNotFound is returned when a module or installation does not exist.
	ErrNotFound = errors.New("module not found")
	// ErrNotInstalled is returned for lifecycle ops on uninstalled modules.
	ErrNotInstalled = errors.New("module is not installed in this workspace")
	// ErrForbidden is returned when the caller lacks the owner role.
	ErrForbidden = errors.New("only workspace owners can manage modules")
)
