// mPackStation 本地服务：开发期先看门（/api/health），业务端点逐页接入。
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"mpackstation/internal/store"
)

var version = "dev"

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18871", "listen address")
	dataDir := flag.String("data", "../../data", "data directory (db, cache, jars)")
	flag.Parse()
	if v := os.Getenv("MPACK_DATA"); v != "" {
		*dataDir = v
	}

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}
	db, err := store.Open(filepath.Join(*dataDir, "mpackstation.db"))
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		dbOK := db.Ping() == nil
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"version": version,
			"db":      dbOK,
			"time":    time.Now().UnixMilli(),
		})
	})

	log.Printf("mpackstation server listening on http://%s (data: %s)", *addr, *dataDir)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
