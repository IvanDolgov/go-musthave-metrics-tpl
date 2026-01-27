package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

// MockSender реализация MetricsSender для примеров.
type MockSender struct{}

func (m *MockSender) SendMetricsBatch(ctx context.Context, metrics []models.Metrics) error {
	fmt.Printf("Sent %d metrics\n", len(metrics))
	return nil
}

// ExampleMetricsAgent демонстрирует создание и запуск агента метрик.
func ExampleMetricsAgent() {
	// Конфигурация агента
	cfg := models.Config{
		PollInterval:   2 * time.Second,
		ReportInterval: 10 * time.Second,
		RateLimit:      5,
	}

	// Создаем mock отправитель
	sender := &MockSender{}

	// Создаем агент
	agent := NewMetricsAgent(cfg, sender)

	// Запускаем агент
	agent.Start()

	// Даем агенту поработать некоторое время
	fmt.Println("Agent started. Collecting metrics for 3 seconds...")
	time.Sleep(3 * time.Second)

	// Останавливаем агент
	agent.Stop()
	fmt.Println("Agent stopped")

	// Вывод (примерный):
	// Agent started. Collecting metrics for 3 seconds...
	// Sent 36 metrics
	// Sent 4 metrics
	// Agent stopped
}

// ExampleMetricsAgent_config демонстрирует различные конфигурации агента.
func ExampleMetricsAgent_config() {
	// Конфигурация для разработки (частый сбор метрик)
	devConfig := models.Config{
		PollInterval:   1 * time.Second,
		ReportInterval: 5 * time.Second,
		RateLimit:      10,
	}
	fmt.Printf("Dev config - Poll: %v, Report: %v, RateLimit: %d\n",
		devConfig.PollInterval, devConfig.ReportInterval, devConfig.RateLimit)

	// Конфигурация для продакшена (редкий сбор для экономии ресурсов)
	prodConfig := models.Config{
		PollInterval:   10 * time.Second,
		ReportInterval: 30 * time.Second,
		RateLimit:      2,
	}
	fmt.Printf("Prod config - Poll: %v, Report: %v, RateLimit: %d\n",
		prodConfig.PollInterval, prodConfig.ReportInterval, prodConfig.RateLimit)

	// Вывод:
	// Dev config - Poll: 1s, Report: 5s, RateLimit: 10
	// Prod config - Poll: 10s, Report: 30s, RateLimit: 2
}

// ExampleRuntimeMetrics демонстрирует сбор runtime метрик.
func Example_runtimeMetrics() {
	cfg := models.Config{
		PollInterval:   2 * time.Second,
		ReportInterval: 10 * time.Second,
		RateLimit:      5,
	}

	sender := &MockSender{}
	agent := NewMetricsAgent(cfg, sender)

	// Получаем runtime метрики
	metrics := agent.getRuntimeMetrics(1)
	fmt.Printf("Collected %d runtime metrics\n", len(metrics))

	// Показываем примеры метрик
	for i, metric := range metrics {
		if i < 3 { // Покажем первые 3 метрики
			fmt.Printf("Metric %d: %s (type: %s)\n", i+1, metric.ID, metric.MType)
		}
	}

	// Вывод (примерный):
	// Collected 36 runtime metrics
	// Metric 1: Alloc (type: gauge)
	// Metric 2: BuckHashSys (type: gauge)
	// Metric 3: Frees (type: gauge)
}

// ExampleGopsutilMetrics демонстрирует сбор системных метрик через gopsutil.
func Example_gopsutilMetrics() {
	cfg := models.Config{
		PollInterval:   2 * time.Second,
		ReportInterval: 10 * time.Second,
		RateLimit:      5,
	}

	sender := &MockSender{}
	agent := NewMetricsAgent(cfg, sender)

	// Получаем gopsutil метрики
	metrics := agent.getGopsutilMetrics()
	fmt.Printf("Collected %d gopsutil metrics\n", len(metrics))

	// Показываем типы собираемых метрик
	for _, metric := range metrics {
		fmt.Printf("System metric: %s (type: %s)\n", metric.ID, metric.MType)
	}

	// Вывод (примерный):
	// Collected 4 gopsutil metrics
	// System metric: TotalMemory (type: gauge)
	// System metric: FreeMemory (type: gauge)
	// System metric: CPUutilization1 (type: gauge)
	// System metric: CPUutilization2 (type: gauge)
}
