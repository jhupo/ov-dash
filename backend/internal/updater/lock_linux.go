//go:build linux

package updater

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

type runtimeLock struct {
	file *os.File
}

func acquireRuntimeLock(path string) (*runtimeLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open update lock: %w", err)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		file.Close()
		return nil, fmt.Errorf("acquire update lock: %w", err)
	}
	return &runtimeLock{file: file}, nil
}

func (l *runtimeLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	_ = unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
	return l.file.Close()
}
