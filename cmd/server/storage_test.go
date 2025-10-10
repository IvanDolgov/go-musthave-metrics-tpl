package main

import (
	"testing"
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
