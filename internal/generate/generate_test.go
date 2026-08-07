package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wesscoby/loadout/internal/catalog"
	"github.com/wesscoby/loadout/internal/config"

	"gopkg.in/yaml.v3"
)

// loadFixture reads the parity config, which describes the shell environment
// that predates loadout.
func loadFixture(t *testing.T) *config.Config {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "parity", "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	var cfg config.Config
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}
	return &cfg
}

func render(t *testing.T, cfg *config.Config, goos string) string {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("loading catalog: %v", err)
	}
	g, err := New(cat, cfg, Options{Shell: "zsh", GOOS: goos, BinDir: "/tmp/bin"})
	if err != nil {
		t.Fatalf("building generator: %v", err)
	}
	out, err := g.Render()
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	return string(out)
}

func aliasNames(rendered string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(rendered, "\n") {
		if !strings.HasPrefix(line, "alias ") {
			continue
		}
		rest := strings.TrimPrefix(line, "alias ")
		eq := strings.Index(rest, "=")
		if eq < 0 {
			continue
		}
		out[rest[:eq]] = rest[eq+1:]
	}
	return out
}

// The parity fixture must reproduce the original alias set: 180 aliases less
// the 14 the triage drops.
func TestParityFixtureAliasCount(t *testing.T) {
	got := len(aliasNames(render(t, loadFixture(t), "linux")))
	const want = 166
	if got != want {
		t.Errorf("alias count = %d, want %d", got, want)
	}
}

// Every template must render without error and produce a definition.
func TestAllFunctionsRender(t *testing.T) {
	rendered := render(t, loadFixture(t), "linux")
	for _, fn := range []string{
		"activate", "denv", "gcfg", "kp", "lenv", "mpush",
		"msh", "nig", "p", "pv", "pw", "s", "venv", "wt",
	} {
		if !strings.Contains(rendered, fn+"() {") {
			t.Errorf("function %q not present in output", fn)
		}
	}
}

// A tilde interpolated into a double-quoted string does not expand, so paths
// must be emitted in $HOME form. This is the bug the shpath helper prevents.
func TestNoUnexpandedTildeInQuotes(t *testing.T) {
	for _, line := range strings.Split(render(t, loadFixture(t), "linux"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "alias ") {
			continue // aliases expand at use time, where a tilde is fine
		}
		if strings.Contains(line, `"~/`) {
			t.Errorf("tilde inside double quotes will not expand: %s", strings.TrimSpace(line))
		}
	}
}

// pw used an associative array, which needs bash 4 while macOS ships 3.2, and
// whose unquoted subscripts shfmt would rewrite as arithmetic ([a11-wb] became
// [a11 - wb]). A case statement avoids both problems, so no associative array
// may reappear in the generated output.
func TestNoAssociativeArrays(t *testing.T) {
	rendered := render(t, loadFixture(t), "linux")
	for _, banned := range []string{"local -A", "typeset -A", "declare -A"} {
		if strings.Contains(rendered, banned) {
			t.Errorf("generated output uses %q, which bash 3.2 on macOS cannot parse", banned)
		}
	}
}

// A workspace name containing a hyphen must still resolve. This is the case
// the old associative-array implementation kept getting wrong.
func TestHyphenatedWorkspaceResolves(t *testing.T) {
	rendered := render(t, loadFixture(t), "linux")
	if !strings.Contains(rendered, "a11-wb) target=") {
		t.Error("hyphenated workspace a11-wb has no case branch in the generated pw")
	}
}

// The zsh-only constructs that bash either rejects or, worse, silently
// misinterprets must not come back.
func TestNoZshOnlyConstructs(t *testing.T) {
	banned := map[string]string{
		"${=": "zsh-only word splitting; bash reports a bad substitution",
		":t}": "zsh tail modifier; bash silently returns the whole path instead",
	}
	for _, line := range strings.Split(render(t, loadFixture(t), "linux"), "\n") {
		// Comments explaining these constructs mention them by name, which is
		// not the same as using them.
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		for construct, why := range banned {
			if strings.Contains(line, construct) {
				t.Errorf("generated code contains %q: %s\n  %s", construct, why, strings.TrimSpace(line))
			}
		}
	}
}

// zsh makes OPTIND local to a function automatically; bash does not, so
// without an explicit reset a second call silently ignores its flags.
func TestGetoptsResetsOptind(t *testing.T) {
	rendered := render(t, loadFixture(t), "linux")
	if strings.Contains(rendered, "getopts") && !strings.Contains(rendered, "OPTIND=1") {
		t.Error("a function uses getopts without resetting OPTIND; in bash its flags " +
			"would be ignored on every call after the first")
	}
}

// Platform-conditional entries must resolve differently per platform.
func TestPlatformConditionals(t *testing.T) {
	cfg := loadFixture(t)
	linux := aliasNames(render(t, cfg, "linux"))
	darwin := aliasNames(render(t, cfg, "darwin"))

	if !strings.Contains(linux["ctl"], "systemctl") {
		t.Errorf("linux ctl = %s, want systemctl", linux["ctl"])
	}
	if !strings.Contains(darwin["ctl"], "launchctl") {
		t.Errorf("darwin ctl = %s, want launchctl", darwin["ctl"])
	}
	if !strings.Contains(linux["ports"], "netstat") {
		t.Errorf("linux ports = %s, want netstat", linux["ports"])
	}
	if !strings.Contains(darwin["ports"], "lsof") {
		t.Errorf("darwin ports = %s, want lsof", darwin["ports"])
	}
	// debi is Linux-only and must be absent on darwin entirely.
	if _, ok := darwin["debi"]; ok {
		t.Error("debi should not be generated on darwin")
	}
	if _, ok := linux["debi"]; !ok {
		t.Error("debi should be generated on linux")
	}
}

// Packs are off unless enabled.
func TestPacksAreOptional(t *testing.T) {
	cfg := loadFixture(t)
	cfg.Packs = nil
	got := aliasNames(render(t, cfg, "linux"))
	for _, name := range []string{"fl", "jk", "g:dev", "xsm", "ff", "cf", "ans"} {
		if _, ok := got[name]; ok {
			t.Errorf("pack alias %q present with no packs enabled", name)
		}
	}
	if _, ok := got["gpush"]; !ok {
		t.Error("core alias gpush missing")
	}
}

// A disabled alias must not be generated.
func TestDisabledAliases(t *testing.T) {
	cfg := loadFixture(t)
	cfg.Aliases.Disabled = []string{"gpush", "dk"}
	got := aliasNames(render(t, cfg, "linux"))
	for _, name := range cfg.Aliases.Disabled {
		if _, ok := got[name]; ok {
			t.Errorf("disabled alias %q was generated", name)
		}
	}
}

// Single quotes inside an alias body must survive quoting intact.
func TestAliasQuoteEscaping(t *testing.T) {
	cfg := loadFixture(t)
	cfg.Aliases.Custom = append(cfg.Aliases.Custom, config.CustomAlias{
		Name:    "quoted",
		Command: `echo 'hello world'`,
	})
	got := aliasNames(render(t, cfg, "linux"))
	want := `'echo '\''hello world'\'''`
	if got["quoted"] != want {
		t.Errorf("quoted alias = %s, want %s", got["quoted"], want)
	}
}
