package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeKey creates ~/.ssh/<name> under home, so a key check can pass.
func writeKey(t *testing.T, name string) func(home string) {
	return func(home string) {
		dir := filepath.Join(home, ".ssh")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte("key"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestMshKey runs the generated msh with ssh stubbed.
//
// A configured key is checked and passed with -i, as it always was. With no
// key from the flag, the host or the defaults, msh must hand the connection to
// ssh without -i, so ~/.ssh/config and the agent choose: that is how a
// hardware- or TPM-backed key is used, since it has no private key file.
func TestMshKey(t *testing.T) {
	keyed := loadFixture(t)
	if keyed.SSH.DefaultKey == "" {
		t.Fatal("fixture no longer sets ssh.default_key")
	}
	keyless := loadFixture(t)
	keyless.SSH.DefaultKey = ""
	stubs := []string{"ssh"}

	for _, shell := range testShells {
		t.Run(shell, func(t *testing.T) {
			script := writeScript(t, keyed, shell)

			r := runFunction(t, shell, script, stubs, writeKey(t, "deploy"), "msh", "-m", "api")
			if r.err != nil {
				t.Fatalf("msh -m api: %v\n%s", r.err, r.out)
			}
			calls := r.callsTo("ssh")
			if len(calls) != 1 || !strings.HasPrefix(calls[0], "ssh -i "+r.home+"/.ssh/deploy ") ||
				!strings.HasSuffix(calls[0], " azureuser@api.example.internal") {
				t.Errorf("msh -m api with the host's key: got %q", calls)
			}

			r = runFunction(t, shell, script, stubs, nil, "msh", "-m", "api")
			if r.err == nil || len(r.callsTo("ssh")) > 0 {
				t.Errorf("msh -m api with its key missing should fail before ssh; calls %q, output %q", r.calls, r.out)
			}

			script = writeScript(t, keyless, shell)
			r = runFunction(t, shell, script, stubs, nil, "msh", "-m", "box-a")
			if r.err != nil {
				t.Fatalf("msh -m box-a with no key configured: %v\n%s", r.err, r.out)
			}
			calls = r.callsTo("ssh")
			if len(calls) != 1 || strings.Contains(calls[0], " -i ") || !strings.HasSuffix(calls[0], " root@box-a") {
				t.Errorf("msh with no key should leave key choice to ssh: got %q", calls)
			}

			r = runFunction(t, shell, script, stubs, writeKey(t, "explicit"), "msh", "-k", "explicit", "-m", "box-a")
			calls = r.callsTo("ssh")
			if r.err != nil || len(calls) != 1 || !strings.HasPrefix(calls[0], "ssh -i "+r.home+"/.ssh/explicit ") {
				t.Errorf("msh -k should still pass the named key: got %q (%v)", calls, r.err)
			}
		})
	}
}

// TestMpushKey runs the generated mpush with scp stubbed. A host with a key
// passes it with -i; a host without one must not pass -i at all, rather than
// -i "" which ssh warns about and which hides the real key choice.
func TestMpushKey(t *testing.T) {
	cfg := loadFixture(t)
	stubs := []string{"scp"}
	upload := func(home string) {
		if err := os.WriteFile(filepath.Join(home, "build.tar"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, shell := range testShells {
		t.Run(shell, func(t *testing.T) {
			script := writeScript(t, cfg, shell)

			r := runFunction(t, shell, script, stubs, upload, "mpush", "api", "build.tar")
			calls := r.callsTo("scp")
			if r.err != nil || len(calls) != 1 || !strings.HasPrefix(calls[0], "scp -i "+r.home+"/.ssh/deploy ") {
				t.Errorf("mpush to a host with a key: got %q (%v)\n%s", calls, r.err, r.out)
			}

			r = runFunction(t, shell, script, stubs, upload, "mpush", "box-a", "build.tar")
			calls = r.callsTo("scp")
			if r.err != nil || len(calls) != 1 || strings.Contains(calls[0], "-i") ||
				!strings.Contains(calls[0], " root@box-a:") {
				t.Errorf("mpush to a host without a key should not pass -i: got %q (%v)\n%s", calls, r.err, r.out)
			}
		})
	}
}
