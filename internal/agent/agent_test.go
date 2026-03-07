package agent

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

// MockMetricsSender - мок только для отправки
type MockMetricsSender struct {
	mu          sync.Mutex
	sentMetrics [][]models.Metrics
	shouldFail  bool
	failAfter   int
	callCount   int
}

func (m *MockMetricsSender) SendMetricsBatch(ctx context.Context, metrics []models.Metrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callCount++

	if m.shouldFail && (m.failAfter == 0 || m.callCount > m.failAfter) {
		return fmt.Errorf("mock send error")
	}

	metricsCopy := make([]models.Metrics, len(metrics))
	copy(metricsCopy, metrics)
	m.sentMetrics = append(m.sentMetrics, metricsCopy)

	return nil
}

// Вспомогательные методы для тестов
func (m *MockMetricsSender) GetSentMetrics() [][]models.Metrics {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([][]models.Metrics, len(m.sentMetrics))
	for i, metrics := range m.sentMetrics {
		metricsCopy := make([]models.Metrics, len(metrics))
		copy(metricsCopy, metrics)
		result[i] = metricsCopy
	}
	return result
}

func (m *MockMetricsSender) GetCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// MockLifecycle - мок для управления жизненным циклом
type MockLifecycle struct {
	started bool
	stopped bool
	waited  bool
	mu      sync.Mutex
}

func (m *MockLifecycle) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
}

func (m *MockLifecycle) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
}

func (m *MockLifecycle) Wait() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.waited = true
}

// NewMockMetricsSender создает новый мок для тестов
func NewMockMetricsSender(shouldFail bool) *MockMetricsSender {
	return &MockMetricsSender{
		sentMetrics: make([][]models.Metrics, 0),
		shouldFail:  shouldFail,
		failAfter:   0,
		callCount:   0,
	}
}

// TestNewMetricsAgent проверяет создание агента
func TestNewMetricsAgent(t *testing.T) {
	cfg := models.Config{
		PollInterval:   2 * time.Second,
		ReportInterval: 10 * time.Second,
		RateLimit:      2,
	}

	sender := NewMockMetricsSender(false)
	lifecycle := &MockLifecycle{}
	agent := NewMetricsAgent(cfg, sender, lifecycle)

	if agent == nil {
		t.Fatal("Agent should not be nil")
	}

	if agent.cfg.PollInterval != cfg.PollInterval {
		t.Errorf("Expected poll interval %v, got %v", cfg.PollInterval, agent.cfg.PollInterval)
	}

	if agent.sender != sender {
		t.Error("Agent should have correct sender")
	}

	if agent.lifecycle != lifecycle {
		t.Error("Agent should have correct lifecycle")
	}

	agent.cancel() // очистка контекста
}

// TestMetricsAgent_StartStop проверяет запуск и остановку агента
func TestMetricsAgent_StartStop(t *testing.T) {
	cfg := models.Config{
		PollInterval:   50 * time.Millisecond,
		ReportInterval: 100 * time.Millisecond,
		RateLimit:      1,
	}

	sender := NewMockMetricsSender(false)
	lifecycle := &MockLifecycle{}
	agent := NewMetricsAgent(cfg, sender, lifecycle)

	// Запускаем агент
	agent.Start()

	// Проверяем что lifecycle был запущен
	if !lifecycle.started {
		t.Error("Lifecycle should be started")
	}

	// Ждем немного для сбора метрик
	time.Sleep(150 * time.Millisecond)

	// Останавливаем агент
	agent.Stop()

	// Проверяем что lifecycle был остановлен
	if !lifecycle.stopped {
		t.Error("Lifecycle should be stopped")
	}

	// Даем время на graceful shutdown
	time.Sleep(50 * time.Millisecond)

	// Проверяем, что sender вызывался
	if sender.GetCallCount() == 0 {
		t.Error("Sender should have been called at least once")
	}
}

