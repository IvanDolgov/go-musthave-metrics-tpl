package storage

import (
	"context"
	"encoding/json"
	"os"
	"sync"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

// Storage интерфейс для работы с метриками
type Storage interface {
	SetGauge(ctx context.Context, name string, value float64)
	IncrementCounter(ctx context.Context, name string, delta int64)
	GetMetric(ctx context.Context, name string, metricType models.MetricType) (interface{}, bool)
	GetAllMetrics(ctx context.Context) (map[string]float64, map[string]int64)
	GetMetricForJSON(ctx context.Context, name string, metricType models.MetricType) models.Metrics
	SaveToFile(ctx context.Context, filename string) error
	LoadFromFile(ctx context.Context, filename string) error
	UpdateMetricsBatch(ctx context.Context, metrics []models.Metrics) error
}

// MemStorage - хранилище для метрик в памяти
type MemStorage struct {
	gauges   map[string]float64
	counters map[string]int64
	mu       sync.RWMutex

	// Pools для уменьшения аллокаций
	gaugeMapPool   *sync.Pool
	counterMapPool *sync.Pool
}

// NewMemStorage создает и возвращает новый экземпляр MemStorage
func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauges:   make(map[string]float64, 50), // предварительное выделение
		counters: make(map[string]int64, 50),   // предварительное выделение

		gaugeMapPool: &sync.Pool{
			New: func() interface{} {
				return make(map[string]float64, 50)
			},
		},

		counterMapPool: &sync.Pool{
			New: func() interface{} {
				return make(map[string]int64, 50)
			},
		},
	}
}

// SetGauge устанавливает значение для gauge-метрики
func (m *MemStorage) SetGauge(ctx context.Context, name string, value float64) {
	// Проверяем отмену контекста перед операцией
	if err := ctx.Err(); err != nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.gauges[name] = value
}

// IncrementCounter увеличивает значение counter-метрики
func (m *MemStorage) IncrementCounter(ctx context.Context, name string, delta int64) {
	if err := ctx.Err(); err != nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name] += delta
}

// GetAllMetrics возвращает все метрики из хранилища (оптимизированная версия)
func (m *MemStorage) GetAllMetrics(ctx context.Context) (map[string]float64, map[string]int64) {
	if err := ctx.Err(); err != nil {
		return make(map[string]float64), make(map[string]int64)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Берем мапы из пулов вместо создания новых
	gaugesCopy := m.gaugeMapPool.Get().(map[string]float64)
	countersCopy := m.counterMapPool.Get().(map[string]int64)

	// Очищаем мапы перед использованием
	for k := range gaugesCopy {
		delete(gaugesCopy, k)
	}
	for k := range countersCopy {
		delete(countersCopy, k)
	}

	// Копируем данные
	for k, v := range m.gauges {
		gaugesCopy[k] = v
	}
	for k, v := range m.counters {
		countersCopy[k] = v
	}

	return gaugesCopy, countersCopy
}

// GetMetric возвращает метрику по имени и типу
func (m *MemStorage) GetMetric(ctx context.Context, name string, metricType models.MetricType) (interface{}, bool) {
	if err := ctx.Err(); err != nil {
		return nil, false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	switch metricType {
	case models.Gauge:
		value, exists := m.gauges[name]
		if !exists {
			return nil, false
		}
		return value, true
	case models.Counter:
		value, exists := m.counters[name]
		if !exists {
			return nil, false
		}
		return value, true
	default:
		return nil, false
	}
}

// GetMetricForJSON возвращает метрику в формате для JSON (оптимизированная версия)
func (m *MemStorage) GetMetricForJSON(ctx context.Context, name string, metricType models.MetricType) models.Metrics {
	if err := ctx.Err(); err != nil {
		return models.Metrics{}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Используем пул для временных переменных
	switch metricType {
	case models.Gauge:
		if value, exists := m.gauges[name]; exists {
			// Создаем копию значения
			v := value
			return models.Metrics{
				ID:    name,
				MType: "gauge",
				Value: &v,
			}
		}
	case models.Counter:
		if value, exists := m.counters[name]; exists {
			v := value
			return models.Metrics{
				ID:    name,
				MType: "counter",
				Delta: &v,
			}
		}
	}
	return models.Metrics{}
}

// SaveToFile сохраняет все метрики в файл в формате JSON (оптимизированная версия)
func (m *MemStorage) SaveToFile(ctx context.Context, filename string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Предварительно выделяем слайс с нужной capacity
	totalMetrics := len(m.gauges) + len(m.counters)
	metrics := make([]models.FileMetric, 0, totalMetrics)

	// Пул для временных переменных
	for name, value := range m.gauges {
		v := value
		metrics = append(metrics, models.FileMetric{
			ID:    name,
			Type:  "gauge",
			Value: &v,
		})
	}

	for name, value := range m.counters {
		v := value
		metrics = append(metrics, models.FileMetric{
			ID:    name,
			Type:  "counter",
			Delta: &v,
		})
	}

	// Используем Marshal вместо MarshalIndent для производительности
	data, err := json.Marshal(metrics)
	if err != nil {
		return err
	}

	// Записываем с буферизацией
	return os.WriteFile(filename, data, 0644)
}

// LoadFromFile загружает метрики из файла
func (m *MemStorage) LoadFromFile(ctx context.Context, filename string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	var fileMetrics []models.FileMetric
	if err := json.Unmarshal(data, &fileMetrics); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Очищаем существующие метрики
	m.gauges = make(map[string]float64, len(fileMetrics))
	m.counters = make(map[string]int64, len(fileMetrics))

	for _, fm := range fileMetrics {
		switch fm.Type {
		case "gauge":
			if fm.Value != nil {
				m.gauges[fm.ID] = *fm.Value
			}
		case "counter":
			if fm.Delta != nil {
				m.counters[fm.ID] = *fm.Delta
			}
		}
	}

	return nil
}

// UpdateMetricsBatch обновляет метрики батчем (оптимизированная версия)
func (m *MemStorage) UpdateMetricsBatch(ctx context.Context, metrics []models.Metrics) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Предварительно проверяем capacity мапов
	if len(m.gauges) < len(metrics) {
		// Увеличиваем capacity если нужно
		newGauges := make(map[string]float64, len(m.gauges)+len(metrics))
		for k, v := range m.gauges {
			newGauges[k] = v
		}
		m.gauges = newGauges
	}

	if len(m.counters) < len(metrics) {
		newCounters := make(map[string]int64, len(m.counters)+len(metrics))
		for k, v := range m.counters {
			newCounters[k] = v
		}
		m.counters = newCounters
	}

	for _, metric := range metrics {
		switch metric.MType {
		case "gauge":
			if metric.Value != nil {
				m.gauges[metric.ID] = *metric.Value
			}
		case "counter":
			if metric.Delta != nil {
				m.counters[metric.ID] += *metric.Delta
			}
		}
	}

	return nil
}

// Cleanup вызывается для возврата мапов в пул
func (m *MemStorage) Cleanup(gauges map[string]float64, counters map[string]int64) {
	if gauges != nil {
		m.gaugeMapPool.Put(gauges)
	}
	if counters != nil {
		m.counterMapPool.Put(counters)
	}
}
