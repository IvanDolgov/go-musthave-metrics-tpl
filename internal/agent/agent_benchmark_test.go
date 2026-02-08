package agent

import (
	"context"
	"testing"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

// mockSender для тестов
type benchmarkMockSender struct{}

func (m *benchmarkMockSender) SendMetricsBatch(ctx context.Context, metrics []models.Metrics) error {
	// Возвращаем метрики в pool если используется
	if len(metrics) > 0 {
		for i := range metrics {
			metrics[i].Value = nil
			metrics[i].Delta = nil
		}
		metricsPool.Put(metrics[:0])
	}
	return nil
}

func BenchmarkGetRuntimeMetrics(b *testing.B) {
	cfg := models.Config{
		PollInterval:   2 * time.Second,
		ReportInterval: 10 * time.Second,
		RateLimit:      1,
	}

	agent := NewMetricsAgent(cfg, &benchmarkMockSender{})
	defer agent.cancel()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		metrics := agent.getRuntimeMetrics(int64(i))
		// Возвращаем в pool для чистоты теста
		for j := range metrics {
			metrics[j].Value = nil
			metrics[j].Delta = nil
		}
		metricsPool.Put(metrics[:0])
	}
}

func BenchmarkGetGopsutilMetrics(b *testing.B) {
	cfg := models.Config{
		PollInterval:   2 * time.Second,
		ReportInterval: 10 * time.Second,
		RateLimit:      1,
	}

	agent := NewMetricsAgent(cfg, &benchmarkMockSender{})
	defer agent.cancel()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		metrics := agent.getGopsutilMetrics()
		// Возвращаем в pool
		for j := range metrics {
			metrics[j].Value = nil
			metrics[j].Delta = nil
		}
		metricsPool.Put(metrics[:0])
	}
}

func BenchmarkAgentCollector(b *testing.B) {
	cfg := models.Config{
		PollInterval:   100 * time.Millisecond,
		ReportInterval: 200 * time.Millisecond,
		RateLimit:      1,
	}

	agent := NewMetricsAgent(cfg, &benchmarkMockSender{})
	defer agent.cancel()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		agent.pollCount++
		runtimeMetrics := agent.getRuntimeMetrics(agent.pollCount)
		gopsutilMetrics := agent.getGopsutilMetrics()

		// Возвращаем в pool
		allMetrics := append(runtimeMetrics, gopsutilMetrics...)
		for j := range allMetrics {
			allMetrics[j].Value = nil
			allMetrics[j].Delta = nil
		}
		metricsPool.Put(allMetrics[:0])
	}
}