// TestMetricsAgent_getRuntimeMetrics проверяет сбор runtime метрик
func TestMetricsAgent_getRuntimeMetrics(t *testing.T) {
	cfg := models.Config{
		PollInterval:   2 * time.Second,
		ReportInterval: 10 * time.Second,
		RateLimit:      1,
	}

	sender := NewMockMetricsSender(false)
	lifecycle := &MockLifecycle{}
	agent := NewMetricsAgent(cfg, sender, lifecycle)

	// Собираем метрики с pollCount = 1
	metrics := agent.getRuntimeMetrics(1)

	// Проверяем, что метрики собраны
	if len(metrics) == 0 {
		t.Fatal("Should collect runtime metrics")
	}

	// Проверяем наличие ключевых метрик
	metricNames := make(map[string]bool)
	for _, m := range metrics {
		metricNames[m.ID] = true
	}

	// Проверяем обязательные метрики
	requiredMetrics := []string{"Alloc", "PollCount", "RandomValue"}
	for _, metric := range requiredMetrics {
		if !metricNames[metric] {
			t.Errorf("Should contain %s metric", metric)
		}
	}

	// Проверяем, что PollCount - counter
	var pollCountMetric *models.Metrics
	for i, m := range metrics {
		if m.ID == "PollCount" {
			pollCountMetric = &metrics[i]
			break
		}
	}

	if pollCountMetric == nil {
		t.Fatal("PollCount metric should exist")
	}

	if pollCountMetric.MType != "counter" {
		t.Error("PollCount should be counter type")
	}

	if pollCountMetric.Delta == nil {
		t.Error("PollCount should have Delta")
	} else if *pollCountMetric.Delta != 1 {
		t.Errorf("PollCount Delta should be 1, got %d", *pollCountMetric.Delta)
	}

	agent.cancel() // очистка
}

// TestMetricsAgent_getGopsutilMetrics проверяет сбор gopsutil метрик
func TestMetricsAgent_getGopsutilMetrics(t *testing.T) {
	cfg := models.Config{
		PollInterval:   2 * time.Second,
		ReportInterval: 10 * time.Second,
		RateLimit:      1,
	}

	sender := NewMockMetricsSender(false)
	lifecycle := &MockLifecycle{}
	agent := NewMetricsAgent(cfg, sender, lifecycle)

	// Собираем метрики
	metrics := agent.getGopsutilMetrics()

	// Метрики могут быть пустыми если gopsutil не работает в тестовой среде
	t.Logf("Collected %d gopsutil metrics", len(metrics))

	// Проверяем структуру метрик если они есть
	for _, metric := range metrics {
		if metric.ID == "" {
			t.Error("Metric should have ID")
		}
		if metric.MType != "gauge" {
			t.Errorf("Metric %s should be gauge type, got %s", metric.ID, metric.MType)
		}
		if metric.Value == nil {
			t.Errorf("Gauge metric %s should have Value", metric.ID)
		}
	}

	agent.cancel() // очистка
}

// TestMetricsAgent_getRandomValue проверяет генерацию случайного значения
func TestMetricsAgent_getRandomValue(t *testing.T) {
	cfg := models.Config{
		PollInterval:   2 * time.Second,
		ReportInterval: 10 * time.Second,
		RateLimit:      1,
	}

	sender := NewMockMetricsSender(false)
	lifecycle := &MockLifecycle{}
	agent := NewMetricsAgent(cfg, sender, lifecycle)

	// Устанавливаем pollCount
	agent.mu.Lock()
	agent.pollCount = 42
	agent.mu.Unlock()

	value := agent.getRandomValue()

	// Проверяем что значение в пределах 0-100
	if value < 0.0 || value > 100.0 {
		t.Errorf("Random value should be between 0 and 100, got %f", value)
	}

	// Для pollCount = 42, значение должно быть 42.0
	if value != 42.0 {
		t.Errorf("Expected random value 42.0, got %f", value)
	}

	// Проверяем для другого значения
	agent.mu.Lock()
	agent.pollCount = 150
	agent.mu.Unlock()

	value = agent.getRandomValue()
	expected := 50.0 // 150 % 100 = 50
	if value != expected {
		t.Errorf("Expected random value %f, got %f", expected, value)
	}

	agent.cancel() // очистка
}

