package catalog

import (
	"fmt"
	"sort"
)

// Script is an executable text file loadout places on PATH.
type Script struct {
	Name string
	Body []byte
}

// Scripts returns the scripts shipped with this build.
func (c *Catalog) Scripts() ([]Script, error) {
	entries, err := files.ReadDir("data/scripts")
	if err != nil {
		return nil, err
	}
	out := make([]Script, 0, len(entries))
	for _, e := range entries {
		body, err := files.ReadFile("data/scripts/" + e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, Script{Name: e.Name(), Body: body})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Script returns one shipped script by name.
func (c *Catalog) Script(name string) (Script, error) {
	scripts, err := c.Scripts()
	if err != nil {
		return Script{}, err
	}
	for _, s := range scripts {
		if s.Name == name {
			return s, nil
		}
	}
	return Script{}, fmt.Errorf("no builtin script named %q", name)
}
