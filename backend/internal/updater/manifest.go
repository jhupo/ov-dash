package updater

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	manifestSchemaVersion = 1
	maxManifestBytes      = 1 << 20
	maxSignatureBytes     = 4096
)

var (
	artifactNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	imageDigestPattern  = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?::[0-9]+)?(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)+@sha256:[a-f0-9]{64}$`)
)

type VerifyOptions struct {
	AllowedImages []string
	Now           func() time.Time
}

type Verifier struct {
	publicKey     ed25519.PublicKey
	allowedImages map[string]struct{}
	now           func() time.Time
}

func NewVerifier(publicKey ed25519.PublicKey, options VerifyOptions) (*Verifier, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, errors.New("invalid Ed25519 public key")
	}
	if len(options.AllowedImages) == 0 {
		return nil, errors.New("at least one allowed image is required")
	}
	allowed := make(map[string]struct{}, len(options.AllowedImages))
	for _, name := range options.AllowedImages {
		if !artifactNamePattern.MatchString(name) {
			return nil, fmt.Errorf("invalid allowed image name %q", name)
		}
		if _, exists := allowed[name]; exists {
			return nil, fmt.Errorf("duplicate allowed image name %q", name)
		}
		allowed[name] = struct{}{}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Verifier{publicKey: append(ed25519.PublicKey(nil), publicKey...), allowedImages: allowed, now: now}, nil
}

func (v *Verifier) Verify(bundle ReleaseBundle, requestedID string, current InstalledRelease) (ReleaseManifest, error) {
	manifest, err := v.Inspect(bundle, requestedID)
	if err != nil {
		return ReleaseManifest{}, err
	}
	if err := v.validateUpgrade(manifest, current); err != nil {
		return ReleaseManifest{}, err
	}
	return manifest, nil
}

func (v *Verifier) Inspect(bundle ReleaseBundle, requestedID string) (ReleaseManifest, error) {
	if len(bundle.Manifest) == 0 || len(bundle.Manifest) > maxManifestBytes {
		return ReleaseManifest{}, errors.New("release manifest size is invalid")
	}
	signature, err := decodeDetachedSignature(bundle.Signature)
	if err != nil {
		return ReleaseManifest{}, err
	}
	if !ed25519.Verify(v.publicKey, bundle.Manifest, signature) {
		return ReleaseManifest{}, errors.New("release manifest signature is invalid")
	}
	if err := rejectDuplicateJSONKeys(bundle.Manifest); err != nil {
		return ReleaseManifest{}, err
	}

	var manifest ReleaseManifest
	decoder := json.NewDecoder(bytes.NewReader(bundle.Manifest))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return ReleaseManifest{}, fmt.Errorf("decode release manifest: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return ReleaseManifest{}, err
	}
	if manifest.Database.Strategy == "" {
		manifest.Database.Strategy = "snapshot"
	}
	if err := v.validateManifest(manifest, requestedID); err != nil {
		return ReleaseManifest{}, err
	}
	return manifest, nil
}

func (v *Verifier) validateManifest(manifest ReleaseManifest, requestedID string) error {
	if manifest.SchemaVersion != manifestSchemaVersion {
		return fmt.Errorf("unsupported manifest schema_version %d", manifest.SchemaVersion)
	}
	if err := ValidateReleaseID(manifest.ReleaseID); err != nil {
		return err
	}
	if manifest.ReleaseID != requestedID {
		return errors.New("manifest release_id does not match the requested release")
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
	now := v.now().UTC()
	if manifest.PublishedAt.IsZero() || manifest.ExpiresAt.IsZero() {
		return errors.New("published_at and expires_at are required")
	}
	if !manifest.ExpiresAt.After(manifest.PublishedAt) {
		return errors.New("expires_at must be after published_at")
	}
	if manifest.PublishedAt.After(now.Add(5 * time.Minute)) {
		return errors.New("release published_at is in the future")
	}
	if !manifest.ExpiresAt.After(now) {
		return errors.New("release manifest has expired")
	}
	if err := v.validateImages(manifest.Images); err != nil {
		return err
	}
	if err := validateDatabaseRelease(manifest.Database); err != nil {
		return err
	}
	if manifest.Health.TimeoutSeconds < 10 || manifest.Health.TimeoutSeconds > 900 {
		return errors.New("health timeout_seconds must be between 10 and 900")
	}
	if manifest.Health.StabilitySeconds < 5 || manifest.Health.StabilitySeconds > manifest.Health.TimeoutSeconds {
		return errors.New("health stability_seconds must be between 5 and timeout_seconds")
	}
	return nil
}

func (v *Verifier) validateUpgrade(manifest ReleaseManifest, current InstalledRelease) error {
	if current.Empty() {
		return errors.New("online update requires an installed release state")
	}
	currentVersion, err := parseSemanticVersion(current.Version)
	if err != nil {
		return errors.New("installed release has an invalid version")
	}
	if manifest.Sequence <= current.Sequence {
		return errors.New("release sequence must increase monotonically")
	}
	targetVersion, err := parseSemanticVersion(manifest.Version)
	if err != nil {
		return errors.New("release version is not valid SemVer")
	}
	if compareSemanticVersions(targetVersion, currentVersion) <= 0 {
		return errors.New("release version must be newer than the installed version")
	}
	if manifest.MinimumVersion == "" {
		return errors.New("minimum_version is required for an upgrade")
	}
	minimumVersion, err := parseSemanticVersion(manifest.MinimumVersion)
	if err != nil {
		return errors.New("minimum_version is not valid SemVer")
	}
	if compareSemanticVersions(currentVersion, minimumVersion) < 0 {
		return errors.New("installed version is below minimum_version")
	}
	if current.Schema < manifest.Database.FromSchema {
		return errors.New("installed schema is below database from_schema")
	}
	if current.Schema > manifest.Database.ToSchema {
		return errors.New("installed schema is newer than database to_schema")
	}
	return nil
}

func (v *Verifier) validateImages(images map[string]string) error {
	if len(images) != len(v.allowedImages) {
		return fmt.Errorf("manifest images must contain exactly: %s", strings.Join(sortedKeys(v.allowedImages), ","))
	}
	for name := range v.allowedImages {
		digest, ok := images[name]
		if !ok {
			return fmt.Errorf("manifest image %q is missing", name)
		}
		if !imageDigestPattern.MatchString(digest) {
			return fmt.Errorf("manifest image %q must be an immutable sha256 digest reference", name)
		}
	}
	for name := range images {
		if _, ok := v.allowedImages[name]; !ok {
			return fmt.Errorf("manifest image %q is not allowed", name)
		}
	}
	return nil
}

func validateDatabaseRelease(database DatabaseRelease) error {
	if database.Strategy != "snapshot" {
		return errors.New("database strategy must be snapshot")
	}
	if !database.BackupRequired {
		return errors.New("snapshot updates require a database backup")
	}
	if database.ToSchema < database.FromSchema {
		return errors.New("database to_schema cannot be lower than from_schema")
	}
	return nil
}

func decodeDetachedSignature(value []byte) ([]byte, error) {
	if len(value) == 0 || len(value) > maxSignatureBytes {
		return nil, errors.New("detached signature size is invalid")
	}
	encoded := strings.TrimSpace(string(value))
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(decoded) != ed25519.SignatureSize {
		return nil, errors.New("detached signature must be a base64 Ed25519 signature")
	}
	return decoded, nil
}

func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read release public key: %w", err)
	}
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
	decoded, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, errors.New("release public key must be PKIX PEM or base64 Ed25519")
	}
	return ed25519.PublicKey(decoded), nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, isDelimiter := token.(json.Delim)
		if !isDelimiter {
			return nil
		}
		switch delimiter {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("JSON object key is not a string")
				}
				if _, exists := seen[key]; exists {
					return fmt.Errorf("release manifest contains duplicate key %q", key)
				}
				seen[key] = struct{}{}
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return errors.New("unexpected JSON delimiter")
		}
	}
	if err := walk(); err != nil {
		return fmt.Errorf("validate release manifest JSON: %w", err)
	}
	return requireJSONEOF(decoder)
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON document contains trailing data")
		}
		return fmt.Errorf("decode trailing JSON data: %w", err)
	}
	return nil
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
