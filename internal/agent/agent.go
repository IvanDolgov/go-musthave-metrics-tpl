// Package agent предоставляет клиент для сбора и отправки метрик на сервер.
// Агент собирает системные метрики (runtime, gopsutil) и отправляет их
// на сервер с заданным интервалом. Поддерживает ограничение RPS и пул воркеров.
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

// MetricsAgent собирает и отправляет метрики на сервер.
// Использует пул воркеров для ограничения RPS и пулы объектов для уменьшения аллокаций.
//
// Основные функции:
//   - Сбор runtime метрик Go (использование памяти, GC, горутины)
//   - Сбор системных метрик через gopsutil (CPU, память)
//   - Отправка метрик на сервер с ограничением RPS
//   - Поддержка graceful shutdown
//
// Пример использования:
//
//	cfg := models.Config{PollInterval: 2*time.Second, ReportInterval: 10*time.Second, RateLimit: 10}
//	sender := NewHTTPSender("http://localhost:8080", "secret-key")
//	agent := NewMetricsAgent(cfg, sender)
//	agent.Start()
//	// ... работа агента
//	agent.Stop()
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

// MetricsSender определяет интерфейс для отправки метрик на сервер.
// Реализации могут использовать HTTP, gRPC или другие протоколы.
type MetricsSender interface {
	// SendMetricsBatch отправляет батч метрик на сервер.
	// Возвращает ошибку если отправка не удалась.
	SendMetricsBatch(ctx context.Context, metrics []models.Metrics) error
}

// Pools для уменьшения аллокаций памяти при частом создании слайсов метрик.
var (
	// metricsPool - пул для слайсов метрик произвольного размера.
	// Используется для gopsutil метрик и других динамических коллекций.
	metricsPool = sync.Pool{
		New: func() interface{} {
			return make([]models.Metrics, 0, 36) // предварительное выделение
		},
	}

	// runtimeMetricsPool - пул для слайсов runtime метрик фиксированного размера.
	// Содержит предварительно инициализированные структуры для 35 gauge + 1 counter метрик.
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

// NewMetricsAgent создает новый экземпляр агента метрик.
//
// Параметры:
//   - cfg: конфигурация агента (интервалы опроса, лимит RPS)
//   - sender: реализация интерфейса MetricsSender для отправки метрик
//
// Возвращает:
//   - *MetricsAgent: новый экземпляр агента, готовый к запуску
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

// Start запускает все горутины агента.
// Запускает:
//   - Воркеры для отправки метрик (количество = RateLimit)
//   - Сборщик runtime метрик
//   - Сборщик gopsutil метрик
//
// После вызова Start агент начинает собирать и отправлять метрики
// с интервалами, указанными в конфигурации.
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

// Stop останавливает агент корректно (graceful shutdown).
// Останавливает все горутины, дожидается их завершения и закрывает каналы.
// После вызова Stop агент больше не собирает и не отправляет метрики.
func (a *MetricsAgent) Stop() {
	logger.Log.Info("Stopping metrics agent")
	a.cancel()
	a.wg.Wait()
	close(a.metricsChan)
	logger.Log.Info("Metrics agent stopped")
}

// worker обрабатывает метрики из канала с ограничением RPS.
// Каждый воркер:
//  1. Ждет метрики из канала metricsChan
//  2. Занимает слот в workerPool (ограничение RPS)
//  3. Отправляет метрики через sender
//  4. Освобождает слот в workerPool
//  5. Возвращает метрики в пул для повторного использования
//
// Параметры:
//   - id: уникальный идентификатор воркера для логирования
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

// collectRuntimeMetrics собирает runtime метрики Go с заданным интервалом.
// Собирает 35 gauge метрик (Alloc, BuckHashSys, Frees, и т.д.) и 1 counter метрику (PollCount).
// Использует sync.Pool для уменьшения аллокаций памяти.
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

// collectGopsutilMetrics собирает системные метрики через gopsutil с заданным интервалом.
// Собирает метрики:
//   - TotalMemory: общий объем памяти системы
//   - FreeMemory: свободная память
//   - CPUutilization{1..N}: загрузка CPU по ядрам
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

// gaugeValue представляет пару имя-значение для gauge метрики
type gaugeValue struct {
	name  string
	value *float64
}

// getRuntimeMetrics возвращает runtime метрики Go (оптимизированная версия)
func (a *MetricsAgent) getRuntimeMetrics(pollCount int64) []models.Metrics {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Берем из пула предварительно выделенный слайс
	metrics := runtimeMetricsPool.Get().([]models.Metrics)

	// Восстанавливаем length до 0, но сохраняем capacity
	metrics = metrics[:0]

	// Собираем gauge метрики
	metrics = a.collectGaugeMetrics(memStats, metrics)

	// Добавляем counter метрику
	metrics = a.addCounterMetric(pollCount, metrics)

	return metrics
}

// collectGaugeMetrics собирает все gauge метрики из runtime.MemStats
func (a *MetricsAgent) collectGaugeMetrics(memStats runtime.MemStats, metrics []models.Metrics) []models.Metrics {
	// Извлекаем значения gauge метрик
	gaugeValues := a.extractGaugeValues(memStats)

	// Добавляем random value
	randomValue := a.getRandomValue()
	gaugeValues = append(gaugeValues, gaugeValue{"RandomValue", &randomValue})

	// Создаем метрики из значений
	for _, gv := range gaugeValues {
		metrics = append(metrics, models.Metrics{
			ID:    gv.name,
			MType: "gauge",
			Value: gv.value,
		})
	}

	return metrics
}

// extractGaugeValues извлекает значения gauge метрик из runtime.MemStats
func (a *MetricsAgent) extractGaugeValues(memStats runtime.MemStats) []gaugeValue {
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

	return []gaugeValue{
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
	}
}

// addCounterMetric добавляет counter метрику PollCount
func (a *MetricsAgent) addCounterMetric(pollCount int64, metrics []models.Metrics) []models.Metrics {
	return append(metrics, models.Metrics{
		ID:    "PollCount",
		MType: "counter",
		Delta: &pollCount,
	})
}

// getGopsutilMetrics возвращает системные метрики через gopsutil (оптимизированная версия).
// Собирает метрики памяти и загрузки CPU.
// Использует sync.Pool для уменьшения аллокаций памяти.
//
// Возвращает:
//   - []models.Metrics: слайс с системными метриками
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

// getRandomValue возвращает псевдослучайное значение на основе счетчика опросов.
// Используется для тестирования и демонстрации работы с gauge метриками.
//
// Возвращает:
//   - float64: случайное значение в диапазоне 0-99
func (a *MetricsAgent) getRandomValue() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return float64(a.pollCount % 100)
}
