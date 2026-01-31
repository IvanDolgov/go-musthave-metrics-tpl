package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

// TestNewMemStorage тестирует создание нового хранилища
func TestNewMemStorage(t *testing.T) {
	storage := NewMemStorage()

	if storage == nil {
		t.Fatal("NewMemStorage should not return nil")
	}

	if storage.gauges == nil {
		t.Error("gauges map should be initialized")
	}

	if storage.counters == nil {
		t.Error("counters map should be initialized")
	}

	// Проверяем что хранилище пустое
	ctx := context.Background()
	gauges, counters := storage.GetAllMetrics(ctx)

	if len(gauges) != 0 {
		t.Errorf("New storage should have 0 gauges, got %d", len(gauges))
	}

	if len(counters) != 0 {
		t.Errorf("New storage should have 0 counters, got %d", len(counters))
	}
}

// TestMemStorage_SetGauge тестирует установку gauge метрик
func TestMemStorage_SetGauge(t *testing.T) {
	ctx := context.Background()
	storage := NewMemStorage()

	t.Run("Set new gauge", func(t *testing.T) {
		storage.SetGauge(ctx, "cpu_usage", 75.5)

		value, exists := storage.GetMetric(ctx, "cpu_usage", models.Gauge)
		if !exists {
			t.Error("Gauge should exist after setting")
		}

		if value != 75.5 {
			t.Errorf("Expected gauge value 75.5, got %v", value)
		}
	})

	t.Run("Update existing gauge", func(t *testing.T) {
		storage.SetGauge(ctx, "cpu_usage", 80.0)

		value, exists := storage.GetMetric(ctx, "cpu_usage", models.Gauge)
		if !exists {
			t.Error("Gauge should exist")
		}

		if value != 80.0 {
			t.Errorf("Expected updated gauge value 80.0, got %v", value)
		}
	})

	t.Run("Set gauge with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		storage.SetGauge(ctx, "should_not_be_set", 1.0)

		// Проверяем, что метрика не была добавлена при отмененном контексте
		_, exists := storage.GetMetric(context.Background(), "should_not_be_set", models.Gauge)
		if exists {
			t.Error("Metric should not be set with cancelled context")
		}
	})
}

// TestMemStorage_IncrementCounter тестирует инкремент counter метрик
func TestMemStorage_IncrementCounter(t *testing.T) {
	ctx := context.Background()
	storage := NewMemStorage()

	t.Run("Increment new counter", func(t *testing.T) {
		storage.IncrementCounter(ctx, "requests", 1)

		value, exists := storage.GetMetric(ctx, "requests", models.Counter)
		if !exists {
			t.Error("Counter should exist after incrementing")
		}

		if value != int64(1) {
			t.Errorf("Expected counter value 1, got %v", value)
		}
	})

	t.Run("Increment existing counter multiple times", func(t *testing.T) {
		storage.IncrementCounter(ctx, "requests", 5)
		storage.IncrementCounter(ctx, "requests", 3)

		value, exists := storage.GetMetric(ctx, "requests", models.Counter)
		if !exists {
			t.Error("Counter should exist")
		}

		// 1 + 5 + 3 = 9
		if value != int64(9) {
			t.Errorf("Expected counter value 9, got %v", value)
		}
	})

	t.Run("Increment with negative value", func(t *testing.T) {
		storage.IncrementCounter(ctx, "errors", -2)

		value, exists := storage.GetMetric(ctx, "errors", models.Counter)
		if !exists {
			t.Error("Counter should exist")
		}

		if value != int64(-2) {
			t.Errorf("Expected counter value -2, got %v", value)
		}
	})

	t.Run("Increment counter with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		storage.IncrementCounter(ctx, "cancelled_counter", 1)

		// Проверяем, что метрика не была добавлена при отмененном контексте
		_, exists := storage.GetMetric(context.Background(), "cancelled_counter", models.Counter)
		if exists {
			t.Error("Counter should not be incremented with cancelled context")
		}
	})
}

