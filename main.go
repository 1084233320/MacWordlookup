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

	cfg := config{publishToken: os.Getenv("PUBLISH_TOKEN")}
	if cfg.publishToken == "" {
		log.Printf("WARN: PUBLISH_TOKEN 未设置，发布接口返回 503；读取接口公开可用")
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
