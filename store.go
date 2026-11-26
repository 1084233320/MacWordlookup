package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

type store struct {
	dir    string
	mu     sync.RWMutex
	latest []byte
}

var windowNameSanitizer = regexp.MustCompile(`[^0-9A-Za-z._-]`)

func newStore(dir string) (*store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "windows"), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	s := &store{dir: dir}
	data, err := os.ReadFile(filepath.Join(dir, "latest.json"))
	if err == nil {
		s.latest = data
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read latest: %w", err)
	}
	return s, nil
}

func (s *store) put(payload []byte, windowEnd string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := writeFileAtomic(filepath.Join(s.dir, "latest.json"), payload); err != nil {
		return err
	}
	if name := windowNameSanitizer.ReplaceAllString(windowEnd, "_"); name != "" {
		if err := writeFileAtomic(filepath.Join(s.dir, "windows", name+".json"), payload); err != nil {
			return err
		}
	}
	s.latest = append([]byte(nil), payload...)
	return nil
}

func (s *store) latestPayload() ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.latest == nil {
		return nil, false
	}
	return append([]byte(nil), s.latest...), true
}

func writeFileAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
