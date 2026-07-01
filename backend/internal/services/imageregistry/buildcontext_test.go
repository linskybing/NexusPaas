package imageregistry

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

type archiveEntry struct {
	name     string
	body     string
	typeflag byte // tar only; 0 => regular
	linkname string
}

func makeTarGz(t *testing.T, entries []archiveEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typeflag := e.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		hdr := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.body)), Typeflag: typeflag, Linkname: e.linkname}
		if typeflag != tar.TypeReg {
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %q: %v", e.name, err)
		}
		if typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("write body %q: %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func makeZip(t *testing.T, entries []archiveEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatalf("zip create %q: %v", e.name, err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatalf("zip write %q: %v", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func TestValidateBuildContextArchiveHappyPathAndDeterminism(t *testing.T) {
	forward := []archiveEntry{{name: "Dockerfile", body: "FROM scratch"}, {name: "src/app.go", body: "package main"}}
	reversed := []archiveEntry{{name: "src/app.go", body: "package main"}, {name: "Dockerfile", body: "FROM scratch"}}

	tarInfo, err := validateBuildContextArchive(makeTarGz(t, forward))
	if err != nil {
		t.Fatalf("tar.gz valid context: %v", err)
	}
	if tarInfo.Digest == "" || tarInfo.FileCount != 2 {
		t.Fatalf("tar info = %#v, want digest + 2 files", tarInfo)
	}

	// Order-independent: same files in reverse order hash to the same digest.
	reorder, err := validateBuildContextArchive(makeTarGz(t, reversed))
	if err != nil {
		t.Fatalf("reordered context: %v", err)
	}
	if reorder.Digest != tarInfo.Digest {
		t.Fatalf("digest not order-independent: %s vs %s", reorder.Digest, tarInfo.Digest)
	}

	// Content-addressed across formats: an identical zip hashes to the same digest.
	zipInfo, err := validateBuildContextArchive(makeZip(t, forward))
	if err != nil {
		t.Fatalf("zip valid context: %v", err)
	}
	if zipInfo.Digest != tarInfo.Digest {
		t.Fatalf("tar.gz and zip of identical files disagree: %s vs %s", zipInfo.Digest, tarInfo.Digest)
	}
}

func TestValidateBuildContextArchiveRejectsMaliciousEntries(t *testing.T) {
	cases := []struct {
		name    string
		entries []archiveEntry
		want    string
	}{
		{"path traversal", []archiveEntry{{name: "../escape", body: "x"}}, "escapes"},
		{"absolute path", []archiveEntry{{name: "/etc/passwd", body: "x"}}, "absolute"},
		{"symlink", []archiveEntry{{name: "link", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"}}, "link entry"},
		{"hardlink", []archiveEntry{{name: "hard", typeflag: tar.TypeLink, linkname: "Dockerfile"}}, "link entry"},
		{"char device", []archiveEntry{{name: "dev", typeflag: tar.TypeChar}}, "special file"},
		{"fifo", []archiveEntry{{name: "pipe", typeflag: tar.TypeFifo}}, "special file"},
		{"no regular files", []archiveEntry{{name: "dir/", typeflag: tar.TypeDir}}, "no regular files"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateBuildContextArchive(makeTarGz(t, tc.entries))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestValidateBuildContextArchiveRejectsZipTraversal(t *testing.T) {
	_, err := validateBuildContextArchive(makeZip(t, []archiveEntry{{name: "../../evil", body: "x"}}))
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("zip traversal err = %v, want escape rejection", err)
	}
}

func TestValidateBuildContextArchiveRejectsUnsupportedAndEmpty(t *testing.T) {
	if _, err := validateBuildContextArchive(nil); err == nil {
		t.Fatal("empty archive should be rejected")
	}
	if _, err := validateBuildContextArchive([]byte("not an archive")); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("plain bytes err, want unsupported-format rejection")
	}
}

func TestValidateBuildContextArchiveEnforcesLimits(t *testing.T) {
	restore := func(a int, u int64, c int) {
		maxBuildContextArchiveBytes, maxBuildContextUncompressedBytes, maxBuildContextFileCount = a, u, c
	}
	defer restore(maxBuildContextArchiveBytes, maxBuildContextUncompressedBytes, maxBuildContextFileCount)

	// Zip-bomb guard: uncompressed total over the cap fails while reading.
	maxBuildContextUncompressedBytes = 4
	if _, err := validateBuildContextArchive(makeTarGz(t, []archiveEntry{{name: "big", body: "way too many bytes"}})); err == nil || !strings.Contains(err.Error(), "uncompressed size") {
		t.Fatalf("zip-bomb err = %v, want uncompressed-size rejection", err)
	}
	maxBuildContextUncompressedBytes = 512 << 20

	// File-count guard.
	maxBuildContextFileCount = 1
	if _, err := validateBuildContextArchive(makeTarGz(t, []archiveEntry{{name: "a", body: "1"}, {name: "b", body: "2"}})); err == nil || !strings.Contains(err.Error(), "files") {
		t.Fatalf("file-count err = %v, want file-count rejection", err)
	}
	maxBuildContextFileCount = 20000

	// Compressed archive size guard.
	maxBuildContextArchiveBytes = 8
	if _, err := validateBuildContextArchive(makeTarGz(t, []archiveEntry{{name: "a", body: "1"}})); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("archive-size err = %v, want size rejection", err)
	}
}
