package updates

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUpdaterScriptWritesValidTerminalStatusJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}
	tmp := t.TempDir()
	statusPath := filepath.Join(tmp, "status.json")
	script := updaterScript("origin", "v9.9.9", statusPath, "backend", "frontend", false)
	prefix, _, ok := strings.Cut(script, "\nbackend_image=")
	if !ok {
		t.Fatal("updater script layout changed")
	}

	cmd := exec.Command("sh", "-c", prefix+"\nwrite_status error 'image pull failed' 0\n")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("write_status failed: %v\n%s", err, output)
	}

	data, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	var state UpdateResult
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("status is not valid json: %v\n%s", err, data)
	}
	if state.Status != "error" || state.Progress != 0 || state.Version != "v9.9.9" {
		t.Fatalf("unexpected status: %+v", state)
	}
	if state.EndedAt == nil {
		t.Fatal("terminal status should include endedAt")
	}
}
