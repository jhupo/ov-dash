package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxLatestMetadataBytes = 1024

type ReleaseSource interface {
	Latest(context.Context) (string, error)
	Download(context.Context, string) (ReleaseBundle, error)
}

type HTTPReleaseSource struct {
	baseURL   *url.URL
	client    *http.Client
	allowHTTP bool
}

func NewHTTPReleaseSource(baseURL string, client *http.Client, allowHTTP bool) (*HTTPReleaseSource, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && !(allowHTTP && parsed.Scheme == "http")) {
		return nil, errors.New("release source must be an absolute HTTPS URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("release source URL cannot contain credentials, query, or fragment")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &HTTPReleaseSource{baseURL: parsed, client: client, allowHTTP: allowHTTP}, nil
}

func (s *HTTPReleaseSource) Latest(ctx context.Context) (string, error) {
	data, err := s.fetch(ctx, "latest.json", "latest.json", maxLatestMetadataBytes)
	if err != nil {
		return "", err
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return "", fmt.Errorf("decode latest release metadata: %w", err)
	}
	var metadata struct {
		ReleaseID string `json:"release_id"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&metadata); err != nil {
		return "", fmt.Errorf("decode latest release metadata: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return "", fmt.Errorf("decode latest release metadata: %w", err)
	}
	if err := ValidateReleaseID(metadata.ReleaseID); err != nil {
		return "", fmt.Errorf("decode latest release metadata: %w", err)
	}
	return metadata.ReleaseID, nil
}

func (s *HTTPReleaseSource) Download(ctx context.Context, releaseID string) (ReleaseBundle, error) {
	if err := ValidateReleaseID(releaseID); err != nil {
		return ReleaseBundle{}, err
	}
	manifest, err := s.fetch(ctx, url.PathEscape(releaseID)+"/release.json", "release.json", maxManifestBytes)
	if err != nil {
		return ReleaseBundle{}, err
	}
	signature, err := s.fetch(ctx, url.PathEscape(releaseID)+"/release.json.sig", "release.json.sig", maxSignatureBytes)
	if err != nil {
		return ReleaseBundle{}, err
	}
	return ReleaseBundle{Manifest: manifest, Signature: signature}, nil
}

func (s *HTTPReleaseSource) fetch(ctx context.Context, path, name string, limit int64) ([]byte, error) {
	target := *s.baseURL
	target.Path = strings.TrimRight(s.baseURL.Path, "/") + "/" + path
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/octet-stream")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", name, err)
	}
	defer response.Body.Close()
	if response.Request.URL.Host != s.baseURL.Host || (response.Request.URL.Scheme != "https" && !(s.allowHTTP && response.Request.URL.Scheme == "http")) {
		return nil, errors.New("release source redirected outside the configured origin")
	}
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("download %s: unexpected HTTP status %d", name, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", name, err)
	}
	if len(data) == 0 || int64(len(data)) > limit {
		return nil, fmt.Errorf("download %s: response size is invalid", name)
	}
	return data, nil
}
