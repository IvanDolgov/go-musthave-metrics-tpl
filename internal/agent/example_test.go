package agent_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/agent"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

// MockSender - мок для примеров
type MockSender struct {
	metrics [][]models.Metrics
	mu      sync.Mutex
}

func (m *MockSender) SendMetricsBatch(ctx context.Context, metrics []models.Metrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	metricsCopy := make([]models.Metrics, len(metrics))
	copy(metricsCopy, metrics)
	m.metrics = append(m.metrics, metricsCopy)
	return nil
}

// MockLifecycle - мок для примеров
type MockLifecycle struct{}

func (m *MockLifecycle) Start() {}
func (m *MockLifecycle) Stop()  {}
func (m *MockLifecycle) Wait()  {}

func ExampleNewMetricsAgent() {
	// Создаем конфигурацию
	cfg := models.Config{
		PollInterval:   time.Second,
		ReportInterval: time.Second * 2,
		RateLimit:      5,
	}

	// Создаем моки
	sender := &MockSender{}
	lifecycle := &MockLifecycle{}

	// Создаем агента
	metricsAgent := agent.NewMetricsAgent(cfg, sender, lifecycle)

	// Запускаем агента
	metricsAgent.Start()
	defer metricsAgent.Stop()

	// Даем время на сбор метрик
	time.Sleep(time.Second * 3)

	// Останавливаем агента
	metricsAgent.Stop()
	metricsAgent.Wait()

	fmt.Println("Agent stopped")
	// Output: Agent stopped
}

func ExampleMetricsAgent_Start() {
	cfg := models.Config{
		PollInterval:   time.Millisecond * 100,
		ReportInterval: time.Millisecond * 200,
		RateLimit:      2,
	}

	sender := &MockSender{}
	lifecycle := &MockLifecycle{}

	metricsAgent := agent.NewMetricsAgent(cfg, sender, lifecycle)

	// Запускаем агента
	metricsAgent.Start()

	// Даем время поработать
	time.Sleep(time.Millisecond * 500)

	// Останавливаем
	metricsAgent.Stop()
	metricsAgent.Wait()

	fmt.Println("Agent started and stopped")
	// Output: Agent started and stopped
}
