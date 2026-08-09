package updater

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestVerifierAcceptsSignedSnapshotRelease(t *testing.T) {
	verifier, privateKey, now := newTestVerifier(t)
	bundle := signedTestBundle(t, privateKey, testManifest(now))

	manifest, err := verifier.Verify(bundle, "ov-dash-2.0.0", testInstalledRelease())
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if manifest.Database.Strategy != "snapshot" {
		t.Fatalf("strategy = %q, want snapshot", manifest.Database.Strategy)
	}
}

func TestVerifierDefaultsDatabaseStrategyToSnapshot(t *testing.T) {
	verifier, privateKey, now := newTestVerifier(t)
	manifest := testManifest(now)
	manifest.Database.Strategy = ""
	bundle := signedTestBundle(t, privateKey, manifest)

	verified, err := verifier.Verify(bundle, manifest.ReleaseID, testInstalledRelease())
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if verified.Database.Strategy != "snapshot" {
		t.Fatalf("strategy = %q, want snapshot", verified.Database.Strategy)
	}
}

func TestVerifierRejectsTamperingRollbackAndMutableImages(t *testing.T) {
	verifier, privateKey, now := newTestVerifier(t)
	base := testManifest(now)

	tests := []struct {
		name    string
		mutate  func(*ReleaseManifest)
		current InstalledRelease
		want    string
	}{
		{name: "sequence rollback", mutate: func(m *ReleaseManifest) { m.Sequence = 10 }, current: testInstalledRelease(), want: "sequence"},
		{name: "version rollback", mutate: func(m *ReleaseManifest) { m.Version = "1.0.0"; m.ReleaseID = "ov-dash-1.0.0" }, current: testInstalledRelease(), want: "newer"},
		{name: "mutable tag", mutate: func(m *ReleaseManifest) { m.Images["backend"] = "ghcr.io/example/backend:latest" }, current: testInstalledRelease(), want: "immutable"},
		{name: "wrong strategy", mutate: func(m *ReleaseManifest) { m.Database.Strategy = "expand" }, current: testInstalledRelease(), want: "snapshot"},
		{name: "missing installed state", mutate: func(*ReleaseManifest) {}, current: InstalledRelease{}, want: "installed release"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := cloneManifest(base)
			test.mutate(&manifest)
			bundle := signedTestBundle(t, privateKey, manifest)
			_, err := verifier.Verify(bundle, manifest.ReleaseID, test.current)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Verify() error = %v, want substring %q", err, test.want)
			}
		})
	}

	bundle := signedTestBundle(t, privateKey, base)
	bundle.Manifest = append([]byte(nil), bundle.Manifest...)
	bundle.Manifest[len(bundle.Manifest)-2] ^= 1
	if _, err := verifier.Verify(bundle, base.ReleaseID, testInstalledRelease()); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("tampered Verify() error = %v, want signature error", err)
	}
}

func TestVerifierRejectsUnknownAndDuplicateFields(t *testing.T) {
	verifier, privateKey, _ := newTestVerifier(t)
	current := testInstalledRelease()

	unknown := []byte(`{"schema_version":1,"release_id":"ov-dash-2.0.0","version":"2.0.0","sequence":20,"published_at":"2026-08-09T00:00:00Z","expires_at":"2026-08-10T00:00:00Z","minimum_version":"1.0.0","images":{"backend":"ghcr.io/example/backend@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","frontend":"ghcr.io/example/frontend@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},"database":{"from_schema":10,"to_schema":11,"strategy":"snapshot","transactional":false,"backup_required":true},"health":{"timeout_seconds":30,"stability_seconds":5},"command":"rm"}`)
	unknownBundle := ReleaseBundle{Manifest: unknown, Signature: encodeSignature(ed25519.Sign(privateKey, unknown))}
	if _, err := verifier.Verify(unknownBundle, "ov-dash-2.0.0", current); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}

	duplicate := []byte(`{"schema_version":1,"schema_version":1}`)
	duplicateBundle := ReleaseBundle{Manifest: duplicate, Signature: encodeSignature(ed25519.Sign(privateKey, duplicate))}
	if _, err := verifier.Verify(duplicateBundle, "ov-dash-2.0.0", current); err == nil || !strings.Contains(err.Error(), "duplicate key") {
		t.Fatalf("duplicate key error = %v", err)
	}
}

func TestVerifierAcceptsSchemaWithinCompatibilityRange(t *testing.T) {
	verifier, privateKey, now := newTestVerifier(t)
	manifest := testManifest(now)
	manifest.Database.FromSchema = 5
	current := testInstalledRelease()
	current.Schema = 8

	if _, err := verifier.Verify(signedTestBundle(t, privateKey, manifest), manifest.ReleaseID, current); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	current.Schema = 4
	if _, err := verifier.Verify(signedTestBundle(t, privateKey, manifest), manifest.ReleaseID, current); err == nil || !strings.Contains(err.Error(), "below") {
		t.Fatalf("below-range Verify() error = %v", err)
	}
	current.Schema = 12
	if _, err := verifier.Verify(signedTestBundle(t, privateKey, manifest), manifest.ReleaseID, current); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("above-range Verify() error = %v", err)
	}
}

func newTestVerifier(t *testing.T) (*Verifier, ed25519.PrivateKey, time.Time) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	verifier, err := NewVerifier(publicKey, VerifyOptions{
		AllowedImages: []string{"backend", "frontend"},
		Now:           func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	return verifier, privateKey, now
}

func testManifest(now time.Time) ReleaseManifest {
	return ReleaseManifest{
		SchemaVersion:  1,
		ReleaseID:      "ov-dash-2.0.0",
		Version:        "2.0.0",
		Sequence:       20,
		PublishedAt:    now.Add(-time.Hour),
		ExpiresAt:      now.Add(time.Hour),
		MinimumVersion: "1.0.0",
		Images: map[string]string{
			"backend":  "ghcr.io/example/backend@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"frontend": "ghcr.io/example/frontend@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
		Database: DatabaseRelease{FromSchema: 10, ToSchema: 11, Strategy: "snapshot", BackupRequired: true},
		Health:   HealthPolicy{TimeoutSeconds: 30, StabilitySeconds: 5},
	}
}

func testInstalledRelease() InstalledRelease {
	return InstalledRelease{
		ReleaseID: "ov-dash-1.0.0", Version: "1.0.0", Sequence: 10, Schema: 10,
		Images: map[string]string{
			"backend":  "ghcr.io/example/backend@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
			"frontend": "ghcr.io/example/frontend@sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		},
	}
}

func signedTestBundle(t *testing.T, privateKey ed25519.PrivateKey, manifest ReleaseManifest) ReleaseBundle {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return ReleaseBundle{Manifest: data, Signature: encodeSignature(ed25519.Sign(privateKey, data))}
}

func encodeSignature(signature []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(signature))
}

func cloneManifest(manifest ReleaseManifest) ReleaseManifest {
	manifest.Images = cloneStrings(manifest.Images)
	return manifest
}
