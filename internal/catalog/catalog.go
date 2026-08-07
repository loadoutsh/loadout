// Package catalog holds the curated alias and function set that loadout
// ships. It is embedded in the binary, so a build carries its own catalog.
package catalog

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed data/aliases.yaml
//go:embed data/functions.yaml
//go:embed data/functions/*.tmpl
var files embed.FS

// Alias is one shipped alias.
type Alias struct {
	Name string `yaml:"name"`

	// Command is the whole alias body. Platforms replaces it where a single
	// string cannot serve both; exactly one of the two is set.
	Command   string            `yaml:"command,omitempty"`
	Platforms map[string]string `yaml:"platforms,omitempty"`

	Category string `yaml:"category"`

	// Pack empty means a core default, on for everyone. Otherwise the alias
	// appears only when that pack is enabled.
	Pack string `yaml:"pack,omitempty"`

	// Platform empty means all platforms.
	Platform []string `yaml:"platform,omitempty"`

	Requires []string `yaml:"requires,omitempty"`
	Comment  string   `yaml:"comment,omitempty"`
}

// Function is one shipped shell function: a text/template rendered against
// the user's config, plus the metadata that explains and checks it.
type Function struct {
	Name     string
	Template string

	Summary  string   `yaml:"summary"`
	Usage    string   `yaml:"usage"`
	Requires []string `yaml:"requires"`
	Config   []string `yaml:"config"`
	Notes    string   `yaml:"notes"`

	// NamedBy, when set, means this template generates a family of functions
	// whose names come from that config section rather than from the catalog.
	// clone-sources works this way: the user's clone_sources aliases become
	// the command names.
	NamedBy string `yaml:"named_by,omitempty"`
}

// Catalog is the full shipped set.
type Catalog struct {
	Aliases   []Alias
	Functions []Function

	// meta is the declared function metadata, kept so validate can report
	// entries that describe a function no template provides.
	meta []Function
}

type aliasFile struct {
	Aliases []Alias `yaml:"aliases"`
}

type functionFile struct {
	Functions []Function `yaml:"functions"`
}

// Function returns one function by name.
func (c *Catalog) Function(name string) (Function, bool) {
	for _, f := range c.Functions {
		if f.Name == name {
			return f, true
		}
	}
	return Function{}, false
}

// Alias returns one alias by name.
func (c *Catalog) Alias(name string) (Alias, bool) {
	for _, a := range c.Aliases {
		if a.Name == name {
			return a, true
		}
	}
	return Alias{}, false
}

// Load reads the embedded catalog.
func Load() (*Catalog, error) {
	data, err := files.ReadFile("data/aliases.yaml")
	if err != nil {
		return nil, err
	}
	var af aliasFile
	if err := yaml.Unmarshal(data, &af); err != nil {
		return nil, fmt.Errorf("catalog aliases: %w", err)
	}

	metaData, err := files.ReadFile("data/functions.yaml")
	if err != nil {
		return nil, err
	}
	var mf functionFile
	if err := yaml.Unmarshal(metaData, &mf); err != nil {
		return nil, fmt.Errorf("catalog functions: %w", err)
	}
	meta := map[string]Function{}
	for _, m := range mf.Functions {
		meta[m.Name] = m
	}

	entries, err := files.ReadDir("data/functions")
	if err != nil {
		return nil, err
	}
	var fns []Function
	for _, e := range entries {
		body, err := files.ReadFile("data/functions/" + e.Name())
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(e.Name(), ".tmpl")
		fn := meta[name] // zero value when the template carries no metadata
		fn.Name = name
		fn.Template = string(body)
		fns = append(fns, fn)
	}
	sort.Slice(fns, func(i, j int) bool { return fns[i].Name < fns[j].Name })

	c := &Catalog{Aliases: af.Aliases, Functions: fns, meta: mf.Functions}
	return c, c.validate()
}

