package storage

import (
	"context"
	"testing"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

func BenchmarkMemStorage_SetGauge(b *testing.B) {
	storage := NewMemStorage()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storage.SetGauge(ctx, "test", float64(i))
	}
}

func BenchmarkMemStorage_IncrementCounter(b *testing.B) {
	storage := NewMemStorage()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storage.IncrementCounter(ctx, "counter", 1)
	}
}

func BenchmarkMemStorage_GetAllMetrics(b *testing.B) {
	storage := NewMemStorage()
	ctx := context.Background()

	// Предварительно заполняем данными
	for i := 0; i < 1000; i++ {
		storage.SetGauge(ctx, "gauge"+string(rune('a'+i%26)), float64(i))
		storage.IncrementCounter(ctx, "counter"+string(rune('a'+i%26)), int64(i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storage.GetAllMetrics(ctx)
	}
}

func BenchmarkMemStorage_UpdateMetricsBatch(b *testing.B) {
	storage := NewMemStorage()
	ctx := context.Background()

	// Создаем батч метрик
	metrics := make([]models.Metrics, 100)
	for i := range metrics {
		val := float64(i)
		delta := int64(i)
		metrics[i] = models.Metrics{
			ID:    "metric" + string(rune('a'+i%26)),
			MType: "gauge",
			Value: &val,
		}
		if i%2 == 0 {
			metrics[i].MType = "counter"
			metrics[i].Delta = &delta
			metrics[i].Value = nil
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storage.UpdateMetricsBatch(ctx, metrics)
	}
}
