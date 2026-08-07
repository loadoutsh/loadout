package generate

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// update rewrites the golden files instead of comparing against them:
//
//	go test ./internal/generate -update
//
// The resulting diff is the point. It shows exactly how a catalog or template
// change alters what lands in every user's shell, which is the thing worth
// reviewing.
var update = flag.Bool("update", false, "rewrite the golden files")

// goldenCases cover every combination the generator supports. Both shells and
// both platforms, because a change can easily be correct in one and wrong in
// another.
var goldenCases = []struct {
	name  string
	shell string
	goos  string
}{
	{"zsh-linux", "zsh", "linux"},
	{"zsh-darwin", "zsh", "darwin"},
	{"bash-linux", "bash", "linux"},
	{"bash-darwin", "bash", "darwin"},
}

// TestGolden pins the complete generated output.
//
// This replaces the parity script, which compared against the hand-written
// files this project grew out of. That comparison proved the migration was
// faithful, which it did, once. What matters now is different: that a change
// to the catalog or a template does not alter the generated shell in some way
// nobody intended. A snapshot answers that; a frozen 2023 snapshot plus a
// growing list of documented exceptions does not.
func TestGolden(t *testing.T) {
	cfg := loadFixture(t)

	for _, c := range goldenCases {
		t.Run(c.name, func(t *testing.T) {
			cat := mustCatalog(t)
			g, err := New(cat, cfg, Options{
				Shell: c.shell,
				GOOS:  c.goos,
				// A fixed path, so the golden file does not depend on whose
				// machine generated it.
				BinDir: "/home/example/.local/share/loadout/bin",
			})
			if err != nil {
				t.Fatalf("building generator: %v", err)
			}
			got, err := g.Render()
			if err != nil {
				t.Fatalf("rendering: %v", err)
			}

			path := filepath.Join("..", "..", "testdata", "golden", c.name+"."+c.shell)

			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				t.Logf("wrote %s (%d bytes)", path, len(got))
				return
			}

			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading golden file: %v\nrun: go test ./internal/generate -update", err)
			}
			if string(got) == string(want) {
				return
			}

			t.Errorf("generated output differs from %s\n%s\n\n"+
				"If the change is intended, re-record with:\n"+
				"  go test ./internal/generate -update\n"+
				"and review the resulting diff.",
				path, firstDifference(string(want), string(got)))
		})
	}
}

// firstDifference reports the first differing line with a little context,
// which is more use than dumping six hundred lines.
func firstDifference(want, got string) string {
	w := strings.Split(want, "\n")
	g := strings.Split(got, "\n")

	for i := 0; i < len(w) || i < len(g); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl == gl {
			continue
		}
		var b strings.Builder
		b.WriteString("first difference at line ")
		b.WriteString(itoa(i + 1))
		b.WriteString(":\n")
		for j := max(0, i-2); j < min(max(len(w), len(g)), i+3); j++ {
			marker := "  "
			if j == i {
				marker = "> "
			}
			var a, c string
			if j < len(w) {
				a = w[j]
			}
			if j < len(g) {
				c = g[j]
			}
			if a == c {
				b.WriteString("  " + marker + "  " + a + "\n")
				continue
			}
			b.WriteString("  " + marker + "- " + a + "\n")
			b.WriteString("  " + marker + "+ " + c + "\n")
		}
		b.WriteString("\n(" + itoa(len(w)) + " lines expected, " + itoa(len(g)) + " produced)")
		return b.String()
	}
	return "files differ only in trailing content"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
