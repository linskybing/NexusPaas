package imageregistry

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

// Build-context archive safety limits. These bound the untrusted upload before it
// is ever unpacked so a malicious archive cannot exhaust disk/inodes (zip bomb) or
// escape the context directory (path traversal / links). Vars (not consts) so the
// safety tests can lower them cheaply instead of streaming hundreds of MB. (P0-1)
var (
	maxBuildContextArchiveBytes      = 100 << 20        // compressed upload cap
	maxBuildContextUncompressedBytes = int64(512 << 20) // zip-bomb guard
	maxBuildContextFileCount         = 20000
	maxBuildContextPathDepth         = 64
	maxBuildContextPathLength        = 4096
)

// errBuildContextArchive wraps every rejection so callers can map it to a 400.
var errBuildContextArchive = errors.New("invalid build context archive")

type buildContextInfo struct {
	Digest     string
	FileCount  int
	TotalBytes int64
}

type buildContextEntry struct {
	name   string
	sha256 string
}

func buildContextError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errBuildContextArchive, fmt.Sprintf(format, args...))
}

// validateBuildContextArchive enforces the archive-safety rules for an uploaded
// build context (tar.gz or zip) and returns a deterministic, order-independent
// content digest. Format is auto-detected from the magic bytes. (P0-1)
func validateBuildContextArchive(data []byte) (buildContextInfo, error) {
	if len(data) == 0 {
		return buildContextInfo{}, buildContextError("archive is empty")
	}
	if len(data) > maxBuildContextArchiveBytes {
		return buildContextInfo{}, buildContextError("archive exceeds %d bytes", maxBuildContextArchiveBytes)
	}
	switch {
	case bytes.HasPrefix(data, []byte{0x1f, 0x8b}):
		return validateTarGzBuildContext(data)
	case bytes.HasPrefix(data, []byte{'P', 'K', 0x03, 0x04}), bytes.HasPrefix(data, []byte{'P', 'K', 0x05, 0x06}):
		return validateZipBuildContext(data)
	default:
		return buildContextInfo{}, buildContextError("unsupported archive format (want tar.gz or zip)")
	}
}

func validateTarGzBuildContext(data []byte) (buildContextInfo, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return buildContextInfo{}, buildContextError("gzip: %v", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	entries := make([]buildContextEntry, 0)
	var total int64
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return buildContextInfo{}, buildContextError("tar: %v", err)
		}
		// Links (symlink + hardlink) are the classic context-escape vector.
		isLink := hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink
		skip, err := archiveEntryDisposition(hdr.Name, hdr.FileInfo().Mode(), isLink)
		if err != nil {
			return buildContextInfo{}, err
		}
		if skip {
			continue
		}
		entry, n, err := hashedBuildContextEntry(hdr.Name, tr, total)
		if err != nil {
			return buildContextInfo{}, err
		}
		total += n
		entries = append(entries, entry)
		if len(entries) > maxBuildContextFileCount {
			return buildContextInfo{}, buildContextError("archive exceeds %d files", maxBuildContextFileCount)
		}
	}
	return finalizeBuildContext(entries, total)
}

// archiveEntryDisposition classifies an archive entry by its mode: skip=true for
// directories, an error for links and special files (device/fifo/socket), and
// (false, nil) for a regular file that should be hashed into the context.
func archiveEntryDisposition(name string, mode os.FileMode, isLink bool) (skip bool, err error) {
	if isLink {
		return false, buildContextError("archive contains link entry %q (not permitted)", name)
	}
	if mode.IsDir() {
		return true, nil
	}
	if !mode.IsRegular() {
		return false, buildContextError("archive contains special file %q (not permitted)", name)
	}
	return false, nil
}

