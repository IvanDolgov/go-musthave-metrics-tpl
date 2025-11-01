package main

import (
	"testing"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

func TestNewMemStorage(t *testing.T) {
	storage := NewMemStorage()

	if storage.gauges == nil {
		t.Error("Gauges map should be initialized")
	}

	if storage.counters == nil {
		t.Error("Counters map should be initialized")
	}

	if len(storage.gauges) != 0 {
		t.Error("Gauges map should be empty initially")
	}

	if len(storage.counters) != 0 {
		t.Error("Counters map should be empty initially")
	}
}

func TestSetGauge(t *testing.T) {
	storage := NewMemStorage()

	// тест назначение gauge
	storage.SetGauge("temperature", 25.5)

	if value, exists := storage.gauges["temperature"]; !exists {
		t.Error("Gauge 'temperature' should exist")
	} else if value != 25.5 {
		t.Errorf("Expected temperature 25.5, got %f", value)
	}

	// Test updating existing gauge
	storage.SetGauge("temperature", 30.0)

	if value := storage.gauges["temperature"]; value != 30.0 {
		t.Errorf("Expected updated temperature 30.0, got %f", value)
	}
}

func TestIncrementCounter(t *testing.T) {
	storage := NewMemStorage()

	// Test incrementing new counter
	storage.IncrementCounter("requests", 1)

	if value, exists := storage.counters["requests"]; !exists {
		t.Error("Counter 'requests' should exist")
	} else if value != 1 {
		t.Errorf("Expected requests counter 1, got %d", value)
	}

	// Test incrementing existing counter
	storage.IncrementCounter("requests", 5)

	if value := storage.counters["requests"]; value != 6 {
		t.Errorf("Expected requests counter 6, got %d", value)
	}

	// Test negative increment
	storage.IncrementCounter("errors", -2)

	if value := storage.counters["errors"]; value != -2 {
		t.Errorf("Expected errors counter -2, got %d", value)
	}
}

func TestGetAllMetrics(t *testing.T) {
	storage := NewMemStorage()

	// Add some test data
	storage.SetGauge("cpu_usage", 75.3)
	storage.SetGauge("memory_usage", 45.8)
	storage.IncrementCounter("requests", 10)
	storage.IncrementCounter("errors", 2)

	gauges, counters := storage.GetAllMetrics()

	// Test gauges
	if len(gauges) != 2 {
		t.Errorf("Expected 2 gauges, got %d", len(gauges))
	}

	if gauges["cpu_usage"] != 75.3 {
		t.Errorf("Expected cpu_usage 75.3, got %f", gauges["cpu_usage"])
	}

	if gauges["memory_usage"] != 45.8 {
		t.Errorf("Expected memory_usage 45.8, got %f", gauges["memory_usage"])
	}

	// Test counters
	if len(counters) != 2 {
		t.Errorf("Expected 2 counters, got %d", len(counters))
	}

	if counters["requests"] != 10 {
		t.Errorf("Expected requests 10, got %d", counters["requests"])
	}

	if counters["errors"] != 2 {
		t.Errorf("Expected errors 2, got %d", counters["errors"])
	}
}

// Тесты для метода GetMetric
func TestMemStorage_GetMetric(t *testing.T) {
	storage := NewMemStorage()

	// Заполняем тестовыми данными
	storage.SetGauge("temperature", 25.5)
	storage.SetGauge("memory", 1024.0)
	storage.IncrementCounter("requests", 10)
	storage.IncrementCounter("errors", 2)

	tests := []struct {
		name       string
		metricName string
		metricType models.MetricType
		wantValue  interface{}
		wantExists bool
	}{
		{
			name:       "Existing gauge metric",
			metricName: "temperature",
			metricType: models.Gauge,
			wantValue:  25.5,
			wantExists: true,
		},
		{
			name:       "Existing counter metric",
			metricName: "requests",
			metricType: models.Counter,
			wantValue:  int64(10),
			wantExists: true,
		},
		{
			name:       "Non-existing gauge metric",
			metricName: "nonexistent_gauge",
			metricType: models.Gauge,
			wantValue:  nil,
			wantExists: false,
		},
		{
			name:       "Non-existing counter metric",
			metricName: "nonexistent_counter",
			metricType: models.Counter,
			wantValue:  nil,
			wantExists: false,
		},
		{
			name:       "Wrong type for existing metric - gauge as counter",
			metricName: "temperature",
			metricType: models.Counter, // temperature is gauge, not counter
			wantValue:  nil,
			wantExists: false,
		},
		{
			name:       "Wrong type for existing metric - counter as gauge",
			metricName: "requests",
			metricType: models.Gauge, // requests is counter, not gauge
			wantValue:  nil,
			wantExists: false,
		},
		{
			name:       "Another existing gauge metric",
			metricName: "memory",
			metricType: models.Gauge,
			wantValue:  1024.0,
			wantExists: true,
		},
		{
			name:       "Another existing counter metric",
			metricName: "errors",
			metricType: models.Counter,
			wantValue:  int64(2),
			wantExists: true,
		},
		{
			name:       "Empty metric name",
			metricName: "",
			metricType: models.Gauge,
			wantValue:  nil,
			wantExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValue, gotExists := storage.GetMetric(tt.metricName, tt.metricType)

			if gotExists != tt.wantExists {
				t.Errorf("GetMetric() exists = %v, want %v", gotExists, tt.wantExists)
			}

			if gotExists {
				// Проверяем значение только если метрика существует
				switch tt.metricType {
				case models.Gauge:
					if gaugeVal, ok := gotValue.(float64); !ok || gaugeVal != tt.wantValue {
						t.Errorf("GetMetric() gauge value = %v, want %v", gotValue, tt.wantValue)
					}
				case models.Counter:
					if counterVal, ok := gotValue.(int64); !ok || counterVal != tt.wantValue {
						t.Errorf("GetMetric() counter value = %v, want %v", gotValue, tt.wantValue)
					}
				}
			} else {
				// Если метрика не существует, значение должно быть nil
				if gotValue != nil {
					t.Errorf("GetMetric() value = %v, want nil when not exists", gotValue)
				}
			}
		})
	}
}

