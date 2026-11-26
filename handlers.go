package main

import (
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

//go:embed web/index.html
var indexHTML []byte

//go:embed web/manifest.webmanifest
var manifestJSON []byte

//go:embed web/sw.js
var swJS []byte

type config struct {
	publishToken string
}

type payload struct {
	Version     int      `json:"version"`
	WindowStart string   `json:"windowStart"`
	WindowEnd   string   `json:"windowEnd"`
	GeneratedAt string   `json:"generatedAt"`
	Words       []word   `json:"words"`
	History     []window `json:"history,omitempty"`
}

type window struct {
	WindowStart string `json:"windowStart"`
	WindowEnd   string `json:"windowEnd"`
	GeneratedAt string `json:"generatedAt"`
	Words       []word `json:"words"`
}

type word struct {
	Word        string `json:"word"`
	Translation string `json:"translation"`
	Phonetic    string `json:"phonetic"`
}

func newMux(cfg config, st *store) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/publish", func(w http.ResponseWriter, r *http.Request) {
		handlePublish(cfg, st, w, r)
	})
	mux.HandleFunc("/latest.json", func(w http.ResponseWriter, r *http.Request) {
		handleLatest(st, w, r)
	})
	mux.HandleFunc("/history.json", func(w http.ResponseWriter, r *http.Request) {
		handleHistory(st, w, r)
	})
	mux.HandleFunc("/manifest.webmanifest", func(w http.ResponseWriter, r *http.Request) {
		handleAsset(w, r, "application/manifest+json; charset=utf-8", false, manifestJSON)
	})
	mux.HandleFunc("/sw.js", func(w http.ResponseWriter, r *http.Request) {
		handleAsset(w, r, "application/javascript; charset=utf-8", true, swJS)
	})
	mux.HandleFunc("/r/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/", handleIndex)
	return mux
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(indexHTML)
}

func handleLatest(st *store, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if data, ok := st.latestPayload(); ok {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(data)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": 1, "words": []any{}})
}

func handleHistory(st *store, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")

	windows := []window{}
	if data, ok := st.latestPayload(); ok {
		var p payload
		if err := json.Unmarshal(data, &p); err != nil {
			http.Error(w, "invalid stored payload", http.StatusInternalServerError)
			return
		}
		if len(p.History) > 0 {
			windows = p.History
		} else {
			windows = append(windows, window{
				WindowStart: p.WindowStart,
				WindowEnd:   p.WindowEnd,
				GeneratedAt: p.GeneratedAt,
				Words:       p.Words,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": 1, "windows": windows})
}

func handleAsset(w http.ResponseWriter, r *http.Request, contentType string, noStore bool, data []byte) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", contentType)
	if noStore {
		w.Header().Set("Cache-Control", "no-store")
	}
	_, _ = w.Write(data)
}

func handlePublish(cfg config, st *store, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if cfg.publishToken == "" {
		http.Error(w, "not configured", http.StatusServiceUnavailable)
		return
	}
	if !bearerMatches(r.Header.Get("Authorization"), cfg.publishToken) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if err != nil {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}
	if !json.Valid(body) {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	var p payload
	if err := json.Unmarshal(body, &p); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	if !validPayload(&p) {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if err := st.put(body, p.WindowEnd); err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "words": len(p.Words)})
}

func validPayload(p *payload) bool {
	if p.Version != 1 || p.WindowStart == "" || p.WindowEnd == "" {
		return false
	}
	if len(p.Words) > 5000 {
		return false
	}
	total := len(p.Words)
	for _, w := range p.Words {
		if w.Word == "" {
			return false
		}
	}
	if len(p.History) > 200 {
		return false
	}
	for _, win := range p.History {
		if win.WindowStart == "" || win.WindowEnd == "" {
			return false
		}
		if len(win.Words) > 5000 {
			return false
		}
		total += len(win.Words)
		for _, w := range win.Words {
			if w.Word == "" {
				return false
			}
		}
	}
	return total <= 20000
}

func bearerMatches(header, want string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	return constantTimeEqual(strings.TrimPrefix(header, prefix), want)
}

func constantTimeEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
