// Package uninstall works out everything loadout has put on a machine, and
// removes it.
//
// A tool that stores secrets and writes to PATH has to be removable by itself.
// Deleting the directories by hand orphans every keychain entry, and does it
// irreversibly in a quiet way: the config is the only record of what those
// entries were called, so once it is gone the secrets are unfindable without
// knowing the backend's own search syntax.
package uninstall

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/loadoutsh/loadout/internal/config"
	"github.com/loadoutsh/loadout/internal/credentials"
	"github.com/loadoutsh/loadout/internal/selfupdate"
)

// Kind distinguishes the sorts of thing that can be removed, because they
// carry very different risk.
type Kind string

const (
	// KindPath is a file or directory loadout created.
	KindPath Kind = "path"
	// KindBinary is a managed executable on PATH.
	KindBinary Kind = "binary"
	// KindCredential is a secret in the OS keychain. Unrecoverable if the
	// value is not stored elsewhere, which is why it is confirmed separately.
	KindCredential Kind = "credential"
)

// Item is one thing that would be removed.
type Item struct {
	Kind Kind
	// Name is the path, or the keychain key in "credential/entry" form.
	Name string
	Size int64
	Note string
}

// Plan is the full inventory. Nothing is removed until Execute runs.
type Plan struct {
	ConfigDir string
	DataDir   string

	Paths       []Item
	Binaries    []Item
	Credentials []Item

	// CredentialsComplete reports whether the keychain could be enumerated.
	// When false the list came from the config alone and may be short, which
	// the user needs telling.
	CredentialsComplete bool
	Keyring             string

	// ShellRC and ShellLine describe the integration line the user added.
	// loadout never wrote it and does not remove it.
	ShellRC   string
	ShellLine string

	// SelfPath is loadout's own binary, and SelfHint how to remove it. A
	// process cannot reliably delete the executable it is running from, and a
	// package manager owns its own files.
	SelfPath string
	SelfHint string
}

// Options controls what a plan covers.
type Options struct {
	KeepConfig      bool
	KeepCredentials bool
}

// Build inventories everything without touching anything.
func Build(opt Options) (*Plan, error) {
	cfgDir, err := config.Dir()
	if err != nil {
		return nil, err
	}
	dataDir, err := dataDir()
	if err != nil {
		return nil, err
	}

	p := &Plan{ConfigDir: cfgDir, DataDir: dataDir}

	if !opt.KeepConfig {
		p.Paths = append(p.Paths, dirItem(cfgDir, "configuration, generated shell file, and sync repository if present")...)
	} else {
		// The generated file is loadout's output, not the user's data, so it
		// goes even when the configuration is kept.
		for _, shell := range []string{"zsh", "bash"} {
			f := filepath.Join(cfgDir, "init."+shell)
			if st, err := os.Stat(f); err == nil {
				p.Paths = append(p.Paths, Item{Kind: KindPath, Name: f, Size: st.Size(),
					Note: "generated output"})
			}
		}
	}

	p.Paths = append(p.Paths, dirItem(dataDir, "managed binaries and install state")...)
	p.Binaries = managedBinaries(dataDir)

	if !opt.KeepCredentials {
		p.Credentials, p.CredentialsComplete, p.Keyring = credentialItems()
	}

	p.ShellRC, p.ShellLine = shellIntegration(cfgDir)
	p.SelfPath, p.SelfHint = self()

	return p, nil
}

func dataDir() (string, error) {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "loadout"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "loadout"), nil
}

// dirItem describes a directory, or nothing when it does not exist.
func dirItem(path, note string) []Item {
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	return []Item{{Kind: KindPath, Name: path, Size: dirSize(path), Note: note}}
}

