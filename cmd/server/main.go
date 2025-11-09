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
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/storage/database"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// run запускает приложение с переданной конфигурацией
func run(cfg models.Config) error {
	var store storage.Storage

	// Выбираем тип хранилища в порядке приоритета:
	// 1. PostgreSQL (если указан DSN)
	// 2. File storage (если указан путь к файлу)
	// 3. In-memory storage (по умолчанию)

	if cfg.DatabaseDSN != "" {
		logger.Log.Info("Using PostgreSQL storage", zap.String("dsn", cfg.DatabaseDSN))
		pgStorage, err := database.NewDBStorage(cfg.DatabaseDSN)
		if err != nil {
			logger.Log.Error("Failed to initialize PostgreSQL storage, falling back to file storage",
				zap.Error(err))
			// Продолжаем с file storage
		} else {
			store = pgStorage
			defer pgStorage.Close()
		}
	}

	// Если PostgreSQL не инициализирован, проверяем file storage
	if store == nil && cfg.FileStoragePath != "" {
		logger.Log.Info("Using file storage", zap.String("path", cfg.FileStoragePath))
		fileStorage := storage.NewMemStorage()

		// Загружаем метрики из файла при старте, если указано в конфиге
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
	} else if store == nil {
		// Если ни PostgreSQL, ни file storage не указаны - используем memory storage
		logger.Log.Info("Using in-memory storage (no database or file storage configured)")
		store = storage.NewMemStorage()
	}

	// создаем строку с сервером или без
	fullPathServer := buildServerAddress(cfg.Server, cfg.Port)

	// создаем роутер
	router := chi.NewRouter()

	// Добавляем middleware логирования для всех маршрутов
	router.Use(middleware.WithLogging)

	// Добавляем middleware сжатия gzip для всех маршрутов
	router.Use(middleware.WithGzip)

	// Добавляем middleware для синхронного сохранения если StoreInterval = 0
	// Только для file storage (MemStorage)
	if cfg.StoreInterval == 0 {
		if memStorage, ok := store.(*storage.MemStorage); ok && cfg.FileStoragePath != "" {
			router.Use(middleware.WithSyncSave(memStorage, cfg.FileStoragePath))
		}
	}

	// Инициализируем database storage для проверки подключения к БД
	var dbStorage database.DatabaseStorage
	if cfg.DatabaseDSN != "" {
		dbStorage, _ = database.NewDBStorage(cfg.DatabaseDSN)
		if dbStorage != nil {
			defer dbStorage.Close()
		}
	}

	// список ручек
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

	router.Get(`/ping`, checkConnectDatabase(dbStorage))
	router.Get(`/ping/`, checkConnectDatabase(dbStorage))

	// Создаем HTTP сервер с таймаутами
	server := &http.Server{
		Addr:         fullPathServer,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Канал для получения ошибок от сервера
	serverErr := make(chan error, 1)

	// Запускаем сервер в отдельной горутине
	go func() {
		storageType := "memory"
		if cfg.DatabaseDSN != "" && dbStorage != nil {
			storageType = "postgres"
		} else if cfg.FileStoragePath != "" {
			storageType = "file"
		}
		// логируем запуск сервера
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

	// Канал для получения сигналов ОС
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	// ГОРУТИНА сохранения МЕТРИК (с интервалом StoreInterval) - только для file storage
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

	// Ожидаем сигнал завершения или ошибку сервера
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

	// Создаем контекст с таймаутом для graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Останавливаем сервер
	logger.Log.Info("Shutting down server...")
	if err := server.Shutdown(ctx); err != nil {
		logger.Log.Error("Server shutdown error", zap.Error(err))
		return err
	}

	logger.Log.Info("Server stopped gracefully")
	return nil
}

func main() {
	// Получаем конфигурацию
	cfg := config.ParseServerFlags()

	// Инициализируем логер с уровнем Info
	if err := logger.Initialize("info"); err != nil {
		fmt.Fprintf(os.Stderr, "Logger initialization error: %v\n", err)
		os.Exit(1)
	}
	defer logger.Log.Sync()

	// Запускаем приложение
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
