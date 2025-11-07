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
	// создаем хранилище
	var store storage.Storage = storage.NewMemStorage()

	// Инициализируем database storage для проверки подключения к БД
	var dbStorage database.DatabaseStorage
	if cfg.DatabaseDSN != "" {
		var err error
		dbStorage, err = database.NewDBStorage(cfg.DatabaseDSN)
		if err != nil {
			logger.Log.Error("Failed to connect to database",
				zap.String("dsn", cfg.DatabaseDSN),
				zap.Error(err))
			// Можно продолжить работу без БД, если это допустимо
		} else {
			defer dbStorage.Close()
			logger.Log.Info("Database connection established")
		}
	} else {
		logger.Log.Info("Database DSN not provided, database features disabled")
	}

	// Загружаем метрики из файла при старте, если указано в конфиге
	if cfg.Restore {
		if err := store.LoadFromFile(cfg.FileStoragePath); err != nil {
			logger.Log.Warn("Failed to load metrics from file", zap.String("file", cfg.FileStoragePath), zap.Error(err))
		} else {
			logger.Log.Info("Metrics loaded from file", zap.String("file", cfg.FileStoragePath))
		}
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
	// Добавляем middleware для синхронного сохранения если StoreInterval = 0
	if cfg.StoreInterval == 0 {
		router.Use(middleware.WithSyncSave(store, cfg.FileStoragePath))
	}

	// список ручек (используем обычные обработчики)
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
		ReadTimeout:  10 * time.Second,  // время на чтение запроса
		WriteTimeout: 10 * time.Second,  // время на запись ответа
		IdleTimeout:  120 * time.Second, // время неактивного соединения
	}

	// Канал для получения ошибок от сервера
	serverErr := make(chan error, 1)

	// Запускаем сервер в отдельной горутине
	go func() {
		// логируем запуск сервера
		logger.Log.Info("Starting server",
			zap.String("address", fullPathServer),
			zap.Int64("store_interval", cfg.StoreInterval),
			zap.String("file_storage_path", cfg.FileStoragePath),
			zap.Bool("restore", cfg.Restore),
			zap.Bool("sync_mode", cfg.StoreInterval == 0),
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Канал для получения сигналов ОС
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	// ГОРУТИНА сохранения МЕТРИК (с интервалом StoreInterval) - только для асинхронного режима
	if cfg.StoreInterval > 0 {
		go func() {
			ticker := time.NewTicker(time.Duration(cfg.StoreInterval) * time.Second)
			defer ticker.Stop()

			for range ticker.C {
				if err := store.SaveToFile(cfg.FileStoragePath); err != nil {
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

	// Ожидаем сигнал завершения или ошибку сервера
	select {
	case sig := <-sigChan:
		logger.Log.Info("Received signal, shutting down gracefully", zap.String("signal", sig.String()))

		// Сохраняем метрики перед завершением
		logger.Log.Info("Saving metrics before shutdown")
		if err := store.SaveToFile(cfg.FileStoragePath); err != nil {
			logger.Log.Error("Failed to save metrics before shutdown", zap.Error(err))
		} else {
			logger.Log.Info("Metrics saved successfully before shutdown")
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
