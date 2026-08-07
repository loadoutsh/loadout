package backend

import (
	"strings"
	"testing"
)

func all(t *testing.T) []Backend {
	t.Helper()
	var out []Backend
	for _, name := range Names() {
		b, err := Get(name)
		if err != nil {
			t.Fatalf("Get(%q): %v", name, err)
		}
		out = append(out, b)
	}
	if len(out) < 2 {
		t.Fatalf("expected at least two backends, got %v", Names())
	}
	return out
}

func TestRegistry(t *testing.T) {
	for _, name := range []string{"zsh", "bash"} {
		b, err := Get(name)
		if err != nil {
			t.Errorf("Get(%q): %v", name, err)
			continue
		}
		if b.Name() != name {
			t.Errorf("backend %q reports name %q", name, b.Name())
		}
	}
	if _, err := Get("fish"); err == nil {
		t.Error("Get should fail for an unregistered dialect")
	}
}

// Every backend must produce a distinct output filename, or one would
// overwrite the other.
func TestExtensionsAreDistinct(t *testing.T) {
	seen := map[string]string{}
	for _, b := range all(t) {
		if prev, dup := seen[b.Ext()]; dup {
			t.Errorf("%s and %s both use extension %q", prev, b.Name(), b.Ext())
		}
		seen[b.Ext()] = b.Name()
	}
}

// A secret or a command containing a quote must survive intact in every
// dialect, since the alias body is written verbatim into a sourced file.
func TestAliasQuotingIsSafeInEveryBackend(t *testing.T) {
	cases := map[string]string{
		"plain":       "git push",
		"with quotes": `echo 'hi there'`,
		"with dollar": `echo $HOME`,
		"with subst":  "echo $(whoami)",
		"with pipe":   "dig +short x | pbcopy",
	}
	for _, b := range all(t) {
		for name, cmd := range cases {
			got := b.Alias("a", cmd)
			if !strings.HasPrefix(got, "alias a=") {
				t.Errorf("%s/%s: %q does not start with the alias keyword", b.Name(), name, got)
			}
			// Single quotes mean nothing is expanded when the file is sourced.
			if !strings.Contains(got, "'") {
				t.Errorf("%s/%s: body is not single-quoted: %s", b.Name(), name, got)
			}
		}
		// An embedded single quote must be closed, escaped and reopened.
		if got := b.Alias("q", "it's"); got != `alias q='it'\''s'` {
			t.Errorf("%s: quote escaping = %s", b.Name(), got)
		}
	}
}

// Re-sourcing the generated file must not grow PATH without bound.
func TestPathAppendIsIdempotent(t *testing.T) {
	for _, b := range all(t) {
		got := b.PathAppend("/opt/loadout/bin")
		if !strings.Contains(got, `":$PATH:"`) {
			t.Errorf("%s: PathAppend does not guard against duplicates: %s", b.Name(), got)
		}
		if !strings.Contains(got, "/opt/loadout/bin") {
			t.Errorf("%s: PathAppend omits the directory", b.Name())
		}
	}
}

// bash expands aliases only in interactive shells unless told otherwise, so a
// script sourcing the generated file would get the functions and none of the
// aliases. zsh needs nothing.
func TestPreambles(t *testing.T) {
	zsh, _ := Get("zsh")
	if p := zsh.Preamble(); p != "" {
		t.Errorf("zsh needs no preamble, got %q", p)
	}

	bash, _ := Get("bash")
	p := bash.Preamble()
	if !strings.Contains(p, "expand_aliases") {
		t.Errorf("bash preamble must enable alias expansion, got %q", p)
	}
}

func TestCommentAndSectionHeader(t *testing.T) {
	for _, b := range all(t) {
		if got := b.Comment("hello"); !strings.HasPrefix(got, "#") {
			t.Errorf("%s: comment %q does not start with #", b.Name(), got)
		}
		if got := b.Comment(""); got != "#" {
			t.Errorf("%s: empty comment = %q, want #", b.Name(), got)
		}
		got := b.SectionHeader("Git")
		if !strings.HasPrefix(got, "# Git ") {
			t.Errorf("%s: section header = %q", b.Name(), got)
		}
	}
}
