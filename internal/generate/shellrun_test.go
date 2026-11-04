package generate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loadoutsh/loadout/internal/config"
)

// testShells are the dialects behaviour tests run in. Parsing is not
// behaviour, so these tests execute the generated functions in each one.
var testShells = []string{"bash", "zsh"}

// shellRun is one run of a generated function under stubbed commands.
type shellRun struct {
	home  string // HOME the function saw
	calls string // one line per stubbed command call: "<name> <args>"
	out   string // combined stdout and stderr of the shell
	err   error  // non-nil when the function returned non-zero
}

// writeScript renders cfg for shell and writes it where a shell can source it.
func writeScript(t *testing.T, cfg *config.Config, shell string) string {
	t.Helper()
	if _, err := exec.LookPath(shell); err != nil {
		t.Skipf("%s is not installed", shell)
	}
	g, err := New(mustCatalog(t), cfg, Options{Shell: shell, GOOS: "linux", BinDir: "/tmp/bin"})
	if err != nil {
		t.Fatalf("building generator: %v", err)
	}
	out, err := g.Render()
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	script := filepath.Join(t.TempDir(), "loadout."+shell)
	if err := os.WriteFile(script, out, 0o644); err != nil {
		t.Fatal(err)
	}
	return script
}

// runFunction sources script in shell and runs call with args. Each name in
// stubs is replaced by a command that only logs how it was called. setup, if
// given, runs against the fresh HOME first, to create files the function
// checks for.
func runFunction(t *testing.T, shell, script string, stubs []string, setup func(home string), call string, args ...string) shellRun {
	t.Helper()
	home := t.TempDir()
	if setup != nil {
		setup(home)
	}
	stubDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "calls.log")
	for _, name := range stubs {
		stub := "#!/bin/sh\necho \"" + name + " $*\" >> \"$CALL_LOG\"\n"
		if err := os.WriteFile(filepath.Join(stubDir, name), []byte(stub), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	cmd := exec.Command(shell, append([]string{"-c", `. "$1" && shift && "$@"`, "loadout-test", script, call}, args...)...)
	// Run from HOME, so relative paths in args resolve to files setup made.
	cmd.Dir = home
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + stubDir + ":/usr/bin:/bin",
		"CALL_LOG=" + logPath,
	}
	out, err := cmd.CombinedOutput()
	calls, _ := os.ReadFile(logPath)
	return shellRun{home: home, calls: string(calls), out: string(out), err: err}
}

// callsTo returns the logged calls of one stubbed command.
func (r shellRun) callsTo(name string) []string {
	var out []string
	for _, line := range strings.Split(r.calls, "\n") {
		if line == name || strings.HasPrefix(line, name+" ") {
			out = append(out, line)
		}
	}
	return out
}

func hasLine(log, want string) bool {
	for _, line := range strings.Split(log, "\n") {
		if line == want {
			return true
		}
	}
	return false
}
