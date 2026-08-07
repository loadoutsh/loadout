package credentials

import (
	"bytes"
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

// --- macOS Keychain -------------------------------------------------------

type securityBackend struct{}

func newSecurity() Keyring { return securityBackend{} }

func (securityBackend) Name() string { return "macOS Keychain (security)" }

func (securityBackend) Available() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	_, err := exec.LookPath("security")
	return err == nil
}

func (securityBackend) Get(key string) (string, error) {
	out, err := exec.Command("security", "find-generic-password",
		"-s", service, "-a", key, "-w").Output()
	if err != nil {
		// security exits non-zero when the item is absent; there is no
		// distinct code for it, so absence is the assumed cause.
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", ErrNotFound
		}
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func (s securityBackend) Set(key, value string) error {
	// -U updates in place when the item already exists.
	cmd := exec.Command("security", "add-generic-password",
		"-s", service, "-a", key, "-w", value, "-U")
	return run(cmd)
}

func (securityBackend) Delete(key string) error {
	cmd := exec.Command("security", "delete-generic-password",
		"-s", service, "-a", key)
	if err := run(cmd); err != nil {
		// Deleting something absent is not a failure.
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil
		}
		return err
	}
	return nil
}

// --- Linux, libsecret -----------------------------------------------------

type secretToolBackend struct{}

func newSecretTool() Keyring { return secretToolBackend{} }

func (secretToolBackend) Name() string { return "libsecret (secret-tool)" }

func (secretToolBackend) Available() bool {
	if runtime.GOOS == "darwin" {
		return false
	}
	_, err := exec.LookPath("secret-tool")
	return err == nil
}

func (secretToolBackend) Get(key string) (string, error) {
	out, err := exec.Command("secret-tool", "lookup", "service", service, "key", key).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", ErrNotFound
		}
		return "", err
	}
	// secret-tool returns the value with no trailing newline, but a value
	// that legitimately ends in one would be indistinguishable, so only the
	// single newline the tool itself may add is trimmed.
	return strings.TrimSuffix(string(out), "\n"), nil
}

func (secretToolBackend) Set(key, value string) error {
	cmd := exec.Command("secret-tool", "store", "--label", service+": "+key,
		"service", service, "key", key)
	cmd.Stdin = strings.NewReader(value)
	return run(cmd)
}

func (secretToolBackend) Delete(key string) error {
	cmd := exec.Command("secret-tool", "clear", "service", service, "key", key)
	return run(cmd)
}

// --- Linux, pass ----------------------------------------------------------

type passBackend struct{}

func newPass() Keyring { return passBackend{} }

func (passBackend) Name() string { return "pass" }

func (passBackend) Available() bool {
	if runtime.GOOS == "darwin" {
		return false
	}
	if _, err := exec.LookPath("pass"); err != nil {
		return false
	}
	// pass is useless without an initialised store, and reporting it as
	// available would produce a confusing failure on first use.
	return run(exec.Command("pass", "ls")) == nil
}

func (passBackend) path(key string) string { return service + "/" + key }

func (p passBackend) Get(key string) (string, error) {
	out, err := exec.Command("pass", "show", p.path(key)).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", ErrNotFound
		}
		return "", err
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

func (p passBackend) Set(key, value string) error {
	cmd := exec.Command("pass", "insert", "--multiline", "--force", p.path(key))
	cmd.Stdin = strings.NewReader(value + "\n")
	return run(cmd)
}

func (p passBackend) Delete(key string) error {
	cmd := exec.Command("pass", "rm", "--force", p.path(key))
	if err := run(cmd); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil
		}
		return err
	}
	return nil
}

// run executes a command, folding stderr into the error so a backend failure
// says what actually went wrong instead of just reporting an exit code.
func run(cmd *exec.Cmd) error {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return err
		}
		return errors.New(msg)
	}
	return nil
}

// List enumerates loadout's entries in the libsecret collection.
func (secretToolBackend) List() ([]string, error) {
	out, err := exec.Command("secret-tool", "search", "--all", "service", service).CombinedOutput()
	if err != nil {
		// A search matching nothing exits non-zero, which is not a failure.
		if len(bytes.TrimSpace(out)) == 0 {
			return nil, nil
		}
		return nil, err
	}
	var keys []string
	for _, line := range strings.Split(string(out), "\n") {
		// Lines look like: attribute.key = npm_token/work
		if rest, found := strings.CutPrefix(strings.TrimSpace(line), "attribute.key = "); found {
			keys = append(keys, rest)
		}
	}
	return keys, nil
}

// List enumerates loadout's entries in the password store.
func (p passBackend) List() ([]string, error) {
	out, err := exec.Command("pass", "ls", service).Output()
	if err != nil {
		return nil, nil // no such subtree is not a failure
	}
	var keys []string
	var group string
	for _, line := range strings.Split(string(out), "\n") {
		t := strings.TrimLeft(line, "|`-└├─ ")
		t = strings.TrimSpace(t)
		if t == "" || t == service {
			continue
		}
		// pass ls prints a tree: credentials are groups, entries are leaves.
		if strings.HasSuffix(t, "/") {
			group = strings.TrimSuffix(t, "/")
			continue
		}
		if group != "" {
			keys = append(keys, group+"/"+t)
		}
	}
	return keys, nil
}
