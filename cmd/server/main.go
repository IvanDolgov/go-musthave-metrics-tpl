package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/audit"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/buildinfo"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/config"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/middleware"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/storage"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/storage/postgres"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	_ "net/http/pprof"
)

// Глобальные переменные для версии сборки
var (
	buildVersion string
	buildDate    string
	buildCommit  string
)

// run запускает приложение с переданной конфигурацией
func run(cfg models.Config) error {
	// Вывод информации о сборке
	buildinfo.Print()

	var store storage.Storage
	var dbStorage postgres.DatabaseStorage

	// ИНИЦИАЛИЗАЦИЯ АУДИТА
	auditSubject := audit.NewConcreteSubject()

	// Создаем корневой контекст
	ctx := context.Background()

	// ВЫБОР ХРАНИЛИЩА ПО ПРИОРИТЕТУ:
	if cfg.DatabaseDSN != "" {
		logger.Log.Info("Using PostgreSQL storage", zap.String("dsn", cfg.DatabaseDSN))
		pgStorage, err := postgres.NewPostgresStorage(ctx, cfg.DatabaseDSN)
		if err != nil {
			return fmt.Errorf("failed to initialize PostgreSQL storage: %w", err)
		}
		store = pgStorage
		dbStorage = pgStorage
		defer pgStorage.Close()
	}

	if store == nil && cfg.FileStoragePath != "" {
		logger.Log.Info("Using file storage", zap.String("path", cfg.FileStoragePath))
		fileStorage := storage.NewMemStorage()

		if cfg.Restore {
			if err := fileStorage.LoadFromFile(ctx, cfg.FileStoragePath); err != nil {
				logger.Log.Warn("Failed to load metrics from file",
					zap.String("file", cfg.FileStoragePath),
					zap.Error(err))
			} else {
				logger.Log.Info("Metrics loaded from file",
					zap.String("file", cfg.FileStoragePath))
			}
		}
		store = fileStorage
	}

	if store == nil {
		logger.Log.Info("Using in-memory storage (no database or file storage configured)")
		store = storage.NewMemStorage()
	}

	if cfg.AuditFile != "" {
		fileObserver := audit.NewFileObserver(cfg.AuditFile)
		auditSubject.Register(fileObserver)
		logger.Log.Info("File audit enabled", zap.String("file", cfg.AuditFile))
	}

	if cfg.AuditURL != "" {
		remoteObserver := audit.NewRemoteObserver(cfg.AuditURL)
		auditSubject.Register(remoteObserver)
		logger.Log.Info("Remote audit enabled", zap.String("url", cfg.AuditURL))
	}

	fullPathServer := buildServerAddress(cfg.Server, cfg.Port)

	// создаем роутер
	router := chi.NewRouter()

	// Middleware (порядок важен!)
	router.Use(middleware.WithLogging)
	router.Use(middleware.WithGzip)

	// Используем middleware из internal/middleware
	router.Use(middleware.DecryptionMiddleware(cfg.CryptoKey))

	router.Use(middleware.HashValidation(cfg.Key))
	router.Use(middleware.HashResponse(cfg.Key))
	router.Use(middleware.WithAudit(auditSubject))

	if cfg.StoreInterval == 0 {
		if memStorage, ok := store.(*storage.MemStorage); ok && cfg.FileStoragePath != "" {
			router.Use(middleware.WithSyncSave(memStorage, cfg.FileStoragePath))
		}
	}

	// Ручки для метрик (эти функции должны быть определены в пакете handlers)
	router.Get(`/`, summaryMetrics(store))
	router.Post("/update/{type_metric}//{value_metric}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Metric name cannot be empty", http.StatusNotFound)
	})
	router.Post(`/update/{type_metric}/{metric}/{value_metric}`, getMetrics(store))
	router.Post(`/update/{type_metric}/{metric}/{value_metric}/`, getMetrics(store))
	router.Post(`/updates`, updateMetricsBatch(store))
	router.Post(`/updates/`, updateMetricsBatch(store))
	router.Post(`/update`, getJSONMetric(store))
	router.Post(`/update/`, getJSONMetric(store))
	router.Post(`/value`, sendJSONMetric(store))
	router.Post(`/value/`, sendJSONMetric(store))
	router.Get(`/value/{type_metric}/{metric}`, sendMetrics(store))
	router.Get(`/value/{type_metric}/{metric}/`, sendMetrics(store))
	router.Get(`/ping`, checkConnectDatabase(dbStorage))
	router.Get(`/ping/`, checkConnectDatabase(dbStorage))

	// Регистрируем pprof handlers
	router.Mount("/debug/pprof", http.DefaultServeMux)

	// HTTP сервер
	server := &http.Server{
		Addr:         fullPathServer,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	serverErr := make(chan error, 1)

	// Запускаем сервер
	go func() {
		storageType := "memory"
		if dbStorage != nil {
			storageType = "postgres"
		} else if cfg.FileStoragePath != "" {
			storageType = "file"
		}

		logger.Log.Info("Starting server",
			zap.String("address", fullPathServer),
			zap.String("storage_type", storageType),
			zap.Int64("store_interval", cfg.StoreInterval),
			zap.String("file_storage_path", cfg.FileStoragePath),
			zap.Bool("restore", cfg.Restore),
			zap.Bool("database_enabled", dbStorage != nil),
			zap.Bool("encryption_enabled", cfg.CryptoKey != ""),
		)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Периодическое сохранение (только для file storage)
	if cfg.StoreInterval > 0 && cfg.FileStoragePath != "" {
		if memStorage, ok := store.(*storage.MemStorage); ok {
			go func() {
				ticker := time.NewTicker(time.Duration(cfg.StoreInterval) * time.Second)
				defer ticker.Stop()

				for {
					select {
					case <-ticker.C:
						if err := memStorage.SaveToFile(ctx, cfg.FileStoragePath); err != nil {
							logger.Log.Error("Failed to save metrics to file",
								zap.String("file", cfg.FileStoragePath),
								zap.Error(err),
							)
						} else {
							logger.Log.Debug("Metrics saved to file",
								zap.String("file", cfg.FileStoragePath),
							)
						}
					case <-ctx.Done():
						return
					}
				}
			}()
		}
	}

	// Канал для сигналов ОС
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	select {
	case sig := <-sigChan:
		logger.Log.Info("Received signal, initiating graceful shutdown",
			zap.String("signal", sig.String()))

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		logger.Log.Info("Shutting down HTTP server...")
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Log.Error("HTTP server shutdown error", zap.Error(err))
		}

		if memStorage, ok := store.(*storage.MemStorage); ok && cfg.FileStoragePath != "" {
			logger.Log.Info("Saving metrics before shutdown")
			saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer saveCancel()

			if err := memStorage.SaveToFile(saveCtx, cfg.FileStoragePath); err != nil {
				logger.Log.Error("Failed to save metrics before shutdown", zap.Error(err))
			} else {
				logger.Log.Info("Metrics saved successfully before shutdown")
			}
		}

		logger.Log.Info("Server stopped gracefully")

	case err := <-serverErr:
		logger.Log.Error("Server error", zap.Error(err))
		return err
	}

	return nil
}

func main() {
	cfg := config.ParseServerFlags()

	if err := logger.Initialize("info"); err != nil {
		fmt.Fprintf(os.Stderr, "Logger initialization error: %v\n", err)
		os.Exit(1)
	}
	defer logger.Log.Sync()

	if err := run(cfg); err != nil {
		logger.Log.Error("Application error", zap.Error(err))
		fmt.Fprintf(os.Stderr, "Application error: %v\n", err)
		os.Exit(1)
	}
}

func buildServerAddress(server, port string) string {
	if strings.TrimSpace(server) == "" {
		return ":" + port
	}
	return server + ":" + port
}
