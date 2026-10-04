// mPackStation 本地服务入口：只做进程装配（参数、数据目录、单实例锁、
// 数据库、HTTP server、优雅退出），路由与中间件在 internal/httpapi。
package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"mpackstation/internal/config"
	"mpackstation/internal/httpapi"
	"mpackstation/internal/instlock"
	"mpackstation/internal/provider"
	"mpackstation/internal/service"
	"mpackstation/internal/store"
	"mpackstation/internal/task"
)

var version = "dev"

// providerRegistry assembles real provider adapters. Modrinth works without a
// token for public catalog reads; CurseForge is enabled when a key exists —
// the CURSEFORGE_API_KEY environment variable wins, otherwise the key saved
// from the settings page (secrets table) is used.
// (https://console.curseforge.com).
func providerRegistry(db *sql.DB) *provider.Registry {
	adapters := []provider.Adapter{}
	// Adapter paths already carry the API version prefix (/v2, /v1), so the
	// base URLs must be bare hosts.
	mr, err := provider.NewHTTPAdapter(provider.Modrinth, "https://api.modrinth.com", os.Getenv("MODRINTH_TOKEN"), nil)
	if err == nil {
		adapters = append(adapters, mr)
	}
	key := os.Getenv("CURSEFORGE_API_KEY")
	if key == "" && db != nil {
		if saved, err := store.NewRepository(db).GetSecret(context.Background(), "curseforge_api_key"); err == nil {
			key = saved
		} else {
			log.Printf("load saved curseforge key: %v", err)
		}
	}
	if key != "" {
		if cf, err := provider.NewHTTPAdapter(provider.CurseForge, "https://api.curseforge.com", key, nil); err == nil {
			adapters = append(adapters, cf)
		}
	}
	return provider.NewRegistry(adapters...)
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18871", "listen address")
	dataDir := flag.String("data", "../../data", "data directory (db, cache, jars)")
	configPath := flag.String("config", "", "config file path (default: the per-OS user config dir, see config.DefaultPath)")
	flag.Parse()

	// 默认配置文件放在用户目录的 OS 约定位置（Windows %APPDATA%\mPackStation、
	// macOS ~/Library/Application Support/mPackStation、Linux ~/.config/mpackstation，
	// 见 config.DefaultPath），不存在的文件/键一律忽略。优先级：
	// 显式 flag > MPACK_* 环境变量 > 配置文件 > 内置默认。
	path := *configPath
	if path == "" {
		path = config.DefaultPath()
	}
	cfg, err := config.Load(path)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	// 只有显式传的 flag 才覆盖配置文件 —— flag 变量自带默认值，直接覆盖会把
	// 配置文件里的设定静默冲掉。
	explicit := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	if explicit["addr"] {
		cfg.ListenAddr = *addr
	}
	if explicit["data"] {
		cfg.DataDir = *dataDir
	}
	if v := os.Getenv("MPACK_DATA"); v != "" {
		cfg.DataDir = v
	}
	if abs, err := filepath.Abs(cfg.DataDir); err == nil {
		cfg.DataDir = abs
	} else {
		log.Fatalf("resolve data directory: %v", err)
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	// 单实例锁：避免两个进程同时写同一个 SQLite 数据库。
	lock, err := instlock.Acquire(cfg.DataDir)
	if err != nil {
		log.Fatalf("acquire instance lock: %v", err)
	}
	defer func() {
		if err := lock.Release(); err != nil {
			log.Printf("release instance lock: %v", err)
		}
	}()

	db, err := store.Open(filepath.Join(cfg.DataDir, "mpackstation.db"))
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

	registry := providerRegistry(db)
	// 启动后探测一次双平台可达性并写入 settings, 让状态卡片反映真实结果
	// 而不是永远停在"未探测"。尽力而为, 失败不影响启动。
	go func() {
		probe := service.New(db)
		probe.SetProviderRegistry(registry)
		pctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		probe.ProbeProviderStatus(pctx)
	}()

	log.Printf("mpackstation server listening on http://%s (data: %s)", cfg.ListenAddr, cfg.DataDir)
	server := &http.Server{
		Addr: cfg.ListenAddr, Handler: httpapi.NewRouterWithProviders(db, version, registry, queue),
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
