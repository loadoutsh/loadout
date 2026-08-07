package uninstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sandbox points config and data at temporary directories, so no test can
// touch the developer's real install.
func sandbox(t *testing.T) (cfgDir, dataDir string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))

	cfgDir = filepath.Join(root, "config", "loadout")
	dataDir = filepath.Join(root, "data", "loadout")
	for _, d := range []string{cfgDir, filepath.Join(dataDir, "bin")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(cfgDir, "config.yaml"), "version: 1\n")
	write(t, filepath.Join(cfgDir, "init.zsh"), "alias x=y\n")
	write(t, filepath.Join(dataDir, "binaries.json"), "{}\n")
	write(t, filepath.Join(dataDir, "bin", "duckdb"), "#!/bin/sh\n")
	write(t, filepath.Join(dataDir, "bin", "csf"), "#!/bin/sh\n")
	return cfgDir, dataDir
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildFindsEverything(t *testing.T) {
	cfgDir, dataDir := sandbox(t)

	p, err := Build(Options{KeepCredentials: true})
	if err != nil {
		t.Fatal(err)
	}
	if p.Empty() {
		t.Fatal("plan should not be empty")
	}

	var paths []string
	for _, i := range p.Paths {
		paths = append(paths, i.Name)
	}
	for _, want := range []string{cfgDir, dataDir} {
		found := false
		for _, got := range paths {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("plan omits %s (has %v)", want, paths)
		}
	}

	// The managed executables are listed by name, so the summary can say what
	// leaves PATH rather than only naming a directory.
	var bins []string
	for _, b := range p.Binaries {
		bins = append(bins, b.Name)
	}
	if len(bins) != 2 {
		t.Errorf("binaries = %v, want duckdb and csf", bins)
	}
}

// Building a plan must never remove anything. This is the whole basis of the
// dry run being safe.
func TestBuildRemovesNothing(t *testing.T) {
	cfgDir, dataDir := sandbox(t)

	if _, err := Build(Options{}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{cfgDir, dataDir, filepath.Join(cfgDir, "config.yaml")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("Build removed %s", p)
		}
	}
}

func TestExecuteRemovesPaths(t *testing.T) {
	cfgDir, dataDir := sandbox(t)

	p, err := Build(Options{KeepCredentials: true})
	if err != nil {
		t.Fatal(err)
	}
	res := p.Execute(false)
	if len(res.Failed) > 0 {
		t.Fatalf("failures: %v", res.Failed)
	}
	for _, path := range []string{cfgDir, dataDir} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("%s still exists after Execute", path)
		}
	}
}

// --keep-config must leave the user's configuration but still remove the
// generated file, which is loadout's output rather than the user's data.
func TestKeepConfigRetainsConfigButNotOutput(t *testing.T) {
	cfgDir, dataDir := sandbox(t)

	p, err := Build(Options{KeepConfig: true, KeepCredentials: true})
	if err != nil {
		t.Fatal(err)
	}
	if res := p.Execute(false); len(res.Failed) > 0 {
		t.Fatalf("failures: %v", res.Failed)
	}

	if _, err := os.Stat(filepath.Join(cfgDir, "config.yaml")); err != nil {
		t.Error("config.yaml should survive --keep-config")
	}
	if _, err := os.Stat(filepath.Join(cfgDir, "init.zsh")); err == nil {
		t.Error("the generated init.zsh should be removed even with --keep-config")
	}
	if _, err := os.Stat(dataDir); err == nil {
		t.Error("the data directory should still be removed")
	}
}

func TestKeepCredentialsListsNone(t *testing.T) {
	sandbox(t)
	p, err := Build(Options{KeepCredentials: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Credentials) != 0 {
		t.Errorf("credentials = %v, want none with --keep-credentials", p.Credentials)
	}
}

// The guard is the difference between an uninstall and an accident.
func TestSafeRemoveRefusesPathsItDoesNotOwn(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "important.txt"), "do not delete me")

	for _, path := range []string{home, "/", filepath.Join(dir, "important.txt")} {
		if err := safeRemove(path); err == nil {
			t.Errorf("safeRemove(%q) should have refused", path)
		}
	}
	// The bystander file must still be there.
	if _, err := os.Stat(filepath.Join(dir, "important.txt")); err != nil {
		t.Error("safeRemove deleted a file it should have refused")
	}
}

// A path is only removable because it belongs to loadout, not because it was
// passed in.
func TestSafeRemoveAcceptsLoadoutPaths(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "loadout")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "config.yaml"), "version: 1\n")

	if err := safeRemove(dir); err != nil {
		t.Fatalf("safeRemove refused a loadout path: %v", err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("directory should be gone")
	}
}

func TestEmptyWhenNothingInstalled(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "nope"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "also-nope"))

	p, err := Build(Options{KeepCredentials: true})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Empty() {
		t.Errorf("plan should be empty when nothing is installed, got %+v", p.Paths)
	}
}

// The shell config is the user's file; loadout reports the line and never
// edits it.
func TestShellIntegrationIsReportedNotRemoved(t *testing.T) {
	cfgDir, _ := sandbox(t)

	home := t.TempDir()
	t.Setenv("HOME", home)
	rc := filepath.Join(home, ".zshrc")
	line := "source " + cfgDir + "/init.zsh"
	write(t, rc, "# my config\n"+line+"\nexport FOO=bar\n")

	p, err := Build(Options{KeepCredentials: true})
	if err != nil {
		t.Fatal(err)
	}
	if p.ShellRC != rc {
		t.Errorf("ShellRC = %q, want %q", p.ShellRC, rc)
	}
	if !strings.Contains(p.ShellLine, "init.zsh") {
		t.Errorf("ShellLine = %q, want the source line", p.ShellLine)
	}

	p.Execute(false)

	after, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), line) {
		t.Error("Execute modified the shell config; it must never do that")
	}
}

// A commented-out line is not active integration and should not be reported
// as the thing to remove.
func TestCommentedShellLineIsIgnored(t *testing.T) {
	cfgDir, _ := sandbox(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	write(t, filepath.Join(home, ".zshrc"), "# source "+cfgDir+"/init.zsh\n")

	p, err := Build(Options{KeepCredentials: true})
	if err != nil {
		t.Fatal(err)
	}
	if p.ShellLine != "" {
		t.Errorf("ShellLine = %q, want empty for a commented-out line", p.ShellLine)
	}
}
