package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		current, latest string
		want            Comparison
	}{
		{"v0.1.0", "v0.2.0", Older},
		{"0.1.0", "0.1.1", Older},
		{"v1.0.0", "v1.0.0", Same},
		{"v2.0.0", "v1.9.9", Newer},
		{"v0.10.0", "v0.9.0", Newer}, // numeric, not lexical
		{"v0.9.0", "v0.10.0", Older},
		// A development build cannot be compared with a tag, and guessing
		// either way would be worse than saying so.
		{"dev", "v1.0.0", Unknown},
		{"cb594b4", "v1.0.0", Unknown},
		{"v1.0.0-rc1", "v1.0.0", Unknown},
		{"v1.0", "v1.0.0", Unknown},
		{"", "v1.0.0", Unknown},
	}
	for _, c := range cases {
		if got := Compare(c.current, c.latest); got != c.want {
			t.Errorf("Compare(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

// Only a manually installed binary may be replaced. Overwriting a file a
// package manager owns leaves it reporting a version it never installed.
func TestOnlyManualIsSelfUpdatable(t *testing.T) {
	cases := map[Method]bool{
		MethodManual:    true,
		MethodHomebrew:  false,
		MethodGoInstall: false,
		MethodSource:    false,
	}
	for m, want := range cases {
		if got := m.SelfUpdatable(); got != want {
			t.Errorf("%s.SelfUpdatable() = %v, want %v", m, got, want)
		}
		if m.UpdateCommand() == "" {
			t.Errorf("%s has no update command to suggest", m)
		}
	}
	if MethodHomebrew.UpdateCommand() != "brew upgrade loadout" {
		t.Errorf("homebrew command = %q", MethodHomebrew.UpdateCommand())
	}
}

// releaseServer serves a tarball and a checksums file the way the Makefile
// produces them.
func releaseServer(t *testing.T, version, binaryContent string, corrupt bool) (*Client, string) {
	t.Helper()

	var tarBuf bytes.Buffer
	gz := gzip.NewWriter(&tarBuf)
	tw := tar.NewWriter(gz)
	body := []byte(binaryContent)
	if err := tw.WriteHeader(&tar.Header{
		Name: "loadout", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	tw.Write(body)
	tw.Close()
	gz.Close()
	archive := tarBuf.Bytes()

	sum := sha256.Sum256(archive)
	declared := hex.EncodeToString(sum[:])
	if corrupt {
		declared = strings.Repeat("0", 64)
	}
	asset := AssetName(version)
	checksums := fmt.Sprintf("%s  %s\n", declared, asset)

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch filepath.Base(r.URL.Path) {
		case asset:
			w.Write(archive)
		case "checksums.txt":
			w.Write([]byte(checksums))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewClient()
	c.HTTP = srv.Client()
	c.BaseURL = srv.URL
	c.Version = version
	return c, srv.URL
}

func TestApplyReplacesTheBinary(t *testing.T) {
	c, _ := releaseServer(t, "v1.0.0", "new binary contents", false)

	dir := t.TempDir()
	exe := filepath.Join(dir, "loadout")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	rel, err := c.Latest()
	if err != nil {
		t.Fatal(err)
	}
	if rel.Checksum == "" {
		t.Fatal("the release should have advertised a checksum")
	}
	if err := c.Apply(rel, exe); err != nil {
		t.Fatalf("apply: %v", err)
	}

	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new binary contents" {
		t.Errorf("binary contents = %q", got)
	}
	info, _ := os.Stat(exe)
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("replaced binary is not executable (%v)", info.Mode())
	}
	// No debris left beside it.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory holds %v, want only the binary", names)
	}
}

// The important one: a bad checksum must leave the existing binary alone.
func TestApplyRefusesOnChecksumMismatch(t *testing.T) {
	c, _ := releaseServer(t, "v1.0.0", "malicious", true)

	dir := t.TempDir()
	exe := filepath.Join(dir, "loadout")
	os.WriteFile(exe, []byte("original"), 0o755)

	rel, _ := c.Latest()
	err := c.Apply(rel, exe)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v, want a checksum mismatch", err)
	}

	got, _ := os.ReadFile(exe)
	if string(got) != "original" {
		t.Error("the existing binary was modified despite the mismatch")
	}
}

// A release with no checksum file must not be installed. Replacing the
// running executable with something unverified is exactly what this guards.
func TestApplyRefusesWithoutAChecksum(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if filepath.Base(r.URL.Path) == "checksums.txt" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("anything"))
	}))
	defer srv.Close()

	c := NewClient()
	c.HTTP = srv.Client()
	c.BaseURL = srv.URL
	c.Version = "v1.0.0"

	dir := t.TempDir()
	exe := filepath.Join(dir, "loadout")
	os.WriteFile(exe, []byte("original"), 0o755)

	rel, _ := c.Latest()
	err := c.Apply(rel, exe)
	if err == nil || !strings.Contains(err.Error(), "no checksum") {
		t.Fatalf("error = %v, want a refusal about the missing checksum", err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "original" {
		t.Error("the binary was replaced without verification")
	}
}

func TestApplyRefusesPlainHTTP(t *testing.T) {
	c := NewClient()
	dir := t.TempDir()
	exe := filepath.Join(dir, "loadout")
	os.WriteFile(exe, []byte("original"), 0o755)

	err := c.Apply(Release{AssetURL: "http://example.com/loadout.tar.gz", Checksum: "x"}, exe)
	if err == nil || !strings.Contains(err.Error(), "https required") {
		t.Errorf("error = %v, want an https refusal", err)
	}
}

// An archive without the binary is a broken release, not something to install.
func TestApplyRejectsAnArchiveWithoutTheBinary(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "README.md", Mode: 0o644, Size: 3, Typeflag: tar.TypeReg})
	tw.Write([]byte("hi\n"))
	tw.Close()
	gz.Close()

	sum := sha256.Sum256(buf.Bytes())
	asset := AssetName("v1.0.0")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if filepath.Base(r.URL.Path) == "checksums.txt" {
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), asset)
			return
		}
		w.Write(buf.Bytes())
	}))
	defer srv.Close()

	c := NewClient()
	c.HTTP = srv.Client()
	c.BaseURL = srv.URL
	c.Version = "v1.0.0"

	dir := t.TempDir()
	exe := filepath.Join(dir, "loadout")
	os.WriteFile(exe, []byte("original"), 0o755)

	rel, _ := c.Latest()
	err := c.Apply(rel, exe)
	if err == nil || !strings.Contains(err.Error(), "no loadout binary") {
		t.Errorf("error = %v, want a complaint about the archive", err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "original" {
		t.Error("the binary was replaced from a broken archive")
	}
}

// A permission problem should surface before anything is downloaded.
func TestApplyReportsAnUnwritableDirectory(t *testing.T) {
	c, _ := releaseServer(t, "v1.0.0", "new", false)

	dir := t.TempDir()
	exe := filepath.Join(dir, "loadout")
	os.WriteFile(exe, []byte("original"), 0o755)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Skip("cannot make the directory read-only")
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	rel, _ := c.Latest()
	err := c.Apply(rel, exe)
	if err == nil || !strings.Contains(err.Error(), "cannot write to") {
		t.Errorf("error = %v, want a permission message naming the directory", err)
	}
}

func TestAssetNameMatchesTheReleaseLayout(t *testing.T) {
	got := AssetName("v1.2.3")
	if !strings.HasPrefix(got, "loadout_v1.2.3_") || !strings.HasSuffix(got, ".tar.gz") {
		t.Errorf("AssetName = %q, want loadout_v1.2.3_<os>_<arch>.tar.gz", got)
	}
}

// A base url without a version cannot name the asset, since release
// filenames carry the version. Failing clearly beats fetching a 404.
func TestBaseURLRequiresAVersion(t *testing.T) {
	c := NewClient()
	c.BaseURL = "https://mirror.example.com/loadout"
	_, err := c.Latest()
	if err == nil || !strings.Contains(err.Error(), "needs a version") {
		t.Errorf("error = %v, want a complaint about the missing version", err)
	}
}