// TestMetricsAgent_WorkerProcessing проверяет обработку метрик воркерами
func TestMetricsAgent_WorkerProcessing(t *testing.T) {
	cfg := models.Config{
		PollInterval:   2 * time.Second,
		ReportInterval: 10 * time.Second,
		RateLimit:      2,
	}

	sender := NewMockMetricsSender(false)
	lifecycle := &MockLifecycle{}
	agent := NewMetricsAgent(cfg, sender, lifecycle)

	// Запускаем воркеров
	for i := 0; i < int(cfg.RateLimit); i++ {
		agent.wg.Add(1)
		go agent.worker(i)
	}

	// Отправляем тестовые метрики в канал
	testMetrics := []models.Metrics{
		{ID: "test1", MType: "gauge", Value: floatPtr(1.0)},
		{ID: "test2", MType: "gauge", Value: floatPtr(2.0)},
	}

	// Отправляем несколько батчей
	for i := 0; i < 3; i++ {
		select {
		case agent.metricsChan <- testMetrics:
			// успешно отправлено
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Failed to send metrics to channel")
		}
	}

	// Даем время на обработку
	time.Sleep(200 * time.Millisecond)

	// Останавливаем контекст
	agent.cancel()

	// Ждем завершения воркеров с таймаутом
	done := make(chan struct{})
	go func() {
		agent.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// успешно
		t.Log("Workers stopped successfully")
	case <-time.After(500 * time.Millisecond):
		t.Error("Workers didn't stop in time")
	}

	// Проверяем что sender вызывался
	if sender.GetCallCount() == 0 {
		t.Error("Sender should have processed metrics")
	}
}

// TestMetricsAgent_ContextCancellation проверяет отмену по контексту
func TestMetricsAgent_ContextCancellation(t *testing.T) {
	cfg := models.Config{
		PollInterval:   100 * time.Millisecond,
		ReportInterval: 200 * time.Millisecond,
		RateLimit:      1,
	}

	sender := NewMockMetricsSender(false)
	lifecycle := &MockLifecycle{}
	agent := NewMetricsAgent(cfg, sender, lifecycle)

	// Запускаем агент
	agent.Start()

	// Немного ждем
	time.Sleep(50 * time.Millisecond)

	// Отменяем контекст
	agent.cancel()

	// Ждем остановки
	time.Sleep(100 * time.Millisecond)

	// Проверяем что контекст отменен
	select {
	case <-agent.ctx.Done():
		t.Log("Context cancelled successfully")
	default:
		t.Error("Context should be cancelled")
	}

	// Останавливаем агент
	agent.Stop()
}

// TestMetricsAgent_ConcurrentAccess проверяет конкурентный доступ к pollCount
func TestMetricsAgent_ConcurrentAccess(t *testing.T) {
	cfg := models.Config{
		PollInterval:   10 * time.Millisecond,
		ReportInterval: 20 * time.Millisecond,
		RateLimit:      5,
	}

	sender := NewMockMetricsSender(false)
	lifecycle := &MockLifecycle{}
	agent := NewMetricsAgent(cfg, sender, lifecycle)

	// Запускаем несколько горутин которые инкрементят pollCount
	const goroutines = 10
	const increments = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < increments; j++ {
				agent.mu.Lock()
				agent.pollCount++
				agent.mu.Unlock()
				time.Sleep(time.Microsecond)
			}
		}()
	}

	wg.Wait()

	// Проверяем итоговое значение
	expected := int64(goroutines * increments)
	agent.mu.RLock()
	actual := agent.pollCount
	agent.mu.RUnlock()

	if actual != expected {
		t.Errorf("Poll count should be %d, got %d", expected, actual)
	}

	agent.cancel() // очистка
}

// TestMetricsAgent_MetricStructure проверяет структуру метрик
func TestMetricsAgent_MetricStructure(t *testing.T) {
	cfg := models.Config{
		PollInterval:   2 * time.Second,
		ReportInterval: 10 * time.Second,
		RateLimit:      1,
	}

	sender := NewMockMetricsSender(false)
	lifecycle := &MockLifecycle{}
	agent := NewMetricsAgent(cfg, sender, lifecycle)

	// Получаем runtime метрики
	metrics := agent.getRuntimeMetrics(1)

	for _, metric := range metrics {
		// Проверяем обязательные поля
		if metric.ID == "" {
			t.Error("Metric should have ID")
		}

		if metric.MType == "" {
			t.Error("Metric should have type")
		}

		// В зависимости от типа проверяем соответствующее поле
		if metric.MType == "gauge" {
			if metric.Value == nil {
				t.Errorf("Gauge metric %s should have Value", metric.ID)
			}
			if metric.Delta != nil {
				t.Errorf("Gauge metric %s should not have Delta", metric.ID)
			}
		} else if metric.MType == "counter" {
			if metric.Delta == nil {
				t.Errorf("Counter metric %s should have Delta", metric.ID)
			}
			if metric.Value != nil {
				t.Errorf("Counter metric %s should not have Value", metric.ID)
			}
		}
	}

	agent.cancel() // очистка
}

// Вспомогательные функции
func floatPtr(f float64) *float64 {
	return &f
}

func int64Ptr(i int64) *int64 {
	return &i
}
