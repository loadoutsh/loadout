package sync

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- scanner ---------------------------------------------------------------

func TestScanCatchesRealCredentialShapes(t *testing.T) {
	cases := map[string]string{
		"GitHub classic token":     "  token: ghp_" + strings.Repeat("A", 36),
		"GitHub fine-grained":      "  token: github_pat_" + strings.Repeat("B", 30),
		"npm token":                "  npm: npm_" + strings.Repeat("C", 36),
		"AWS access key":           "  key: AKIAIOSFODNN7EXAMPLE",
		"Slack token":              "  slack: xoxb-1234567890-abcdefghij",
		"OpenAI key":               "  openai: sk-" + strings.Repeat("d", 32),
		"JWT":                      "  motherduck: eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N",
		"PEM private key":          "-----BEGIN OPENSSH PRIVATE KEY-----",
		"literal password field":   "  password: hunter2",
		"literal api_key field":    "  api_key: abcdef123456",
		"password inside a commen": "# old value was ghp_" + strings.Repeat("E", 36),
	}
	for name, line := range cases {
		if got := Scan(line); len(got) == 0 {
			t.Errorf("%s: not detected in %q", name, line)
		}
	}
}

// The config legitimately contains hostnames, paths, entry names and a
// keychain reference. None of those may trip the scanner, or it becomes noise
// that people learn to bypass.
func TestScanDoesNotFireOnNormalConfig(t *testing.T) {
	cfg := `version: 1
paths:
  project_root: ~/Projects
  venv_root: ~/.venv
ssh:
  default_key: sucasa
  default_user: root
  subnet_prefix: "10.0.0."
credentials:
  npm_token:
    active: personal
    entries: [personal, work]
  gh_token:
    active: work
    entries: [personal, work]
identities:
  - id: wc
    git_user: Wess
    email: wess@example.com
    ssh_key: ~/.ssh/wc_ed
    activates:
      npm_token: work
hosts:
  - alias: api
    hostname: api.example.internal
    user: azureuser
    key: ~/.ssh/deploy
    password: cred:host_password
binaries:
  - name: duckdb
    version: v1.1.3
    checksum: sha256:efd0fccdb1a28d9ec7a6ebfcde59900068b8ba43a846c9b553c0fd2bbe4acf43
packs: [flutter, ansible]
`
	if got := Scan(cfg); len(got) != 0 {
		for _, f := range got {
			t.Errorf("false positive: %s", f)
		}
	}
}

// "password: cred:name" is the correct way to reference a secret and must not
// be confused with a literal one.
func TestScanAllowsCredentialReferences(t *testing.T) {
	for _, line := range []string{
		`    password: cred:host_password`,
		`    password: "cred:host_password"`,
		`    token: cred:npm_token`,
	} {
		if got := Scan(line); len(got) != 0 {
			t.Errorf("%q should be allowed, got %v", line, got)
		}
	}
}

func TestScanReportsTheLineNumber(t *testing.T) {
	content := "version: 1\npaths:\n  a: b\n  token: ghp_" + strings.Repeat("A", 36) + "\n"
	got := Scan(content)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	if got[0].Line != 4 {
		t.Errorf("line = %d, want 4", got[0].Line)
	}
}

// --- repo ------------------------------------------------------------------

func gitAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

