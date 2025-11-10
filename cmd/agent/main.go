package main

import (
	"bytes"
	"compress/gzip"
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

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/agent"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/config"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"go.uber.org/zap"
)

// Структура для хранения текущих метрик (старый режим)
type CurrentMetrics struct {
	MemStats    runtime.MemStats
	PollCount   int64
	RandomValue float64
	mu          sync.RWMutex
}

// Collector для сбора метрик (новый режим)
type Collector struct {
	metrics map[string]models.Metrics
	mu      sync.RWMutex
}

// NewCollector создает новый коллектор
func NewCollector() *Collector {
	return &Collector{
		metrics: make(map[string]models.Metrics),
	}
}

func (c *Collector) UpdateMetrics() {
	c.mu.Lock()
	defer c.mu.Unlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Обновляем gauge метрики
	c.updateGauge("Alloc", float64(memStats.Alloc))
	c.updateGauge("BuckHashSys", float64(memStats.BuckHashSys))
	c.updateGauge("Frees", float64(memStats.Frees))
	c.updateGauge("GCCPUFraction", memStats.GCCPUFraction)
	c.updateGauge("GCSys", float64(memStats.GCSys))
	c.updateGauge("HeapAlloc", float64(memStats.HeapAlloc))
	c.updateGauge("HeapIdle", float64(memStats.HeapIdle))
	c.updateGauge("HeapInuse", float64(memStats.HeapInuse))
	c.updateGauge("HeapObjects", float64(memStats.HeapObjects))
	c.updateGauge("HeapReleased", float64(memStats.HeapReleased))
	c.updateGauge("HeapSys", float64(memStats.HeapSys))
	c.updateGauge("LastGC", float64(memStats.LastGC))
	c.updateGauge("Lookups", float64(memStats.Lookups))
	c.updateGauge("MCacheInuse", float64(memStats.MCacheInuse))
	c.updateGauge("MCacheSys", float64(memStats.MCacheSys))
	c.updateGauge("MSpanInuse", float64(memStats.MSpanInuse))
	c.updateGauge("MSpanSys", float64(memStats.MSpanSys))
	c.updateGauge("Mallocs", float64(memStats.Mallocs))
	c.updateGauge("NextGC", float64(memStats.NextGC))
	c.updateGauge("NumForcedGC", float64(memStats.NumForcedGC))
	c.updateGauge("NumGC", float64(memStats.NumGC))
	c.updateGauge("OtherSys", float64(memStats.OtherSys))
	c.updateGauge("PauseTotalNs", float64(memStats.PauseTotalNs))
	c.updateGauge("StackInuse", float64(memStats.StackInuse))
	c.updateGauge("StackSys", float64(memStats.StackSys))
	c.updateGauge("Sys", float64(memStats.Sys))
	c.updateGauge("TotalAlloc", float64(memStats.TotalAlloc))
	c.updateGauge("RandomValue", rand.Float64()) // случайное значение

	// Обновляем counter метрики
	c.updateCounter("PollCount", 1)
}

func (c *Collector) updateGauge(name string, value float64) {
	metric := models.Metrics{
		ID:    name,
		MType: "gauge",
		Value: &value,
	}
	c.metrics["gauge_"+name] = metric
}

func (c *Collector) updateCounter(name string, delta int64) {
	if existing, exists := c.metrics["counter_"+name]; exists {
		if existing.Delta != nil {
			newDelta := *existing.Delta + delta
			existing.Delta = &newDelta
			c.metrics["counter_"+name] = existing
		}
	} else {
		metric := models.Metrics{
			ID:    name,
			MType: "counter",
			Delta: &delta,
		}
		c.metrics["counter_"+name] = metric
	}
}

func (c *Collector) GetAllMetrics() []models.Metrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	metrics := make([]models.Metrics, 0, len(c.metrics))
	for _, metric := range c.metrics {
		metrics = append(metrics, metric)
	}
	return metrics
}

// run запускает приложение в режиме совместимости (старый метод)
func run(cfg models.Config) error {
	// Канал для сигналов завершения
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	logger.Log.Info("Программа запущена. Нажмите Ctrl+C для остановки")

	// Текущие метрики
	currentMetrics := &CurrentMetrics{}

	// ГОРУТИНА СБОРА МЕТРИК (с интервалом PollInterval)
	go func() {
		pollTicker := time.NewTicker(cfg.PollInterval)
		defer pollTicker.Stop()

		for range pollTicker.C {
			currentMetrics.mu.Lock()
			currentMetrics.PollCount++
			currentMetrics.RandomValue = rand.Float64() * 100
			runtime.ReadMemStats(&currentMetrics.MemStats)
			currentMetrics.mu.Unlock()

			logger.Log.Debug("Collected metrics batch", zap.Int64("batch_number", currentMetrics.PollCount))
		}
	}()

	// ГОРУТИНА ОТПРАВКИ МЕТРИК (с интервалом ReportInterval)
	go func() {
		reportTicker := time.NewTicker(cfg.ReportInterval)
		defer reportTicker.Stop()

		for range reportTicker.C {
			currentMetrics.mu.RLock()
			pollCount := currentMetrics.PollCount
			randomValue := currentMetrics.RandomValue
			memStats := currentMetrics.MemStats
			currentMetrics.mu.RUnlock()

			logger.Log.Info("Sending metrics",
				zap.Int64("batch_number", pollCount),
				zap.String("address", cfg.Address),
			)

			// Отправляем метрики через старый метод для обратной совместимости
			if err := sendRuntimeMetrics(cfg, pollCount, randomValue, memStats); err != nil {
				logger.Log.Error("Error sending metrics", zap.Error(err))
			}
		}
	}()

	// Ждем сигнал завершения
	<-stop
	logger.Log.Info("Завершение программы...")
	return nil
}