// hashedBuildContextEntry normalizes an entry name and streams its content into a
// content hash, returning the entry and its uncompressed byte count.
func hashedBuildContextEntry(rawName string, r io.Reader, priorTotal int64) (buildContextEntry, int64, error) {
	name, err := normalizeBuildContextPath(rawName)
	if err != nil {
		return buildContextEntry{}, 0, err
	}
	sum, n, err := hashBuildContextEntry(r, priorTotal)
	if err != nil {
		return buildContextEntry{}, 0, err
	}
	return buildContextEntry{name: name, sha256: sum}, n, nil
}

func validateZipBuildContext(data []byte) (buildContextInfo, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return buildContextInfo{}, buildContextError("zip: %v", err)
	}
	entries := make([]buildContextEntry, 0, len(zr.File))
	var total int64
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		skip, err := archiveEntryDisposition(f.Name, f.Mode(), f.Mode()&os.ModeSymlink != 0)
		if err != nil {
			return buildContextInfo{}, err
		}
		if skip {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return buildContextInfo{}, buildContextError("open entry %q: %v", f.Name, err)
		}
		entry, n, err := hashedBuildContextEntry(f.Name, rc, total)
		_ = rc.Close()
		if err != nil {
			return buildContextInfo{}, err
		}
		total += n
		entries = append(entries, entry)
		if len(entries) > maxBuildContextFileCount {
			return buildContextInfo{}, buildContextError("archive exceeds %d files", maxBuildContextFileCount)
		}
	}
	return finalizeBuildContext(entries, total)
}

// hashBuildContextEntry streams one entry into a sha256, capping the running
// uncompressed total so a declared-small-but-huge entry (zip bomb) is caught while
// reading rather than by trusting the header.
func hashBuildContextEntry(r io.Reader, priorTotal int64) (string, int64, error) {
	remaining := maxBuildContextUncompressedBytes - priorTotal + 1
	sum := sha256.New()
	n, err := io.Copy(sum, io.LimitReader(r, remaining))
	if err != nil {
		return "", 0, buildContextError("read entry: %v", err)
	}
	if priorTotal+n > maxBuildContextUncompressedBytes {
		return "", 0, buildContextError("archive uncompressed size exceeds %d bytes", maxBuildContextUncompressedBytes)
	}
	return hex.EncodeToString(sum.Sum(nil)), n, nil
}

func normalizeBuildContextPath(name string) (string, error) {
	if name == "" {
		return "", buildContextError("archive entry has empty name")
	}
	if len(name) > maxBuildContextPathLength {
		return "", buildContextError("archive entry name exceeds %d characters", maxBuildContextPathLength)
	}
	if strings.ContainsRune(name, 0) {
		return "", buildContextError("archive entry name contains a NUL byte")
	}
	slashed := strings.ReplaceAll(name, `\`, "/")
	if strings.HasPrefix(slashed, "/") || (len(name) >= 2 && name[1] == ':') {
		return "", buildContextError("archive entry has an absolute path: %q", name)
	}
	clean := path.Clean(slashed)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", buildContextError("archive entry escapes the context directory: %q", name)
	}
	if strings.Count(strings.Trim(clean, "/"), "/")+1 > maxBuildContextPathDepth {
		return "", buildContextError("archive entry path depth exceeds %d", maxBuildContextPathDepth)
	}
	return clean, nil
}

func finalizeBuildContext(entries []buildContextEntry, total int64) (buildContextInfo, error) {
	if len(entries) == 0 {
		return buildContextInfo{}, buildContextError("archive contains no regular files")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	digest := sha256.New()
	for i, e := range entries {
		if i > 0 && entries[i-1].name == e.name {
			return buildContextInfo{}, buildContextError("archive contains a duplicate path: %q", e.name)
		}
		fmt.Fprintf(digest, "%s\x00%s\n", e.name, e.sha256)
	}
	return buildContextInfo{
		Digest:     "sha256:" + hex.EncodeToString(digest.Sum(nil)),
		FileCount:  len(entries),
		TotalBytes: total,
	}, nil
}
