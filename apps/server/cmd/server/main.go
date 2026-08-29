// mPackStation 本地服务入口：只做进程装配（参数、数据目录、单实例锁、
// 数据库、HTTP server、优雅退出），路由与中间件在 internal/httpapi。
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"mpackstation/internal/httpapi"
	"mpackstation/internal/instlock"
	"mpackstation/internal/service"
	"mpackstation/internal/store"
	"mpackstation/internal/task"
)

var version = "dev"

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
	if err != nil { log.Fatalf("open task queue: %v", err) }
	workerService := service.NewP7Service(db)
	if err := workerService.RegisterTaskHandlersOnQueue(queue); err != nil { log.Fatalf("register task handlers: %v", err) }
	if _, err := queue.Recover(context.Background()); err != nil { log.Fatalf("recover tasks: %v", err) }
	worker := task.NewWorker(queue, "server-worker")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		if err := worker.Run(ctx); err != nil && ctx.Err() == nil { log.Printf("task worker stopped: %v", err) }
	}()

	log.Printf("mpackstation server listening on http://%s (data: %s)", *addr, *dataDir)
	server := &http.Server{
		Addr: *addr, Handler: httpapi.NewRouter(db, version),
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
