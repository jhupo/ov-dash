package updates

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ov-dash/backend/internal/config"

	"go.uber.org/zap"
)

var (
	ErrUpdateDisabled = errors.New("online update is disabled")
	ErrUpdateRunning  = errors.New("online update is already running")
	ErrNoNewVersion   = errors.New("no new version available")
	ErrNotGitRepo     = errors.New("update workdir is not a git repository")
)

type Service struct {
	cfg    config.Config
	logger *zap.Logger

	mu       sync.Mutex
	check    *CheckResult
	updating bool
	update   *UpdateResult
}

type CheckResult struct {
	CurrentVersion string     `json:"currentVersion"`
	CurrentCommit  string     `json:"currentCommit"`
	LatestVersion  string     `json:"latestVersion"`
	HasUpdate      bool       `json:"hasUpdate"`
	CheckedAt      *time.Time `json:"checkedAt,omitempty"`
	Message        string     `json:"message"`
	Enabled        bool       `json:"enabled"`
	Updating       bool       `json:"updating"`
}

type UpdateResult struct {
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
	Version   string     `json:"version"`
	Status    string     `json:"status"`
	Message   string     `json:"message"`
}

func NewService(cfg config.Config, logger *zap.Logger) *Service {
	return &Service{cfg: cfg, logger: logger}
}

func (s *Service) Status(ctx context.Context) (CheckResult, *UpdateResult, error) {
	current, commit := s.localVersion(ctx)
	fileUpdate := s.readUpdateState()

	s.mu.Lock()
	defer s.mu.Unlock()

	if fileUpdate != nil && fileUpdate.Status == "running" && !updaterContainerRunning(ctx, s.cfg.Update.Project) {
		endedAt := time.Now().UTC()
		fileUpdate.EndedAt = &endedAt
		fileUpdate.Status = "error"
		fileUpdate.Message = "更新进程已停止，请查看 Docker 日志"
		s.writeUpdateState(*fileUpdate)
	}
	if fileUpdate != nil {
		s.update = fileUpdate
		s.updating = fileUpdate.Status == "running"
	}

	result := CheckResult{
		CurrentVersion: current,
		CurrentCommit:  commit,
		Enabled:        s.cfg.Update.Enabled,
		Updating:       s.updating,
	}
	if s.check != nil {
		result.LatestVersion = s.check.LatestVersion
		result.HasUpdate = s.check.LatestVersion != "" && s.check.LatestVersion != current
		result.CheckedAt = s.check.CheckedAt
		result.Message = s.check.Message
	}
	return result, s.update, nil
}

func (s *Service) Check(ctx context.Context) (CheckResult, error) {
	if !s.cfg.Update.Enabled {
		return CheckResult{Enabled: false, Message: ErrUpdateDisabled.Error()}, ErrUpdateDisabled
	}
	if !s.isGitRepo(ctx) {
		return CheckResult{Enabled: true, Message: ErrNotGitRepo.Error()}, ErrNotGitRepo
	}

	current, commit := s.localVersion(ctx)
	if err := s.git(ctx, "fetch", "--tags", "--force", s.cfg.Update.Remote); err != nil {
		return CheckResult{}, err
	}
	tagsRaw, err := s.gitOutput(ctx, "tag", "--list")
	if err != nil {
		return CheckResult{}, err
	}
	latest := latestTag(strings.Fields(tagsRaw))
	result := CheckResult{
		CurrentVersion: current,
		CurrentCommit:  commit,
		LatestVersion:  latest,
		HasUpdate:      latest != "" && latest != current,
		Enabled:        true,
	}
	checkedAt := time.Now().UTC()
	result.CheckedAt = &checkedAt
	if latest == "" {
		result.Message = "没有找到可用 tag"
	} else if result.HasUpdate {
		result.Message = "发现新版本"
	} else {
		result.Message = "当前已是最新版本"
	}

	s.mu.Lock()
	result.Updating = s.updating
	s.check = &result
	s.mu.Unlock()

	return result, nil
}

func (s *Service) Update(ctx context.Context) (UpdateResult, error) {
	if !s.cfg.Update.Enabled {
		return UpdateResult{}, ErrUpdateDisabled
	}

	check, err := s.Check(ctx)
	if err != nil {
		return UpdateResult{}, err
	}
	if !check.HasUpdate {
		return UpdateResult{}, ErrNoNewVersion
	}

	s.mu.Lock()
	if s.updating || updaterContainerRunning(ctx, s.cfg.Update.Project) {
		s.mu.Unlock()
		return UpdateResult{}, ErrUpdateRunning
	}
	result := UpdateResult{
		StartedAt: time.Now().UTC(),
		Version:   check.LatestVersion,
		Status:    "running",
		Message:   "正在更新",
	}
	s.updating = true
	s.update = &result
	s.mu.Unlock()
	s.writeUpdateState(result)

	if err := s.startUpdater(ctx, check.LatestVersion); err != nil {
		endedAt := time.Now().UTC()
		result.EndedAt = &endedAt
		result.Status = "error"
		result.Message = trimOutput(err.Error())
		s.mu.Lock()
		s.updating = false
		s.update = &result
		s.mu.Unlock()
		s.writeUpdateState(result)
		return UpdateResult{}, err
	}

	return result, nil
}

