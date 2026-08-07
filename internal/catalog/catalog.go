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

// Function is one shipped shell function, as a text/template rendered against
// the user's config.
type Function struct {
	Name     string
	Template string
}

// Catalog is the full shipped set.
type Catalog struct {
	Aliases   []Alias
	Functions []Function
}

type aliasFile struct {
	Aliases []Alias `yaml:"aliases"`
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
		fns = append(fns, Function{
			Name:     strings.TrimSuffix(e.Name(), ".tmpl"),
			Template: string(body),
		})
	}
	sort.Slice(fns, func(i, j int) bool { return fns[i].Name < fns[j].Name })

	c := &Catalog{Aliases: af.Aliases, Functions: fns}
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
	return nil
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

// RequiredBinaries reports what must be installed for this alias to work on
// the given platform. Platform-conditional entries derive it from the resolved
// command, since the answer differs per platform.
func (a Alias) RequiredBinaries(goos string) []string {
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
