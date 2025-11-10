package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/agent"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/config"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"go.uber.org/zap"
)

// Collector для сбора метрик
type Collector struct {
	metrics map[string]models.Metrics
	mu      sync.RWMutex
}

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

func run(cfg models.Config) error {
	// Канал для сигналов завершения
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	logger.Log.Info("Программа запущена. Нажмите Ctrl+C для остановки")

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

func buildServerAddress(server, port string) string {
	if server == "" {
		return "localhost:" + port
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

	// Запускаем приложение
	if err := run(cfg); err != nil {
		logger.Log.Error("Application error", zap.Error(err))
		os.Exit(1)
	}
}
