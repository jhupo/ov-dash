package updater

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

const (
	maxManifestBytes  = 1 << 20
	maxSignatureBytes = 4096
)

var artifactFilenamePattern = regexp.MustCompile(`^ov-dash_[0-9A-Za-z.-]+_linux_(amd64|arm64)\.tar\.gz(?:\.sha256(?:\.sig)?)?$`)

type Verifier struct {
	publicKey ed25519.PublicKey
}

func NewVerifier(value string) (*Verifier, error) {
	key, err := parsePublicKey(value)
	if err != nil {
		return nil, err
	}
	return &Verifier{publicKey: key}, nil
}

func (v *Verifier) VerifyManifest(manifestBytes, signatureBytes []byte) (ReleaseManifest, error) {
	if len(manifestBytes) == 0 || len(manifestBytes) > maxManifestBytes {
		return ReleaseManifest{}, errors.New("release manifest size is invalid")
	}
	if err := v.verify(manifestBytes, signatureBytes); err != nil {
		return ReleaseManifest{}, fmt.Errorf("verify release manifest: %w", err)
	}
	var manifest ReleaseManifest
	if err := decodeStrictJSON(manifestBytes, &manifest); err != nil {
		return ReleaseManifest{}, fmt.Errorf("decode release manifest: %w", err)
	}
	if err := validateManifest(manifest); err != nil {
		return ReleaseManifest{}, err
	}
	return manifest, nil
}

func (v *Verifier) VerifyChecksum(checksumBytes, signatureBytes []byte, filename string) (string, error) {
	if err := v.verify(checksumBytes, signatureBytes); err != nil {
		return "", fmt.Errorf("verify release checksum: %w", err)
	}
	fields := strings.Fields(string(checksumBytes))
	if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != filename {
		return "", errors.New("checksum file does not match release artifact")
	}
	if len(fields[0]) != 64 {
		return "", errors.New("release checksum must be SHA-256")
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return "", errors.New("release checksum is invalid")
	}
	return strings.ToLower(fields[0]), nil
}

func (v *Verifier) verify(payload, encodedSignature []byte) error {
	if v == nil || len(v.publicKey) != ed25519.PublicKeySize {
		return errors.New("release public key is unavailable")
	}
	if len(encodedSignature) == 0 || len(encodedSignature) > maxSignatureBytes {
		return errors.New("detached signature size is invalid")
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(string(encodedSignature)))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("detached signature must be base64 Ed25519")
	}
	if !ed25519.Verify(v.publicKey, payload, signature) {
		return errors.New("detached signature is invalid")
	}
	return nil
}

func parsePublicKey(value string) (ed25519.PublicKey, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("UPDATE_PUBLIC_KEY is required for online updates")
	}
	data := []byte(value)
	if block, rest := pem.Decode(data); block != nil {
		if len(bytes.TrimSpace(rest)) != 0 {
			return nil, errors.New("release public key contains trailing data")
		}
		parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse release public key: %w", err)
		}
		key, ok := parsed.(ed25519.PublicKey)
		if !ok {
			return nil, errors.New("release public key is not Ed25519")
		}
		return key, nil
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(value)
	if err == nil && len(decoded) == ed25519.PublicKeySize {
		return ed25519.PublicKey(decoded), nil
	}
	if err == nil {
		parsed, parseErr := x509.ParsePKIXPublicKey(decoded)
		if parseErr == nil {
			if key, ok := parsed.(ed25519.PublicKey); ok {
				return key, nil
			}
		}
	}
	return nil, errors.New("UPDATE_PUBLIC_KEY must be PKIX PEM or base64 Ed25519")
}

func validateManifest(manifest ReleaseManifest) error {
	if manifest.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("unsupported release schema_version %d", manifest.SchemaVersion)
	}
	if err := ValidateReleaseID(manifest.ReleaseID); err != nil {
		return err
	}
	if _, err := parseSemanticVersion(manifest.Version); err != nil {
		return fmt.Errorf("invalid release version: %w", err)
	}
	if manifest.ReleaseID != "ov-dash-"+manifest.Version {
		return errors.New("release_id must equal ov-dash-<version>")
	}
	if manifest.Sequence == 0 {
		return errors.New("release sequence must be greater than zero")
	}
	if manifest.PublishedAt.IsZero() || manifest.PublishedAt.After(time.Now().UTC().Add(10*time.Minute)) {
		return errors.New("release published_at is invalid")
	}
	if manifest.MinimumVersion == "" {
		return errors.New("release minimum_version is required")
	}
	if _, err := parseSemanticVersion(manifest.MinimumVersion); err != nil {
		return fmt.Errorf("invalid minimum_version: %w", err)
	}
	if manifest.Database.ToSchema < manifest.Database.FromSchema {
		return errors.New("database to_schema cannot be lower than from_schema")
	}
	if len(manifest.Artifacts) == 0 {
		return errors.New("release artifacts are required")
	}
	for key, artifact := range manifest.Artifacts {
		if key != "linux_amd64" && key != "linux_arm64" {
			return fmt.Errorf("unsupported release artifact %q", key)
		}
		for _, filename := range []string{artifact.Filename, artifact.ChecksumFilename, artifact.SignatureFilename} {
			if !artifactFilenamePattern.MatchString(filename) || strings.Contains(filename, "..") {
				return fmt.Errorf("invalid release artifact filename %q", filename)
			}
		}
		if artifact.ChecksumFilename != artifact.Filename+".sha256" ||
			artifact.SignatureFilename != artifact.ChecksumFilename+".sig" {
			return fmt.Errorf("release artifact metadata is inconsistent for %s", key)
		}
	}
	return nil
}

func validateUpgrade(candidate ReleaseManifest, current InstalledRelease) error {
	currentVersion, err := parseSemanticVersion(current.Version)
	if err != nil {
		return errors.New("installed release has an invalid version")
	}
	targetVersion, _ := parseSemanticVersion(candidate.Version)
	minimumVersion, _ := parseSemanticVersion(candidate.MinimumVersion)
	if compareSemanticVersions(targetVersion, currentVersion) <= 0 {
		return errors.New("release version must be newer than the installed version")
	}
	if candidate.Sequence <= current.Sequence {
		return errors.New("release sequence must increase")
	}
	if compareSemanticVersions(currentVersion, minimumVersion) < 0 {
		return errors.New("installed version is below release minimum_version")
	}
	if current.Schema < candidate.Database.FromSchema || current.Schema > candidate.Database.ToSchema {
		return errors.New("installed database schema is outside the supported update range")
	}
	return nil
}

func decodeStrictJSON(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