// runBatchMode запускает агент в режиме батчевой отправки (новый режим)
func runBatchMode(cfg models.Config) error {
	// Канал для сигналов завершения
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	logger.Log.Info("Программа запущена в batch режиме. Нажмите Ctrl+C для остановки")

	// Создаем batch sender
	fullPathServer := buildServerAddress(cfg.Server, cfg.Port)
	batchSender := agent.NewBatchSender("http://"+fullPathServer, 10) // batch size = 10

	// Создаем коллектор метрик
	collector := NewCollector()

	// ГОРУТИНА СБОРА МЕТРИК (с интервалом PollInterval)
	go func() {
		pollTicker := time.NewTicker(cfg.PollInterval)
		defer pollTicker.Stop()

		for range pollTicker.C {
			collector.UpdateMetrics()
			logger.Log.Debug("Collected metrics")
		}
	}()

	// ГОРУТИНА ОТПРАВКИ МЕТРИК (с интервалом ReportInterval)
	go func() {
		reportTicker := time.NewTicker(cfg.ReportInterval)
		defer reportTicker.Stop()

		for range reportTicker.C {
			metrics := collector.GetAllMetrics()

			if len(metrics) == 0 {
				logger.Log.Warn("No metrics to send")
				continue
			}

			logger.Log.Info("Sending metrics batch",
				zap.Int("metrics_count", len(metrics)))

			// Отправляем все метрики батчами
			for _, metric := range metrics {
				batchSender.AddMetric(metric)
			}

			// Принудительно отправляем оставшиеся метрики
			batchSender.Flush()
		}
	}()

	// Ждем сигнал завершения
	<-stop
	logger.Log.Info("Завершение программы...")

	// Перед выходом принудительно отправляем оставшиеся метрики
	batchSender.Flush()

	return nil
}

// sendRuntimeMetrics отправляет метрики через старый метод (для обратной совместимости)
func sendRuntimeMetrics(cfg models.Config, pollCount int64, randomValue float64, memStats runtime.MemStats) error {
	// Отправляем ВСЕ необходимые метрики из автотеста
	metricsToSend := map[string]float64{
		// Runtime метрики из memStats
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

		// Кастомные метрики
		"RandomValue": randomValue,
	}

	// Собираем ошибки отправки
	var errors []string

	// Отправляем gauge метрики
	for name, value := range metricsToSend {
		if err := sendMetric("gauge", name, value, cfg); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", name, err))
		}
	}

	// Отправляем counter метрику
	if err := sendMetric("counter", "PollCount", pollCount, cfg); err != nil {
		errors = append(errors, fmt.Sprintf("PollCount: %v", err))
	}

	// Если были ошибки, возвращаем их
	if len(errors) > 0 {
		return fmt.Errorf("failed to send metrics: %s", strings.Join(errors, "; "))
	}

	return nil
}

// sendMetric отправляет одну метрику через старый эндпоинт /update
func sendMetric(metricType string, name string, value interface{}, cfg models.Config) error {
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
	compressedData, err := compressGzip(jsonData)
	if err != nil {
		return fmt.Errorf("gzip compress error: %w", err)
	}

	// Создаем клиент с таймаутом
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Создаем запрос
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(compressedData))
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

// compressGzip сжимает данные с помощью gzip
func compressGzip(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)

	if _, err := gz.Write(data); err != nil {
		return nil, err
	}

	if err := gz.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
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
		// Если логер не инициализировался, используем fmt для ошибки
		fmt.Fprintf(os.Stderr, "Logger initialization error: %v\n", err)
		os.Exit(1)
	}
	defer logger.Log.Sync()

	// Выбираем режим работы
	if cfg.BatchMode {
		logger.Log.Info("Starting agent in BATCH mode")
		if err := runBatchMode(cfg); err != nil {
			logger.Log.Error("Application error", zap.Error(err))
			os.Exit(1)
		}
	} else {
		logger.Log.Info("Starting agent in COMPATIBILITY mode")
		if err := run(cfg); err != nil {
			logger.Log.Error("Application error", zap.Error(err))
			os.Exit(1)
		}
	}
}
