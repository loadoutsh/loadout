// Package backend abstracts the shell dialect the generator emits.
//
// Only zsh is implemented. The interface exists so a second dialect can be
// added without touching the generator, which is the whole reason shell
// syntax is confined to this package.
package backend

import "fmt"

// Backend renders shell constructs for one dialect.
type Backend interface {
	// Name is the dialect identifier, e.g. "zsh".
	Name() string

	// Ext is the generated file's extension, without the dot.
	Ext() string

	// Alias renders a single alias definition.
	Alias(name, command string) string

	// PathAppend renders appending a directory to PATH.
	PathAppend(dir string) string

	// Comment renders a comment line.
	Comment(text string) string

	// SectionHeader renders a visually distinct section break.
	SectionHeader(title string) string

	// Preamble is emitted once at the top of the generated file, for anything
	// the dialect must enable before the rest will behave. Empty is normal.
	Preamble() string
}

// Registry of available backends, keyed by name.
var registry = map[string]Backend{}

func register(b Backend) { registry[b.Name()] = b }

// Get returns the backend for a dialect.
func Get(name string) (Backend, error) {
	b, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown shell %q (have: %v)", name, Names())
	}
	return b, nil
}

// Names lists the registered dialects.
func Names() []string {
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	return out
}
