package selfupdate

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Repo is where releases come from. BaseURL overrides it for mirrors and for
// testing, mirroring the LOADOUT_BASE_URL escape hatch in install.sh.
const Repo = "loadoutsh/loadout"

// maxDownload caps the release archive.
const maxDownload = 128 << 20

// Release is a published version.
type Release struct {
	Version  string
	AssetURL string
	// Checksum is the expected sha256, or empty when the release published
	// none.
	Checksum string
}

// Client fetches releases.
type Client struct {
	HTTP *http.Client
	// BaseURL, when set, replaces the GitHub release URL entirely. Everything
	// is fetched from directly beneath it.
	BaseURL string
	// Version names the release to fetch when BaseURL is set. A mirror serves
	// the same asset filenames as a real release, and those carry the version,
	// so it cannot be inferred from the URL.
	Version string
}

// NewClient builds a Client that refuses any plaintext hop, because what it
// downloads replaces the running executable.
func NewClient() *Client {
	return &Client{
		HTTP: &http.Client{
			Timeout: 5 * time.Minute,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return errors.New("stopped after 10 redirects")
				}
				if req.URL.Scheme != "https" {
					return fmt.Errorf("refusing redirect to %q: https required all the way", req.URL.Scheme)
				}
				return nil
			},
		},
	}
}

// Latest resolves the newest published version.
//
// It follows the /releases/latest redirect and reads the tag out of the final
// URL, rather than calling the API, which is rate limited for unauthenticated
// callers and would fail on a shared network.
func (c *Client) Latest() (Release, error) {
	if c.BaseURL != "" {
		if c.Version == "" {
			return Release{}, errors.New("a base url needs a version too: pass --release")
		}
		return c.releaseAt(c.BaseURL, c.Version)
	}

	resp, err := c.HTTP.Get("https://github.com/" + Repo + "/releases/latest")
	if err != nil {
		return Release{}, fmt.Errorf("checking for a newer version: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	final := resp.Request.URL.String()
	idx := strings.LastIndex(final, "/tag/")
	if idx < 0 {
		return Release{}, fmt.Errorf("no published release found for %s", Repo)
	}
	version := final[idx+len("/tag/"):]
	if version == "" {
		return Release{}, fmt.Errorf("could not read a version from %s", final)
	}
	return c.releaseAt("https://github.com/"+Repo+"/releases/download/"+version, version)
}

// releaseAt assembles a Release from a download base, fetching the checksum
// list when the release publishes one.
func (c *Client) releaseAt(base, version string) (Release, error) {
	asset := AssetName(version)
	rel := Release{Version: version, AssetURL: strings.TrimSuffix(base, "/") + "/" + asset}

	sums, err := c.fetch(strings.TrimSuffix(base, "/") + "/checksums.txt")
	if err == nil {
		for _, line := range strings.Split(string(sums), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && fields[1] == asset {
				rel.Checksum = fields[0]
				break
			}
		}
	}
	return rel, nil
}

func (c *Client) fetch(rawURL string) ([]byte, error) {
	resp, err := c.HTTP.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", rawURL, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// Apply downloads the release and replaces the binary at exePath.
//
// A rename over a running executable is safe on Linux and macOS: the running
// process keeps its open inode, and the next invocation gets the new file.
func (c *Client) Apply(rel Release, exePath string) error {
	u, err := url.Parse(rel.AssetURL)
	if err != nil {
		return fmt.Errorf("bad asset url %q: %w", rel.AssetURL, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("refusing to download over %q, https required", u.Scheme)
	}

	// Write next to the target rather than in /tmp, so the final step is a
	// rename within one filesystem and cannot half-succeed.
	dir := filepath.Dir(exePath)
	if err := writableDir(dir); err != nil {
		return err
	}

	resp, err := c.HTTP.Get(rel.AssetURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", rel.AssetURL, resp.Status)
	}

	archive, err := os.CreateTemp(dir, ".loadout-dl-*")
	if err != nil {
		return err
	}
	archivePath := archive.Name()
	defer os.Remove(archivePath)

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(archive, h), io.LimitReader(resp.Body, maxDownload+1))
	archive.Close()
	if err != nil {
		return err
	}
	if n > maxDownload {
		return fmt.Errorf("download exceeds the %d byte limit", maxDownload)
	}

	got := hex.EncodeToString(h.Sum(nil))
	if rel.Checksum == "" {
		return errors.New("this release publishes no checksum; refusing to replace the binary unverified")
	}
	if !strings.EqualFold(rel.Checksum, got) {
		return fmt.Errorf("checksum mismatch\n  want sha256:%s\n  got  sha256:%s\nRefusing to update.",
			rel.Checksum, got)
	}

	newPath := exePath + ".new"
	if err := extractBinary(archivePath, newPath); err != nil {
		return err
	}
	if err := os.Chmod(newPath, 0o755); err != nil {
		os.Remove(newPath)
		return err
	}
	if err := os.Rename(newPath, exePath); err != nil {
		os.Remove(newPath)
		return fmt.Errorf("replacing %s: %w", exePath, err)
	}
	return nil
}

// writableDir checks up front, so a permission problem is reported before
// anything is downloaded rather than after.
func writableDir(dir string) error {
	probe, err := os.CreateTemp(dir, ".loadout-probe-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s: %w\n"+
			"Either re-run with the necessary permissions, or reinstall with install.sh.", dir, err)
	}
	name := probe.Name()
	probe.Close()
	return os.Remove(name)
}

// extractBinary pulls the loadout executable out of the release tarball.
func extractBinary(archivePath, dst string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("opening the release archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading the release archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != "loadout" {
			continue
		}
		out, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, io.LimitReader(tr, maxDownload)); err != nil {
			return err
		}
		return nil
	}
	return errors.New("the release archive contains no loadout binary")
}

// Comparison reports how a running version relates to a published one.
type Comparison int

const (
	// Older means an update is available.
	Older Comparison = iota
	// Same means the running version matches.
	Same
	// Newer means the running build is ahead of the last release.
	Newer
	// Unknown means the running version is not a release tag, so no
	// comparison is meaningful. A development build hits this.
	Unknown
)

// Comparable reports whether each side is a plain release tag. Knowing which
// side is not tells the user something useful; "not comparable" on its own
// blames whichever one they happen to look at first.
func Comparable(current, latest string) (currentOK, latestOK bool) {
	_, currentOK = parseVersion(current)
	_, latestOK = parseVersion(latest)
	return
}

// Compare relates the running version to a published one.
func Compare(current, latest string) Comparison {
	c, okC := parseVersion(current)
	l, okL := parseVersion(latest)
	if !okC || !okL {
		return Unknown
	}
	for i := 0; i < 3; i++ {
		switch {
		case c[i] < l[i]:
			return Older
		case c[i] > l[i]:
			return Newer
		}
	}
	return Same
}

// parseVersion reads a plain vMAJOR.MINOR.PATCH tag. Anything carrying a
// pre-release or build suffix, a commit sha, or the word dev is not
// comparable, and saying so is better than guessing.
func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return out, false
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
