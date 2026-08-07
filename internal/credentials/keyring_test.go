package credentials

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestKeyRoundTrip(t *testing.T) {
	key := Key("npm_token", "work")
	if key != "npm_token/work" {
		t.Errorf("Key = %q, want npm_token/work", key)
	}
	cred, entry, ok := SplitKey(key)
	if !ok || cred != "npm_token" || entry != "work" {
		t.Errorf("SplitKey = %q/%q ok=%v", cred, entry, ok)
	}
}

// An entry name containing a slash must not corrupt the credential it belongs
// to when the key is split back apart.
func TestSplitKeyTakesTheFirstSeparator(t *testing.T) {
	cred, entry, ok := SplitKey("gh_token/work/extra")
	if !ok || cred != "gh_token" || entry != "work/extra" {
		t.Errorf("SplitKey = %q/%q ok=%v, want gh_token/work/extra", cred, entry, ok)
	}
}

func TestFakeBehavesLikeAKeyring(t *testing.T) {
	f := NewFake()

	if _, err := f.Get("absent"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get on an absent key = %v, want ErrNotFound", err)
	}
	if err := f.Set("a/b", "secret"); err != nil {
		t.Fatal(err)
	}
	got, err := f.Get("a/b")
	if err != nil || got != "secret" {
		t.Errorf("Get = %q/%v, want secret", got, err)
	}
	// Set must replace rather than append.
	if err := f.Set("a/b", "replaced"); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.Get("a/b"); got != "replaced" {
		t.Errorf("Get after replace = %q, want replaced", got)
	}
	// Deleting something absent must not error.
	if err := f.Delete("never-set"); err != nil {
		t.Errorf("Delete on an absent key = %v, want nil", err)
	}
	if err := f.Delete("a/b"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Get("a/b"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete = %v, want ErrNotFound", err)
	}
}

// When nothing is available, the error must tell the user what to install
// rather than just failing.
func TestOpenErrorIsActionable(t *testing.T) {
	saved := backends
	backends = nil
	defer func() { backends = saved }()

	_, err := Open()
	if !errors.Is(err, ErrNoBackend) {
		t.Fatalf("error = %v, want ErrNoBackend", err)
	}
	for _, want := range []string{"libsecret-tools", "pass", "security"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}

func TestOpenPicksTheFirstAvailable(t *testing.T) {
	saved := backends
	defer func() { backends = saved }()

	first, second := NewFake(), NewFake()
	backends = []func() Keyring{
		func() Keyring { return unavailable{} },
		func() Keyring { return first },
		func() Keyring { return second },
	}
	got, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if got != Keyring(first) {
		t.Error("Open should return the first available backend")
	}
}

// unavailable stands in for a backend whose tool is not installed.
type unavailable struct{}

func (unavailable) Name() string               { return "unavailable" }
func (unavailable) Available() bool            { return false }
func (unavailable) Get(string) (string, error) { return "", ErrNotFound }
func (unavailable) Set(_, _ string) error      { return ErrNoBackend }
func (unavailable) Delete(string) error        { return ErrNoBackend }

// Exercises the real keychain on this machine. Off by default because it
// writes to the developer's actual keyring; run with LOADOUT_KEYRING_TEST=1.
func TestRealKeyringRoundTrip(t *testing.T) {
	if os.Getenv("LOADOUT_KEYRING_TEST") == "" {
		t.Skip("set LOADOUT_KEYRING_TEST=1 to exercise the real keychain")
	}
	k, err := Open()
	if err != nil {
		t.Skipf("no keyring available: %v", err)
	}
	t.Logf("backend: %s", k.Name())

	key := Key("loadout_selftest", "scratch")
	t.Cleanup(func() { _ = k.Delete(key) })

	const want = "value with spaces and $pecial ch@rs"
	if err := k.Set(key, want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := k.Get(key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Errorf("Get = %q, want %q", got, want)
	}
	if err := k.Delete(key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := k.Get(key); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete = %v, want ErrNotFound", err)
	}
}