func (s *Service) startUpdater(ctx context.Context, version string) error {
	statusPath := filepath.Join(s.cfg.Update.WorkDir, ".ovdash-update-status.json")
	script := updaterScript(s.cfg.Update.Remote, version, statusPath)
	args := []string{
		"run",
		"--detach",
		"--rm",
		"--user",
		"0:0",
		"--name",
		updaterName(s.cfg.Update.Project),
		"--env",
		"HTTP_PROXY",
		"--env",
		"HTTPS_PROXY",
		"--env",
		"ALL_PROXY",
		"--env",
		"NO_PROXY",
		"-v",
		s.cfg.Update.WorkDir + ":" + s.cfg.Update.WorkDir,
		"-v",
		"/root/.netrc:/root/.netrc:ro",
		"-v",
		"/var/run/docker.sock:/var/run/docker.sock",
		"-w",
		s.cfg.Update.WorkDir,
		s.cfg.Update.Image,
		"sh",
		"-c",
		script,
	}
	if _, err := s.command(ctx, "", "docker", args...); err != nil {
		return err
	}
	return nil
}

func updaterScript(remote string, version string, statusPath string) string {
	startedAt := time.Now().UTC().Format(time.RFC3339Nano)
	return fmt.Sprintf(`set -eu
write_status() {
  status="$1"
  message="$2"
  ended=""
  if [ "$status" != "running" ]; then ended=", \"endedAt\": \"$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ\")\"; fi
  printf '{"startedAt":"%s"%%s,"version":"%s","status":"%%s","message":"%%s"}\n' "$ended" "$status" "$(printf '%%s' "$message" | sed 's/\\/\\\\/g; s/"/\\"/g')" > %s
}
write_status running 正在更新
if git fetch --tags --force %s && git checkout --force %s && git reset --hard %s && docker compose --env-file .env up -d --build; then
  write_status success 更新完成
else
  write_status error 更新失败
  exit 1
fi`,
		startedAt,
		jsonEscape(version),
		shellQuote(statusPath),
		shellQuote(remote),
		shellQuote(version),
		shellQuote(version),
	)
}

func (s *Service) localVersion(ctx context.Context) (string, string) {
	version := strings.TrimSpace(s.cfg.App.Version)
	commit, err := s.gitOutput(ctx, "rev-parse", "--short", "HEAD")
	if err != nil {
		commit = ""
	}
	commit = strings.TrimSpace(commit)
	if version == "" || version == "local" {
		if tag, err := s.gitOutput(ctx, "describe", "--tags", "--exact-match"); err == nil {
			version = strings.TrimSpace(tag)
		}
	}
	if version == "" {
		version = "local"
	}
	return version, commit
}

func (s *Service) readUpdateState() *UpdateResult {
	path := filepath.Join(s.cfg.Update.WorkDir, ".ovdash-update-status.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var result UpdateResult
	if err := json.Unmarshal(data, &result); err != nil {
		s.logger.Warn("read update state", zap.Error(err))
		return nil
	}
	return &result
}

func (s *Service) writeUpdateState(result UpdateResult) {
	path := filepath.Join(s.cfg.Update.WorkDir, ".ovdash-update-status.json")
	data, err := json.Marshal(result)
	if err != nil {
		s.logger.Warn("encode update state", zap.Error(err))
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		s.logger.Warn("write update state", zap.Error(err))
	}
}

func (s *Service) git(ctx context.Context, args ...string) error {
	_, err := s.command(ctx, s.cfg.Update.WorkDir, "git", args...)
	return err
}

func (s *Service) gitOutput(ctx context.Context, args ...string) (string, error) {
	return s.command(ctx, s.cfg.Update.WorkDir, "git", args...)
}

func (s *Service) isGitRepo(ctx context.Context) bool {
	output, err := s.gitOutput(ctx, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(output) == "true"
}

func (s *Service) command(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	output := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
	if err != nil {
		return trimOutput(output), fmt.Errorf("%s %s failed: %w: %s", name, strings.Join(args, " "), err, trimOutput(output))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func updaterContainerRunning(ctx context.Context, project string) bool {
	name := updaterName(project)
	cmd := exec.CommandContext(ctx, "docker", "ps", "--filter", "name=^/"+name+"$", "--format", "{{.Names}}")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return false
	}
	return strings.TrimSpace(stdout.String()) == name
}

func updaterName(project string) string {
	project = strings.TrimSpace(project)
	if project == "" {
		project = "ov-dash"
	}
	project = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, project)
	return project + "-updater"
}

var tagPartPattern = regexp.MustCompile(`\d+|[A-Za-z]+`)

func latestTag(tags []string) string {
	values := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			values = append(values, tag)
		}
	}
	sort.Slice(values, func(i, j int) bool {
		return compareTags(values[i], values[j]) < 0
	})
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

func compareTags(a string, b string) int {
	ap := tagParts(a)
	bp := tagParts(b)
	for i := 0; i < len(ap) || i < len(bp); i++ {
		if i >= len(ap) {
			return -1
		}
		if i >= len(bp) {
			return 1
		}
		if ap[i].num && bp[i].num {
			if ap[i].number < bp[i].number {
				return -1
			}
			if ap[i].number > bp[i].number {
				return 1
			}
			continue
		}
		if ap[i].value < bp[i].value {
			return -1
		}
		if ap[i].value > bp[i].value {
			return 1
		}
	}
	return strings.Compare(a, b)
}

type tagPart struct {
	value  string
	number int
	num    bool
}

func tagParts(tag string) []tagPart {
	raw := tagPartPattern.FindAllString(strings.TrimPrefix(tag, "v"), -1)
	parts := make([]tagPart, 0, len(raw))
	for _, value := range raw {
		if number, err := strconv.Atoi(value); err == nil {
			parts = append(parts, tagPart{value: value, number: number, num: true})
			continue
		}
		parts = append(parts, tagPart{value: strings.ToLower(value)})
	}
	return parts
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func jsonEscape(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	return strings.Trim(string(data), `"`)
}

func trimOutput(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 1200 {
		return value[:1200]
	}
	return value
}
