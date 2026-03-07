package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

// benchmarkMockSender - мок для бенчмарков
type benchmarkMockSender struct {
	mu        sync.Mutex
	sentCount int
}

func (m *benchmarkMockSender) SendMetricsBatch(ctx context.Context, metrics []models.Metrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentCount += len(metrics)
	return nil
}

func (m *benchmarkMockSender) Start() {}

func (m *benchmarkMockSender) Stop() {}

func (m *benchmarkMockSender) Wait() {}

func (m *benchmarkMockSender) GetSentCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sentCount
}

// BenchmarkMetricsAgent- бенчмарк для агента
func BenchmarkMetricsAgent(b *testing.B) {
	cfg := models.Config{
		PollInterval:   time.Millisecond,
		ReportInterval: time.Millisecond * 10,
		RateLimit:      5,
	}

	sender := &benchmarkMockSender{}
	agent := NewMetricsAgent(cfg, sender, sender)

	// Запускаем агента
	agent.Start()

	// Ждем немного для сбора метрик
	time.Sleep(100 * time.Millisecond)

	// Останавливаем агента
	agent.Stop()

	b.ReportMetric(float64(sender.GetSentCount()), "metrics_sent")
}

// BenchmarkMetricsAgentWithLoad - бенчмарк с нагрузкой
func BenchmarkMetricsAgentWithLoad(b *testing.B) {
	cfg := models.Config{
		PollInterval:   time.Millisecond,
		ReportInterval: time.Millisecond * 5,
		RateLimit:      10,
	}

	for i := 0; i < b.N; i++ {
		sender := &benchmarkMockSender{}
		agent := NewMetricsAgent(cfg, sender, sender)

		agent.Start()
		time.Sleep(50 * time.Millisecond)
		agent.Stop()
	}
}
