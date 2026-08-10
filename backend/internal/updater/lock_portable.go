//go:build !linux

package updater

import (
	"fmt"
	"os"
	"sync"
)

var portableRuntimeLock sync.Mutex

type runtimeLock struct {
	file *os.File
}

func acquireRuntimeLock(path string) (*runtimeLock, error) {
	portableRuntimeLock.Lock()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		portableRuntimeLock.Unlock()
		return nil, fmt.Errorf("open update lock: %w", err)
	}
	return &runtimeLock{file: file}, nil
}

func (l *runtimeLock) Close() error {
	if l == nil {
		return nil
	}
	if l.file != nil {
		_ = l.file.Close()
	}
	portableRuntimeLock.Unlock()
	return nil
}
