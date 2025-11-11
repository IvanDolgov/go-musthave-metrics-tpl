package storage

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

// Storage интерфейс для работы с метриками
type Storage interface {
	SetGauge(name string, value float64)
	IncrementCounter(name string, delta int64)
	GetMetric(name string, metricType models.MetricType) (interface{}, bool)
	GetAllMetrics() (map[string]float64, map[string]int64)
	GetMetricForJSON(name string, metricType models.MetricType) models.Metrics
	SaveToFile(filename string) error
	LoadFromFile(filename string) error
	UpdateMetricsBatch(metrics []models.Metrics) error
}

// MemStorage - хранилище для метрик в памяти
type MemStorage struct {
	gauges   map[string]float64
	counters map[string]int64
	mu       sync.RWMutex
}

// NewMemStorage создает и возвращает новый экземпляр MemStorage
func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
	}
}

// SetGauge устанавливает значение для gauge-метрики
func (m *MemStorage) SetGauge(name string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gauges[name] = value
}

// IncrementCounter увеличивает значение counter-метрики
func (m *MemStorage) IncrementCounter(name string, delta int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name] += delta
}

// GetAllMetrics возвращает все метрики из хранилища
func (m *MemStorage) GetAllMetrics() (map[string]float64, map[string]int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	gaugesCopy := make(map[string]float64, len(m.gauges))
	for k, v := range m.gauges {
		gaugesCopy[k] = v
	}

	countersCopy := make(map[string]int64, len(m.counters))
	for k, v := range m.counters {
		countersCopy[k] = v
	}

	return gaugesCopy, countersCopy
}

// GetMetric возвращает метрику по имени и типу
func (m *MemStorage) GetMetric(name string, metricType models.MetricType) (interface{}, bool) {
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

// GetMetricForJSON возвращает метрику в формате для JSON
func (m *MemStorage) GetMetricForJSON(name string, metricType models.MetricType) models.Metrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	switch metricType {
	case models.Gauge:
		if value, exists := m.gauges[name]; exists {
			return models.Metrics{
				ID:    name,
				MType: "gauge",
				Value: &value,
			}
		}
	case models.Counter:
		if value, exists := m.counters[name]; exists {
			return models.Metrics{
				ID:    name,
				MType: "counter",
				Delta: &value,
			}
		}
	}
	return models.Metrics{}
}

// SaveToFile сохраняет все метрики в файл в формате JSON
func (m *MemStorage) SaveToFile(filename string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var metrics []models.FileMetric

	for name, value := range m.gauges {
		valueCopy := value
		metrics = append(metrics, models.FileMetric{
			ID:    name,
			Type:  "gauge",
			Value: &valueCopy,
		})
	}

	for name, value := range m.counters {
		valueCopy := value
		metrics = append(metrics, models.FileMetric{
			ID:    name,
			Type:  "counter",
			Delta: &valueCopy,
		})
	}

	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

// LoadFromFile загружает метрики из файла
func (m *MemStorage) LoadFromFile(filename string) error {
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

	m.gauges = make(map[string]float64)
	m.counters = make(map[string]int64)

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

// UpdateMetricsBatch обновляет метрики батчем
func (m *MemStorage) UpdateMetricsBatch(metrics []models.Metrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()

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
