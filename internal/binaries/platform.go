// Package binaries installs third-party executables into a managed directory
// on PATH.
//
// This is the only part of loadout that downloads and then executes code, so
// it is deliberately strict: HTTPS only, checksums verified when declared,
// nothing installed outside the managed directory, and provenance recorded so
// a user can always see where a binary on their PATH came from.
package binaries

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
)

// Platform identifies a build target as "<os>-<arch>", matching the keys used
// in the asset and url maps in config.
type Platform struct {
	OS   string
	Arch string
}

// Current is the platform loadout is running on.
func Current() Platform {
	return Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
}

// Key is the map key form, e.g. "linux-amd64".
func (p Platform) Key() string { return p.OS + "-" + p.Arch }

func (p Platform) String() string { return p.Key() }

// Parse reads a platform key.
func Parse(key string) (Platform, error) {
	os, arch, ok := strings.Cut(key, "-")
	if !ok || os == "" || arch == "" {
		return Platform{}, fmt.Errorf("bad platform key %q, want <os>-<arch>", key)
	}
	return Platform{OS: os, Arch: arch}, nil
}

// fallbacks lists platform keys to try, most specific first.
//
// Rosetta and universal builds mean an arm64 Mac can run an amd64 binary, and
// projects often publish a single universal darwin asset. Trying those in
// order beats failing when a usable asset is right there.
func (p Platform) fallbacks() []string {
	keys := []string{p.Key()}
	if p.OS == "darwin" {
		keys = append(keys, "darwin-universal")
		if p.Arch == "arm64" {
			keys = append(keys, "darwin-amd64")
		}
	}
	// A bare OS key means "one asset serves every arch on this OS".
	keys = append(keys, p.OS)
	return keys
}

// Select picks the value for this platform from a keyed map, returning the key
// that matched so the caller can report a fallback rather than hide it.
func (p Platform) Select(m map[string]string) (value, matchedKey string, ok bool) {
	for _, k := range p.fallbacks() {
		if v, found := m[k]; found {
			return v, k, true
		}
	}
	return "", "", false
}

// KeysOf lists a map's platform keys, sorted, for error messages.
func KeysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
