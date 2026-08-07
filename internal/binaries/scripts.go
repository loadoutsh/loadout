package binaries

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wesscoby/loadout/internal/config"
	"github.com/wesscoby/loadout/internal/requires"
)

// ScriptResult describes what happened to one script.
type ScriptResult struct {
	Name    string
	Source  string // "builtin" or the path it was linked from
	Action  string // "wrote", "linked", "unchanged"
	Err     error
	Symlink bool
}

// BuiltinLookup fetches a shipped script's contents by name. The catalog
// supplies this; taking it as a function keeps this package from importing
// the catalog and creating a cycle.
type BuiltinLookup func(name string) ([]byte, error)

// InstallScripts places the configured scripts into the managed bin directory.
//
// A builtin is written out; a user script is symlinked so edits at its real
// location take effect immediately, which is what you want while iterating on
// a script you own.
func (i *Installer) InstallScripts(scripts []config.Script, builtin BuiltinLookup) []ScriptResult {
	out := make([]ScriptResult, 0, len(scripts))
	for _, s := range scripts {
		out = append(out, i.installScript(s, builtin))
	}
	return out
}

func (i *Installer) installScript(s config.Script, builtin BuiltinLookup) ScriptResult {
	res := ScriptResult{Name: s.Name, Source: s.Source}

	if err := os.MkdirAll(i.BinDir, 0o755); err != nil {
		res.Err = err
		return res
	}
	dst := filepath.Join(i.BinDir, s.Name)

	if s.Source == "builtin" {
		body, err := builtin(s.Name)
		if err != nil {
			res.Err = err
			return res
		}
		// Skip the write when the content already matches, so `apply` does not
		// churn mtimes on every run.
		if existing, err := os.ReadFile(dst); err == nil && string(existing) == string(body) {
			res.Action = "unchanged"
			return res
		}
		if err := os.WriteFile(dst, body, 0o755); err != nil {
			res.Err = err
			return res
		}
		res.Action = "wrote"
		return res
	}

	// A user script: symlink to wherever it lives.
	src := requires.Expand(s.Source)
	abs, err := filepath.Abs(src)
	if err != nil {
		res.Err = err
		return res
	}
	if _, err := os.Stat(abs); err != nil {
		res.Err = fmt.Errorf("script source does not exist: %s", abs)
		return res
	}

	res.Symlink = true
	if current, err := os.Readlink(dst); err == nil && current == abs {
		res.Action = "unchanged"
		return res
	}
	// Replace whatever is there. Removing first is necessary because symlink
	// fails on an existing path.
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		res.Err = err
		return res
	}
	if err := os.Symlink(abs, dst); err != nil {
		res.Err = err
		return res
	}
	res.Action = "linked"
	return res
}
