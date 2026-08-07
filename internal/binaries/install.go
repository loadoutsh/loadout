package binaries

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/loadoutsh/loadout/internal/config"
)

// maxDownloadSize caps a single download.
const maxDownloadSize = 512 << 20 // 512 MiB

// Record is what got installed, kept so a user can always see the provenance
// of anything loadout put on their PATH.
type Record struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	URL         string    `json:"url"`
	PlatformKey string    `json:"platform_key"`
	SHA256      string    `json:"sha256"`
	Verified    bool      `json:"verified"`
	InstalledAt time.Time `json:"installed_at"`
}

// State is the on-disk record of every managed binary.
type State struct {
	Binaries map[string]Record `json:"binaries"`
}

// Installer places binaries into BinDir and records them in StateDir.
type Installer struct {
	BinDir   string
	StateDir string
	Client   *http.Client

	// Now is injectable so tests get deterministic timestamps.
	Now func() time.Time
}

// NewInstaller builds an Installer with sensible defaults.
func NewInstaller(binDir, stateDir string) *Installer {
	return &Installer{
		BinDir:   binDir,
		StateDir: stateDir,
		Client: &http.Client{
			Timeout:       10 * time.Minute,
			CheckRedirect: requireHTTPSRedirect,
		},
		Now: time.Now,
	}
}

// requireHTTPSRedirect refuses a redirect that would downgrade to plain HTTP.
//
// Checking only the URL in config is not enough: release hosts commonly
// redirect to a CDN, and the content ends up executable on PATH, so every hop
// has to stay encrypted.
func requireHTTPSRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	if req.URL.Scheme != "https" {
		return fmt.Errorf("refusing redirect to %q: https required all the way", req.URL.Scheme)
	}
	return nil
}

func (i *Installer) statePath() string { return filepath.Join(i.StateDir, "binaries.json") }

// LoadState reads the install record, returning an empty state if none exists.
func (i *Installer) LoadState() (*State, error) {
	data, err := os.ReadFile(i.statePath())
	if err != nil {
		if os.IsNotExist(err) {
			return &State{Binaries: map[string]Record{}}, nil
		}
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", i.statePath(), err)
	}
	if s.Binaries == nil {
		s.Binaries = map[string]Record{}
	}
	return &s, nil
}

func (i *Installer) saveState(s *State) error {
	if err := os.MkdirAll(i.StateDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(i.statePath(), append(data, '\n'), 0o644)
}

// resolveURL works out where to fetch a binary from for this platform.
//
// A GitHub source is turned into a release download URL directly rather than
// through the API, which avoids rate limits and needing a token.
func resolveURL(b config.Binary, p Platform) (rawURL, matchedKey string, err error) {
	switch {
	case len(b.Source.URL) > 0:
		u, key, ok := p.Select(b.Source.URL)
		if !ok {
			return "", "", fmt.Errorf("%s: no url for %s (have: %s)",
				b.Name, p, strings.Join(KeysOf(b.Source.URL), ", "))
		}
		return u, key, nil

	case b.Source.GitHub != "":
		if b.Version == "" {
			return "", "", fmt.Errorf("%s: a github source needs a pinned version", b.Name)
		}
		asset, key, ok := p.Select(b.Source.Asset)
		if !ok {
			return "", "", fmt.Errorf("%s: no asset for %s (have: %s)",
				b.Name, p, strings.Join(KeysOf(b.Source.Asset), ", "))
		}
		return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s",
			b.Source.GitHub, b.Version, asset), key, nil

	default:
		return "", "", fmt.Errorf("%s: source has neither github nor url", b.Name)
	}
}

