package updater

import (
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const ManifestSchemaVersion = 1

type State string

const (
	StateRequested  State = "requested"
	StateDownloaded State = "downloaded"
	StateVerified   State = "verified"
	StateSwitching  State = "switching"
	StateCommitted  State = "committed"
	StateFailed     State = "failed"
)

type Artifact struct {
	Filename          string `json:"filename"`
	ChecksumFilename  string `json:"checksum_filename"`
	SignatureFilename string `json:"signature_filename"`
}

type ReleaseManifest struct {
	SchemaVersion  int                 `json:"schema_version"`
	ReleaseID      string              `json:"release_id"`
	Version        string              `json:"version"`
	Sequence       uint64              `json:"sequence"`
	PublishedAt    time.Time           `json:"published_at"`
	MinimumVersion string              `json:"minimum_version"`
	Artifacts      map[string]Artifact `json:"artifacts"`
	Database       DatabaseRelease     `json:"database"`
	Architecture   string              `json:"architecture,omitempty"`
	Arch           string              `json:"arch,omitempty"`
}

type DatabaseRelease struct {
	FromSchema uint64 `json:"from_schema"`
	ToSchema   uint64 `json:"to_schema"`
}

type InstalledRelease struct {
	ReleaseID   string            `json:"release_id"`
	Version     string            `json:"version"`
	Sequence    uint64            `json:"sequence"`
	Schema      uint64            `json:"schema"`
	Images      map[string]string `json:"images"`
	CommittedAt time.Time         `json:"committed_at"`
}

type Operation struct {
	ID        string    `json:"id"`
	ReleaseID string    `json:"release_id"`
	State     State     `json:"state"`
	Revision  uint64    `json:"revision"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	LastError string    `json:"last_error,omitempty"`
}

type Status struct {
	Current   InstalledRelease `json:"current"`
	Operation *Operation       `json:"operation"`
}

type CheckResult struct {
	Current   InstalledRelease `json:"current"`
	Candidate ReleaseManifest  `json:"candidate"`
	HasUpdate bool             `json:"has_update"`
}

type Pending struct {
	ReleaseID string    `json:"release_id"`
	Version   string    `json:"version"`
	Operation Operation `json:"operation"`
	CreatedAt time.Time `json:"created_at"`
}

var (
	releaseIDPattern  = regexp.MustCompile(`^ov-dash-[0-9A-Za-z][0-9A-Za-z.-]{0,79}$`)
	repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
)

func ValidateReleaseID(id string) error {
	if !releaseIDPattern.MatchString(id) || strings.Contains(id, "..") {
		return errors.New("release_id must be an ov-dash release identifier")
	}
	return nil
}

func validateRepository(repository string) error {
	if !repositoryPattern.MatchString(repository) || strings.Contains(repository, "..") {
		return errors.New("release repository must use owner/name format")
	}
	return nil
}

func artifactKey() string {
	return runtime.GOOS + "_" + runtime.GOARCH
}

func (m ReleaseManifest) ArtifactForCurrentPlatform() (Artifact, error) {
	artifact, ok := m.Artifacts[artifactKey()]
	if !ok {
		return Artifact{}, fmt.Errorf("release does not contain an artifact for %s", artifactKey())
	}
	return artifact, nil
}
