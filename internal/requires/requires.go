// Package requires answers "can this actually work here?" for a single
// declared prerequisite.
//
// One declaration drives three things: what `doctor` reports, what `describe`
// tells a user to set up, and the status dot the web UI will show. Keeping
// them on one mechanism is why they cannot drift apart.
package requires

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Kind is what sort of prerequisite this is.
type Kind string

const (
	// Binary must be findable on PATH.
	Binary Kind = "binary"
	// Path must exist on disk.
	Path Kind = "path"
	// Config must be a non-empty section of the user's config.
	Config Kind = "config"
)

// Requirement is one prerequisite.
type Requirement struct {
	Kind  Kind
	Value string
}

// Parse reads a requirement written as "<kind>:<value>". A bare string with
// no prefix is a binary, which is the common case and keeps the catalog terse.
func Parse(s string) (Requirement, error) {
	kind, value, found := strings.Cut(s, ":")
	if !found {
		return Requirement{Kind: Binary, Value: s}, nil
	}
	switch Kind(kind) {
	case Binary, Path, Config:
		return Requirement{Kind: Kind(kind), Value: value}, nil
	default:
		return Requirement{}, fmt.Errorf("unknown requirement kind %q in %q", kind, s)
	}
}

// MustParseAll parses a list, treating anything unparseable as a binary so a
// malformed catalog entry degrades rather than breaking the whole run.
func MustParseAll(specs []string) []Requirement {
	out := make([]Requirement, 0, len(specs))
	for _, s := range specs {
		r, err := Parse(s)
		if err != nil {
			r = Requirement{Kind: Binary, Value: s}
		}
		out = append(out, r)
	}
	return out
}

// String renders the requirement back to its spec form.
func (r Requirement) String() string {
	if r.Kind == Binary {
		return r.Value
	}
	return string(r.Kind) + ":" + r.Value
}

// Describe explains the requirement in a sentence a user can act on.
func (r Requirement) Describe() string {
	switch r.Kind {
	case Binary:
		return fmt.Sprintf("%s must be installed and on PATH", r.Value)
	case Path:
		return fmt.Sprintf("%s must exist", r.Value)
	case Config:
		return fmt.Sprintf("config needs at least one entry under %q", r.Value)
	default:
		return r.Value
	}
}

// Result is the outcome of checking one requirement.
type Result struct {
	Requirement
	OK     bool
	Detail string
}

// ConfigPresence reports whether a named config section holds anything. The
// checker cannot see the config type directly, so the caller supplies this.
type ConfigPresence func(section string) (present bool, known bool)

// Checker evaluates requirements, caching lookups so a catalog of a few
// hundred entries does not re-stat and re-search PATH for the same handful of
// binaries over and over.
type Checker struct {
	binaries map[string]bool
	paths    map[string]bool
	presence ConfigPresence
}

// New builds a Checker. presence may be nil, in which case config
// requirements report as unknown rather than failing.
func New(presence ConfigPresence) *Checker {
	return &Checker{
		binaries: map[string]bool{},
		paths:    map[string]bool{},
		presence: presence,
	}
}

// Check evaluates one requirement.
func (c *Checker) Check(r Requirement) Result {
	switch r.Kind {
	case Binary:
		return Result{Requirement: r, OK: c.hasBinary(r.Value)}
	case Path:
		expanded := Expand(r.Value)
		ok := c.hasPath(expanded)
		detail := ""
		if !ok {
			detail = expanded
		}
		return Result{Requirement: r, OK: ok, Detail: detail}
	case Config:
		if c.presence == nil {
			return Result{Requirement: r, OK: true, Detail: "not checked"}
		}
		present, known := c.presence(r.Value)
		if !known {
			return Result{Requirement: r, OK: true, Detail: "unknown section"}
		}
		return Result{Requirement: r, OK: present}
	default:
		return Result{Requirement: r, OK: false, Detail: "unknown requirement kind"}
	}
}

// CheckAll evaluates a list and returns the results plus whether all passed.
func (c *Checker) CheckAll(rs []Requirement) ([]Result, bool) {
	out := make([]Result, 0, len(rs))
	all := true
	for _, r := range rs {
		res := c.Check(r)
		if !res.OK {
			all = false
		}
		out = append(out, res)
	}
	return out, all
}

func (c *Checker) hasBinary(name string) bool {
	if v, ok := c.binaries[name]; ok {
		return v
	}
	_, err := exec.LookPath(name)
	v := err == nil
	c.binaries[name] = v
	return v
}

func (c *Checker) hasPath(p string) bool {
	if v, ok := c.paths[p]; ok {
		return v
	}
	_, err := os.Stat(p)
	v := err == nil
	c.paths[p] = v
	return v
}

// Expand resolves a leading ~ against the home directory. Shell would do this
// itself, but these paths are checked in Go before any shell sees them.
func Expand(p string) string {
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}