// Тест для неизвестного типа метрики
func TestMemStorage_GetMetric_UnknownType(t *testing.T) {
	storage := NewMemStorage()
	storage.SetGauge("test_gauge", 1.0)
	storage.IncrementCounter("test_counter", 1)

	// Создаем невалидный тип метрики
	unknownType := models.MetricType("unknown")

	value, exists := storage.GetMetric("test_gauge", unknownType)
	if exists {
		t.Errorf("GetMetric() with unknown type should not exist, but exists = %v", exists)
	}
	if value != nil {
		t.Errorf("GetMetric() with unknown type should return nil, but got %v", value)
	}

	value, exists = storage.GetMetric("test_counter", unknownType)
	if exists {
		t.Errorf("GetMetric() with unknown type should not exist, but exists = %v", exists)
	}
	if value != nil {
		t.Errorf("GetMetric() with unknown type should return nil, but got %v", value)
	}
}

// Тест для проверки, что GetMetric не изменяет состояние хранилища
func TestMemStorage_GetMetric_NoSideEffects(t *testing.T) {
	storage := NewMemStorage()
	storage.SetGauge("test", 10.5)
	storage.IncrementCounter("count", 5)

	// Получаем метрики несколько раз
	val1, exists1 := storage.GetMetric("test", models.Gauge)
	val2, exists2 := storage.GetMetric("test", models.Gauge)
	val3, exists3 := storage.GetMetric("count", models.Counter)
	val4, exists4 := storage.GetMetric("count", models.Counter)

	// Проверяем, что значения consistent
	if val1 != val2 {
		t.Errorf("GetMetric() returned different values for same metric: %v vs %v", val1, val2)
	}
	if exists1 != exists2 {
		t.Errorf("GetMetric() returned different exists flags: %v vs %v", exists1, exists2)
	}
	if val3 != val4 {
		t.Errorf("GetMetric() returned different values for same metric: %v vs %v", val3, val4)
	}
	if exists3 != exists4 {
		t.Errorf("GetMetric() returned different exists flags: %v vs %v", exists3, exists4)
	}

	// Проверяем, что оригинальные значения не изменились
	gauge, counter := storage.GetAllMetrics()
	if gauge["test"] != 10.5 {
		t.Errorf("Original gauge value was modified")
	}
	if counter["count"] != 5 {
		t.Errorf("Original counter value was modified")
	}
}

// Тест для проверки типов возвращаемых значений
func TestMemStorage_GetMetric_TypeAssertions(t *testing.T) {
	storage := NewMemStorage()
	storage.SetGauge("gauge_metric", 3.14)
	storage.IncrementCounter("counter_metric", 42)

	// Проверяем gauge
	gaugeVal, exists := storage.GetMetric("gauge_metric", models.Gauge)
	if !exists {
		t.Fatal("Gauge metric should exist")
	}

	// Проверяем, что можно безопасно привести к float64
	if floatVal, ok := gaugeVal.(float64); !ok {
		t.Errorf("Gauge value should be float64, got %T", gaugeVal)
	} else if floatVal != 3.14 {
		t.Errorf("Gauge value = %v, want 3.14", floatVal)
	}

	// Проверяем counter
	counterVal, exists := storage.GetMetric("counter_metric", models.Counter)
	if !exists {
		t.Fatal("Counter metric should exist")
	}

	// Проверяем, что можно безопасно привести к int64
	if intVal, ok := counterVal.(int64); !ok {
		t.Errorf("Counter value should be int64, got %T", counterVal)
	} else if intVal != 42 {
		t.Errorf("Counter value = %v, want 42", intVal)
	}
}
