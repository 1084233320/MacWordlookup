package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorePutAndReload(t *testing.T) {
	dir := t.TempDir()
	st, err := newStore(dir)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	if _, ok := st.latestPayload(); ok {
		t.Fatal("expected empty store")
	}

	data := []byte(`{"version":1,"windowStart":"a","windowEnd":"2026-11-10T13:00:00Z","generatedAt":"b","words":[]}`)
	if err := st.put(data, "2026-11-10T13:00:00Z"); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, ok := st.latestPayload()
	if !ok || string(got) != string(data) {
		t.Fatalf("latest = %q, ok = %v", got, ok)
	}
	if _, err := os.Stat(filepath.Join(dir, "windows", "2026-11-10T13_00_00Z.json")); err != nil {
		t.Fatalf("window archive missing: %v", err)
	}

	reopened, err := newStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, ok = reopened.latestPayload()
	if !ok || string(got) != string(data) {
		t.Fatalf("latest after reopen = %q, ok = %v", got, ok)
	}
}
