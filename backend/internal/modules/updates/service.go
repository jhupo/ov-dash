package updates

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"ov-dash/backend/internal/updater"
)

const (
	updaterBaseURL     = "http://updater"
	maxResponseBytes   = 1 << 20
	healthTimeout      = 2 * time.Second
	statusTimeout      = 5 * time.Second
	checkTimeout       = 30 * time.Second
	operationTimeout   = 5 * time.Second
	applyTimeout       = 10 * time.Second
	unixConnectTimeout = 2 * time.Second
)

var operationIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

var (
	ErrInvalidSocketPath = errors.New("updater socket path is required")
	ErrInvalidOperation  = errors.New("invalid updater operation id")
	ErrResponseTooLarge  = errors.New("updater response exceeds size limit")
)

type Service struct {
	socketPath string
	client     *http.Client
}

type Health struct {
	Status string `json:"status"`
}

type Status struct {
	Current   updater.InstalledRelease `json:"current"`
	Operation *updater.Operation       `json:"operation"`
}

type OperationEvent struct {
	Revision   uint64        `json:"revision"`
	Previous   updater.State `json:"previous,omitempty"`
	State      updater.State `json:"state"`
	RecordedAt time.Time     `json:"recorded_at"`
	Error      string        `json:"error,omitempty"`
}

type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("updater request failed with status %d (%s)", e.StatusCode, e.Code)
	}
	return fmt.Sprintf("updater request failed with status %d (%s): %s", e.StatusCode, e.Code, e.Message)
}

func NewService(socketPath string, client *http.Client) *Service {
	socketPath = strings.TrimSpace(socketPath)
	if client == nil {
		dialer := &net.Dialer{Timeout: unixConnectTimeout}
		client = &http.Client{
			Transport: &http.Transport{
				DisableCompression: true,
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return dialer.DialContext(ctx, "unix", socketPath)
				},
			},
		}
	}
	return &Service{socketPath: socketPath, client: client}
}

func (s *Service) Health(ctx context.Context) (Health, error) {
	var result Health
	err := s.request(ctx, healthTimeout, http.MethodGet, "/healthz", nil, &result)
	return result, err
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	var result Status
	err := s.request(ctx, statusTimeout, http.MethodGet, "/v1/status", nil, &result)
	return result, err
}

func (s *Service) Check(ctx context.Context) (updater.CheckResult, error) {
	var result updater.CheckResult
	err := s.request(ctx, checkTimeout, http.MethodPost, "/v1/check", nil, &result)
	return result, err
}

func (s *Service) Apply(ctx context.Context, releaseID string) (updater.Operation, error) {
	if err := updater.ValidateReleaseID(releaseID); err != nil {
		return updater.Operation{}, fmt.Errorf("invalid release_id: %w", err)
	}
	request := struct {
		ReleaseID string `json:"release_id"`
	}{ReleaseID: releaseID}
	var result updater.Operation
	err := s.request(ctx, applyTimeout, http.MethodPost, "/v1/operations", request, &result)
	return result, err
}

func (s *Service) Operation(ctx context.Context, operationID string) (updater.Operation, error) {
	if !operationIDPattern.MatchString(operationID) {
		return updater.Operation{}, ErrInvalidOperation
	}
	var result updater.Operation
	err := s.request(ctx, operationTimeout, http.MethodGet, "/v1/operations/"+operationID, nil, &result)
	return result, err
}

func (s *Service) OperationEvents(ctx context.Context, operationID string) ([]OperationEvent, error) {
	if !operationIDPattern.MatchString(operationID) {
		return nil, ErrInvalidOperation
	}
	var response struct {
		Items []OperationEvent `json:"items"`
	}
	err := s.request(ctx, operationTimeout, http.MethodGet, "/v1/operations/"+operationID+"/events", nil, &response)
	return response.Items, err
}

func (s *Service) request(ctx context.Context, timeout time.Duration, method, path string, body any, result any) error {
	if s == nil || strings.TrimSpace(s.socketPath) == "" {
		return ErrInvalidSocketPath
	}
	if s.client == nil {
		return errors.New("updater HTTP client is required")
	}

	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode updater request: %w", err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(requestContext, method, updaterBaseURL+path, requestBody)
	if err != nil {
		return fmt.Errorf("create updater request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("call updater: %w", err)
	}
	defer response.Body.Close()

	data, err := readResponse(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decodeAPIError(response.StatusCode, data)
	}
	if err := decodeStrictJSON(data, result); err != nil {
		return fmt.Errorf("decode updater response: %w", err)
	}
	return nil
}

func readResponse(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read updater response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return nil, ErrResponseTooLarge
	}
	return data, nil
}

func decodeAPIError(statusCode int, data []byte) error {
	var response struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := decodeStrictJSON(data, &response); err != nil || response.Error.Code == "" {
		return fmt.Errorf("updater returned malformed error response with status %d", statusCode)
	}
	return &APIError{
		StatusCode: statusCode,
		Code:       response.Error.Code,
		Message:    response.Error.Message,
	}
}

func decodeStrictJSON(data []byte, destination any) error {
	if len(data) == 0 {
		return errors.New("empty response body")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values in response")
		}
		return err
	}
	return nil
}
