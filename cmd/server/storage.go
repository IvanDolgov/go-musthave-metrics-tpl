package main

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
	m.gauges[name] = value
}

// // SetCounter устанавливает значение для counter-метрики
// // Если метрика с таким именем уже существует, ее значение перезаписывается
// // name - имя метрики, value - новое значение (целое число)
// func (m *MemStorage) SetCounter(name string, value int64) {
// 	m.counters[name] = value
// }

// IncrementCounter увеличивает значение counter-метрики на указанную величину
// Если метрика с таким именем не существует, она создается с начальным значением delta
// name - имя метрики, delta - значение для увеличения (может быть отрицательным)
func (m *MemStorage) IncrementCounter(name string, delta int64) {
	m.counters[name] += delta
}

// GetAllMetrics возвращает все метрики из хранилища
// Возвращает два map: gauge-метрики и counter-метрики
func (m *MemStorage) GetAllMetrics() (map[string]float64, map[string]int64) {
	return m.gauges, m.counters
}
