package updater

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type UnixServer struct {
	SocketPath string
	SocketMode os.FileMode
	Handler    http.Handler
	Ready      func() error
	server     *http.Server
}

func (s *UnixServer) ListenAndServe(ctx context.Context) error {
	if s.Handler == nil || !filepath.IsAbs(s.SocketPath) {
		return errors.New("Unix server requires a handler and absolute socket path")
	}
	if err := os.MkdirAll(filepath.Dir(s.SocketPath), 0o750); err != nil {
		return fmt.Errorf("create Unix socket directory: %w", err)
	}
	if err := removeStaleSocket(s.SocketPath); err != nil {
		return err
	}
	listener, err := net.Listen("unix", s.SocketPath)
	if err != nil {
		return fmt.Errorf("listen on Unix socket: %w", err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(s.SocketPath)
	}()
	mode := s.SocketMode
	if mode == 0 {
		mode = 0o600
	}
	if mode.Perm()&0o007 != 0 {
		return errors.New("Unix socket cannot grant permissions to other users")
	}
	if err := os.Chmod(s.SocketPath, mode.Perm()); err != nil {
		return fmt.Errorf("set Unix socket permissions: %w", err)
	}
	if s.Ready != nil {
		if err := s.Ready(); err != nil {
			return fmt.Errorf("signal Unix server readiness: %w", err)
		}
	}
	s.server = &http.Server{
		Handler:           s.Handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       30 * time.Second,
	}
	serveError := make(chan error, 1)
	go func() {
		serveError <- s.server.Serve(listener)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		err := <-serveError
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-serveError:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func removeStaleSocket(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect Unix socket: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return errors.New("refusing to replace a non-socket filesystem entry")
	}
	connection, dialErr := net.DialTimeout("unix", path, 250*time.Millisecond)
	if dialErr == nil {
		_ = connection.Close()
		return errors.New("updater Unix socket is already active")
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove stale Unix socket: %w", err)
	}
	return nil
}
