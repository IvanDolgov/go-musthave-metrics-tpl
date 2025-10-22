package main

import "sync"

// MetricType представляет тип метрики
type MetricType string

const (
	Gauge   MetricType = "gauge"
	Counter MetricType = "counter"
)

// Metric представляет отдельную метрику с именем, значением и типом
type Metric struct {
	Name  string      // Имя метрики (уникальный идентификатор)
	Value interface{} // Значение метрики (может быть разного типа)
	Type  MetricType  // Тип метрики (gauge или counter)
}

// MemStorage - хранилище для метрик в памяти
// Разделяет метрики по типам для эффективного хранения и доступа
type MemStorage struct {
	gauges   map[string]float64 // Хранилище для gauge-метрик (имя -> значение)
	counters map[string]int64   // Хранилище для counter-метрик (имя -> значение)
	mu       sync.RWMutex       // RWMutex для потокобезопасного доступа
}

// NewMemStorage создает и возвращает новый экземпляр MemStorage
// Инициализирует внутренние хранилища для метрик разных типов
func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
	}
}

// SetGauge устанавливает значение для gauge-метрики
// Если метрика с таким именем уже существует, ее значение перезаписывается
// name - имя метрики, value - новое значение (дробное число)
func (m *MemStorage) SetGauge(name string, value float64) {
	m.mu.Lock() // ставим флаг, что мы пишем в хранилище
	defer m.mu.Unlock()
	m.gauges[name] = value
}

// // SetCounter устанавливает значение для counter-метрики
// // Если метрика с таким именем уже существует, ее значение перезаписывается
// // name - имя метрики, value - новое значение (целое число)
// func (m *MemStorage) SetCounter(name string, value int64) {
// m.mu.Lock() // ставим флаг, что мы читаем
// defer m.mu.Unlock()
// 	m.counters[name] = value
// }

// IncrementCounter увеличивает значение counter-метрики на указанную величину
// Если метрика с таким именем не существует, она создается с начальным значением delta
// name - имя метрики, delta - значение для увеличения (может быть отрицательным)
func (m *MemStorage) IncrementCounter(name string, delta int64) {
	m.mu.Lock() // ставим флаг, что мы пишем в хранилище
	defer m.mu.Unlock()
	m.counters[name] += delta
}

// GetAllMetrics возвращает все метрики из хранилища
// Возвращает два map: gauge-метрики и counter-метрики
func (m *MemStorage) GetAllMetrics() (map[string]float64, map[string]int64) {
	m.mu.RLock() // ставим флаг, что мы читаем из хранилища
	defer m.mu.RUnlock()

	// Создаем копии map для безопасного возврата
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
// Возвращает значение в виде interface{} и флаг существования метрики
func (m *MemStorage) GetMetric(name string, metricType MetricType) (interface{}, bool) {
	m.mu.RLock() // ставим флаг, что мы читаем из хранилища
	defer m.mu.RUnlock()
	switch metricType {
	case Gauge:
		value, exists := m.gauges[name]
		// если нет метрики то возвращаем nil
		if !exists {
			return nil, false
		}
		return value, true
	case Counter:
		value, exists := m.counters[name]
		// если нет метрики то возвращаем nil
		if !exists {
			return nil, false
		}
		return value, true
	// если нет типа метрики то возвращаем nil
	default:
		return nil, false
	}
}
