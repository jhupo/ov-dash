package updater

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Executor interface {
	Discover(context.Context) (string, error)
	Download(context.Context, string) (ReleaseBundle, error)
	Current(context.Context) (InstalledRelease, error)
	Preflight(context.Context, ReleaseManifest) error
	Quiesce(context.Context, ReleaseManifest) error
	Backup(context.Context, Operation) (Backup, error)
	Migrate(context.Context, Operation) error
	Switch(context.Context, Operation) error
	HealthCheck(context.Context, Operation) error
	Commit(context.Context, Operation) error
	Rollback(context.Context, Operation) error
	Recover(context.Context, Operation) (RecoveryDecision, error)
}

type Command struct {
	Name string
	Args []string
	Dir  string
	Env  []string
}

type CommandResult struct {
	Output   string
	ExitCode int
}

type CommandRunner interface {
	Run(context.Context, Command) (CommandResult, error)
	RunStream(context.Context, Command, io.Reader, io.Writer) (CommandResult, error)
}

type OSCommandRunner struct {
	MaxOutputBytes int
}

func (r OSCommandRunner) Run(ctx context.Context, command Command) (CommandResult, error) {
	return r.run(ctx, command, nil, nil)
}

func (r OSCommandRunner) RunStream(ctx context.Context, command Command, stdin io.Reader, stdout io.Writer) (CommandResult, error) {
	if stdout == nil {
		return CommandResult{}, errors.New("stream command stdout is required")
	}
	return r.run(ctx, command, stdin, stdout)
}

func (r OSCommandRunner) run(ctx context.Context, command Command, stdin io.Reader, stdout io.Writer) (CommandResult, error) {
	if err := validateCommand(command); err != nil {
		return CommandResult{}, err
	}
	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	cmd.Dir = command.Dir
	if len(command.Env) > 0 {
		cmd.Env = append(os.Environ(), command.Env...)
	}
	limit := r.MaxOutputBytes
	if limit <= 0 {
		limit = 64 << 10
	}
	output := &limitedBuffer{limit: limit}
	cmd.Stdin = stdin
	if stdout == nil {
		cmd.Stdout = output
	} else {
		cmd.Stdout = stdout
	}
	cmd.Stderr = output
	err := cmd.Run()
	result := CommandResult{Output: output.String(), ExitCode: 0}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	}
	return result, fmt.Errorf("command %s failed with exit code %d: %w", filepath.Base(command.Name), result.ExitCode, err)
}

func validateCommand(command Command) error {
	name := strings.TrimSpace(command.Name)
	if name == "" || strings.ContainsRune(name, '\x00') {
		return errors.New("command name is invalid")
	}
	extension := strings.ToLower(filepath.Ext(name))
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(name), extension))
	switch extension {
	case ".bat", ".cmd", ".ps1", ".vbs", ".vbe", ".wsf":
		return errors.New("script and shell command files are forbidden")
	}
	switch base {
	case "sh", "bash", "dash", "zsh", "fish", "cmd", "powershell", "pwsh", "wscript", "cscript":
		return errors.New("shell interpreters are forbidden")
	}
	for _, arg := range command.Args {
		if strings.ContainsRune(arg, '\x00') {
			return errors.New("command argument contains a NUL byte")
		}
	}
	for _, variable := range command.Env {
		if strings.ContainsRune(variable, '\x00') || !strings.Contains(variable, "=") {
			return errors.New("command environment entry is invalid")
		}
	}
	if command.Dir != "" && !filepath.IsAbs(command.Dir) {
		return errors.New("command working directory must be absolute")
	}
	if runtime.GOOS == "windows" && strings.ContainsAny(name, "&|<>") {
		return errors.New("command name contains forbidden characters")
	}
	return nil
}

type limitedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	originalLength := len(data)
	remaining := b.limit - b.buffer.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
			b.truncated = true
		}
		_, _ = b.buffer.Write(data)
	} else if originalLength > 0 {
		b.truncated = true
	}
	return originalLength, nil
}

func (b *limitedBuffer) String() string {
	if b.truncated {
		return b.buffer.String() + "\n[output truncated]"
	}
	return b.buffer.String()
}
