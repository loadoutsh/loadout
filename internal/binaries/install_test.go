package binaries

import (
	"archive/tar"
	"archive/zip"
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
	"time"

	"github.com/wesscoby/loadout/internal/config"
)

const payload = "#!/bin/sh\necho hello from the test binary\n"

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func makeZip(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeTarGz(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func makeGz(t *testing.T, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(body); err != nil {
		t.Fatal(err)
	}
	gz.Close()
	return buf.Bytes()
}

// harness serves one asset over TLS and returns an installer pointed at it.
func harness(t *testing.T, assetName string, asset []byte) (*Installer, string) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if filepath.Base(r.URL.Path) != assetName {
			http.NotFound(w, r)
			return
		}
		w.Write(asset)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	inst := NewInstaller(filepath.Join(dir, "bin"), filepath.Join(dir, "state"))
	inst.Client = srv.Client()
	inst.Now = func() time.Time { return time.Unix(0, 0).UTC() }
	return inst, srv.URL + "/" + assetName
}

func TestInstallFromEachArchiveFormat(t *testing.T) {
	body := []byte(payload)
	cases := []struct {
		name  string
		asset string
		data  []byte
	}{
		{"zip", "tool.zip", makeZip(t, "tool", body)},
		{"zip nested in a directory", "tool.zip", makeZip(t, "tool-v1.0/tool", body)},
		{"tar.gz", "tool.tar.gz", makeTarGz(t, "tool", body)},
		{"tgz", "tool.tgz", makeTarGz(t, "tool-v1.0/bin/tool", body)},
		{"bare gzip", "tool.gz", makeGz(t, body)},
		{"raw binary", "tool", body},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inst, url := harness(t, c.asset, c.data)
			rec, err := inst.Install(config.Binary{
				Name:    "tool",
				Version: "v1.0",
				Source:  config.BinarySource{URL: map[string]string{Current().Key(): url}},
			}, Current())
			if err != nil {
				t.Fatalf("install: %v", err)
			}

			got, err := os.ReadFile(filepath.Join(inst.BinDir, "tool"))
			if err != nil {
				t.Fatalf("reading installed binary: %v", err)
			}
			if string(got) != payload {
				t.Errorf("content = %q, want %q", got, payload)
			}

			info, err := os.Stat(filepath.Join(inst.BinDir, "tool"))
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm()&0o111 == 0 {
				t.Errorf("installed binary is not executable (mode %v)", info.Mode())
			}
			if rec.SHA256 != sum(c.data) {
				t.Errorf("recorded sha256 = %s, want %s", rec.SHA256, sum(c.data))
			}
			if rec.Verified {
				t.Error("Verified should be false when no checksum was declared")
			}
		})
	}
}

// A declared checksum that matches must mark the record verified.
func TestChecksumVerified(t *testing.T) {
	asset := makeZip(t, "tool", []byte(payload))
	inst, url := harness(t, "tool.zip", asset)

	rec, err := inst.Install(config.Binary{
		Name:     "tool",
		Version:  "v1.0",
		Checksum: config.Checksum{Any: "sha256:" + sum(asset)},
		Source:   config.BinarySource{URL: map[string]string{Current().Key(): url}},
	}, Current())
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !rec.Verified {
		t.Error("record should be marked verified")
	}
}

// The important one: a mismatched checksum must abort before anything lands
// on PATH.
func TestChecksumMismatchRejects(t *testing.T) {
	asset := makeZip(t, "tool", []byte(payload))
	inst, url := harness(t, "tool.zip", asset)

	_, err := inst.Install(config.Binary{
		Name:     "tool",
		Version:  "v1.0",
		Checksum: config.Checksum{Any: "sha256:" + strings.Repeat("0", 64)},
		Source:   config.BinarySource{URL: map[string]string{Current().Key(): url}},
	}, Current())
	if err == nil {
		t.Fatal("install should fail on a checksum mismatch")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error = %v, want a checksum mismatch", err)
	}
	if _, statErr := os.Stat(filepath.Join(inst.BinDir, "tool")); statErr == nil {
		t.Error("nothing should have been installed after a checksum failure")
	}
}

// Plain HTTP must be refused: this content gets executed.
func TestRefusesNonHTTPS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(payload))
	}))
	defer srv.Close()

	dir := t.TempDir()
	inst := NewInstaller(filepath.Join(dir, "bin"), filepath.Join(dir, "state"))

	_, err := inst.Install(config.Binary{
		Name:   "tool",
		Source: config.BinarySource{URL: map[string]string{Current().Key(): srv.URL + "/tool"}},
	}, Current())
	if err == nil || !strings.Contains(err.Error(), "https required") {
		t.Errorf("error = %v, want an https refusal", err)
	}
}

func TestArchiveWithoutTheExecutable(t *testing.T) {
	inst, url := harness(t, "tool.zip", makeZip(t, "something-else", []byte(payload)))
	_, err := inst.Install(config.Binary{
		Name:   "tool",
		Source: config.BinarySource{URL: map[string]string{Current().Key(): url}},
	}, Current())
	if err == nil || !strings.Contains(err.Error(), "no executable named") {
		t.Errorf("error = %v, want a missing-executable error", err)
	}
}

