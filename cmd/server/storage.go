package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	models "github.com/IvanDolgov/go-musthave-metrics-tpl/internal/model"
	"go.uber.org/zap"
)

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

// FileMetric представляет метрику для сохранения в файл
type FileMetric struct {
	ID    string   `json:"id"`
	Type  string   `json:"type"`
	Value *float64 `json:"value,omitempty"`
	Delta *int64   `json:"delta,omitempty"`
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

func (m *MemStorage) GetMetricForJSON(name string, metricType MetricType) models.Metrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	switch metricType {
	case Gauge:
		if value, exists := m.gauges[name]; exists {
			return models.Metrics{
				ID:    name,
				MType: "gauge",
				Value: &value,
			}
		}
	case Counter:
		if value, exists := m.counters[name]; exists {
			return models.Metrics{
				ID:    name,
				MType: "counter",
				Delta: &value,
			}
		}
	}
	return models.Metrics{} // возвращаем пустую структуру если метрика не найдена
}

// SaveToFile сохраняет все метрики в файл в формате JSON
func (m *MemStorage) SaveToFile(filename string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Создаем слайс для хранения всех метрик
	var metrics []FileMetric

	// Добавляем gauge метрики
	for name, value := range m.gauges {
		valueCopy := value
		metrics = append(metrics, FileMetric{
			ID:    name,
			Type:  "gauge",
			Value: &valueCopy,
		})
	}

	// Добавляем counter метрики
	for name, value := range m.counters {
		valueCopy := value
		metrics = append(metrics, FileMetric{
			ID:    name,
			Type:  "counter",
			Delta: &valueCopy,
		})
	}

	// Сериализуем в JSON
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return err
	}

	// Сохраняем в файл
	return os.WriteFile(filename, data, 0644)
}

// LoadFromFile загружает метрики из файла
func (m *MemStorage) LoadFromFile(filename string) error {
	// Проверяем существует ли файл
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil // файл не существует - это не ошибка
	}

	// Читаем файл
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	// Десериализуем JSON
	var fileMetrics []FileMetric
	if err := json.Unmarshal(data, &fileMetrics); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Очищаем текущие метрики
	m.gauges = make(map[string]float64)
	m.counters = make(map[string]int64)

	// Загружаем метрики из файла
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

// withSyncSave добавляет middleware для синхронного сохранения после каждого запроса на обновление
func (m *MemStorage) withSyncSave(filename string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Пропускаем запрос через следующий обработчик
			next.ServeHTTP(w, r)

			// Сохраняем метрики только после запросов на обновление
			if r.Method == http.MethodPost && (strings.HasPrefix(r.URL.Path, "/update") || r.URL.Path == "/update/") {
				if err := m.SaveToFile(filename); err != nil {
					logger.Log.Error("Failed to sync save metrics", zap.String("file", filename), zap.Error(err))
				}
			}
		})
	}
}
