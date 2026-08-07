// Package credentials stores secret values in the operating system's keychain.
//
// Nothing here ever writes a secret to a file loadout controls. The config
// records only which entries exist and which one is active; the values live
// in the keychain and are read on demand.
package credentials

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// service is the namespace every entry is stored under, so loadout's secrets
// are distinguishable from everything else in the user's keychain.
const service = "loadout"

// ErrNotFound means the entry has no stored value yet.
var ErrNotFound = errors.New("no value stored for this entry")

// ErrNoBackend means this machine has no usable keychain.
var ErrNoBackend = errors.New("no keychain backend available")

// Keyring stores and retrieves secret values.
type Keyring interface {
	// Name identifies the backend, for reporting.
	Name() string
	// Get returns the stored value, or ErrNotFound.
	Get(key string) (string, error)
	// Set stores a value, replacing any existing one.
	Set(key, value string) error
	// Delete removes a value. Removing something absent is not an error.
	Delete(key string) error
	// Available reports whether this backend can be used right now.
	Available() bool
}

// Key builds the keychain key for one entry of one credential. Entries are
// namespaced by credential so npm_token/work and gh_token/work stay distinct.
func Key(credential, entry string) string {
	return credential + "/" + entry
}

// SplitKey reverses Key.
func SplitKey(key string) (credential, entry string, ok bool) {
	credential, entry, ok = strings.Cut(key, "/")
	return
}

// backends are tried in order; the first available one wins.
var backends = []func() Keyring{
	newSecurity,   // macOS
	newSecretTool, // Linux, libsecret
	newPass,       // Linux, pass
}

// Open returns the first usable keyring on this machine.
func Open() (Keyring, error) {
	var tried []string
	for _, mk := range backends {
		k := mk()
		if k.Available() {
			return k, nil
		}
		tried = append(tried, k.Name())
	}
	return nil, fmt.Errorf("%w (tried: %s)\n"+
		"install one of:\n"+
		"  macOS   already present as 'security'\n"+
		"  Linux   apt install libsecret-tools   (secret-tool)\n"+
		"  Linux   apt install pass              (pass)",
		ErrNoBackend, strings.Join(tried, ", "))
}

// Available reports which backends could be used, for doctor.
func Available() []string {
	var out []string
	for _, mk := range backends {
		k := mk()
		if k.Available() {
			out = append(out, k.Name())
		}
	}
	return out
}

// Lister is implemented by backends that can enumerate what they hold under
// loadout's namespace.
//
// Not every backend can. libsecret and pass both support a prefix search;
// the macOS security tool has no equivalent that works without prompting for
// approval on every item. Uninstall therefore treats the config as the
// authoritative list and uses this only to catch entries the config has
// forgotten about.
type Lister interface {
	// List returns the keys stored under the loadout service, in "cred/entry"
	// form.
	List() ([]string, error)
}

// ListKeys enumerates a keyring's contents when the backend supports it.
// The second return reports whether enumeration was possible at all, which
// the caller needs in order to distinguish "nothing stored" from "cannot
// tell".
func ListKeys(k Keyring) ([]string, bool) {
	l, ok := k.(Lister)
	if !ok {
		return nil, false
	}
	keys, err := l.List()
	if err != nil {
		return nil, false
	}
	sort.Strings(keys)
	return keys, true
}
