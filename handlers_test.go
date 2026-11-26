package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer(t *testing.T, cfg config) *httptest.Server {
	t.Helper()
	st, err := newStore(t.TempDir())
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	srv := httptest.NewServer(newMux(cfg, st))
	t.Cleanup(srv.Close)
	return srv
}

func samplePayload(t *testing.T) []byte {
	t.Helper()
	body := map[string]any{
		"version":     1,
		"windowStart": "2026-11-09T13:00:00Z",
		"windowEnd":   "2026-11-10T13:00:00Z",
		"generatedAt": "2026-11-10T13:00:05Z",
		"words": []map[string]any{
			{"word": "Focus", "translation": "专注", "phonetic": "/ˈfəʊkəs/"},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

func TestPublishAndRead(t *testing.T) {
	srv := newTestServer(t, config{publishToken: "secret", readKey: "rk"})
	body := samplePayload(t)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/publish", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("publish status = %d, want 200", resp.StatusCode)
	}

	latest, err := http.Get(srv.URL + "/r/rk/latest.json")
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	defer latest.Body.Close()
	if latest.StatusCode != http.StatusOK {
		t.Fatalf("latest status = %d", latest.StatusCode)
	}
	var got payload
	if err := json.NewDecoder(latest.Body).Decode(&got); err != nil {
		t.Fatalf("decode latest: %v", err)
	}
	if len(got.Words) != 1 || got.Words[0].Word != "Focus" {
		t.Fatalf("unexpected payload: %+v", got)
	}
	if got.WindowEnd == "" || got.GeneratedAt == "" {
		t.Fatalf("missing window fields: %+v", got)
	}

	page, err := http.Get(srv.URL + "/r/rk/")
	if err != nil {
		t.Fatalf("get page: %v", err)
	}
	defer page.Body.Close()
	if page.StatusCode != http.StatusOK {
		t.Fatalf("page status = %d", page.StatusCode)
	}
	if ct := page.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("page content-type = %q", ct)
	}
}

func TestReadRejectsWrongKey(t *testing.T) {
	srv := newTestServer(t, config{publishToken: "secret", readKey: "rk"})
	resp, err := http.Get(srv.URL + "/r/wrong/latest.json")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestReadWhenEmpty(t *testing.T) {
	srv := newTestServer(t, config{publishToken: "secret", readKey: "rk"})
	resp, err := http.Get(srv.URL + "/r/rk/latest.json")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got payload
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Words) != 0 {
		t.Fatalf("want empty words, got %+v", got.Words)
	}
}

func TestPublishAuthAndValidation(t *testing.T) {
	srv := newTestServer(t, config{publishToken: "secret", readKey: "rk"})

	cases := []struct {
		name   string
		auth   string
		body   []byte
		status int
	}{
		{"no auth", "", samplePayload(t), http.StatusUnauthorized},
		{"wrong auth", "Bearer nope", samplePayload(t), http.StatusUnauthorized},
		{"bad json", "Bearer secret", []byte("{"), http.StatusBadRequest},
		{"wrong version", "Bearer secret", []byte(`{"version":2,"windowStart":"a","windowEnd":"b","words":[]}`), http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, srv.URL+"/publish", bytes.NewReader(c.body))
			if c.auth != "" {
				req.Header.Set("Authorization", c.auth)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("do: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != c.status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, c.status)
			}
		})
	}
}

func TestUnconfigured(t *testing.T) {
	srv := newTestServer(t, config{})

	resp, err := http.Post(srv.URL+"/publish", "application/json", bytes.NewReader(samplePayload(t)))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("publish status = %d, want 503", resp.StatusCode)
	}

	page, err := http.Get(srv.URL + "/r/anything/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	page.Body.Close()
	if page.StatusCode != http.StatusNotFound {
		t.Fatalf("page status = %d, want 404", page.StatusCode)
	}
}

func TestHealthAndAssets(t *testing.T) {
	srv := newTestServer(t, config{publishToken: "secret", readKey: "rk"})

	for _, path := range []string{"/health", "/", "/r/rk/manifest.webmanifest", "/r/rk/sw.js"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, resp.StatusCode)
		}
	}
}
