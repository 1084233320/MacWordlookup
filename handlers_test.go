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
	srv := newTestServer(t, config{publishToken: "secret"})
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

	latest, err := http.Get(srv.URL + "/latest.json")
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

	page, err := http.Get(srv.URL + "/")
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

func TestLegacyReadPathRedirects(t *testing.T) {
	srv := newTestServer(t, config{publishToken: "secret"})
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(srv.URL + "/r/anything/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/" {
		t.Fatalf("location = %q, want /", loc)
	}
}

func TestReadWhenEmpty(t *testing.T) {
	srv := newTestServer(t, config{publishToken: "secret"})
	resp, err := http.Get(srv.URL + "/latest.json")
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

func TestPublishHistoryAndRead(t *testing.T) {
	srv := newTestServer(t, config{publishToken: "secret"})
	body, err := json.Marshal(map[string]any{
		"version":     1,
		"windowStart": "2026-11-09T13:00:00Z",
		"windowEnd":   "2026-11-10T13:00:00Z",
		"generatedAt": "2026-11-10T13:00:05Z",
		"words": []map[string]any{
			{"word": "Focus", "translation": "专注", "phonetic": "/ˈfəʊkəs/"},
		},
		"history": []map[string]any{
			{
				"windowStart": "2026-11-08T13:00:00Z",
				"windowEnd":   "2026-11-09T13:00:00Z",
				"generatedAt": "2026-11-10T13:00:05Z",
				"words": []map[string]any{
					{"word": "Past", "translation": "过去", "phonetic": ""},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
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

	history, err := http.Get(srv.URL + "/history.json")
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	defer history.Body.Close()
	if history.StatusCode != http.StatusOK {
		t.Fatalf("history status = %d", history.StatusCode)
	}
	var got struct {
		Version int      `json:"version"`
		Windows []window `json:"windows"`
	}
	if err := json.NewDecoder(history.Body).Decode(&got); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if got.Version != 1 || len(got.Windows) != 1 {
		t.Fatalf("unexpected history: %+v", got)
	}
	if len(got.Windows[0].Words) != 1 || got.Windows[0].Words[0].Word != "Past" {
		t.Fatalf("unexpected history window: %+v", got.Windows[0])
	}
}

func TestHistoryFallsBackToLatestWindow(t *testing.T) {
	srv := newTestServer(t, config{publishToken: "secret"})
	body := samplePayload(t)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/publish", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	resp.Body.Close()

	history, err := http.Get(srv.URL + "/history.json")
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	defer history.Body.Close()
	var got struct {
		Windows []window `json:"windows"`
	}
	if err := json.NewDecoder(history.Body).Decode(&got); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(got.Windows) != 1 || got.Windows[0].WindowEnd != "2026-11-10T13:00:00Z" {
		t.Fatalf("unexpected fallback history: %+v", got)
	}
	if len(got.Windows[0].Words) != 1 || got.Windows[0].Words[0].Word != "Focus" {
		t.Fatalf("unexpected fallback words: %+v", got.Windows[0].Words)
	}
}

func TestPublishRejectsInvalidHistory(t *testing.T) {
	srv := newTestServer(t, config{publishToken: "secret"})
	body, err := json.Marshal(map[string]any{
		"version":     1,
		"windowStart": "2026-11-09T13:00:00Z",
		"windowEnd":   "2026-11-10T13:00:00Z",
		"generatedAt": "2026-11-10T13:00:05Z",
		"words":       []map[string]any{},
		"history": []map[string]any{
			{"windowStart": "2026-11-08T13:00:00Z", "words": []map[string]any{}},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/publish", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPublishAuthAndValidation(t *testing.T) {
	srv := newTestServer(t, config{publishToken: "secret"})

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

	latest, err := http.Get(srv.URL + "/latest.json")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	latest.Body.Close()
	if latest.StatusCode != http.StatusOK {
		t.Fatalf("latest status = %d, want 200 (read stays public)", latest.StatusCode)
	}

	page, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	page.Body.Close()
	if page.StatusCode != http.StatusOK {
		t.Fatalf("page status = %d, want 200", page.StatusCode)
	}
}

func TestHealthAndAssets(t *testing.T) {
	srv := newTestServer(t, config{publishToken: "secret"})

	for _, path := range []string{"/health", "/", "/manifest.webmanifest", "/sw.js", "/history.json"} {
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
