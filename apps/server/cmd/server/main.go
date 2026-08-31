// mPackStation 本地服务入口：只做进程装配（参数、数据目录、单实例锁、
// 数据库、HTTP server、优雅退出），路由与中间件在 internal/httpapi。
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"mpackstation/internal/httpapi"
	"mpackstation/internal/instlock"
	"mpackstation/internal/provider"
	"mpackstation/internal/service"
	"mpackstation/internal/store"
	"mpackstation/internal/task"
)

var version = "dev"

// resolveWriteToken implements auth.md: MPACK_TOKEN takes precedence;
// otherwise a random high-entropy token is generated on first start and
// persisted to <data>/runtime-token (0600) for reuse on later starts. No
// hardcoded fallback exists anywhere in the chain.
func resolveWriteToken(dataDir string) (string, error) {
	if v := strings.TrimSpace(os.Getenv("MPACK_TOKEN")); v != "" {
		return v, nil
	}
	path := filepath.Join(dataDir, "runtime-token")
	b, err := os.ReadFile(path)
	if err == nil {
		if t := strings.TrimSpace(string(b)); t != "" {
			return t, nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", err
	}
	return token, nil
}

// providerRegistry assembles real provider adapters. Modrinth works without a
// token for public catalog reads; CurseForge is enabled only when
// CURSEFORGE_API_KEY is set (https://console.curseforge.com).
func providerRegistry() *provider.Registry {
	adapters := []provider.Adapter{}
	// Adapter paths already carry the API version prefix (/v2, /v1), so the
	// base URLs must be bare hosts.
	mr, err := provider.NewHTTPAdapter(provider.Modrinth, "https://api.modrinth.com", os.Getenv("MODRINTH_TOKEN"), nil)
	if err == nil {
		adapters = append(adapters, mr)
	}
	if key := os.Getenv("CURSEFORGE_API_KEY"); key != "" {
		if cf, err := provider.NewHTTPAdapter(provider.CurseForge, "https://api.curseforge.com", key, nil); err == nil {
			adapters = append(adapters, cf)
		}
	}
	return provider.NewRegistry(adapters...)
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18871", "listen address")
	dataDir := flag.String("data", "../../data", "data directory (db, cache, jars)")
	flag.Parse()
	if v := os.Getenv("MPACK_DATA"); v != "" {
		*dataDir = v
	}
	if abs, err := filepath.Abs(*dataDir); err == nil {
		*dataDir = abs
	} else {
		log.Fatalf("resolve data directory: %v", err)
	}

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	// 单实例锁：避免两个进程同时写同一个 SQLite 数据库。
	lock, err := instlock.Acquire(*dataDir)
	if err != nil {
		log.Fatalf("acquire instance lock: %v", err)
	}
	defer func() {
		if err := lock.Release(); err != nil {
			log.Printf("release instance lock: %v", err)
		}
	}()

	db, err := store.Open(filepath.Join(*dataDir, "mpackstation.db"))
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	queue, err := task.NewQueue(db)
	if err != nil {
		log.Fatalf("open task queue: %v", err)
	}
	workerService := service.NewP7Service(db)
	if err := workerService.RegisterTaskHandlersOnQueue(queue); err != nil {
		log.Fatalf("register task handlers: %v", err)
	}
	importService := service.NewImportService(db)
	if err := importService.RegisterTaskHandlerOnQueue(queue); err != nil {
		log.Fatalf("register import handler: %v", err)
	}
	if _, err := queue.Recover(context.Background()); err != nil {
		log.Fatalf("recover tasks: %v", err)
	}
	worker := task.NewWorker(queue, "server-worker")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("task worker stopped: %v", err)
		}
	}()

	token, err := resolveWriteToken(*dataDir)
	if err != nil {
		log.Fatalf("resolve write token: %v", err)
	}

	log.Printf("mpackstation server listening on http://%s (data: %s)", *addr, *dataDir)
	server := &http.Server{
		Addr: *addr, Handler: httpapi.NewRouterWithProviders(db, version, token, providerRegistry(), queue),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 60 * time.Second, IdleTimeout: 120 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown: %v", err)
		}
	}()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