// TestMemStorage_GetMetric тестирует получение метрик
func TestMemStorage_GetMetric(t *testing.T) {
	ctx := context.Background()
	storage := NewMemStorage()

	storage.SetGauge(ctx, "test_gauge", 42.5)
	storage.IncrementCounter(ctx, "test_counter", 10)

	t.Run("Get existing gauge", func(t *testing.T) {
		value, exists := storage.GetMetric(ctx, "test_gauge", models.Gauge)

		if !exists {
			t.Error("Existing gauge should be found")
		}

		if value != 42.5 {
			t.Errorf("Expected gauge value 42.5, got %v", value)
		}
	})

	t.Run("Get existing counter", func(t *testing.T) {
		value, exists := storage.GetMetric(ctx, "test_counter", models.Counter)

		if !exists {
			t.Error("Existing counter should be found")
		}

		if value != int64(10) {
			t.Errorf("Expected counter value 10, got %v", value)
		}
	})

	t.Run("Get non-existent gauge", func(t *testing.T) {
		value, exists := storage.GetMetric(ctx, "non_existent_gauge", models.Gauge)

		if exists {
			t.Error("Non-existent gauge should not be found")
		}

		if value != nil {
			t.Errorf("Expected nil for non-existent gauge, got %v", value)
		}
	})

	t.Run("Get non-existent counter", func(t *testing.T) {
		value, exists := storage.GetMetric(ctx, "non_existent_counter", models.Counter)

		if exists {
			t.Error("Non-existent counter should not be found")
		}

		if value != nil {
			t.Errorf("Expected nil for non-existent counter, got %v", value)
		}
	})

	t.Run("Get with unknown metric type", func(t *testing.T) {
		value, exists := storage.GetMetric(ctx, "test_gauge", models.MetricType("unknown"))

		if exists {
			t.Error("Should not exist for unknown type")
		}

		if value != nil {
			t.Errorf("Expected nil for unknown type, got %v", value)
		}
	})

	t.Run("Get with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		value, exists := storage.GetMetric(ctx, "test_gauge", models.Gauge)

		if exists {
			t.Error("Should not exist with cancelled context")
		}

		if value != nil {
			t.Errorf("Expected nil with cancelled context, got %v", value)
		}
	})
}

// TestMemStorage_GetAllMetrics тестирует получение всех метрик
func TestMemStorage_GetAllMetrics(t *testing.T) {
	ctx := context.Background()

	t.Run("Empty storage", func(t *testing.T) {
		storage := NewMemStorage()
		gauges, counters := storage.GetAllMetrics(ctx)

		if len(gauges) != 0 {
			t.Errorf("Empty storage should have 0 gauges, got %d", len(gauges))
		}

		if len(counters) != 0 {
			t.Errorf("Empty storage should have 0 counters, got %d", len(counters))
		}
	})

	t.Run("With metrics", func(t *testing.T) {
		storage := NewMemStorage()
		storage.SetGauge(ctx, "gauge1", 1.0)
		storage.SetGauge(ctx, "gauge2", 2.0)
		storage.IncrementCounter(ctx, "counter1", 10)
		storage.IncrementCounter(ctx, "counter2", 20)

		gauges, counters := storage.GetAllMetrics(ctx)

		if len(gauges) != 2 {
			t.Errorf("Expected 2 gauges, got %d", len(gauges))
		}

		if len(counters) != 2 {
			t.Errorf("Expected 2 counters, got %d", len(counters))
		}

		if gauges["gauge1"] != 1.0 {
			t.Errorf("Expected gauge1 = 1.0, got %v", gauges["gauge1"])
		}

		if counters["counter1"] != 10 {
			t.Errorf("Expected counter1 = 10, got %v", counters["counter1"])
		}
	})

	t.Run("GetAllMetrics with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		storage := NewMemStorage()
		storage.SetGauge(ctx, "test_gauge", 1.0)

		// Создаем новый контекст для проверки, так как оригинальный отменен
		checkCtx := context.Background()
		gauges, counters := storage.GetAllMetrics(checkCtx)

		if len(gauges) != 1 {
			t.Errorf("Expected 1 gauge, got %d", len(gauges))
		}

		if len(counters) != 0 {
			t.Errorf("Expected 0 counters, got %d", len(counters))
		}
	})

	t.Run("GetAllMetrics returns correct values after updates", func(t *testing.T) {
		storage := NewMemStorage()
		storage.SetGauge(ctx, "test_gauge", 99.9)

		gauges1, _ := storage.GetAllMetrics(ctx)

		// Обновляем значение
		storage.SetGauge(ctx, "test_gauge", 100.0)

		gauges2, _ := storage.GetAllMetrics(ctx)

		if gauges1["test_gauge"] != 99.9 {
			t.Errorf("First call: expected 99.9, got %v", gauges1["test_gauge"])
		}

		if gauges2["test_gauge"] != 100.0 {
			t.Errorf("Second call: expected 100.0, got %v", gauges2["test_gauge"])
		}
	})
}

