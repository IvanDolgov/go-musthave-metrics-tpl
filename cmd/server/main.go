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

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/config"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/middleware"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/storage"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/storage/postgres"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// run запускает приложение с переданной конфигурацией
func run(cfg models.Config) error {
	var store storage.Storage
	var dbStorage postgres.DatabaseStorage

	// ВЫБОР ХРАНИЛИЩА ПО ПРИОРИТЕТУ:
	// 1. PostgreSQL (если указан DSN)
	// 2. File storage (если указан путь к файлу)
	// 3. In-memory storage (по умолчанию)

	if cfg.DatabaseDSN != "" {
		logger.Log.Info("Using PostgreSQL storage", zap.String("dsn", cfg.DatabaseDSN))
		pgStorage, err := postgres.NewPostgresStorage(cfg.DatabaseDSN)
		if err != nil {
			logger.Log.Error("Failed to initialize PostgreSQL storage, falling back to file storage",
				zap.Error(err))
		} else {
			store = pgStorage
			dbStorage = pgStorage
			defer pgStorage.Close()
		}
	}

	// Если PostgreSQL не инициализирован, проверяем file storage
	if store == nil && cfg.FileStoragePath != "" {
		logger.Log.Info("Using file storage", zap.String("path", cfg.FileStoragePath))
		fileStorage := storage.NewMemStorage()

		// Загружаем метрики из файла при старте
		if cfg.Restore {
			if err := fileStorage.LoadFromFile(cfg.FileStoragePath); err != nil {
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

	// Если ни PostgreSQL, ни file storage не указаны - используем memory storage
	if store == nil {
		logger.Log.Info("Using in-memory storage (no database or file storage configured)")
		store = storage.NewMemStorage()
	}

	// создаем строку с сервером
	fullPathServer := buildServerAddress(cfg.Server, cfg.Port)

	// создаем роутер
	router := chi.NewRouter()

	// Middleware
	router.Use(middleware.WithLogging)
	router.Use(middleware.WithGzip)

	// Middleware для синхронного сохранения (только для file storage)
	if cfg.StoreInterval == 0 {
		if memStorage, ok := store.(*storage.MemStorage); ok && cfg.FileStoragePath != "" {
			router.Use(middleware.WithSyncSave(memStorage, cfg.FileStoragePath))
		}
	}

	// Ручки для метрик
	router.Get(`/`, summaryMetrics(store))
	router.Post("/update/{type_metric}//{value_metric}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Metric name cannot be empty", http.StatusNotFound)
	})
	router.Post(`/update/{type_metric}/{metric}/{value_metric}`, getMetrics(store))
	router.Post(`/update/{type_metric}/{metric}/{value_metric}/`, getMetrics(store))

	router.Post(`/update`, getJSONMetric(store))
	router.Post(`/update/`, getJSONMetric(store))

	router.Post(`/value`, sendJSONMetric(store))
	router.Post(`/value/`, sendJSONMetric(store))

	router.Get(`/value/{type_metric}/{metric}`, sendMetrics(store))
	router.Get(`/value/{type_metric}/{metric}/`, sendMetrics(store))

	// Хендлер проверки подключения к БД
	router.Get(`/ping`, checkConnectDatabase(dbStorage))
	router.Get(`/ping/`, checkConnectDatabase(dbStorage))

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

				for range ticker.C {
					if err := memStorage.SaveToFile(cfg.FileStoragePath); err != nil {
						logger.Log.Error("Failed to save metrics to file",
							zap.String("file", cfg.FileStoragePath),
							zap.Error(err),
						)
					} else {
						logger.Log.Debug("Metrics saved to file",
							zap.String("file", cfg.FileStoragePath),
						)
					}
				}
			}()
		}
	}

	// Ожидаем сигналы завершения
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	select {
	case sig := <-sigChan:
		logger.Log.Info("Received signal, shutting down gracefully",
			zap.String("signal", sig.String()))

		// Сохраняем метрики перед завершением (только для file storage)
		if memStorage, ok := store.(*storage.MemStorage); ok && cfg.FileStoragePath != "" {
			logger.Log.Info("Saving metrics before shutdown")
			if err := memStorage.SaveToFile(cfg.FileStoragePath); err != nil {
				logger.Log.Error("Failed to save metrics before shutdown", zap.Error(err))
			} else {
				logger.Log.Info("Metrics saved successfully before shutdown")
			}
		}

	case err := <-serverErr:
		logger.Log.Error("Server error", zap.Error(err))
		return err
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Log.Info("Shutting down server...")
	if err := server.Shutdown(ctx); err != nil {
		logger.Log.Error("Server shutdown error", zap.Error(err))
		return err
	}

	logger.Log.Info("Server stopped gracefully")
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
		logger.Log.Info("Application error", zap.Error(err))
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
