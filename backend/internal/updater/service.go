package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxMetadataBytes = 1 << 20
	maxArchiveBytes  = 2 << 30
	maxExtractBytes  = 4 << 30
	maxArchiveFiles  = 20000
)

var (
	ErrOperationActive   = errors.New("an update operation is already active")
	ErrOperationNotFound = errors.New("update operation not found")
	ErrReleaseMismatch   = errors.New("requested release does not match latest signed release")
)

type ProxyResolver func(context.Context) (string, error)

type ServiceConfig struct {
	RuntimeDir        string
	ReleaseRepository string
	PublicKey         string
	ExitDelay         time.Duration
	HTTPClient        *http.Client
	ResolveProxy      ProxyResolver
	RequestShutdown   func()
}

type Service struct {
	runtimeDir      string
	repository      string
	verifier        *Verifier
	verifierErr     error
	client          *http.Client
	resolveProxy    ProxyResolver
	requestShutdown func()
	exitDelay       time.Duration
	mu              sync.Mutex
}

func NewService(config ServiceConfig) (*Service, error) {
	config.RuntimeDir = filepath.Clean(strings.TrimSpace(config.RuntimeDir))
	config.ReleaseRepository = strings.TrimSpace(config.ReleaseRepository)
	if config.RuntimeDir == "." || config.RuntimeDir == "" {
		return nil, errors.New("update runtime directory is required")
	}
	if err := validateRepository(config.ReleaseRepository); err != nil {
		return nil, err
	}
	verifier, verifierErr := NewVerifier(config.PublicKey)
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 30 * time.Minute}
	}
	if config.ExitDelay <= 0 {
		config.ExitDelay = 2 * time.Second
	}
	return &Service{
		runtimeDir:      config.RuntimeDir,
		repository:      config.ReleaseRepository,
		verifier:        verifier,
		verifierErr:     verifierErr,
		client:          config.HTTPClient,
		resolveProxy:    config.ResolveProxy,
		requestShutdown: config.RequestShutdown,
		exitDelay:       config.ExitDelay,
	}, nil
}

func (s *Service) Status() (Status, error) {
	current, err := readInstalledRelease(s.runtimeDir)
	if err != nil {
		return Status{}, err
	}
	operation, err := readOperation(s.runtimeDir)
	if err != nil {
		return Status{}, err
	}
	return Status{Current: current, Operation: operation}, nil
}