// TestMemStorage_GetMetricForJSON тестирует получение метрик для JSON
func TestMemStorage_GetMetricForJSON(t *testing.T) {
	ctx := context.Background()
	storage := NewMemStorage()

	storage.SetGauge(ctx, "json_gauge", 42.5)
	storage.IncrementCounter(ctx, "json_counter", 100)

	t.Run("Get gauge for JSON", func(t *testing.T) {
		metric := storage.GetMetricForJSON(ctx, "json_gauge", models.Gauge)

		if metric.ID != "json_gauge" {
			t.Errorf("Expected ID 'json_gauge', got %s", metric.ID)
		}

		if metric.MType != "gauge" {
			t.Errorf("Expected MType 'gauge', got %s", metric.MType)
		}

		if metric.Value == nil || *metric.Value != 42.5 {
			t.Errorf("Expected Value 42.5, got %v", metric.Value)
		}

		if metric.Delta != nil {
			t.Errorf("Expected Delta nil for gauge, got %v", metric.Delta)
		}
	})

	t.Run("Get counter for JSON", func(t *testing.T) {
		metric := storage.GetMetricForJSON(ctx, "json_counter", models.Counter)

		if metric.ID != "json_counter" {
			t.Errorf("Expected ID 'json_counter', got %s", metric.ID)
		}

		if metric.MType != "counter" {
			t.Errorf("Expected MType 'counter', got %s", metric.MType)
		}

		if metric.Delta == nil || *metric.Delta != 100 {
			t.Errorf("Expected Delta 100, got %v", metric.Delta)
		}

		if metric.Value != nil {
			t.Errorf("Expected Value nil for counter, got %v", metric.Value)
		}
	})

	t.Run("Get non-existent metric for JSON", func(t *testing.T) {
		metric := storage.GetMetricForJSON(ctx, "non_existent", models.Gauge)

		if metric.ID != "" {
			t.Errorf("Expected empty ID for non-existent metric, got %s", metric.ID)
		}

		if metric.Value != nil {
			t.Errorf("Expected nil Value for non-existent metric, got %v", metric.Value)
		}

		if metric.Delta != nil {
			t.Errorf("Expected nil Delta for non-existent metric, got %v", metric.Delta)
		}
	})

	t.Run("Get metric for JSON with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		metric := storage.GetMetricForJSON(ctx, "json_gauge", models.Gauge)

		// При отмененном контексте должна вернуться пустая структура
		if metric.ID != "" {
			t.Errorf("Expected empty ID with cancelled context, got %s", metric.ID)
		}
	})
}

// TestMemStorage_SaveToFile тестирует сохранение в файл
func TestMemStorage_SaveToFile(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	t.Run("Save metrics to file", func(t *testing.T) {
		storage := NewMemStorage()
		filename := filepath.Join(tmpDir, "save_test.json")

		storage.SetGauge(ctx, "save_gauge", 99.9)
		storage.IncrementCounter(ctx, "save_counter", 50)

		err := storage.SaveToFile(ctx, filename)
		if err != nil {
			t.Fatalf("SaveToFile failed: %v", err)
		}

		// Проверяем что файл создан
		if _, err := os.Stat(filename); os.IsNotExist(err) {
			t.Error("File should be created")
		}

		// Проверяем содержимое файла
		data, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}

		var metrics []models.FileMetric
		if err := json.Unmarshal(data, &metrics); err != nil {
			t.Fatalf("Failed to unmarshal file: %v", err)
		}

		if len(metrics) != 2 {
			t.Errorf("Expected 2 metrics in file, got %d", len(metrics))
		}

		// Ищем gauge и counter
		foundGauge, foundCounter := false, false
		for _, m := range metrics {
			if m.ID == "save_gauge" && m.Type == "gauge" && m.Value != nil && *m.Value == 99.9 {
				foundGauge = true
			}
			if m.ID == "save_counter" && m.Type == "counter" && m.Delta != nil && *m.Delta == 50 {
				foundCounter = true
			}
		}

		if !foundGauge {
			t.Error("Gauge not found in saved file")
		}
		if !foundCounter {
			t.Error("Counter not found in saved file")
		}
	})

	t.Run("Save empty storage", func(t *testing.T) {
		storage := NewMemStorage()
		filename := filepath.Join(tmpDir, "empty_save.json")

		err := storage.SaveToFile(ctx, filename)
		if err != nil {
			t.Fatalf("SaveToFile failed for empty storage: %v", err)
		}

		data, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}

		var metrics []models.FileMetric
		if err := json.Unmarshal(data, &metrics); err != nil {
			t.Fatalf("Failed to unmarshal file: %v", err)
		}

		if len(metrics) != 0 {
			t.Errorf("Expected 0 metrics in empty file, got %d", len(metrics))
		}
	})

	t.Run("Save with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		storage := NewMemStorage()
		filename := filepath.Join(tmpDir, "cancelled_save.json")

		err := storage.SaveToFile(ctx, filename)
		if err == nil {
			t.Error("SaveToFile should fail with cancelled context")
		}
	})

	t.Run("Save to invalid path", func(t *testing.T) {
		storage := NewMemStorage()

		// Пытаемся сохранить в несуществующую директорию
		err := storage.SaveToFile(ctx, "/invalid/path/metrics.json")
		if err == nil {
			t.Error("SaveToFile should fail for invalid path")
		}
	})
}

