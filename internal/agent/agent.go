package agent

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"go.uber.org/zap"
)

// MetricsAgent управляет сбором и отправкой метрик
type MetricsAgent struct {
	cfg         models.Config
	metricsChan chan []models.Metrics
	workerPool  chan struct{}
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	pollCount   int64
	mu          sync.RWMutex
}

// NewMetricsAgent создает новый экземпляр агента
func NewMetricsAgent(cfg models.Config) *MetricsAgent {
	ctx, cancel := context.WithCancel(context.Background())
	return &MetricsAgent{
		cfg:         cfg,
		metricsChan: make(chan []models.Metrics, 100),   // буферизованный канал
		workerPool:  make(chan struct{}, cfg.RateLimit), // worker pool с ограничением
		ctx:         ctx,
		cancel:      cancel,
		pollCount:   0,
	}
}

// Start запускает все горутины агента
func (a *MetricsAgent) Start() {
	logger.Log.Info("Starting metrics agent",
		zap.Int64("rate_limit", a.cfg.RateLimit),
		zap.Duration("poll_interval", a.cfg.PollInterval),
		zap.Duration("report_interval", a.cfg.ReportInterval),
	)

	// Запускаем воркеры для отправки метрик
	for i := 0; i < int(a.cfg.RateLimit); i++ {
		a.wg.Add(1)
		go a.worker(i)
	}

	// ГОРУТИНА 1: Сбор runtime метрик
	a.wg.Add(1)
	go a.collectRuntimeMetrics()

	// ГОРУТИНА 2: Сбор метрик gopsutil
	a.wg.Add(1)
	go a.collectGopsutilMetrics()

	logger.Log.Info("All agent goroutines started")
}

// Stop останавливает агент
func (a *MetricsAgent) Stop() {
	logger.Log.Info("Stopping metrics agent")
	a.cancel()
	a.wg.Wait()
	close(a.metricsChan)
	logger.Log.Info("Metrics agent stopped")
}

// worker обрабатывает метрики из канала с ограничением RPS
func (a *MetricsAgent) worker(id int) {
	defer a.wg.Done()

	logger.Log.Debug("Worker started", zap.Int("worker_id", id))

	for {
		select {
		case metrics, ok := <-a.metricsChan:
			if !ok {
				logger.Log.Debug("Worker stopping - channel closed", zap.Int("worker_id", id))
				return
			}

			if len(metrics) == 0 {
				continue
			}

			// Занимаем слот в worker pool (ограничение RPS)
			select {
			case a.workerPool <- struct{}{}:
				// Слот получен, можно отправлять
				logger.Log.Debug("Worker sending metrics",
					zap.Int("worker_id", id),
					zap.Int("metrics_count", len(metrics)),
				)

				if err := a.sendMetricsBatch(metrics); err != nil {
					logger.Log.Error("Worker error sending metrics",
						zap.Int("worker_id", id),
						zap.Error(err),
					)
				}

				// Освобождаем слот
				<-a.workerPool

			case <-a.ctx.Done():
				logger.Log.Debug("Worker stopping - context done", zap.Int("worker_id", id))
				return
			}

		case <-a.ctx.Done():
			logger.Log.Debug("Worker stopping - context done", zap.Int("worker_id", id))
			return
		}
	}
}

// collectRuntimeMetrics собирает runtime метрики
func (a *MetricsAgent) collectRuntimeMetrics() {
	defer a.wg.Done()

	logger.Log.Debug("Runtime metrics collector started")

	ticker := time.NewTicker(a.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.mu.Lock()
			a.pollCount++
			pollCount := a.pollCount
			a.mu.Unlock()

			metrics := a.getRuntimeMetrics(pollCount)
			logger.Log.Debug("Collected runtime metrics",
				zap.Int("metrics_count", len(metrics)),
				zap.Int64("poll_count", pollCount),
			)

			// Отправляем метрики в канал для обработки воркерами
			select {
			case a.metricsChan <- metrics:
				// Метрики отправлены в канал
			case <-a.ctx.Done():
				return
			default:
				logger.Log.Warn("Metrics channel full, dropping batch")
			}

		case <-a.ctx.Done():
			logger.Log.Debug("Runtime metrics collector stopping")
			return
		}
	}
}

// collectGopsutilMetrics собирает системные метрики через gopsutil
func (a *MetricsAgent) collectGopsutilMetrics() {
	defer a.wg.Done()

	logger.Log.Debug("Gopsutil metrics collector started")

	ticker := time.NewTicker(a.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			metrics := a.getGopsutilMetrics()
			logger.Log.Debug("Collected gopsutil metrics",
				zap.Int("metrics_count", len(metrics)),
			)

			// Отправляем метрики в канал для обработки воркерами
			select {
			case a.metricsChan <- metrics:
				// Метрики отправлены в канал
			case <-a.ctx.Done():
				return
			default:
				logger.Log.Warn("Metrics channel full, dropping gopsutil batch")
			}

		case <-a.ctx.Done():
			logger.Log.Debug("Gopsutil metrics collector stopping")
			return
		}
	}
}

// getRuntimeMetrics возвращает runtime метрики
func (a *MetricsAgent) getRuntimeMetrics(pollCount int64) []models.Metrics {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

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
		"RandomValue":   a.getRandomValue(),
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

	return metrics
}

// getGopsutilMetrics возвращает системные метрики через gopsutil
func (a *MetricsAgent) getGopsutilMetrics() []models.Metrics {
	var metrics []models.Metrics

	// TotalMemory
	if vmStat, err := mem.VirtualMemory(); err == nil {
		totalMem := float64(vmStat.Total)
		metrics = append(metrics, models.Metrics{
			ID:    "TotalMemory",
			MType: "gauge",
			Value: &totalMem,
		})
	} else {
		logger.Log.Error("Failed to get TotalMemory", zap.Error(err))
	}

	// FreeMemory
	if vmStat, err := mem.VirtualMemory(); err == nil {
		freeMem := float64(vmStat.Free)
		metrics = append(metrics, models.Metrics{
			ID:    "FreeMemory",
			MType: "gauge",
			Value: &freeMem,
		})
	} else {
		logger.Log.Error("Failed to get FreeMemory", zap.Error(err))
	}

	// CPU utilization (по количеству CPU)
	if cpuPercents, err := cpu.Percent(0, true); err == nil { // true - по всем ядрам
		for i, percent := range cpuPercents {
			cpuUtil := percent
			metrics = append(metrics, models.Metrics{
				ID:    fmt.Sprintf("CPUutilization%d", i+1),
				MType: "gauge",
				Value: &cpuUtil,
			})
		}
	} else {
		logger.Log.Error("Failed to get CPU utilization", zap.Error(err))
	}

	return metrics
}

// getRandomValue возвращает случайное значение
func (a *MetricsAgent) getRandomValue() float64 {
	return float64(a.pollCount % 100) // Простая реализация для примера
}

// sendMetricsBatch отправляет батч метрик (реализация будет в основном файле)
func (a *MetricsAgent) sendMetricsBatch(metrics []models.Metrics) error {
	// Эта функция будет реализована в основном файле
	// Здесь только заглушка
	return nil
}
