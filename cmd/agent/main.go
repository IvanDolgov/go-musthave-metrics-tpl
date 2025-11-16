package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/compress"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/config"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/retry"
	"go.uber.org/zap"
)

// Структура для хранения текущих метрик
type CurrentMetrics struct {
	MemStats    runtime.MemStats
	PollCount   int64
	RandomValue float64
	mu          sync.RWMutex
}

// run запускает приложение с переданной конфигурацией
func run(ctx context.Context, cfg models.Config) error {
	// Канал для сигналов завершения
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	logger.Log.Info("Программа запущена. Нажмите Ctrl+C для остановки")

	// Текущие метрики
	currentMetrics := &CurrentMetrics{}

	// ГОРУТИНА СБОРА МЕТРИК (с интервалом PollInterval)
	go func() {
		pollTicker := time.NewTicker(time.Duration(cfg.PollInterval))
		defer pollTicker.Stop()

		for {
			select {
			case <-pollTicker.C:
				currentMetrics.mu.Lock()
				currentMetrics.PollCount++
				currentMetrics.RandomValue = rand.Float64() * 100
				runtime.ReadMemStats(&currentMetrics.MemStats)
				currentMetrics.mu.Unlock()

				logger.Log.Debug("Collected metrics batch", zap.Int64("batch_number", currentMetrics.PollCount))
			case <-ctx.Done():
				logger.Log.Info("Stopping metrics collection")
				return
			}
		}
	}()

	// ГОРУТИНА ОТПРАВКИ МЕТРИК (с интервалом ReportInterval)
	go func() {
		reportTicker := time.NewTicker(time.Duration(cfg.ReportInterval))
		defer reportTicker.Stop()

		for {
			select {
			case <-reportTicker.C:
				currentMetrics.mu.RLock()
				pollCount := currentMetrics.PollCount
				randomValue := currentMetrics.RandomValue
				memStats := currentMetrics.MemStats
				currentMetrics.mu.RUnlock()

				logger.Log.Info("Sending metrics batch",
					zap.Int64("batch_number", pollCount),
					zap.String("address", cfg.Address),
				)
				if err := sendRuntimeMetrics(ctx, cfg, pollCount, randomValue, memStats); err != nil {
					logger.Log.Error("Error sending metrics", zap.Error(err))
				}
			case <-ctx.Done():
				logger.Log.Info("Stopping metrics reporting")
				return
			}
		}
	}()

	// Ждем сигнал завершения или отмену контекста
	select {
	case <-stop:
		logger.Log.Info("Завершение программы по сигналу...")
	case <-ctx.Done():
		logger.Log.Info("Завершение программы по контексту...")
	}
	return nil
}

// sendMetricsBatch отправляет метрики батчем с повторными попытками
func sendMetricsBatch(ctx context.Context, cfg models.Config, metrics []models.Metrics) error {
	if len(metrics) == 0 {
		return nil // Не отправляем пустые батчи
	}

	operation := func() error {
		return sendMetricsBatchOnce(ctx, cfg, metrics)
	}

	// Для агента используем классификатор nil, так как он работает с HTTP, не с PostgreSQL
	err := retry.WithRetry(ctx, retry.DefaultRetryConfig, operation, nil)
	if err != nil {
		return fmt.Errorf("failed to send metrics batch after retries: %w", err)
	}

	return nil
}

// sendMetricsBatchOnce отправляет метрики батчем (одна попытка)
func sendMetricsBatchOnce(ctx context.Context, cfg models.Config, metrics []models.Metrics) error {
	startTime := time.Now()

	// Формируем полный адрес сервера
	fullPathServer := buildServerAddress(cfg.Server, cfg.Port)
	endpoint := fmt.Sprintf("http://%s/updates", fullPathServer)

	// Кодируем метрики в JSON
	jsonData, err := json.Marshal(metrics)
	if err != nil {
		logger.LogErrorWithContext(err, "Failed to marshal metrics to JSON")
		return fmt.Errorf("error encoding JSON: %w", err)
	}

	// Сжимаем данные
	compressedData, err := compress.GzipCompress(jsonData)
	if err != nil {
		logger.LogErrorWithContext(err, "Failed to compress metrics data")
		return fmt.Errorf("gzip compress error: %w", err)
	}

	// Создаем клиент с таймаутом
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Создаем запрос с контекстом
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(compressedData))
	if err != nil {
		logger.LogErrorWithContext(err, "Failed to create HTTP request")
		return fmt.Errorf("error creating request: %w", err)
	}

	// Устанавливаем заголовки
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")

	// Отправляем запрос
	response, err := client.Do(req)
	if err != nil {
		logger.LogNetworkError("send_metrics_batch", endpoint, err, 0)
		return fmt.Errorf("error sending metrics batch: %w", err)
	}
	defer response.Body.Close()

	// Проверяем статус ответа
	if response.StatusCode != http.StatusOK {
		err := fmt.Errorf("server returned non-OK status: %d", response.StatusCode)
		logger.LogNetworkError("send_metrics_batch", endpoint, err, response.StatusCode)
		return err
	}

	duration := time.Since(startTime)
	logger.LogBatchOperation("send_metrics_batch", len(metrics), duration, nil)

	return nil
}