// TestMemStorage_LoadFromFile тестирует загрузку из файла
func TestMemStorage_LoadFromFile(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	t.Run("Load metrics from file", func(t *testing.T) {
		// Сначала создаем файл с данными
		filename := filepath.Join(tmpDir, "load_test.json")
		metrics := []models.FileMetric{
			{
				ID:    "load_gauge",
				Type:  "gauge",
				Value: func() *float64 { v := 88.8; return &v }(),
			},
			{
				ID:    "load_counter",
				Type:  "counter",
				Delta: func() *int64 { v := int64(75); return &v }(),
			},
		}

		data, err := json.Marshal(metrics)
		if err != nil {
			t.Fatalf("Failed to marshal test data: %v", err)
		}

		if err := os.WriteFile(filename, data, 0644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}

		// Теперь загружаем
		storage := NewMemStorage()
		err = storage.LoadFromFile(ctx, filename)
		if err != nil {
			t.Fatalf("LoadFromFile failed: %v", err)
		}

		gauges, counters := storage.GetAllMetrics(ctx)

		if gauges["load_gauge"] != 88.8 {
			t.Errorf("Expected load_gauge = 88.8, got %v", gauges["load_gauge"])
		}

		if counters["load_counter"] != 75 {
			t.Errorf("Expected load_counter = 75, got %v", counters["load_counter"])
		}
	})

	t.Run("Load from non-existent file", func(t *testing.T) {
		storage := NewMemStorage()

		err := storage.LoadFromFile(ctx, filepath.Join(tmpDir, "non_existent.json"))
		if err != nil {
			t.Errorf("LoadFromFile should not fail for non-existent file, got %v", err)
		}

		// Хранилище должно остаться пустым
		gauges, counters := storage.GetAllMetrics(ctx)
		if len(gauges) != 0 || len(counters) != 0 {
			t.Error("Storage should be empty after loading non-existent file")
		}
	})

	t.Run("Load corrupted JSON", func(t *testing.T) {
		filename := filepath.Join(tmpDir, "corrupted.json")

		// Пишем поврежденный JSON
		if err := os.WriteFile(filename, []byte("{invalid json}"), 0644); err != nil {
			t.Fatalf("Failed to write corrupted file: %v", err)
		}

		storage := NewMemStorage()
		err := storage.LoadFromFile(ctx, filename)
		if err == nil {
			t.Error("LoadFromFile should fail for corrupted JSON")
		}
	})

	t.Run("Load with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		storage := NewMemStorage()
		err := storage.LoadFromFile(ctx, "anyfile.json")
		if err == nil {
			t.Error("LoadFromFile should fail with cancelled context")
		}
	})

	t.Run("Load file with nil values", func(t *testing.T) {
		filename := filepath.Join(tmpDir, "nil_values.json")
		metrics := []models.FileMetric{
			{
				ID:    "nil_gauge",
				Type:  "gauge",
				Value: nil,
			},
			{
				ID:    "nil_counter",
				Type:  "counter",
				Delta: nil,
			},
		}

		data, err := json.Marshal(metrics)
		if err != nil {
			t.Fatalf("Failed to marshal test data: %v", err)
		}

		if err := os.WriteFile(filename, data, 0644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}

		storage := NewMemStorage()
		err = storage.LoadFromFile(ctx, filename)
		if err != nil {
			t.Fatalf("LoadFromFile failed: %v", err)
		}

		// Метрики с nil значениями не должны загружаться
		gauges, counters := storage.GetAllMetrics(ctx)
		if _, exists := gauges["nil_gauge"]; exists {
			t.Error("Gauge with nil value should not be loaded")
		}
		if _, exists := counters["nil_counter"]; exists {
			t.Error("Counter with nil delta should not be loaded")
		}
	})
}

