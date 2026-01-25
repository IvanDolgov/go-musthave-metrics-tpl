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
	for i := 0; i < b.N; i++ {
		_ = agent.getRuntimeMetrics(int64(i))
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
	for i := 0; i < b.N; i++ {
		_ = agent.getGopsutilMetrics()
	}
}

func BenchmarkAgentCollector(b *testing.B) {
	cfg := models.Config{
		PollInterval:   100 * time.Millisecond,
		ReportInterval: 200 * time.Millisecond,
		RateLimit:      1,
	}

	agent := NewMetricsAgent(cfg, &benchmarkMockSender{})

	b.ResetTimer()
	// Имитируем несколько циклов сбора метрик
	for i := 0; i < b.N; i++ {
		agent.pollCount++
		_ = agent.getRuntimeMetrics(agent.pollCount)
		_ = agent.getGopsutilMetrics()
	}
	agent.cancel()
}
