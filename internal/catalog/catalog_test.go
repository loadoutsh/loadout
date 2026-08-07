package catalog

import (
	"strings"
	"testing"
)

func load(t *testing.T) *Catalog {
	t.Helper()
	c, err := Load()
	if err != nil {
		t.Fatalf("loading catalog: %v", err)
	}
	return c
}

func TestLoadValidates(t *testing.T) {
	c := load(t)
	if len(c.Aliases) == 0 {
		t.Fatal("catalog has no aliases")
	}
	if len(c.Functions) == 0 {
		t.Fatal("catalog has no functions")
	}
}

// Every function template should carry metadata, otherwise `describe` has
// nothing to say about it and `doctor` cannot check it.
func TestEveryFunctionHasMetadata(t *testing.T) {
	for _, f := range load(t).Functions {
		if f.Summary == "" {
			t.Errorf("function %q has no summary in functions.yaml", f.Name)
		}
	}
}

func TestResolvePlatform(t *testing.T) {
	c := load(t)

	ctl, ok := c.Alias("ctl")
	if !ok {
		t.Fatal("ctl missing from catalog")
	}
	if got, _ := ctl.Resolve("linux"); !strings.Contains(got, "systemctl") {
		t.Errorf("linux ctl = %q, want systemctl", got)
	}
	if got, _ := ctl.Resolve("darwin"); !strings.Contains(got, "launchctl") {
		t.Errorf("darwin ctl = %q, want launchctl", got)
	}

	debi, ok := c.Alias("debi")
	if !ok {
		t.Fatal("debi missing from catalog")
	}
	if _, applies := debi.Resolve("darwin"); applies {
		t.Error("debi should not apply on darwin")
	}
	if _, applies := debi.Resolve("linux"); !applies {
		t.Error("debi should apply on linux")
	}
}

// A requirement must follow the resolved command, not name one platform's
// binary on both.
func TestPlatformRequirementsFollowTheCommand(t *testing.T) {
	c := load(t)
	ctl, _ := c.Alias("ctl")

	if got := c.RequiredBinaries(ctl, "linux"); len(got) != 1 || got[0] != "systemctl" {
		t.Errorf("linux ctl requires %v, want [systemctl]", got)
	}
	if got := c.RequiredBinaries(ctl, "darwin"); len(got) != 1 || got[0] != "launchctl" {
		t.Errorf("darwin ctl requires %v, want [launchctl]", got)
	}
}

// Shell builtins are not things a user installs, so reporting them as missing
// binaries would be a false alarm in doctor.
func TestBuiltinsAreNotRequirements(t *testing.T) {
	c := load(t)
	for _, name := range []string{"..", "...", "....", "downloads", "h", "j", "zrc", "ezrc"} {
		a, ok := c.Alias(name)
		if !ok {
			t.Errorf("%q missing from catalog", name)
			continue
		}
		if got := c.RequiredBinaries(a, "linux"); len(got) != 0 {
			t.Errorf("alias %q requires %v, want nothing (shell builtin)", name, got)
		}
	}
}

// An alias whose target is one of our own functions needs nothing installed
// of its own; that function's requirements are reported against the function.
func TestFunctionTargetsAreNotBinaryRequirements(t *testing.T) {
	c := load(t)
	cg, ok := c.Alias("cg")
	if !ok {
		t.Fatal("cg missing from catalog")
	}
	if got := c.RequiredBinaries(cg, "linux"); len(got) != 0 {
		t.Errorf("alias cg requires %v, want nothing: it calls the gcfg function, "+
			"whose own requirements are checked against gcfg", got)
	}
}

// An alias chaining through another alias must still report the binary it
// ultimately needs. `tl` expands to `t ls`, `t` expands to `tmux`, so tl needs
// tmux installed even though its own first word is an alias.
func TestAliasChainsResolveToTheRealBinary(t *testing.T) {
	c := load(t)
	for _, name := range []string{"tl", "ta", "tkss"} {
		a, ok := c.Alias(name)
		if !ok {
			t.Errorf("%q missing from catalog", name)
			continue
		}
		got := c.RequiredBinaries(a, "linux")
		if len(got) != 1 || got[0] != "tmux" {
			t.Errorf("alias %q requires %v, want [tmux]", name, got)
		}
	}
}

func TestResolvedRequiresSubstitutesPaths(t *testing.T) {
	c := load(t)
	p, ok := c.Function("p")
	if !ok {
		t.Fatal("function p missing")
	}
	got := p.ResolvedRequires("~/Code", "~/envs")
	found := false
	for _, r := range got {
		if r == "path:~/Code" {
			found = true
		}
		if strings.Contains(r, "{{") {
			t.Errorf("unsubstituted placeholder left in %q", r)
		}
	}
	if !found {
		t.Errorf("ResolvedRequires = %v, want it to contain path:~/Code", got)
	}
}

// clone-sources names its generated functions from config, so it must say so.
func TestCloneSourcesIsNamedByConfig(t *testing.T) {
	c := load(t)
	f, ok := c.Function("clone-sources")
	if !ok {
		t.Fatal("clone-sources function missing")
	}
	if f.NamedBy != "clone_sources" {
		t.Errorf("NamedBy = %q, want clone_sources", f.NamedBy)
	}
}