// TestMemStorage_UpdateMetricsBatch тестирует батчевое обновление
func TestMemStorage_UpdateMetricsBatch(t *testing.T) {
	ctx := context.Background()

	t.Run("Empty batch", func(t *testing.T) {
		storage := NewMemStorage()

		err := storage.UpdateMetricsBatch(ctx, []models.Metrics{})
		if err != nil {
			t.Errorf("Empty batch should not return error, got %v", err)
		}
	})

	t.Run("Batch with gauges", func(t *testing.T) {
		storage := NewMemStorage()

		val1 := 1.1
		val2 := 2.2
		metrics := []models.Metrics{
			{ID: "batch_gauge1", MType: "gauge", Value: &val1},
			{ID: "batch_gauge2", MType: "gauge", Value: &val2},
		}

		err := storage.UpdateMetricsBatch(ctx, metrics)
		if err != nil {
			t.Errorf("Batch update failed: %v", err)
		}

		gauges, _ := storage.GetAllMetrics(ctx)
		if gauges["batch_gauge1"] != 1.1 {
			t.Errorf("Expected batch_gauge1 = 1.1, got %v", gauges["batch_gauge1"])
		}
		if gauges["batch_gauge2"] != 2.2 {
			t.Errorf("Expected batch_gauge2 = 2.2, got %v", gauges["batch_gauge2"])
		}
	})

	t.Run("Batch with counters", func(t *testing.T) {
		storage := NewMemStorage()

		delta1 := int64(10)
		delta2 := int64(20)
		metrics := []models.Metrics{
			{ID: "batch_counter1", MType: "counter", Delta: &delta1},
			{ID: "batch_counter2", MType: "counter", Delta: &delta2},
		}

		err := storage.UpdateMetricsBatch(ctx, metrics)
		if err != nil {
			t.Errorf("Batch update failed: %v", err)
		}

		_, counters := storage.GetAllMetrics(ctx)
		if counters["batch_counter1"] != 10 {
			t.Errorf("Expected batch_counter1 = 10, got %v", counters["batch_counter1"])
		}
		if counters["batch_counter2"] != 20 {
			t.Errorf("Expected batch_counter2 = 20, got %v", counters["batch_counter2"])
		}
	})

	t.Run("Batch with mixed metrics", func(t *testing.T) {
		storage := NewMemStorage()

		val := 3.3
		delta := int64(30)
		metrics := []models.Metrics{
			{ID: "mixed_gauge", MType: "gauge", Value: &val},
			{ID: "mixed_counter", MType: "counter", Delta: &delta},
		}

		err := storage.UpdateMetricsBatch(ctx, metrics)
		if err != nil {
			t.Errorf("Batch update failed: %v", err)
		}

		gauges, counters := storage.GetAllMetrics(ctx)
		if gauges["mixed_gauge"] != 3.3 {
			t.Errorf("Expected mixed_gauge = 3.3, got %v", gauges["mixed_gauge"])
		}
		if counters["mixed_counter"] != 30 {
			t.Errorf("Expected mixed_counter = 30, got %v", counters["mixed_counter"])
		}
	})

	t.Run("Batch updates existing metrics", func(t *testing.T) {
		storage := NewMemStorage()

		// Сначала устанавливаем значения
		storage.SetGauge(ctx, "existing_gauge", 1.0)
		storage.IncrementCounter(ctx, "existing_counter", 5)

		// Затем обновляем батчем
		newVal := 2.0
		newDelta := int64(3)
		metrics := []models.Metrics{
			{ID: "existing_gauge", MType: "gauge", Value: &newVal},
			{ID: "existing_counter", MType: "counter", Delta: &newDelta},
		}

		err := storage.UpdateMetricsBatch(ctx, metrics)
		if err != nil {
			t.Errorf("Batch update failed: %v", err)
		}

		gauges, counters := storage.GetAllMetrics(ctx)
		if gauges["existing_gauge"] != 2.0 {
			t.Errorf("Expected existing_gauge = 2.0, got %v", gauges["existing_gauge"])
		}
		if counters["existing_counter"] != 8 { // 5 + 3 = 8
			t.Errorf("Expected existing_counter = 8, got %v", counters["existing_counter"])
		}
	})

	t.Run("Batch with nil values", func(t *testing.T) {
		storage := NewMemStorage()

		metrics := []models.Metrics{
			{ID: "nil_gauge", MType: "gauge", Value: nil},
			{ID: "nil_counter", MType: "counter", Delta: nil},
		}

		err := storage.UpdateMetricsBatch(ctx, metrics)
		if err != nil {
			t.Errorf("Batch with nil values should not fail, got %v", err)
		}

		// Метрики с nil значениями не должны быть добавлены
		gauges, counters := storage.GetAllMetrics(ctx)
		if _, exists := gauges["nil_gauge"]; exists {
			t.Error("Gauge with nil value should not be added")
		}
		if _, exists := counters["nil_counter"]; exists {
			t.Error("Counter with nil delta should not be added")
		}
	})

	t.Run("Batch with cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		storage := NewMemStorage()
		val := 1.0
		metrics := []models.Metrics{
			{ID: "cancelled_gauge", MType: "gauge", Value: &val},
		}

		err := storage.UpdateMetricsBatch(ctx, metrics)
		if err == nil {
			t.Error("UpdateMetricsBatch should fail with cancelled context")
		}
	})
}