func dirSize(path string) int64 {
	var total int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// managedBinaries lists what is on PATH, so the summary can say what the user
// is about to lose rather than just naming a directory.
func managedBinaries(dataDir string) []Item {
	binDir := filepath.Join(dataDir, "bin")
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return nil
	}
	var out []Item
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Item{Kind: KindBinary, Name: e.Name(), Size: info.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// credentialItems lists the keychain entries to remove.
//
// The config is authoritative because every backend can be read from it. When
// the backend can also enumerate, anything it finds that the config does not
// mention is included too: those are exactly the entries a manual uninstall
// would strand.
func credentialItems() (items []Item, complete bool, keyring string) {
	ring, err := credentials.Open()
	if err != nil {
		return nil, false, ""
	}
	keyring = ring.Name()

	declared := map[string]bool{}
	if cfg, err := config.Load(); err == nil {
		for name, c := range cfg.Credentials {
			for _, entry := range c.Entries {
				declared[credentials.Key(name, entry)] = true
			}
		}
	}

	seen := map[string]bool{}
	add := func(key, note string) {
		if seen[key] {
			return
		}
		seen[key] = true
		items = append(items, Item{Kind: KindCredential, Name: key, Note: note})
	}

	stored, enumerated := credentials.ListKeys(ring)
	complete = enumerated

	for _, key := range stored {
		note := ""
		if !declared[key] {
			note = "in the keychain but not in your config"
		}
		add(key, note)
	}
	// Declared entries with no stored value still get listed, so the report
	// matches what the config says exists.
	for key := range declared {
		if !enumerated {
			add(key, "")
			continue
		}
		add(key, "declared, no value stored")
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, complete, keyring
}

// shellIntegration finds the line the user added to their shell config.
func shellIntegration(cfgDir string) (rc, line string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", ""
	}
	for _, name := range []string{".zshrc", ".bashrc", ".bash_profile", ".profile"} {
		path := filepath.Join(home, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, l := range strings.Split(string(data), "\n") {
			if strings.Contains(l, cfgDir) || strings.Contains(l, "loadout/init.") {
				if strings.HasPrefix(strings.TrimSpace(l), "#") {
					continue
				}
				return path, strings.TrimSpace(l)
			}
		}
	}
	return "", ""
}

// self locates loadout's own binary and says how to remove it.
//
// Install-method detection lives in selfupdate, so update and uninstall
// cannot disagree about how loadout got here.
func self() (path, hint string) {
	method, exe := selfupdate.Detect()
	if exe == "" {
		return "", ""
	}
	switch method {
	case selfupdate.MethodHomebrew:
		return exe, "brew uninstall loadout"
	case selfupdate.MethodGoInstall:
		return exe, "rm " + exe + "   (installed with go install)"
	case selfupdate.MethodSource:
		return exe, "rm " + exe + "   (built from a checkout)"
	default:
		return exe, "rm " + exe
	}
}

// Empty reports whether there is anything to do.
func (p *Plan) Empty() bool {
	return len(p.Paths) == 0 && len(p.Credentials) == 0
}

// TotalSize is the disk space the paths occupy.
func (p *Plan) TotalSize() int64 {
	var n int64
	for _, i := range p.Paths {
		n += i.Size
	}
	return n
}

// Result records what Execute did.
type Result struct {
	Removed []string
	Failed  []error
}

// Execute performs the removals.
//
// Credentials are only touched when deleteCredentials is true, which the
// caller sets from an explicit confirmation. A secret deleted here is gone.
func (p *Plan) Execute(deleteCredentials bool) *Result {
	r := &Result{}

	if deleteCredentials && len(p.Credentials) > 0 {
		ring, err := credentials.Open()
		if err != nil {
			r.Failed = append(r.Failed, fmt.Errorf("opening keychain: %w", err))
		} else {
			for _, item := range p.Credentials {
				if err := ring.Delete(item.Name); err != nil {
					r.Failed = append(r.Failed, fmt.Errorf("%s: %w", item.Name, err))
					continue
				}
				r.Removed = append(r.Removed, "keychain: "+item.Name)
			}
		}
	}

	for _, item := range p.Paths {
		if err := safeRemove(item.Name); err != nil {
			r.Failed = append(r.Failed, err)
			continue
		}
		r.Removed = append(r.Removed, item.Name)
	}

	return r
}

// safeRemove refuses to delete anything outside the directories loadout owns.
//
// This guards against a bad config or a surprising environment turning an
// uninstall into an rm -rf of somewhere important.
func safeRemove(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if abs == home || abs == "/" || abs == filepath.Dir(home) {
		return fmt.Errorf("refusing to remove %s", abs)
	}
	if !strings.Contains(abs, "loadout") {
		return fmt.Errorf("refusing to remove %s: not a loadout path", abs)
	}
	if err := os.RemoveAll(abs); err != nil {
		return fmt.Errorf("removing %s: %w", abs, err)
	}
	return nil
}

// ErrNothingToDo is returned when loadout is not installed.
var ErrNothingToDo = errors.New("loadout does not appear to be installed here")

// HostOS is the platform, for the uninstall hint.
func HostOS() string { return runtime.GOOS }
