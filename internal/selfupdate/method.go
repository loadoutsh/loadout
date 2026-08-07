// Package selfupdate replaces the running binary with a newer release.
//
// It only does that when loadout owns the file. When a package manager
// installed it, that package manager owns it, and replacing the file behind
// its back leaves it reporting a version it did not install and unable to
// upgrade cleanly. In those cases this reports the right command instead.
package selfupdate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Method is how loadout got onto this machine.
type Method string

const (
	// MethodHomebrew means brew owns the file.
	MethodHomebrew Method = "homebrew"
	// MethodGoInstall means the Go toolchain placed it in GOPATH/bin.
	MethodGoInstall Method = "go install"
	// MethodManual is install.sh or a direct download: loadout owns the file
	// and is the only method that can update itself.
	MethodManual Method = "manual"
	// MethodSource is a build from a checkout, updated with git and make.
	MethodSource Method = "source"
)

// Detect works out how the running binary was installed, and where it lives.
func Detect() (Method, string) {
	exe, err := os.Executable()
	if err != nil {
		return MethodManual, ""
	}
	// A brew install is a symlink into the Cellar, so resolve before matching.
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)

	switch {
	case strings.Contains(exe, "/Cellar/"), strings.Contains(exe, "/linuxbrew/"):
		return MethodHomebrew, exe
	case filepath.Base(dir) == "bin" && filepath.Base(filepath.Dir(dir)) == "go":
		return MethodGoInstall, exe
	case strings.Contains(dir, string(filepath.Separator)+"go"+string(filepath.Separator)+"bin"):
		return MethodGoInstall, exe
	// A binary sitting next to a go.mod is a build from a checkout, not an
	// install; updating it means pulling and rebuilding.
	case fileExists(filepath.Join(dir, "go.mod")):
		return MethodSource, exe
	default:
		return MethodManual, exe
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// SelfUpdatable reports whether loadout may replace its own binary.
func (m Method) SelfUpdatable() bool { return m == MethodManual }

// UpdateCommand is what the user should run instead, when loadout cannot do
// it itself.
func (m Method) UpdateCommand() string {
	switch m {
	case MethodHomebrew:
		return "brew upgrade loadout"
	case MethodGoInstall:
		return "go install github.com/loadoutsh/loadout/cmd/loadout@latest"
	case MethodSource:
		return "git pull && make install"
	default:
		return "loadout update"
	}
}

// AssetName is the release artefact for this platform.
func AssetName(version string) string {
	return "loadout_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
}
