package updater

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrOperationNotFound = errors.New("update operation not found")
	ErrRevisionConflict  = errors.New("update operation revision conflict")
)

type Store struct {
	root string
	now  func() time.Time
	mu   sync.Mutex
}

func NewStore(root string) (*Store, error) {
	return openStore(root, true)
}

func OpenStore(root string) (*Store, error) {
	return openStore(root, false)
}

func openStore(root string, create bool) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("updater state directory is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve updater state directory: %w", err)
	}
	operationsDir := filepath.Join(abs, "operations")
	if create {
		if err := os.MkdirAll(operationsDir, 0o700); err != nil {
			return nil, fmt.Errorf("create updater state directory: %w", err)
		}
	} else {
		info, err := os.Stat(operationsDir)
		if err != nil {
			return nil, fmt.Errorf("open updater state directory: %w", err)
		}
		if !info.IsDir() {
			return nil, errors.New("updater operations path is not a directory")
		}
	}
	return &Store{root: abs, now: time.Now}, nil
}

func (s *Store) Root() string {
	return s.root
}

func (s *Store) Create(operation Operation) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if operation.ID == "" {
		return Operation{}, errors.New("operation id is required")
	}
	if operation.State != StateRequested {
		return Operation{}, errors.New("new operation must be requested")
	}
	path := s.operationDir(operation.ID)
	if _, err := os.Stat(path); err == nil {
		return Operation{}, errors.New("operation already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Operation{}, fmt.Errorf("inspect operation directory: %w", err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		return Operation{}, fmt.Errorf("create operation directory: %w", err)
	}
	now := s.now().UTC()
	operation.Revision = 1
	operation.CreatedAt = now
	operation.UpdatedAt = now
	event := Event{
		OperationID: operation.ID,
		Revision:    operation.Revision,
		State:       operation.State,
		RecordedAt:  now,
		Operation:   operation,
	}
	if err := s.writeEvent(path, event); err != nil {
		_ = os.RemoveAll(path)
		return Operation{}, err
	}
	return operation, nil
}

func (s *Store) Update(current Operation, mutate func(*Operation) error) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	latest, err := s.load(current.ID)
	if err != nil {
		return Operation{}, err
	}
	if latest.Revision != current.Revision {
		return Operation{}, ErrRevisionConflict
	}
	next := latest
	if err := mutate(&next); err != nil {
		return Operation{}, err
	}
	if next.ID != latest.ID || next.ReleaseID != latest.ReleaseID || !next.CreatedAt.Equal(latest.CreatedAt) {
		return Operation{}, errors.New("immutable operation fields cannot be changed")
	}
	if next.State != latest.State {
		if err := ValidateTransition(latest.State, next.State); err != nil {
			return Operation{}, err
		}
	}
	now := s.now().UTC()
	next.Revision = latest.Revision + 1
	next.UpdatedAt = now
	event := Event{
		OperationID: next.ID,
		Revision:    next.Revision,
		Previous:    latest.State,
		State:       next.State,
		RecordedAt:  now,
		Operation:   next,
	}
	if err := s.writeEvent(s.operationDir(next.ID), event); err != nil {
		return Operation{}, err
	}
	return next, nil
}

func (s *Store) Get(id string) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load(id)
}

func (s *Store) Events(id string) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadEvents(id)
}

func (s *Store) List() ([]Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list()
}

func (s *Store) list() ([]Operation, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "operations"))
	if err != nil {
		return nil, fmt.Errorf("list update operations: %w", err)
	}
	operations := make([]Operation, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		operation, err := s.load(entry.Name())
		if err != nil {
			return nil, err
		}
		operations = append(operations, operation)
	}
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].CreatedAt.After(operations[j].CreatedAt)
	})
	return operations, nil
}

func (s *Store) Active() ([]Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	operations, err := s.list()
	if err != nil {
		return nil, err
	}
	active := make([]Operation, 0, 1)
	for _, operation := range operations {
		if operation.State.BlocksStartup() {
			active = append(active, operation)
		}
	}
	return active, nil
}

func (s *Store) load(id string) (Operation, error) {
	events, err := s.loadEvents(id)
	if err != nil {
		return Operation{}, err
	}
	return events[len(events)-1].Operation, nil
}

func (s *Store) loadEvents(id string) ([]Event, error) {
	if id == "" || strings.ContainsAny(id, `/\\`) || strings.Contains(id, "..") {
		return nil, ErrOperationNotFound
	}
	dir := s.operationDir(id)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrOperationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read operation journal: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("operation %s has no durable events", id)
	}
	events := make([]Event, 0, len(names))
	for index, name := range names {
		revision, err := strconv.ParseUint(strings.TrimSuffix(name, ".json"), 10, 64)
		if err != nil || revision != uint64(index+1) {
			return nil, fmt.Errorf("operation %s journal is not contiguous", id)
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read operation event: %w", err)
		}
		var event Event
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&event); err != nil {
			return nil, fmt.Errorf("decode operation event %s: %w", name, err)
		}
		if err := ensureStoreJSONEOF(decoder); err != nil {
			return nil, fmt.Errorf("decode operation event %s: %w", name, err)
		}
		if event.OperationID != id || event.Revision != revision || event.Operation.ID != id || event.Operation.Revision != revision || event.Operation.State != event.State {
			return nil, fmt.Errorf("operation event %s failed integrity checks", name)
		}
		if index == 0 {
			if event.State != StateRequested || event.Previous != "" {
				return nil, fmt.Errorf("operation %s has an invalid first event", id)
			}
		} else {
			previous := events[index-1]
			if event.Previous != previous.State {
				return nil, fmt.Errorf("operation %s journal previous state mismatch", id)
			}
			if event.State != previous.State {
				if err := ValidateTransition(previous.State, event.State); err != nil {
					return nil, err
				}
			}
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *Store) writeEvent(dir string, event Event) error {
	name := fmt.Sprintf("%020d.json", event.Revision)
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode operation event: %w", err)
	}
	data = append(data, '\n')
	if err := atomicWrite(filepath.Join(dir, name), data, 0o600); err != nil {
		return fmt.Errorf("persist operation event: %w", err)
	}
	return nil
}

func (s *Store) operationDir(id string) string {
	return filepath.Join(s.root, "operations", id)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempName)
	}
	if err := temp.Chmod(mode); err != nil {
		cleanup()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	return syncDirectory(dir)
}

func ensureStoreJSONEOF(decoder *json.Decoder) error {
	var value any
	if err := decoder.Decode(&value); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON data")
		}
		return err
	}
	return nil
}
