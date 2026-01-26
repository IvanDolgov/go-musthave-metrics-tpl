package agent

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"go.uber.org/zap"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

// MetricsAgent собирает и отправляет метрики
type MetricsAgent struct {
	cfg         models.Config
	sender      MetricsSender
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	metricsChan chan []models.Metrics
	workerPool  chan struct{}
	mu          sync.RWMutex
	pollCount   int64
}

// Pools для уменьшения аллокаций
var (
	metricsPool = sync.Pool{
		New: func() interface{} {
			return make([]models.Metrics, 0, 36) // предварительное выделение
		},
	}

	runtimeMetricsPool = sync.Pool{
		New: func() interface{} {
			// Слайс для runtime метрик (35 gauge + 1 counter)
			metrics := make([]models.Metrics, 36)
			// Предварительно инициализируем структуры
			for i := range metrics {
				metrics[i] = models.Metrics{}
			}
			return metrics
		},
	}
)

// NewMetricsAgent создает новый экземпляр агента
func NewMetricsAgent(cfg models.Config, sender MetricsSender) *MetricsAgent {
	ctx, cancel := context.WithCancel(context.Background())

	// Создаем буферизованный канал для уменьшения блокировок
	metricsChan := make(chan []models.Metrics, 100)

	// Worker pool для ограничения RPS
	workerPool := make(chan struct{}, cfg.RateLimit)

	return &MetricsAgent{
		cfg:         cfg,
		sender:      sender,
		ctx:         ctx,
		cancel:      cancel,
		metricsChan: metricsChan,
		workerPool:  workerPool,
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
				// Возвращаем пустой слайс в pool
				metricsPool.Put(metrics[:0])
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

				if err := a.sender.SendMetricsBatch(a.ctx, metrics); err != nil {
					logger.Log.Error("Worker error sending metrics",
						zap.Int("worker_id", id),
						zap.Error(err),
					)
				}

				// Освобождаем слот
				<-a.workerPool

				// ВОЗВРАЩАЕМ МЕТРИКИ В POOL ПОСЛЕ ОТПРАВКИ
				// Очищаем указатели чтобы избежать утечек памяти
				for i := range metrics {
					metrics[i].Value = nil
					metrics[i].Delta = nil
				}
				metricsPool.Put(metrics[:0])

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
			case <-a.ctx.Done():
				return
			default:
				logger.Log.Warn("Metrics channel full, dropping batch")
				// Если канал полный, возвращаем метрики в pool
				for i := range metrics {
					metrics[i].Value = nil
					metrics[i].Delta = nil
				}
				runtimeMetricsPool.Put(metrics)
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
			case <-a.ctx.Done():
				return
			default:
				logger.Log.Warn("Metrics channel full, dropping gopsutil batch")
				// Возвращаем метрики в pool
				for i := range metrics {
					metrics[i].Value = nil
					metrics[i].Delta = nil
				}
				metricsPool.Put(metrics[:0])
			}

		case <-a.ctx.Done():
			logger.Log.Debug("Gopsutil metrics collector stopping")
			return
		}
	}
}