// Install fetches, verifies and installs one binary. It reports the record it
// wrote so the caller can show what happened.
func (i *Installer) Install(b config.Binary, p Platform) (Record, error) {
	rawURL, matchedKey, err := resolveURL(b, p)
	if err != nil {
		return Record{}, err
	}

	// Anything installed here lands on PATH and will be executed, so refuse
	// to fetch it over a channel that can be tampered with in transit.
	u, err := url.Parse(rawURL)
	if err != nil {
		return Record{}, fmt.Errorf("%s: bad url %q: %w", b.Name, rawURL, err)
	}
	if u.Scheme != "https" {
		return Record{}, fmt.Errorf("%s: refusing to download over %q, https required", b.Name, u.Scheme)
	}

	tmpDir, err := os.MkdirTemp("", "loadout-dl-")
	if err != nil {
		return Record{}, err
	}
	defer os.RemoveAll(tmpDir)

	filename := filepath.Base(u.Path)
	download := filepath.Join(tmpDir, filename)

	sum, err := i.fetch(rawURL, download)
	if err != nil {
		return Record{}, fmt.Errorf("%s: %w", b.Name, err)
	}

	verified := false
	// A hash describes one platform's artefact, so the one for the platform
	// actually being installed is what must match.
	if pinned, ok := b.Checksum.For(matchedKey); ok {
		want := strings.TrimPrefix(pinned, "sha256:")
		if want != "" && want != "..." {
			if !strings.EqualFold(want, sum) {
				return Record{}, fmt.Errorf("%s: checksum mismatch for %s\n  want sha256:%s\n  got  sha256:%s\n"+
					"Either the download was tampered with, or upstream published a new build at the same URL.",
					b.Name, matchedKey, want, sum)
			}
			verified = true
		}
	}

	dst := filepath.Join(i.BinDir, b.Name)
	if err := extract(download, filename, b.Name, dst); err != nil {
		return Record{}, fmt.Errorf("%s: %w", b.Name, err)
	}

	rec := Record{
		Name:        b.Name,
		Version:     b.Version,
		URL:         rawURL,
		PlatformKey: matchedKey,
		SHA256:      sum,
		Verified:    verified,
		InstalledAt: i.Now(),
	}

	state, err := i.LoadState()
	if err != nil {
		return rec, err
	}
	state.Binaries[b.Name] = rec
	return rec, i.saveState(state)
}

// fetch downloads to dst and returns the content's sha256.
func (i *Installer) fetch(rawURL, dst string) (string, error) {
	resp, err := i.Client.Get(rawURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", rawURL, resp.Status)
	}

	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, maxDownloadSize+1))
	if err != nil {
		return "", err
	}
	if n > maxDownloadSize {
		return "", fmt.Errorf("download exceeds the %d byte limit", maxDownloadSize)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Remove deletes a managed binary and forgets it.
func (i *Installer) Remove(name string) error {
	state, err := i.LoadState()
	if err != nil {
		return err
	}
	if _, ok := state.Binaries[name]; !ok {
		return fmt.Errorf("%q is not managed by loadout", name)
	}
	path := filepath.Join(i.BinDir, name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	delete(state.Binaries, name)
	return i.saveState(state)
}

// Status pairs a declared binary with what is actually installed.
type Status struct {
	Declared  config.Binary
	Record    Record
	Installed bool
	// Stale means the installed version differs from the declared one.
	Stale bool
	// Orphaned means it is installed but no longer declared in config.
	Orphaned bool
}

// Status reports the state of every declared and installed binary.
func (i *Installer) Status(declared []config.Binary) ([]Status, error) {
	state, err := i.LoadState()
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var out []Status
	for _, b := range declared {
		seen[b.Name] = true
		rec, ok := state.Binaries[b.Name]
		// A record with no file behind it is not installed. This catches a
		// binary deleted by hand outside loadout.
		if ok {
			if _, err := os.Stat(filepath.Join(i.BinDir, b.Name)); err != nil {
				ok = false
			}
		}
		out = append(out, Status{
			Declared:  b,
			Record:    rec,
			Installed: ok,
			Stale:     ok && rec.Version != b.Version,
		})
	}

	for name, rec := range state.Binaries {
		if seen[name] {
			continue
		}
		out = append(out, Status{
			Record:    rec,
			Installed: true,
			Orphaned:  true,
			Declared:  config.Binary{Name: name, Version: rec.Version},
		})
	}

	sort.Slice(out, func(a, b int) bool { return out[a].Declared.Name < out[b].Declared.Name })
	return out, nil
}