// TestMemStorage_ConcurrentAccess тестирует конкурентный доступ
func TestMemStorage_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	storage := NewMemStorage()

	const goroutines = 10
	const operations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				// Чередуем операции записи и чтения
				if j%2 == 0 {
					storage.SetGauge(ctx, "concurrent_gauge", float64(id+j))
					storage.IncrementCounter(ctx, "concurrent_counter", int64(id+j))
				} else {
					storage.GetMetric(ctx, "concurrent_gauge", models.Gauge)
					storage.GetMetric(ctx, "concurrent_counter", models.Counter)
					storage.GetAllMetrics(ctx)
				}
			}
		}(i)
	}

	wg.Wait()

	// Проверяем что не было паники и данные корректны
	gauges, counters := storage.GetAllMetrics(ctx)

	// Должен быть хотя бы один gauge и counter
	if len(gauges) == 0 {
		t.Error("Expected at least one gauge after concurrent operations")
	}

	if len(counters) == 0 {
		t.Error("Expected at least one counter after concurrent operations")
	}
}

// TestMemStorage_EdgeCases тестирует граничные случаи
func TestMemStorage_EdgeCases(t *testing.T) {
	ctx := context.Background()
	storage := NewMemStorage()

	t.Run("Empty metric names", func(t *testing.T) {
		storage.SetGauge(ctx, "", 1.0)
		storage.IncrementCounter(ctx, "", 1)

		gauges, counters := storage.GetAllMetrics(ctx)

		// Проверяем что метрики с пустыми именами сохраняются
		if val, exists := gauges[""]; !exists || val != 1.0 {
			t.Errorf("Gauge with empty name should be stored, got %v, exists: %v", val, exists)
		}

		if val, exists := counters[""]; !exists || val != 1 {
			t.Errorf("Counter with empty name should be stored, got %v, exists: %v", val, exists)
		}
	})

	t.Run("Overwrite with same value", func(t *testing.T) {
		storage.SetGauge(ctx, "same_gauge", 5.0)
		storage.SetGauge(ctx, "same_gauge", 5.0) // То же значение

		value, _ := storage.GetMetric(ctx, "same_gauge", models.Gauge)
		if value != 5.0 {
			t.Errorf("Expected 5.0 after overwrite with same value, got %v", value)
		}
	})

	t.Run("Zero values", func(t *testing.T) {
		storage.SetGauge(ctx, "zero_gauge", 0.0)
		storage.IncrementCounter(ctx, "zero_counter", 0)

		gauge, _ := storage.GetMetric(ctx, "zero_gauge", models.Gauge)
		counter, _ := storage.GetMetric(ctx, "zero_counter", models.Counter)

		if gauge != 0.0 {
			t.Errorf("Expected zero gauge, got %v", gauge)
		}
		if counter != int64(0) {
			t.Errorf("Expected zero counter, got %v", counter)
		}
	})
}

// TestMemStorage_InterfaceCompliance проверяет соответствие интерфейсу
func TestMemStorage_InterfaceCompliance(t *testing.T) {
	var _ Storage = (*MemStorage)(nil)
	t.Log("MemStorage correctly implements Storage interface")
}
