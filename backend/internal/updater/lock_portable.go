//go:build !linux

package updater

import (
	"errors"
	"fmt"
	"os"
)

type portableOperationLock struct {
	file *os.File
	path string
}

func acquireOperationLock(path, owner string) (operationLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil, ErrOperationActive
	}
	if err != nil {
		return nil, fmt.Errorf("create update operation lock: %w", err)
	}
	if _, err := file.WriteString(owner + "\n"); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return &portableOperationLock{file: file, path: path}, nil
}

func (l *portableOperationLock) Release() error {
	return errors.Join(l.file.Close(), os.Remove(l.path))
}