func TestHTTPErrorSurfaces(t *testing.T) {
	inst, url := harness(t, "tool.zip", nil)
	_, err := inst.Install(config.Binary{
		Name:   "tool",
		Source: config.BinarySource{URL: map[string]string{Current().Key(): url + ".missing"}},
	}, Current())
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %v, want a 404", err)
	}
}

func TestGitHubURLConstruction(t *testing.T) {
	b := config.Binary{
		Name:    "duckdb",
		Version: "v1.1.3",
		Source: config.BinarySource{
			GitHub: "duckdb/duckdb",
			Asset:  map[string]string{"linux-amd64": "duckdb_cli-linux-amd64.zip"},
		},
	}
	got, key, err := resolveURL(b, Platform{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	want := "https://github.com/duckdb/duckdb/releases/download/v1.1.3/duckdb_cli-linux-amd64.zip"
	if got != want {
		t.Errorf("url = %s, want %s", got, want)
	}
	if key != "linux-amd64" {
		t.Errorf("matched key = %s, want linux-amd64", key)
	}
}

func TestGitHubSourceNeedsAVersion(t *testing.T) {
	_, _, err := resolveURL(config.Binary{
		Name:   "tool",
		Source: config.BinarySource{GitHub: "o/r", Asset: map[string]string{"linux-amd64": "a.zip"}},
	}, Platform{OS: "linux", Arch: "amd64"})
	if err == nil || !strings.Contains(err.Error(), "pinned version") {
		t.Errorf("error = %v, want a pinned-version error", err)
	}
}

func TestUnsupportedPlatformIsExplicit(t *testing.T) {
	_, _, err := resolveURL(config.Binary{
		Name:    "tool",
		Version: "v1",
		Source:  config.BinarySource{GitHub: "o/r", Asset: map[string]string{"linux-amd64": "a.zip"}},
	}, Platform{OS: "openbsd", Arch: "riscv64"})
	if err == nil {
		t.Fatal("expected an error for an unsupported platform")
	}
	// The message must say what is available, not just that it failed.
	if !strings.Contains(err.Error(), "linux-amd64") {
		t.Errorf("error = %v, want it to list the available platforms", err)
	}
}

// An arm64 Mac can run a universal or amd64 build, so those are tried before
// giving up.
func TestDarwinFallbacks(t *testing.T) {
	arm := Platform{OS: "darwin", Arch: "arm64"}

	if _, key, ok := arm.Select(map[string]string{"darwin-universal": "u.zip"}); !ok || key != "darwin-universal" {
		t.Errorf("universal fallback: key=%s ok=%v", key, ok)
	}
	if _, key, ok := arm.Select(map[string]string{"darwin-amd64": "x.zip"}); !ok || key != "darwin-amd64" {
		t.Errorf("amd64 fallback: key=%s ok=%v", key, ok)
	}
	// An exact match must always win over a fallback.
	if _, key, _ := arm.Select(map[string]string{
		"darwin-arm64": "a.zip", "darwin-universal": "u.zip",
	}); key != "darwin-arm64" {
		t.Errorf("exact match should win, got %s", key)
	}
	// Linux must not pick up darwin's fallbacks.
	if _, _, ok := (Platform{OS: "linux", Arch: "arm64"}).Select(map[string]string{"darwin-universal": "u.zip"}); ok {
		t.Error("linux should not match a darwin asset")
	}
}

func TestStateTracksInstalls(t *testing.T) {
	asset := makeZip(t, "tool", []byte(payload))
	inst, url := harness(t, "tool.zip", asset)
	decl := config.Binary{
		Name: "tool", Version: "v1.0",
		Source: config.BinarySource{URL: map[string]string{Current().Key(): url}},
	}

	if _, err := inst.Install(decl, Current()); err != nil {
		t.Fatal(err)
	}

	st, err := inst.Status([]config.Binary{decl})
	if err != nil {
		t.Fatal(err)
	}
	if len(st) != 1 || !st[0].Installed || st[0].Stale {
		t.Fatalf("status = %+v, want installed and current", st)
	}

	// Declaring a newer version must show as stale rather than as missing.
	decl.Version = "v2.0"
	st, _ = inst.Status([]config.Binary{decl})
	if !st[0].Stale {
		t.Error("a version bump should report as stale")
	}

	// A binary no longer declared is orphaned, not silently forgotten.
	st, _ = inst.Status(nil)
	if len(st) != 1 || !st[0].Orphaned {
		t.Errorf("status = %+v, want one orphaned entry", st)
	}

	if err := inst.Remove("tool"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(inst.BinDir, "tool")); err == nil {
		t.Error("binary should be gone after remove")
	}
	st, _ = inst.Status(nil)
	if len(st) != 0 {
		t.Errorf("status = %+v, want empty after remove", st)
	}
}

// A file deleted by hand outside loadout must report as not installed, so
// `install` puts it back rather than trusting a stale record.
func TestDeletedFileReportsNotInstalled(t *testing.T) {
	asset := makeZip(t, "tool", []byte(payload))
	inst, url := harness(t, "tool.zip", asset)
	decl := config.Binary{
		Name: "tool", Version: "v1.0",
		Source: config.BinarySource{URL: map[string]string{Current().Key(): url}},
	}
	if _, err := inst.Install(decl, Current()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(inst.BinDir, "tool")); err != nil {
		t.Fatal(err)
	}

	st, _ := inst.Status([]config.Binary{decl})
	if st[0].Installed {
		t.Error("a binary deleted outside loadout should report as not installed")
	}
}

func TestRemoveUnknownBinary(t *testing.T) {
	dir := t.TempDir()
	inst := NewInstaller(filepath.Join(dir, "bin"), filepath.Join(dir, "state"))
	if err := inst.Remove("never-installed"); err == nil {
		t.Error("removing an unmanaged binary should fail")
	}
}

func TestUnsafeArchiveMemberRejected(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("../../escape/tool")
	fmt.Fprint(w, payload)
	zw.Close()

	inst, url := harness(t, "tool.zip", buf.Bytes())
	_, err := inst.Install(config.Binary{
		Name:   "tool",
		Source: config.BinarySource{URL: map[string]string{Current().Key(): url}},
	}, Current())
	if err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Errorf("error = %v, want an unsafe-path refusal", err)
	}
}

// A redirect that downgrades to plain HTTP must be refused. Only checking the
// configured URL would miss this, and release hosts redirect constantly.
func TestRefusesHTTPRedirect(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(payload))
	}))
	defer plain.Close()

	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/tool", http.StatusFound)
	}))
	defer secure.Close()

	dir := t.TempDir()
	inst := NewInstaller(filepath.Join(dir, "bin"), filepath.Join(dir, "state"))
	// Keep the TLS trust from the test server but retain the redirect policy.
	inst.Client = &http.Client{
		Transport:     secure.Client().Transport,
		CheckRedirect: requireHTTPSRedirect,
	}

	_, err := inst.Install(config.Binary{
		Name:   "tool",
		Source: config.BinarySource{URL: map[string]string{Current().Key(): secure.URL + "/tool"}},
	}, Current())
	if err == nil || !strings.Contains(err.Error(), "https required all the way") {
		t.Errorf("error = %v, want a redirect downgrade refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(inst.BinDir, "tool")); statErr == nil {
		t.Error("nothing should have been installed")
	}
}

