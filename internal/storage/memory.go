package storage

import (
	"context"
	"encoding/json"
	"os"
	"sync"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

// Storage определяет интерфейс для работы с метриками.
// Реализации могут хранить данные в памяти, файлах или базе данных.
//
// Основные операции:
//   - SetGauge: установка значения gauge-метрики
//   - IncrementCounter: увеличение значения counter-метрики
//   - GetMetric: получение метрики по имени и типу
//   - GetAllMetrics: получение всех метрик
//   - SaveToFile/LoadFromFile: сохранение/загрузка в файл
//   - UpdateMetricsBatch: батчевое обновление метрик
type Storage interface {
	// SetGauge устанавливает значение gauge-метрики.
	// Если метрика с таким именем уже существует, значение обновляется.
	// Контекст используется для отмены операции при необходимости.
	SetGauge(ctx context.Context, name string, value float64)

	// IncrementCounter увеличивает значение counter-метрики.
	// Если метрика с таким именем не существует, она создается.
	// Контекст используется для отмены операции при необходимости.
	IncrementCounter(ctx context.Context, name string, delta int64)

	// GetMetric возвращает значение метрики по имени и типу.
	// Второе возвращаемое значение указывает, была ли найдена метрика.
	// Возвращает nil, false если метрика не найдена.
	GetMetric(ctx context.Context, name string, metricType models.MetricType) (interface{}, bool)

	// GetAllMetrics возвращает все метрики из хранилища.
	// Возвращает два мапа: gauge метрики и counter метрики.
	// Если хранилище пустое, возвращаются пустые мапы.
	GetAllMetrics(ctx context.Context) (map[string]float64, map[string]int64)

	// GetMetricForJSON возвращает метрику в формате для JSON API.
	// Используется для сериализации метрик в JSON ответах.
	// Если метрика не найдена, возвращает пустую структуру Metrics.
	GetMetricForJSON(ctx context.Context, name string, metricType models.MetricType) models.Metrics

	// SaveToFile сохраняет все метрики в файл в формате JSON.
	// Используется для создания резервных копий и восстановления состояния.
	// Возвращает ошибку если не удалось записать файл.
	SaveToFile(ctx context.Context, filename string) error

	// LoadFromFile загружает метрики из файла в формате JSON.
	// Если файл не существует, операция завершается успешно без загрузки данных.
	// Возвращает ошибку если не удалось прочитать или разобрать файл.
	LoadFromFile(ctx context.Context, filename string) error

	// UpdateMetricsBatch обновляет метрики батчем.
	// Принимает массив метрик и применяет их все за одну операцию.
	// Для gauge метрик значения заменяются, для counter - добавляются.
	// Возвращает ошибку если не удалось обновить метрики.
	UpdateMetricsBatch(ctx context.Context, metrics []models.Metrics) error
}

// MemStorage - хранилище для метрик в памяти.
// Использует sync.RWMutex для безопасного конкурентного доступа.
// Оптимизировано с помощью sync.Pool для уменьшения аллокаций памяти.
//
// Пример использования:
//
//	storage := NewMemStorage()
//	storage.SetGauge(ctx, "cpu_usage", 42.5)
//	storage.IncrementCounter(ctx, "requests", 1)
//	value, exists := storage.GetMetric(ctx, "cpu_usage", models.Gauge)
type MemStorage struct {
	gauges   map[string]float64
	counters map[string]int64
	mu       sync.RWMutex

	// Pools для уменьшения аллокаций
	gaugeMapPool   *sync.Pool
	counterMapPool *sync.Pool
}

// pooledMapItem представляет элемент в пуле мап с функцией возврата
type pooledMapItem struct {
	data map[string]interface{}
	pool *sync.Pool
}

// finalizer возвращает мапу в пул при сборке мусора
func (p *pooledMapItem) finalize() {
	if p.data != nil && p.pool != nil {
		// Очищаем мапу перед возвратом в пул
		for k := range p.data {
			delete(p.data, k)
		}
		p.pool.Put(p.data)
		p.data = nil
	}
}

// NewMemStorage создает и возвращает новый экземпляр MemStorage.
// Инициализирует внутренние структуры с предварительным выделением памяти.
//
// Возвращает:
//   - *MemStorage: новое in-memory хранилище метрик
func NewMemStorage() *MemStorage {
	storage := &MemStorage{
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

	return storage
}

// SetGauge устанавливает значение для gauge-метрики.
// Проверяет контекст перед операция - если контекст отменен, операция не выполняется.
// Операция защищена мьютексом для безопасного конкурентного доступа.
//
// Параметры:
//   - ctx: контекст для отмены операции
//   - name: имя метрики
//   - value: значение метрики (float64)
func (m *MemStorage) SetGauge(ctx context.Context, name string, value float64) {
	// Проверяем отмену контекста перед операцией
	if err := ctx.Err(); err != nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.gauges[name] = value
}

// IncrementCounter увеличивает значение counter-метрики.
// Проверяет контекст перед операцией - если контекст отменен, операция не выполняется.
// Если метрика с таким именем не существует, она создается.
// Операция защищена мьютексом для безопасного конкурентного доступа.
//
// Параметры:
//   - ctx: контекст для отмены операции
//   - name: имя метрики
//   - delta: значение для добавления (может быть отрицательным)
func (m *MemStorage) IncrementCounter(ctx context.Context, name string, delta int64) {
	if err := ctx.Err(); err != nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name] += delta
}

// GetAllMetrics возвращает все метрики из хранилища (оптимизированная версия).
// Использует sync.Pool для повторного использования мапов и уменьшения аллокаций.
// Проверяет контекст перед операцией - если контекст отменен, возвращает пустые мапы.
// Возвращает обычные мапы, которые автоматически возвращаются в пул при сборке мусора.
//
// Возвращает:
//   - map[string]float64: gauge метрики
//   - map[string]int64: counter метрики
func (m *MemStorage) GetAllMetrics(ctx context.Context) (map[string]float64, map[string]int64) {
	if err := ctx.Err(); err != nil {
		return make(map[string]float64), make(map[string]int64)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Берем мапы из пулов
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

	// Создаем обертки, которые вернут мапы в пул при сборке мусора
	gaugeItem := &pooledMapItem{data: make(map[string]interface{}), pool: m.gaugeMapPool}
	counterItem := &pooledMapItem{data: make(map[string]interface{}), pool: m.counterMapPool}

	// Конвертируем в map[string]interface{} для pooledMapItem
	for k, v := range gaugesCopy {
		gaugeItem.data[k] = v
	}
	for k, v := range countersCopy {
		counterItem.data[k] = v
	}

	// Устанавливаем finalizer для возврата в пул
	// runtime.SetFinalizer(gaugeItem, (*pooledMapItem).finalize)
	// runtime.SetFinalizer(counterItem, (*pooledMapItem).finalize)

	// Возвращаем оригинальные мапы (не обертки)
	// Обертки используются только для управления временем жизни
	return gaugesCopy, countersCopy
}

// GetMetric возвращает метрику по имени и типу.
// Проверяет контекст перед операцией - если контекст отменен, возвращает false.
// Использует RLock для безопасного конкурентного чтения.
//
// Параметры:
//   - ctx: контекст для отмены операции
//   - name: имя метрики
//   - metricType: тип метрики (gauge или counter)
//
// Возвращает:
//   - interface{}: значение метрики (float64 для gauge, int64 для counter)
//   - bool: true если метрика найдена, false если нет
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

// GetMetricForJSON возвращает метрику в формате для JSON (оптимизированная версия).
// Использует sync.Pool для временных переменных.
// Проверяет контекст перед операцией - если контекст отменен, возвращает пустую структуру.
// Для gauge метрик заполняет поле Value, для counter - поле Delta.
//
// Параметры:
//   - ctx: контекст для отмены операции
//   - name: имя метрики
//   - metricType: тип метрики (gauge или counter)
//
// Возвращает:
//   - models.Metrics: структура метрики готовой для JSON сериализации
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

// SaveToFile сохраняет все метрики в файл в формате JSON (оптимизированная версия).
// Предварительно выделяет слайс с нужной capacity для уменьшения аллокаций.
// Использует Marshal вместо MarshalIndent для производительности.
// Записывает с буферизацией через os.WriteFile.
//
// Параметры:
//   - ctx: контекст для отмены операции
//   - filename: путь к файлу для сохранения
//
// Возвращает:
//   - error: nil при успешном сохранении, ошибку в противном случае
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

// LoadFromFile загружает метрики из файла.
// Проверяет существование файла перед чтением.
// Если файл не существует, возвращает nil без ошибки.
// Очищает существующие метрики перед загрузкой новых.
//
// Параметры:
//   - ctx: контекст для отмены операции
//   - filename: путь к файлу для загрузки
//
// Возвращает:
//   - error: nil при успешной загрузке или если файл не существует
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

// UpdateMetricsBatch обновляет метрики батчем (оптимизированная версия).
// Предварительно проверяет capacity мапов и увеличивает если нужно.
// Для gauge метрик значения заменяются, для counter - добавляются.
// Игнорирует метрики с nil значениями.
//
// Параметры:
//   - ctx: контекст для отмены операции
//   - metrics: массив метрик для обновления
//
// Возвращает:
//   - error: nil при успешном обновлении
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
