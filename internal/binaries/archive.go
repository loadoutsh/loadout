package binaries

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxExtractSize caps how much a single archive member may expand to. Without
// it, a malicious or corrupt archive could fill the disk.
const maxExtractSize = 512 << 20 // 512 MiB

// extract pulls the executable named want out of src and writes it to dst.
//
// The archive format is inferred from the source filename. A file that is not
// an archive is treated as the executable itself, which is how several
// projects publish their releases.
func extract(src, filename, want, dst string) error {
	switch {
	case strings.HasSuffix(filename, ".zip"):
		return extractZip(src, want, dst)
	case strings.HasSuffix(filename, ".tar.gz"), strings.HasSuffix(filename, ".tgz"):
		return extractTarGz(src, want, dst)
	case strings.HasSuffix(filename, ".gz"):
		return extractGz(src, dst)
	default:
		// Not an archive: the download is the binary.
		return copyFile(src, dst)
	}
}

// isWanted matches an archive member against the executable being sought.
// Releases commonly nest the binary under a directory, and Windows-style
// names are ignored since loadout targets Linux and macOS.
func isWanted(memberPath, want string) bool {
	base := filepath.Base(memberPath)
	return base == want || base == want+".exe"
}

// safeMemberPath rejects archive members whose path would escape the target
// directory. Nothing here writes members by their own path, but validating
// keeps a future change from reintroducing a zip-slip.
func safeMemberPath(name string) error {
	if filepath.IsAbs(name) || strings.Contains(name, "..") {
		return fmt.Errorf("refusing archive member with unsafe path %q", name)
	}
	return nil
}

func extractZip(src, want, dst string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}
	defer zr.Close()

	var candidates []string
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if err := safeMemberPath(f.Name); err != nil {
			return err
		}
		candidates = append(candidates, f.Name)
		if !isWanted(f.Name, want) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		return writeExecutable(rc, dst)
	}
	return notFound(want, candidates)
}

func extractTarGz(src, want, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("opening gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var candidates []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if err := safeMemberPath(hdr.Name); err != nil {
			return err
		}
		candidates = append(candidates, hdr.Name)
		if !isWanted(hdr.Name, want) {
			continue
		}
		return writeExecutable(tr, dst)
	}
	return notFound(want, candidates)
}

// extractGz handles a bare gzipped binary, which has no member names at all.
func extractGz(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("opening gzip: %w", err)
	}
	defer gz.Close()

	return writeExecutable(gz, dst)
}

func notFound(want string, candidates []string) error {
	if len(candidates) > 12 {
		candidates = append(candidates[:12], "...")
	}
	return fmt.Errorf("no executable named %q in archive (contains: %s)",
		want, strings.Join(candidates, ", "))
}

// writeExecutable writes to a temporary file first and renames into place, so
// an interrupted install cannot leave a half-written binary on PATH.
func writeExecutable(r io.Reader, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".loadout-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	n, err := io.Copy(tmp, io.LimitReader(r, maxExtractSize+1))
	if err != nil {
		tmp.Close()
		return err
	}
	if n > maxExtractSize {
		tmp.Close()
		return fmt.Errorf("archive member exceeds the %d byte limit", maxExtractSize)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	return os.Rename(tmpName, dst)
}

func copyFile(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	return writeExecutable(f, dst)
}