func sendRuntimeMetrics(ctx context.Context, cfg models.Config, pollCount int64, randomValue float64, memStats runtime.MemStats) error {
	var metrics []models.Metrics

	// Runtime метрики из memStats
	runtimeMetrics := map[string]float64{
		"Alloc":         float64(memStats.Alloc),
		"BuckHashSys":   float64(memStats.BuckHashSys),
		"Frees":         float64(memStats.Frees),
		"GCCPUFraction": memStats.GCCPUFraction,
		"GCSys":         float64(memStats.GCSys),
		"HeapAlloc":     float64(memStats.HeapAlloc),
		"HeapIdle":      float64(memStats.HeapIdle),
		"HeapInuse":     float64(memStats.HeapInuse),
		"HeapObjects":   float64(memStats.HeapObjects),
		"HeapReleased":  float64(memStats.HeapReleased),
		"HeapSys":       float64(memStats.HeapSys),
		"LastGC":        float64(memStats.LastGC),
		"Lookups":       float64(memStats.Lookups),
		"MCacheInuse":   float64(memStats.MCacheInuse),
		"MCacheSys":     float64(memStats.MCacheSys),
		"MSpanInuse":    float64(memStats.MSpanInuse),
		"MSpanSys":      float64(memStats.MSpanSys),
		"Mallocs":       float64(memStats.Mallocs),
		"NextGC":        float64(memStats.NextGC),
		"NumForcedGC":   float64(memStats.NumForcedGC),
		"NumGC":         float64(memStats.NumGC),
		"OtherSys":      float64(memStats.OtherSys),
		"PauseTotalNs":  float64(memStats.PauseTotalNs),
		"StackInuse":    float64(memStats.StackInuse),
		"StackSys":      float64(memStats.StackSys),
		"Sys":           float64(memStats.Sys),
		"TotalAlloc":    float64(memStats.TotalAlloc),
		"RandomValue":   randomValue,
	}

	// Добавляем gauge метрики в батч
	for name, value := range runtimeMetrics {
		valueCopy := value
		metrics = append(metrics, models.Metrics{
			ID:    name,
			MType: "gauge",
			Value: &valueCopy,
		})
	}

	// Добавляем counter метрику в батч
	metrics = append(metrics, models.Metrics{
		ID:    "PollCount",
		MType: "counter",
		Delta: &pollCount,
	})

	// Отправляем батч
	if err := sendMetricsBatch(ctx, cfg, metrics); err != nil {
		return fmt.Errorf("failed to send metrics batch: %w", err)
	}

	return nil
}

func sendMetric(ctx context.Context, metricType string, name string, value interface{}, cfg models.Config) error {
	// Формируем полный адрес сервера
	fullPathServer := buildServerAddress(cfg.Server, cfg.Port)
	endpoint := fmt.Sprintf("http://%s/update", fullPathServer)

	// Создаем структуру для метрики с значением
	var metric models.Metrics

	// Заполняем метрику в зависимости от типа
	switch metricType {
	case "gauge":
		if floatValue, ok := value.(float64); ok {
			metric = models.Metrics{
				ID:    name,
				MType: metricType,
				Value: &floatValue,
			}
		} else {
			return fmt.Errorf("invalid gauge value type: %T", value)
		}
	case "counter":
		var intValue int64
		switch v := value.(type) {
		case int64:
			intValue = v
		case int:
			intValue = int64(v)
		default:
			return fmt.Errorf("invalid counter value type: %T", value)
		}
		metric = models.Metrics{
			ID:    name,
			MType: metricType,
			Delta: &intValue,
		}
	default:
		return fmt.Errorf("unknown metric type: %s", metricType)
	}

	// Кодируем метрику в JSON
	jsonData, err := json.Marshal(metric)
	if err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}

	// Сжимаем данные
	compressedData, err := compress.GzipCompress(jsonData)
	if err != nil {
		return fmt.Errorf("gzip compress error: %w", err)
	}

	// Создаем клиент с таймаутом
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Создаем запрос
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(compressedData))
	if err != nil {
		return fmt.Errorf("error creating request for %s: %w", name, err)
	}

	// Устанавливаем правильные заголовки для REQUEST
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")

	// Отправляем запрос
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending metric %s: %w", name, err)
	}
	defer response.Body.Close()

	// Проверяем статус ответа
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned non-OK status for %s: %d", name, response.StatusCode)
	}

	logger.Log.Debug("Successfully sent metric",
		zap.String("name", name),
		zap.Any("value", value),
	)
	return nil
}

func buildServerAddress(server, port string) string {
	if strings.TrimSpace(server) == "" {
		return ":" + port
	}
	return server + ":" + port
}

func main() {
	// Получаем конфигурацию
	cfg := config.ParseAgentFlags()

	// Инициализируем логер
	if err := logger.Initialize("info"); err != nil {
		fmt.Fprintf(os.Stderr, "Logger initialization error: %v\n", err)
		os.Exit(1)
	}
	defer logger.Log.Sync()

	// Создаем корневой контекст
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Запускаем приложение с контекстом
	if err := run(ctx, cfg); err != nil {
		logger.Log.Error("Application error", zap.Error(err))
		os.Exit(1)
	}
}
