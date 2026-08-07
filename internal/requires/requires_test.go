package requires

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in       string
		wantKind Kind
		wantVal  string
	}{
		{"git", Binary, "git"},
		{"binary:git", Binary, "git"},
		{"path:~/Projects", Path, "~/Projects"},
		{"config:workspaces", Config, "workspaces"},
		// A path containing a colon must keep everything after the first one.
		{"path:/opt/a:b", Path, "/opt/a:b"},
	}
	for _, c := range cases {
		got, err := Parse(c.in)
		if err != nil {
			t.Errorf("Parse(%q) errored: %v", c.in, err)
			continue
		}
		if got.Kind != c.wantKind || got.Value != c.wantVal {
			t.Errorf("Parse(%q) = %v/%q, want %v/%q", c.in, got.Kind, got.Value, c.wantKind, c.wantVal)
		}
	}

	if _, err := Parse("nonsense:x"); err == nil {
		t.Error("Parse should reject an unknown kind")
	}
}

func TestCheckBinary(t *testing.T) {
	c := New(nil)
	// "go" must exist: these tests are running under it.
	if res := c.Check(Requirement{Kind: Binary, Value: "go"}); !res.OK {
		t.Error("go should be found on PATH")
	}
	if res := c.Check(Requirement{Kind: Binary, Value: "definitely-not-a-real-binary"}); res.OK {
		t.Error("a nonexistent binary should not be found")
	}
}

func TestCheckPath(t *testing.T) {
	dir := t.TempDir()
	c := New(nil)

	if res := c.Check(Requirement{Kind: Path, Value: dir}); !res.OK {
		t.Error("temp dir should exist")
	}
	missing := filepath.Join(dir, "nope")
	res := c.Check(Requirement{Kind: Path, Value: missing})
	if res.OK {
		t.Error("missing path should not report OK")
	}
	if res.Detail != missing {
		t.Errorf("detail = %q, want the resolved path %q", res.Detail, missing)
	}
}

func TestCheckConfig(t *testing.T) {
	c := New(func(section string) (bool, bool) {
		switch section {
		case "workspaces":
			return true, true
		case "hosts":
			return false, true
		default:
			return false, false
		}
	})

	if res := c.Check(Requirement{Kind: Config, Value: "workspaces"}); !res.OK {
		t.Error("populated section should report OK")
	}
	if res := c.Check(Requirement{Kind: Config, Value: "hosts"}); res.OK {
		t.Error("empty section should not report OK")
	}
	// An unknown section must not be reported as a failure; it means the
	// caller does not know about it, not that the user did something wrong.
	if res := c.Check(Requirement{Kind: Config, Value: "mystery"}); !res.OK {
		t.Error("unknown section should not be reported as failing")
	}
}

// A nil presence function must not turn every config requirement into a
// failure, since describe runs before any config exists.
func TestCheckConfigWithoutPresence(t *testing.T) {
	c := New(nil)
	if res := c.Check(Requirement{Kind: Config, Value: "workspaces"}); !res.OK {
		t.Error("config checks should pass when no presence function is supplied")
	}
}

func TestExpand(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	if got := Expand("~"); got != home {
		t.Errorf("Expand(~) = %q, want %q", got, home)
	}
	if got, want := Expand("~/x"), filepath.Join(home, "x"); got != want {
		t.Errorf("Expand(~/x) = %q, want %q", got, want)
	}
	// A tilde that is not a home reference must be left alone.
	if got := Expand("/tmp/~x"); got != "/tmp/~x" {
		t.Errorf("Expand(/tmp/~x) = %q, want it unchanged", got)
	}
	if got := Expand("~user/x"); got != "~user/x" {
		t.Errorf("Expand(~user/x) = %q, want it unchanged", got)
	}
}

func TestCheckAll(t *testing.T) {
	c := New(nil)
	_, all := c.CheckAll(MustParseAll([]string{"go", "definitely-not-a-real-binary"}))
	if all {
		t.Error("CheckAll should report false when any requirement fails")
	}
	results, all := c.CheckAll(MustParseAll([]string{"go"}))
	if !all || len(results) != 1 {
		t.Errorf("CheckAll = %v with %d results, want true with 1", all, len(results))
	}
}