func (c *Catalog) validate() error {
	seen := map[string]bool{}
	for _, a := range c.Aliases {
		if a.Name == "" {
			return fmt.Errorf("catalog: alias with no name")
		}
		if seen[a.Name] {
			return fmt.Errorf("catalog: duplicate alias %q", a.Name)
		}
		seen[a.Name] = true
		if a.Command == "" && len(a.Platforms) == 0 {
			return fmt.Errorf("catalog: alias %q has neither command nor platforms", a.Name)
		}
		if a.Command != "" && len(a.Platforms) > 0 {
			return fmt.Errorf("catalog: alias %q has both command and platforms", a.Name)
		}
	}

	// Metadata describing a function that no template provides is a typo that
	// would otherwise show up as silently missing documentation.
	have := map[string]bool{}
	for _, f := range c.Functions {
		have[f.Name] = true
	}
	for _, m := range c.meta {
		if !have[m.Name] {
			return fmt.Errorf("catalog: functions.yaml describes %q, which has no template", m.Name)
		}
	}
	return nil
}

// ResolvedRequires expands the placeholders in a function's requirements
// against the user's paths, so a path requirement names a real location.
func (f Function) ResolvedRequires(projectRoot, venvRoot string) []string {
	out := make([]string, 0, len(f.Requires))
	for _, r := range f.Requires {
		r = strings.ReplaceAll(r, "{{project_root}}", projectRoot)
		r = strings.ReplaceAll(r, "{{venv_root}}", venvRoot)
		out = append(out, r)
	}
	return out
}

// Resolve returns the alias body for the given GOOS, and whether the alias
// applies there at all.
func (a Alias) Resolve(goos string) (string, bool) {
	if len(a.Platforms) > 0 {
		cmd, ok := a.Platforms[goos]
		return cmd, ok
	}
	if len(a.Platform) > 0 {
		for _, p := range a.Platform {
			if p == goos {
				return a.Command, true
			}
		}
		return "", false
	}
	return a.Command, true
}

// shellBuiltins are provided by the shell itself. An alias whose body starts
// with one needs nothing installed, so reporting it as a missing binary would
// be a false alarm.
var shellBuiltins = map[string]bool{
	".": true, ":": true, "alias": true, "bg": true, "break": true,
	"builtin": true, "cd": true, "command": true, "continue": true,
	"declare": true, "echo": true, "eval": true, "exec": true, "exit": true,
	"export": true, "false": true, "fc": true, "fg": true, "getopts": true,
	"hash": true, "history": true, "jobs": true, "let": true, "local": true,
	"popd": true, "print": true, "printf": true, "pushd": true, "pwd": true,
	"read": true, "readonly": true, "return": true, "set": true, "shift": true,
	"source": true, "test": true, "times": true, "trap": true, "true": true,
	"type": true, "typeset": true, "ulimit": true, "umask": true,
	"unalias": true, "unset": true, "wait": true, "whence": true,
}

// RequiredBinaries reports what must be installed for this alias to work on
// the given platform. Platform-conditional entries derive it from the resolved
// command, since the answer differs per platform.
//
// Shell builtins and names provided by the catalog itself are excluded: they
// are never things a user needs to install.
func (c *Catalog) RequiredBinaries(a Alias, goos string) []string {
	if len(a.Requires) > 0 {
		return a.Requires
	}
	cmd, ok := a.Resolve(goos)
	if !ok {
		return nil
	}
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return nil
	}
	first := fields[0]
	if first == "sudo" && len(fields) > 1 {
		first = fields[1]
	}
	if strings.HasPrefix(first, "/") || strings.HasPrefix(first, "~") || strings.HasPrefix(first, "$") {
		return nil
	}
	if shellBuiltins[first] {
		return nil
	}
	// An alias that calls one of our own functions, or another alias, needs
	// nothing installed beyond whatever that target itself needs.
	if _, isFn := c.Function(first); isFn {
		return nil
	}
	if _, isAlias := c.Alias(first); isAlias {
		return nil
	}
	return []string{first}
}

// Packs lists every pack name in the catalog, sorted.
func (c *Catalog) Packs() []string {
	seen := map[string]bool{}
	for _, a := range c.Aliases {
		if a.Pack != "" {
			seen[a.Pack] = true
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
