package agent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

// MockMetricsSender для тестирования
type MockMetricsSender struct {
	sendCalled  int32
	lastMetrics []models.Metrics
	shouldFail  bool
	blockOnSend chan struct{}
	sendError   error
	mu          sync.RWMutex
}

func NewMockMetricsSender(shouldFail bool) *MockMetricsSender {
	return &MockMetricsSender{
		shouldFail:  shouldFail,
		blockOnSend: make(chan struct{}),
	}
}

func (m *MockMetricsSender) SendMetricsBatch(ctx context.Context, metrics []models.Metrics) error {
	atomic.AddInt32(&m.sendCalled, 1)

	m.mu.Lock()
	m.lastMetrics = metrics
	m.mu.Unlock()

	if m.blockOnSend != nil {
		select {
		case <-m.blockOnSend:
			// разблокировано
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if m.shouldFail {
		if m.sendError != nil {
			return m.sendError
		}
		return errors.New("mock send error")
	}

	return nil
}

func (m *MockMetricsSender) GetSendCount() int {
	return int(atomic.LoadInt32(&m.sendCalled))
}

func (m *MockMetricsSender) GetLastMetrics() []models.Metrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastMetrics
}

func (m *MockMetricsSender) Unblock() {
	close(m.blockOnSend)
}

// TestNewMetricsAgent проверяет создание агента
func TestNewMetricsAgent(t *testing.T) {
	cfg := models.Config{
		PollInterval:   2 * time.Second,
		ReportInterval: 10 * time.Second,
		RateLimit:      2,
	}

	sender := NewMockMetricsSender(false)
	agent := NewMetricsAgent(cfg, sender)

	if agent == nil {
		t.Fatal("Agent should not be nil")
	}

	if agent.cfg.PollInterval != cfg.PollInterval {
		t.Errorf("Expected poll interval %v, got %v", cfg.PollInterval, agent.cfg.PollInterval)
	}

	if agent.sender != sender {
		t.Error("Agent should have correct sender")
	}

	agent.cancel() // очистка контекста
}

// TestMetricsAgent_StartStop проверяет запуск и остановку агента
func TestMetricsAgent_StartStop(t *testing.T) {
	cfg := models.Config{
		PollInterval:   50 * time.Millisecond, // очень маленькие интервалы для тестов
		ReportInterval: 100 * time.Millisecond,
		RateLimit:      1,
	}

	sender := NewMockMetricsSender(false)
	agent := NewMetricsAgent(cfg, sender)

	// Запускаем агент
	agent.Start()

	// Ждем немного для сбора метрик
	time.Sleep(150 * time.Millisecond)

	// Останавливаем агент
	agent.Stop()

	// Даем время на graceful shutdown
	time.Sleep(50 * time.Millisecond)

	// Проверяем, что sender вызывался
	if sender.GetSendCount() == 0 {
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
	agent := NewMetricsAgent(cfg, sender)

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
	agent := NewMetricsAgent(cfg, sender)

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
	agent := NewMetricsAgent(cfg, sender)

	// Устанавливаем pollCount
	agent.pollCount = 42
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
	agent.pollCount = 150
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
	agent := NewMetricsAgent(cfg, sender)

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
	if sender.GetSendCount() == 0 {
		t.Error("Sender should have processed metrics")
	}
}

// TestMetricsAgent_WorkerErrorHandling проверяет обработку ошибок воркерами
func TestMetricsAgent_WorkerErrorHandling(t *testing.T) {
	cfg := models.Config{
		PollInterval:   2 * time.Second,
		ReportInterval: 10 * time.Second,
		RateLimit:      1,
	}

	// Sender который всегда падает
	sender := NewMockMetricsSender(true)
	agent := NewMetricsAgent(cfg, sender)

	// Разблокируем sender сразу
	sender.Unblock()

	// Запускаем воркер
	agent.wg.Add(1)
	go agent.worker(0)

	// Отправляем метрики
	testMetrics := []models.Metrics{
		{ID: "test", MType: "gauge", Value: floatPtr(1.0)},
	}

	agent.metricsChan <- testMetrics

	// Даем время на обработку
	time.Sleep(100 * time.Millisecond)

	// Останавливаем
	agent.cancel()

	// Ждем завершения воркера
	done := make(chan struct{})
	go func() {
		agent.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// успешно
	case <-time.After(500 * time.Millisecond):
		t.Error("Worker didn't stop in time")
	}

	// Проверяем что sender вызывался несмотря на ошибку
	if sender.GetSendCount() != 1 {
		t.Errorf("Sender should have been called once, got %d", sender.GetSendCount())
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
	agent := NewMetricsAgent(cfg, sender)

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
	agent := NewMetricsAgent(cfg, sender)

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

// Вспомогательная функция
func floatPtr(f float64) *float64 {
	return &f
}

func int64Ptr(i int64) *int64 {
	return &i
}

// TestMetricsAgent_MetricStructure проверяет структуру метрик
func TestMetricsAgent_MetricStructure(t *testing.T) {
	cfg := models.Config{
		PollInterval:   2 * time.Second,
		ReportInterval: 10 * time.Second,
		RateLimit:      1,
	}

	sender := NewMockMetricsSender(false)
	agent := NewMetricsAgent(cfg, sender)

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
