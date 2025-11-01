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

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/compress"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// run запускает приложение с переданной конфигурацией
func run(cfg Config) error {
	// создаем хранилище
	storage := NewMemStorage()

	// Загружаем метрики из файла при старте, если указано в конфиге
	if cfg.Restore {
		if err := storage.LoadFromFile(cfg.FileStoragePath); err != nil {
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
	router.Use(withLogging)

	// Добавляем middleware сжатия gzip для всех маршрутов
	router.Use(withGzip)

	// Добавляем middleware для синхронного сохранения если StoreInterval = 0
	if cfg.StoreInterval == 0 {
		router.Use(storage.withSyncSave(cfg.FileStoragePath))
	}

	// список ручек (используем обычные обработчики)
	router.Get(`/`, summaryMetrics(storage))
	router.Post("/update/{type_metric}//{value_metric}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Metric name cannot be empty", http.StatusNotFound)
	})
	router.Post(`/update/{type_metric}/{metric}/{value_metric}`, getMetrics(storage))
	router.Post(`/update/{type_metric}/{metric}/{value_metric}/`, getMetrics(storage))

	router.Post(`/update`, getJSONMetric(storage))
	router.Post(`/update/`, getJSONMetric(storage))

	router.Post(`/value`, sendJSONMetric(storage))
	router.Post(`/value/`, sendJSONMetric(storage))

	router.Get(`/value/{type_metric}/{metric}`, sendMetrics(storage))
	router.Get(`/value/{type_metric}/{metric}/`, sendMetrics(storage))

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
	var ticker *time.Ticker
	if cfg.StoreInterval > 0 {
		ticker = time.NewTicker(time.Duration(cfg.StoreInterval) * time.Second)
		defer ticker.Stop()

		go func() {
			for {
				select {
				case <-ticker.C:
					if err := storage.SaveToFile(cfg.FileStoragePath); err != nil {
						logger.Log.Error("Failed to save metrics to file", zap.String("file", cfg.FileStoragePath), zap.Error(err))
					} else {
						logger.Log.Debug("Metrics saved to file", zap.String("file", cfg.FileStoragePath))
					}
				case <-sigChan:
					// Останавливаем тикер при получении сигнала
					ticker.Stop()
					return
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
		if err := storage.SaveToFile(cfg.FileStoragePath); err != nil {
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
	cfg := parseFlags()

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

// withLogging добавляет логирование для всех запросов и ответов
func withLogging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// засекаем время начала обработки запроса
		start := time.Now()

		// Создаем кастомный ResponseWriter для перехвата статуса и размера ответа
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Обслуживаем запрос
		h.ServeHTTP(wrapped, r)

		// Вычисляем продолжительность выполнения
		duration := time.Since(start)

		// Логируем сведения о запросе и ответе на уровне Info
		logger.Log.Info("HTTP request processed",
			zap.String("uri", r.RequestURI),
			zap.String("method", r.Method),
			zap.String("remote_addr", r.RemoteAddr),
			zap.Int("status_code", wrapped.statusCode),
			zap.Int("response_size", wrapped.responseSize),
			zap.Duration("duration", duration),
		)
	})
}

// withGzip добавляет сжатие
func withGzip(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ow := w

		// Всегда распаковываем входящие сжатые данные
		if r.Header.Get("Content-Encoding") == "gzip" {
			cr, err := compress.NewCompressReader(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			r.Body = cr
			defer cr.Close()
		}

		// Всегда сжимаем исходящие данные если клиент поддерживает
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			cw := compress.NewCompressWriter(w)
			ow = cw
			defer cw.Close()
		}

		h.ServeHTTP(ow, r)
	})
}

// responseWriter обертка для http.ResponseWriter для перехвата статуса и размера ответа
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	responseSize int
	wroteHeader  bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.statusCode = code
		rw.ResponseWriter.WriteHeader(code)
		rw.wroteHeader = true
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	size, err := rw.ResponseWriter.Write(b)
	rw.responseSize += size
	return size, err
}

// Status возвращает статус код ответа
func (rw *responseWriter) Status() int {
	return rw.statusCode
}

// Size возвращает размер ответа
func (rw *responseWriter) Size() int {
	return rw.responseSize
}