// A hash describes one platform's artefact. A per-platform map must be
// consulted by the platform actually being installed, or a multi-platform
// config would fail everywhere except the machine the hash was taken on.
func TestPerPlatformChecksum(t *testing.T) {
	asset := makeZip(t, "tool", []byte(payload))
	inst, url := harness(t, "tool.zip", asset)

	decl := config.Binary{
		Name:    "tool",
		Version: "v1.0",
		Source:  config.BinarySource{URL: map[string]string{Current().Key(): url}},
		Checksum: config.Checksum{ByPlatform: map[string]string{
			Current().Key(): "sha256:" + sum(asset),
			// A deliberately wrong hash for a platform we are not on: it must
			// be ignored rather than compared.
			"plan9-386": "sha256:" + strings.Repeat("f", 64),
		}},
	}
	rec, err := inst.Install(decl, Current())
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !rec.Verified {
		t.Error("the matching per-platform hash should have marked this verified")
	}
}

// A per-platform map with no entry for this platform means nothing is pinned
// here, which is unverified rather than a failure.
func TestChecksumAbsentForThisPlatform(t *testing.T) {
	asset := makeZip(t, "tool", []byte(payload))
	inst, url := harness(t, "tool.zip", asset)

	rec, err := inst.Install(config.Binary{
		Name:   "tool",
		Source: config.BinarySource{URL: map[string]string{Current().Key(): url}},
		Checksum: config.Checksum{ByPlatform: map[string]string{
			"plan9-386": "sha256:" + strings.Repeat("f", 64),
		}},
	}, Current())
	if err != nil {
		t.Fatalf("install should succeed unverified, got: %v", err)
	}
	if rec.Verified {
		t.Error("no hash pinned for this platform, so it cannot be verified")
	}
}

// A wrong per-platform hash for the platform being installed must still stop it.
func TestPerPlatformChecksumMismatchRejects(t *testing.T) {
	asset := makeZip(t, "tool", []byte(payload))
	inst, url := harness(t, "tool.zip", asset)

	_, err := inst.Install(config.Binary{
		Name:   "tool",
		Source: config.BinarySource{URL: map[string]string{Current().Key(): url}},
		Checksum: config.Checksum{ByPlatform: map[string]string{
			Current().Key(): "sha256:" + strings.Repeat("0", 64),
		}},
	}, Current())
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error = %v, want a checksum mismatch", err)
	}
}