// getRuntimeMetrics возвращает runtime метрики (оптимизированная версия)
func (a *MetricsAgent) getRuntimeMetrics(pollCount int64) []models.Metrics {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Берем из пула предварительно выделенный слайс
	metrics := runtimeMetricsPool.Get().([]models.Metrics)

	// Восстанавливаем length до 0, но сохраняем capacity
	metrics = metrics[:0]

	// Предварительно вычисляем все значения
	// Используем локальные переменные чтобы уменьшить аллокации
	alloc := float64(memStats.Alloc)
	buckHashSys := float64(memStats.BuckHashSys)
	frees := float64(memStats.Frees)
	gcCPUFraction := memStats.GCCPUFraction
	gcSys := float64(memStats.GCSys)
	heapAlloc := float64(memStats.HeapAlloc)
	heapIdle := float64(memStats.HeapIdle)
	heapInuse := float64(memStats.HeapInuse)
	heapObjects := float64(memStats.HeapObjects)
	heapReleased := float64(memStats.HeapReleased)
	heapSys := float64(memStats.HeapSys)
	lastGC := float64(memStats.LastGC)
	lookups := float64(memStats.Lookups)
	mCacheInuse := float64(memStats.MCacheInuse)
	mCacheSys := float64(memStats.MCacheSys)
	mSpanInuse := float64(memStats.MSpanInuse)
	mSpanSys := float64(memStats.MSpanSys)
	mallocs := float64(memStats.Mallocs)
	nextGC := float64(memStats.NextGC)
	numForcedGC := float64(memStats.NumForcedGC)
	numGC := float64(memStats.NumGC)
	otherSys := float64(memStats.OtherSys)
	pauseTotalNs := float64(memStats.PauseTotalNs)
	stackInuse := float64(memStats.StackInuse)
	stackSys := float64(memStats.StackSys)
	sys := float64(memStats.Sys)
	totalAlloc := float64(memStats.TotalAlloc)
	randomValue := a.getRandomValue()

	// Заполняем метрики, переиспользуя созданные структуры
	// gauge метрики
	gaugeMetrics := []struct {
		name  string
		value *float64
	}{
		{"Alloc", &alloc},
		{"BuckHashSys", &buckHashSys},
		{"Frees", &frees},
		{"GCCPUFraction", &gcCPUFraction},
		{"GCSys", &gcSys},
		{"HeapAlloc", &heapAlloc},
		{"HeapIdle", &heapIdle},
		{"HeapInuse", &heapInuse},
		{"HeapObjects", &heapObjects},
		{"HeapReleased", &heapReleased},
		{"HeapSys", &heapSys},
		{"LastGC", &lastGC},
		{"Lookups", &lookups},
		{"MCacheInuse", &mCacheInuse},
		{"MCacheSys", &mCacheSys},
		{"MSpanInuse", &mSpanInuse},
		{"MSpanSys", &mSpanSys},
		{"Mallocs", &mallocs},
		{"NextGC", &nextGC},
		{"NumForcedGC", &numForcedGC},
		{"NumGC", &numGC},
		{"OtherSys", &otherSys},
		{"PauseTotalNs", &pauseTotalNs},
		{"StackInuse", &stackInuse},
		{"StackSys", &stackSys},
		{"Sys", &sys},
		{"TotalAlloc", &totalAlloc},
		{"RandomValue", &randomValue},
	}

	for _, gm := range gaugeMetrics {
		metrics = append(metrics, models.Metrics{
			ID:    gm.name,
			MType: "gauge",
			Value: gm.value,
		})
	}

	// counter метрика
	metrics = append(metrics, models.Metrics{
		ID:    "PollCount",
		MType: "counter",
		Delta: &pollCount,
	})

	return metrics
}

// getGopsutilMetrics возвращает системные метрики через gopsutil (оптимизированная версия)
func (a *MetricsAgent) getGopsutilMetrics() []models.Metrics {
	// Берем слайс из пула
	metrics := metricsPool.Get().([]models.Metrics)
	metrics = metrics[:0]

	// TotalMemory и FreeMemory можно получить за один вызов
	if vmStat, err := mem.VirtualMemory(); err == nil {
		totalMem := float64(vmStat.Total)
		freeMem := float64(vmStat.Free)

		metrics = append(metrics,
			models.Metrics{
				ID:    "TotalMemory",
				MType: "gauge",
				Value: &totalMem,
			},
			models.Metrics{
				ID:    "FreeMemory",
				MType: "gauge",
				Value: &freeMem,
			},
		)
	} else {
		logger.Log.Error("Failed to get memory stats", zap.Error(err))
	}

	// CPU utilization
	if cpuPercents, err := cpu.Percent(0, true); err == nil {
		// Предварительно создаем слайс для CPU метрик
		cpuMetrics := make([]models.Metrics, len(cpuPercents))
		for i, percent := range cpuPercents {
			cpuUtil := percent
			cpuMetrics[i] = models.Metrics{
				ID:    fmt.Sprintf("CPUutilization%d", i+1),
				MType: "gauge",
				Value: &cpuUtil,
			}
		}
		metrics = append(metrics, cpuMetrics...)
	} else {
		logger.Log.Error("Failed to get CPU utilization", zap.Error(err))
	}

	return metrics
}

// getRandomValue возвращает случайное значение
func (a *MetricsAgent) getRandomValue() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return float64(a.pollCount % 100)
}