func (s *Service) Check(ctx context.Context) (CheckResult, error) {
	current, err := readInstalledRelease(s.runtimeDir)
	if err != nil {
		return CheckResult{}, err
	}
	manifest, err := s.fetchManifest(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	result := CheckResult{Current: current, Candidate: manifest}
	if err := validateUpgrade(manifest, current); err == nil {
		result.HasUpdate = true
	}
	return result, nil
}

func (s *Service) Apply(ctx context.Context, releaseID string) (Operation, error) {
	if err := ValidateReleaseID(releaseID); err != nil {
		return Operation{}, err
	}
	manifest, err := s.fetchManifest(ctx)
	if err != nil {
		return Operation{}, err
	}
	if manifest.ReleaseID != releaseID {
		return Operation{}, ErrReleaseMismatch
	}
	current, err := readInstalledRelease(s.runtimeDir)
	if err != nil {
		return Operation{}, err
	}
	if err := validateUpgrade(manifest, current); err != nil {
		return Operation{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if operation, err := readOperation(s.runtimeDir); err != nil {
		return Operation{}, err
	} else if operation != nil && operation.State != StateCommitted && operation.State != StateFailed {
		return Operation{}, ErrOperationActive
	}
	now := time.Now().UTC()
	operation := Operation{
		ID: newOperationID(), ReleaseID: releaseID, State: StateRequested,
		Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := os.MkdirAll(s.runtimeDir, 0o700); err != nil {
		return Operation{}, fmt.Errorf("create update runtime: %w", err)
	}
	if err := writeJSONAtomic(filepath.Join(s.runtimeDir, "operation.json"), operation, 0o600); err != nil {
		return Operation{}, err
	}
	go s.prepare(context.Background(), manifest, operation)
	return operation, nil
}

func (s *Service) Operation(id string) (Operation, error) {
	operation, err := readOperation(s.runtimeDir)
	if err != nil {
		return Operation{}, err
	}
	if operation == nil || operation.ID != id {
		return Operation{}, ErrOperationNotFound
	}
	return *operation, nil
}

func (s *Service) prepare(ctx context.Context, manifest ReleaseManifest, operation Operation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.prepareLocked(ctx, manifest, &operation); err != nil {
		operation.State = StateFailed
		operation.Revision++
		operation.UpdatedAt = time.Now().UTC()
		operation.LastError = err.Error()
		_ = writeJSONAtomic(filepath.Join(s.runtimeDir, "operation.json"), operation, 0o600)
	}
}

func (s *Service) prepareLocked(ctx context.Context, manifest ReleaseManifest, operation *Operation) error {
	lock, err := acquireRuntimeLock(filepath.Join(s.runtimeDir, "update.lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	if _, err := os.Stat(filepath.Join(s.runtimeDir, "pending.json")); err == nil {
		return ErrOperationActive
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect pending update: %w", err)
	}

	artifact, err := manifest.ArtifactForCurrentPlatform()
	if err != nil {
		return err
	}
	proxyURL, err := s.proxyURL(ctx)
	if err != nil {
		return err
	}
	baseURL := fmt.Sprintf("https://github.com/%s/releases/download/v%s/", s.repository, manifest.Version)
	checksumBytes, err := s.getBytes(ctx, applyDownloadProxy(proxyURL, baseURL+artifact.ChecksumFilename), maxMetadataBytes)
	if err != nil {
		return fmt.Errorf("download release checksum: %w", err)
	}
	signatureBytes, err := s.getBytes(ctx, applyDownloadProxy(proxyURL, baseURL+artifact.SignatureFilename), maxMetadataBytes)
	if err != nil {
		return fmt.Errorf("download release checksum signature: %w", err)
	}
	expectedChecksum, err := s.verifier.VerifyChecksum(checksumBytes, signatureBytes, artifact.Filename)
	if err != nil {
		return err
	}

	downloadsDir := filepath.Join(s.runtimeDir, "downloads")
	if err := os.MkdirAll(downloadsDir, 0o700); err != nil {
		return fmt.Errorf("create update download directory: %w", err)
	}
	archivePath := filepath.Join(downloadsDir, artifact.Filename+".part")
	if err := os.Remove(archivePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale release download: %w", err)
	}
	defer os.Remove(archivePath)
	checksum, err := s.downloadArchive(ctx, applyDownloadProxy(proxyURL, baseURL+artifact.Filename), archivePath)
	if err != nil {
		return err
	}
	operation.State = StateDownloaded
	operation.Revision++
	operation.UpdatedAt = time.Now().UTC()
	if err := writeJSONAtomic(filepath.Join(s.runtimeDir, "operation.json"), operation, 0o600); err != nil {
		return err
	}
	if checksum != expectedChecksum {
		return errors.New("release archive checksum does not match signed checksum")
	}

	releasesDir := filepath.Join(s.runtimeDir, "releases")
	if err := os.MkdirAll(releasesDir, 0o755); err != nil {
		return fmt.Errorf("create releases directory: %w", err)
	}
	stagingDir, err := os.MkdirTemp(releasesDir, ".staging-"+manifest.Version+"-")
	if err != nil {
		return fmt.Errorf("create release staging directory: %w", err)
	}
	defer os.RemoveAll(stagingDir)
	if err := extractArchive(archivePath, stagingDir); err != nil {
		return err
	}
	if err := validateExtractedRelease(stagingDir, manifest); err != nil {
		return err
	}
	operation.State = StateVerified
	operation.Revision++
	operation.UpdatedAt = time.Now().UTC()
	if err := writeJSONAtomic(filepath.Join(s.runtimeDir, "operation.json"), operation, 0o600); err != nil {
		return err
	}

	releaseDir := filepath.Join(releasesDir, manifest.Version)
	if _, err := os.Stat(releaseDir); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(stagingDir, releaseDir); err != nil {
			return fmt.Errorf("install release directory: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect release directory: %w", err)
	} else if err := validateExtractedRelease(releaseDir, manifest); err != nil {
		return fmt.Errorf("existing release directory is invalid: %w", err)
	}

	operation.State = StateSwitching
	operation.Revision++
	operation.UpdatedAt = time.Now().UTC()
	if s.requestShutdown == nil {
		return errors.New("application shutdown callback is unavailable")
	}
	pending := Pending{
		ReleaseID: manifest.ReleaseID, Version: manifest.Version,
		Operation: *operation, CreatedAt: time.Now().UTC(),
	}
	if err := writeJSONAtomic(filepath.Join(s.runtimeDir, "pending.json"), pending, 0o600); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(s.runtimeDir, "operation.json"), operation, 0o600); err != nil {
		return err
	}
	time.AfterFunc(s.exitDelay, s.requestShutdown)
	return nil
}

func (s *Service) fetchManifest(ctx context.Context) (ReleaseManifest, error) {
	if s.verifierErr != nil {
		return ReleaseManifest{}, s.verifierErr
	}
	proxyURL, err := s.proxyURL(ctx)
	if err != nil {
		return ReleaseManifest{}, err
	}
	baseURL := fmt.Sprintf("https://github.com/%s/releases/latest/download/", s.repository)
	manifestBytes, err := s.getBytes(ctx, applyDownloadProxy(proxyURL, baseURL+"release.json"), maxManifestBytes)
	if err != nil {
		return ReleaseManifest{}, fmt.Errorf("download release manifest: %w", err)
	}
	signatureBytes, err := s.getBytes(ctx, applyDownloadProxy(proxyURL, baseURL+"release.json.sig"), maxSignatureBytes)
	if err != nil {
		return ReleaseManifest{}, fmt.Errorf("download release manifest signature: %w", err)
	}
	return s.verifier.VerifyManifest(manifestBytes, signatureBytes)
}

func (s *Service) proxyURL(ctx context.Context) (string, error) {
	if s.resolveProxy == nil {
		return "", nil
	}
	value, err := s.resolveProxy(ctx)
	if err != nil {
		return "", fmt.Errorf("read update proxy setting: %w", err)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", errors.New("update.proxy_url must be an HTTPS URL without credentials")
	}
	return strings.TrimRight(value, "/"), nil
}

func applyDownloadProxy(proxyURL, target string) string {
	if proxyURL == "" {
		return target
	}
	return proxyURL + "/" + target
}

func (s *Service) getBytes(ctx context.Context, target string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/octet-stream, application/json")
	request.Header.Set("Accept-Encoding", "identity")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release server returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("release metadata exceeds size limit")
	}
	return data, nil
}

func (s *Service) downloadArchive(ctx context.Context, target, destination string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept-Encoding", "identity")
	response, err := s.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("download release archive: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("release server returned HTTP %d", response.StatusCode)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("create release archive: %w", err)
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(response.Body, maxArchiveBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return "", fmt.Errorf("download release archive: %w", copyErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close release archive: %w", closeErr)
	}
	if written > maxArchiveBytes {
		return "", errors.New("release archive exceeds size limit")
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func extractArchive(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open release archive: %w", err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open release gzip stream: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	var total int64
	files := 0
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read release archive: %w", err)
		}
		name := strings.TrimPrefix(filepath.ToSlash(header.Name), "./")
		clean := filepath.Clean(filepath.FromSlash(name))
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("release archive contains unsafe path %q", header.Name)
		}
		target := filepath.Join(destination, clean)
		files++
		if files > maxArchiveFiles {
			return errors.New("release archive contains too many files")
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || total+header.Size > maxExtractBytes {
				return errors.New("release archive extracted size exceeds limit")
			}
			total += header.Size
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(header.Mode) & 0o755
			if mode == 0 {
				mode = 0o644
			}
			output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(output, tarReader, header.Size)
			closeErr := output.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("release archive contains unsupported entry %q", header.Name)
		}
	}
	return nil
}

func validateExtractedRelease(directory string, release ReleaseManifest) error {
	requiredFiles := []string{
		"manifest.json", filepath.Join("bin", "app"), filepath.Join("bin", "migrate"),
		filepath.Join("bin", "bootstrap-admin"), filepath.Join("web", "index.html"),
	}
	for _, name := range requiredFiles {
		info, err := os.Stat(filepath.Join(directory, name))
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("release is missing required file %s", filepath.ToSlash(name))
		}
	}
	migrations, err := os.ReadDir(filepath.Join(directory, "migrations"))
	if err != nil || len(migrations) == 0 {
		return errors.New("release migrations directory is missing or empty")
	}
	data, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		return err
	}
	var packageManifest struct {
		Version      string `json:"version"`
		ReleaseID    string `json:"release_id"`
		Architecture string `json:"architecture"`
		Arch         string `json:"arch"`
	}
	if err := json.Unmarshal(data, &packageManifest); err != nil {
		return fmt.Errorf("decode package manifest: %w", err)
	}
	arch := packageManifest.Architecture
	if arch == "" {
		arch = packageManifest.Arch
	}
	if packageManifest.Version != release.Version || (packageManifest.ReleaseID != "" && packageManifest.ReleaseID != release.ReleaseID) {
		return errors.New("package manifest does not match signed release")
	}
	if arch != runtime.GOARCH {
		return fmt.Errorf("package architecture %q does not match %s", arch, runtime.GOARCH)
	}
	for _, name := range []string{filepath.Join(directory, "bin", "app"), filepath.Join(directory, "bin", "migrate"), filepath.Join(directory, "bin", "bootstrap-admin")} {
		if err := os.Chmod(name, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func readInstalledRelease(runtimeDir string) (InstalledRelease, error) {
	currentDir := filepath.Join(runtimeDir, "current")
	data, err := os.ReadFile(filepath.Join(currentDir, "manifest.json"))
	if err != nil {
		return InstalledRelease{}, fmt.Errorf("read installed release manifest: %w", err)
	}
	var manifest struct {
		ReleaseID   string          `json:"release_id"`
		Version     string          `json:"version"`
		Sequence    uint64          `json:"sequence"`
		PublishedAt time.Time       `json:"published_at"`
		Database    DatabaseRelease `json:"database"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return InstalledRelease{}, fmt.Errorf("decode installed release manifest: %w", err)
	}
	if manifest.Version == "" {
		return InstalledRelease{}, errors.New("installed release version is missing")
	}
	if manifest.ReleaseID == "" {
		manifest.ReleaseID = "ov-dash-" + manifest.Version
	}
	schema := manifest.Database.ToSchema
	if schema == 0 {
		schema = migrationSchema(filepath.Join(currentDir, "migrations"))
	}
	committedAt := manifest.PublishedAt
	if committedAt.IsZero() {
		if info, statErr := os.Stat(filepath.Join(currentDir, "manifest.json")); statErr == nil {
			committedAt = info.ModTime().UTC()
		}
	}
	if operation, _ := readOperation(runtimeDir); operation != nil && operation.ReleaseID == manifest.ReleaseID && operation.State == StateCommitted {
		committedAt = operation.UpdatedAt
	}
	return InstalledRelease{
		ReleaseID: manifest.ReleaseID, Version: manifest.Version, Sequence: manifest.Sequence,
		Schema: schema, Images: map[string]string{}, CommittedAt: committedAt,
	}, nil
}

func migrationSchema(directory string) uint64 {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0
	}
	var latest uint64
	for _, entry := range entries {
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			continue
		}
		value, err := strconv.ParseUint(strings.TrimLeft(prefix, "0"), 10, 64)
		if err == nil && value > latest {
			latest = value
		}
	}
	return latest
}

func readOperation(runtimeDir string) (*Operation, error) {
	data, err := os.ReadFile(filepath.Join(runtimeDir, "operation.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read update operation: %w", err)
	}
	var operation Operation
	if err := decodeStrictJSON(data, &operation); err != nil {
		return nil, fmt.Errorf("decode update operation: %w", err)
	}
	return &operation, nil
}

func writeJSONAtomic(path string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func newOperationID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("%032x", time.Now().UnixNano())
	}
	return hex.EncodeToString(value[:])
}
