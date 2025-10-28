package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type Metrics struct {
	ID    string   `json:"id"`              // имя метрики
	MType string   `json:"type"`            // параметр, принимающий значение gauge или counter
	Delta *int64   `json:"delta,omitempty"` // значение метрики в случае передачи counter
	Value *float64 `json:"value,omitempty"` // значение метрики в случае передачи gauge
}

// run запускает приложение с переданной конфигурацией
func run(cfg Config) error {

	// создаем строку с сервером или без
	fullPathServer := buildServerAddress(cfg.Server, cfg.Port)

	// создаем хранилище
	storage := NewMemStorage()

	// создаем роутер
	router := chi.NewRouter()

	// Добавляем middleware логирования для всех маршрутов
	router.Use(withLogging)

	// список ручек
	router.Get(`/`, summaryMetrics(storage))
	router.Post("/update/{type_metric}//{value_metric}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Metric name cannot be empty", http.StatusNotFound)
	})
	router.Post(`/update/{type_metric}/{metric}/{value_metric}`, getMetrics(storage))
	router.Post(`/update`, getJSONMetric(storage))
	router.Post(`/update/`, getJSONMetric(storage))
	router.Post(`/value`, sendJSONMetric(storage))
	router.Post(`/value/`, sendJSONMetric(storage))
	router.Get(`/value/{type_metric}/{metric}`, sendMetrics(storage))

	// логируем запуск сервера
	logger.Log.Info("Starting server", zap.String("address", fullPathServer))

	err := http.ListenAndServe(fullPathServer, router)
	if err != nil {
		logger.Log.Error("Server error", zap.Error(err))
		panic(err)
	}

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