// newRepo builds a repo in a temp dir with an identity, so commits work in
// environments with no global git config.
func newRepo(t *testing.T) *Repo {
	t.Helper()
	gitAvailable(t)
	r := New(t.TempDir())
	if err := r.Init(""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.git("config", "user.email", "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.git("config", "user.name", "Test"); err != nil {
		t.Fatal(err)
	}
	return r
}

func write(t *testing.T, r *Repo, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(r.Dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInitWritesGitignore(t *testing.T) {
	r := newRepo(t)
	data, err := os.ReadFile(filepath.Join(r.Dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"init.*", "local.yaml"} {
		if !strings.Contains(string(data), want) {
			t.Errorf(".gitignore should exclude %q", want)
		}
	}
}

// The generated file and the machine-local overlay must never be tracked:
// one conflicts on every machine, the other exists to stay put.
func TestGeneratedAndLocalFilesAreNotTracked(t *testing.T) {
	r := newRepo(t)
	write(t, r, "config.yaml", "version: 1\n")
	write(t, r, "init.zsh", "alias x=y\n")
	write(t, r, "local.yaml", "paths:\n  project_root: /srv\n")

	if _, err := r.git("add", "-A"); err != nil {
		t.Fatal(err)
	}
	staged, err := r.git("diff", "--cached", "--name-only")
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"init.zsh", "local.yaml"} {
		if strings.Contains(staged, unwanted) {
			t.Errorf("%s should not be staged, got: %s", unwanted, staged)
		}
	}
	if !strings.Contains(staged, "config.yaml") {
		t.Errorf("config.yaml should be staged, got: %s", staged)
	}
}

// The important guarantee: a secret in the config stops the push before the
// commit, so it never reaches the history.
func TestPushRefusesAndDoesNotCommitSecrets(t *testing.T) {
	r := newRepo(t)
	write(t, r, "config.yaml", "version: 1\ntoken: ghp_"+strings.Repeat("A", 36)+"\n")

	_, err := r.Push("should not happen")
	if err == nil {
		t.Fatal("push should have been refused")
	}
	var se *ErrSecrets
	if !asErrSecrets(err, &se) {
		t.Fatalf("error = %T %v, want *ErrSecrets", err, err)
	}
	if se.File != "config.yaml" {
		t.Errorf("file = %q, want config.yaml", se.File)
	}

	// Nothing may have been committed.
	if out, _ := r.git("rev-list", "--count", "--all"); out != "" && out != "0" {
		t.Errorf("a commit was created despite the refusal (%s commits)", out)
	}
	// The working tree must be left as it was found, not half-staged.
	staged, _ := r.git("diff", "--cached", "--name-only")
	if staged != "" {
		t.Errorf("files left staged after refusal: %s", staged)
	}
}

func asErrSecrets(err error, target **ErrSecrets) bool {
	if e, ok := err.(*ErrSecrets); ok {
		*target = e
		return true
	}
	return false
}

func TestPushCommitsCleanConfig(t *testing.T) {
	r := newRepo(t)
	write(t, r, "config.yaml", "version: 1\npaths:\n  project_root: ~/Projects\n")

	// No remote, so the push itself fails, but the commit must have happened.
	_, err := r.Push("first")
	if err == nil || !strings.Contains(err.Error(), "no remote") {
		t.Fatalf("error = %v, want a missing-remote error", err)
	}
	out, err := r.git("rev-list", "--count", "HEAD")
	if err != nil || out != "1" {
		t.Errorf("commit count = %q (err %v), want 1", out, err)
	}
}

func TestStatusReportsTrackedFiles(t *testing.T) {
	r := newRepo(t)
	write(t, r, "config.yaml", "version: 1\n")
	write(t, r, "local.yaml", "version: 1\n")
	if _, err := r.Push("initial"); err == nil {
		t.Fatal("expected a missing-remote error")
	}

	files, err := r.TrackedFiles()
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(files, " ")
	if !strings.Contains(joined, "config.yaml") {
		t.Errorf("tracked = %v, want config.yaml", files)
	}
	if strings.Contains(joined, "local.yaml") {
		t.Errorf("local.yaml must never be tracked, got %v", files)
	}
}

func TestRemoteRoundTrip(t *testing.T) {
	r := newRepo(t)
	if r.Remote() != "" {
		t.Error("a fresh repo should have no remote")
	}
	const url = "git@github.com:user/loadout-config.git"
	if err := r.SetRemote(url); err != nil {
		t.Fatal(err)
	}
	if got := r.Remote(); got != url {
		t.Errorf("remote = %q, want %q", got, url)
	}
	// Setting it again must replace rather than fail.
	const other = "git@github.com:user/other.git"
	if err := r.SetRemote(other); err != nil {
		t.Fatal(err)
	}
	if got := r.Remote(); got != other {
		t.Errorf("remote = %q, want %q", got, other)
	}
}

func TestOperationsRequireInitialisation(t *testing.T) {
	gitAvailable(t)
	r := New(t.TempDir())
	if _, err := r.Status(); err == nil {
		t.Error("Status should fail on an uninitialised directory")
	}
	if _, err := r.Push(""); err == nil {
		t.Error("Push should fail on an uninitialised directory")
	}
	if _, err := r.Pull(); err == nil {
		t.Error("Pull should fail on an uninitialised directory")
	}
}

// Pulling over uncommitted edits could lose them, so it is refused.
func TestPullRefusesWithUncommittedChanges(t *testing.T) {
	r := newRepo(t)
	write(t, r, "config.yaml", "version: 1\n")
	if err := r.SetRemote("git@example.com:x/y.git"); err != nil {
		t.Fatal(err)
	}
	_, err := r.Pull()
	if err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Errorf("error = %v, want a refusal about uncommitted changes", err)
	}
}

// Before the first commit the branch is "unborn" and rev-parse fails on it,
// which left the branch name blank in status output.
func TestStatusReportsBranchBeforeFirstCommit(t *testing.T) {
	r := newRepo(t)
	st, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st.Branch != "main" {
		t.Errorf("branch = %q on a repo with no commits, want main", st.Branch)
	}
}
