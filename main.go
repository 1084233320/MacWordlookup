package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	st, err := newStore(envOr("DATA_DIR", "./data"))
	if err != nil {
		log.Fatalf("init store: %v", err)
	}

	cfg := config{
		publishToken: os.Getenv("PUBLISH_TOKEN"),
		readKey:      os.Getenv("READ_KEY"),
	}
	if cfg.publishToken == "" || cfg.readKey == "" {
		log.Printf("WARN: PUBLISH_TOKEN / READ_KEY 未设置，服务处于未配置状态")
	}

	port := envOr("PORT", "8080")
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           newMux(cfg, st),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("listening on :%s", port)
	log.Fatal(srv.ListenAndServe())
}
