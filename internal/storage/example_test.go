package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

// ExampleMemStorage демонстрирует использование in-memory хранилища.
func ExampleMemStorage() {
	// Создаем новое хранилище
	storage := NewMemStorage()
	ctx := context.Background()

	// Устанавливаем gauge метрику
	storage.SetGauge(ctx, "cpu_usage", 42.5)

	// Увеличиваем counter метрику
	storage.IncrementCounter(ctx, "requests", 1)
	storage.IncrementCounter(ctx, "requests", 2)

	// Получаем значения метрик
	if value, exists := storage.GetMetric(ctx, "cpu_usage", models.Gauge); exists {
		fmt.Printf("Gauge value: %v\n", value)
	}

	if value, exists := storage.GetMetric(ctx, "requests", models.Counter); exists {
		fmt.Printf("Counter value: %v\n", value)
	}

	// Получаем все метрики
	gauges, counters := storage.GetAllMetrics(ctx)
	fmt.Printf("Total gauges: %d, counters: %d\n", len(gauges), len(counters))

	// Вывод:
	// Gauge value: 42.5
	// Counter value: 3
	// Total gauges: 1, counters: 1
}

// ExampleMemStorage_SaveLoad демонстрирует сохранение и загрузку метрик из файла.
func ExampleMemStorage_saveLoad() {
	storage := NewMemStorage()
	ctx := context.Background()

	// Добавляем тестовые метрики
	storage.SetGauge(ctx, "test_gauge", 99.9)
	storage.IncrementCounter(ctx, "test_counter", 50)

	// Создаем временный файл
	tmpDir := os.TempDir()
	filename := filepath.Join(tmpDir, "test_metrics.json")
	defer os.Remove(filename)

	// Сохраняем метрики в файл
	if err := storage.SaveToFile(ctx, filename); err != nil {
		fmt.Printf("Save error: %v\n", err)
		return
	}
	fmt.Println("Metrics saved to file")

	// Создаем новое хранилище и загружаем из файла
	newStorage := NewMemStorage()
	if err := newStorage.LoadFromFile(ctx, filename); err != nil {
		fmt.Printf("Load error: %v\n", err)
		return
	}
	fmt.Println("Metrics loaded from file")

	// Проверяем загруженные метрики
	gauges, counters := newStorage.GetAllMetrics(ctx)
	fmt.Printf("Loaded gauges: %d, counters: %d\n", len(gauges), len(counters))

	// Вывод:
	// Metrics saved to file
	// Metrics loaded from file
	// Loaded gauges: 1, counters: 1
}

// ExampleMemStorage_BatchUpdate демонстрирует батчевое обновление метрик.
func ExampleMemStorage_batchUpdate() {
	storage := NewMemStorage()
	ctx := context.Background()

	// Создаем батч метрик
	val1 := 1.1
	val2 := 2.2
	delta1 := int64(10)
	delta2 := int64(20)

	metrics := []models.Metrics{
		{ID: "gauge1", MType: "gauge", Value: &val1},
		{ID: "gauge2", MType: "gauge", Value: &val2},
		{ID: "counter1", MType: "counter", Delta: &delta1},
		{ID: "counter2", MType: "counter", Delta: &delta2},
	}

	// Обновляем метрики батчем
	if err := storage.UpdateMetricsBatch(ctx, metrics); err != nil {
		fmt.Printf("Batch update error: %v\n", err)
		return
	}
	fmt.Println("Batch update completed")

	// Получаем обновленные значения
	if value, exists := storage.GetMetric(ctx, "gauge1", models.Gauge); exists {
		fmt.Printf("Gauge1: %v\n", value)
	}

	if value, exists := storage.GetMetric(ctx, "counter2", models.Counter); exists {
		fmt.Printf("Counter2: %v\n", value)
	}

	// Вывод:
	// Batch update completed
	// Gauge1: 1.1
	// Counter2: 20
}

// ExampleGetMetricForJSON демонстрирует получение метрик в формате JSON.
func ExampleMemStorage_jsonFormat() {
	storage := NewMemStorage()
	ctx := context.Background()

	// Добавляем метрики
	storage.SetGauge(ctx, "json_gauge", 88.8)
	storage.IncrementCounter(ctx, "json_counter", 100)

	// Получаем метрики в JSON формате
	gaugeJSON := storage.GetMetricForJSON(ctx, "json_gauge", models.Gauge)
	counterJSON := storage.GetMetricForJSON(ctx, "json_counter", models.Counter)

	fmt.Printf("Gauge JSON - ID: %s, Type: %s\n", gaugeJSON.ID, gaugeJSON.MType)
	fmt.Printf("Counter JSON - ID: %s, Type: %s\n", counterJSON.ID, counterJSON.MType)

	// Вывод:
	// Gauge JSON - ID: json_gauge, Type: gauge
	// Counter JSON - ID: json_counter, Type: counter
}
