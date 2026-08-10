package updater

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestVerifierAcceptsSignedManifestAndChecksum(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerifier(base64.StdEncoding.EncodeToString(publicKey))
	if err != nil {
		t.Fatal(err)
	}
	filename := "ov-dash_1.2.3_linux_" + runtime.GOARCH + ".tar.gz"
	manifest := ReleaseManifest{
		SchemaVersion:  1,
		ReleaseID:      "ov-dash-1.2.3",
		Version:        "1.2.3",
		Sequence:       12,
		PublishedAt:    time.Now().UTC().Add(-time.Minute),
		MinimumVersion: "1.0.0",
		Database:       DatabaseRelease{FromSchema: 1, ToSchema: 2},
		Artifacts: map[string]Artifact{
			"linux_" + runtime.GOARCH: {
				Filename: filename, ChecksumFilename: filename + ".sha256",
				SignatureFilename: filename + ".sha256.sig",
			},
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestSignature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, manifestBytes))
	verified, err := verifier.VerifyManifest(manifestBytes, []byte(manifestSignature))
	if err != nil {
		t.Fatalf("VerifyManifest() error = %v", err)
	}
	if verified.ReleaseID != manifest.ReleaseID {
		t.Fatalf("VerifyManifest() release_id = %q", verified.ReleaseID)
	}

	checksum := strings.Repeat("a", 64) + "  " + filename + "\n"
	checksumSignature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, []byte(checksum)))
	value, err := verifier.VerifyChecksum([]byte(checksum), []byte(checksumSignature), filename)
	if err != nil {
		t.Fatalf("VerifyChecksum() error = %v", err)
	}
	if value != strings.Repeat("a", 64) {
		t.Fatalf("VerifyChecksum() = %q", value)
	}
}

func TestExtractArchiveRejectsPathTraversal(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "release.tar.gz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	payload := []byte("unsafe")
	if err := tarWriter.WriteHeader(&tar.Header{Name: "../outside", Mode: 0o644, Size: int64(len(payload))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	destination := t.TempDir()
	if err := extractArchive(archivePath, destination); err == nil {
		t.Fatal("extractArchive() accepted a traversal path")
	}
}

func TestApplyDownloadProxy(t *testing.T) {
	target := "https://github.com/jhupo/ov-dash/releases/latest/download/release.json"
	if got := applyDownloadProxy("", target); got != target {
		t.Fatalf("applyDownloadProxy() without proxy = %q", got)
	}
	want := "https://ghfast.top/" + target
	if got := applyDownloadProxy("https://ghfast.top", target); got != want {
		t.Fatalf("applyDownloadProxy() = %q, want %q", got, want)
	}
}
