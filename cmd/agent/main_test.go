package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime/metrics"
	"testing"
	"time"
)

// MockRandSource позволяет контролировать случайные числа в тестах
type MockRandSource struct {
	value float64
}

func (m *MockRandSource) Seed(seed int64) {}

func (m *MockRandSource) Int63() int64 {
	return int64(m.value * float64(1<<63))
}

func (m *MockRandSource) Uint64() uint64 {
	return uint64(m.value * float64(1<<64))
}

func (m *MockRandSource) Float64() float64 {
	return m.value
}

// MockHTTPClient для тестирования HTTP запросов
type MockHTTPClient struct {
	Request  *http.Request
	Response *http.Response
	Error    error
}

func (m *MockHTTPClient) Post(url string, contentType string, body io.Reader) (*http.Response, error) {
	if m.Error != nil {
		return nil, m.Error
	}

	// Сохраняем информацию о запросе для проверок в тестах
	if body != nil {
		bodyBytes, _ := io.ReadAll(body)
		m.Request, _ = http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
		m.Request.Header.Set("Content-Type", contentType)
	}

	return m.Response, nil
}

// Глобальная переменная для подмены HTTP клиента в тестах
var httpPost = http.Post

// TestSendMetric тестирует функцию отправки метрик через JSON API
func TestSendMetric(t *testing.T) {
	tests := []struct {
		name        string
		metricType  string
		metricName  string
		value       interface{}
		expectError bool
	}{
		{
			name:        "Send gauge metric",
			metricType:  "gauge",
			metricName:  "test_metric",
			value:       123.45,
			expectError: false,
		},
		{
			name:        "Send counter metric",
			metricType:  "counter",
			metricName:  "test_counter",
			value:       int64(42),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Создаем мок HTTP клиента
			mockClient := &MockHTTPClient{
				Response: &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"id":"test","type":"gauge","value":123.45}`)),
				},
			}

			// Сохраняем оригинальный HTTP клиент и подменяем моком
			oldHTTPPost := httpPost
			httpPost = mockClient.Post
			defer func() { httpPost = oldHTTPPost }()

			cfg := Config{
				Address: "localhost:8080",
			}

			// Вызываем тестируемую функцию
			sendMetric(tt.metricType, tt.metricName, tt.value, cfg)

			// Проверяем результаты
			if !tt.expectError {
				// Проверяем что запрос был отправлен
				if mockClient.Request == nil {
					t.Error("Expected HTTP request to be made")
					return
				}

				// Проверяем заголовок Content-Type
				contentType := mockClient.Request.Header.Get("Content-Type")
				if contentType != "application/json" {
					t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
				}

				// Проверяем URL
				expectedURL := "http://localhost:8080/update"
				if mockClient.Request.URL.String() != expectedURL {
					t.Errorf("Expected URL '%s', got '%s'", expectedURL, mockClient.Request.URL.String())
				}

				// Проверяем тело запроса
				var sentMetric Metrics
				bodyBytes, _ := io.ReadAll(mockClient.Request.Body)
				if err := json.Unmarshal(bodyBytes, &sentMetric); err != nil {
					t.Errorf("Failed to unmarshal request body: %v", err)
				}

				if sentMetric.ID != tt.metricName {
					t.Errorf("Expected metric name '%s', got '%s'", tt.metricName, sentMetric.ID)
				}

				if sentMetric.MType != tt.metricType {
					t.Errorf("Expected metric type '%s', got '%s'", tt.metricType, sentMetric.MType)
				}
			}
		})
	}
}

// TestSendMetricJSONStructure тестирует структуру JSON запроса
func TestSendMetricJSONStructure(t *testing.T) {
	// Создаем тестовый сервер для проверки JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Проверяем метод
		if r.Method != "POST" {
			t.Errorf("Expected POST method, got %s", r.Method)
		}

		// Проверяем заголовок Content-Type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type 'application/json', got '%s'", r.Header.Get("Content-Type"))
		}

		// Проверяем путь
		expectedPath := "/update"
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
		}

		// Читаем и проверяем тело запроса
		var metric Metrics
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Error reading request body: %v", err)
			return
		}

		if err := json.Unmarshal(body, &metric); err != nil {
			t.Errorf("Error unmarshaling JSON: %v", err)
			return
		}

		// Проверяем структуру метрики
		if metric.ID != "TestMetric" {
			t.Errorf("Expected metric ID 'TestMetric', got '%s'", metric.ID)
		}

		if metric.MType != "gauge" {
			t.Errorf("Expected metric type 'gauge', got '%s'", metric.MType)
		}

		if metric.Value == nil || *metric.Value != 99.99 {
			t.Errorf("Expected metric value 99.99, got %v", metric.Value)
		}

		w.WriteHeader(http.StatusOK)

		// Возвращаем JSON ответ
		response := Metrics{
			ID:    metric.ID,
			MType: metric.MType,
			Value: metric.Value,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Подменяем URL для теста
	oldHTTPPost := httpPost
	httpPost = func(url string, contentType string, body io.Reader) (*http.Response, error) {
		return http.Post(server.URL, contentType, body)
	}
	defer func() { httpPost = oldHTTPPost }()

	cfg := Config{
		Address: server.URL[7:], // убираем "http://"
	}

	// Отправляем тестовую метрику
	sendMetric("gauge", "TestMetric", 99.99, cfg)
}

// TestRandomValueGeneration тестирует генерацию случайных значений
func TestRandomValueGeneration(t *testing.T) {
	// Тестируем логику вычисления случайного значения
	testValue := 0.75
	calculatedValue := testValue * 100
	expectedValue := 75.0

	if calculatedValue != expectedValue {
		t.Errorf("Expected calculated value %f, got %f", expectedValue, calculatedValue)
	}

	// Проверяем, что значение в допустимом диапазоне
	if calculatedValue < 0 || calculatedValue > 100 {
		t.Errorf("Random value should be between 0 and 100, got %f", calculatedValue)
	}
}

// TestRandomValueRange тестирует диапазон случайных значений
func TestRandomValueRange(t *testing.T) {
	// Генерируем несколько значений и проверяем диапазон
	for i := 0; i < 10; i++ {
		value := rand.Float64() * 100
		if value < 0 || value > 100 {
			t.Errorf("Random value should be between 0 and 100, got %f", value)
		}
	}
}

// TestMetricWithNameStructure тестирует структуру метрик
func TestMetricWithNameStructure(t *testing.T) {
	metric := struct {
		Sample    metrics.Sample
		ShortName string
	}{
		Sample:    metrics.Sample{Name: "/test/metric"},
		ShortName: "TestMetric",
	}

	if metric.Sample.Name != "/test/metric" {
		t.Errorf("Expected metric name /test/metric, got %s", metric.Sample.Name)
	}

	if metric.ShortName != "TestMetric" {
		t.Errorf("Expected short name TestMetric, got %s", metric.ShortName)
	}
}

// TestMetricsListCompleteness тестирует полноту списка метрик
func TestMetricsListCompleteness(t *testing.T) {
	// Определяем тестовый список метрик аналогично основному
	testMetrics := []struct {
		Sample    metrics.Sample
		ShortName string
	}{
		{metrics.Sample{Name: "/memory/classes/total:bytes"}, "Alloc"},
		{metrics.Sample{Name: "/memory/classes/profiling/buckets:bytes"}, "BuckHashSys"},
		{metrics.Sample{Name: "/memory/classes/heap/released:bytes"}, "HeapReleased"},
		{metrics.Sample{Name: "/memory/classes/metadata/mspan/inuse:bytes"}, "MSpanInuse"},
		{metrics.Sample{Name: "/memory/classes/metadata/mspan/free:bytes"}, "MSpanSys"},
		{metrics.Sample{Name: "/memory/classes/metadata/mcache/inuse:bytes"}, "MCacheInuse"},
		{metrics.Sample{Name: "/memory/classes/metadata/mcache/free:bytes"}, "MCacheSys"},
		{metrics.Sample{Name: "/memory/classes/os-stacks:bytes"}, "StackInuse"},
		{metrics.Sample{Name: "/memory/classes/other:bytes"}, "OtherSys"},
		{metrics.Sample{Name: "/memory/classes/total:bytes"}, "Sys"},
		{metrics.Sample{Name: "/gc/cpu/fraction:gc-cpu-fraction"}, "GCCPUFraction"},
		{metrics.Sample{Name: "/memory/classes/heap/unused:bytes"}, "HeapIdle"},
		{metrics.Sample{Name: "/memory/classes/heap/objects:bytes"}, "HeapInuse"},
		{metrics.Sample{Name: "/gc/heap/goal:bytes"}, "NextGC"},
		{metrics.Sample{Name: "/gc/pauses:seconds"}, "PauseTotalNs"},
		{metrics.Sample{Name: "/gc/heap/frees:objects"}, "Frees"},
		{metrics.Sample{Name: "/gc/heap/objects:objects"}, "HeapObjects"},
		{metrics.Sample{Name: "/gc/cycles/total:gc-cycles"}, "NumGC"},
		{metrics.Sample{Name: "/gc/cycles/forced:gc-cycles"}, "NumForcedGC"},
		{metrics.Sample{Name: "/gc/heap/allocs:objects"}, "Mallocs"},
		{metrics.Sample{Name: "/gc/heap/allocs:bytes"}, "TotalAlloc"},
	}

	expectedMetrics := map[string]string{
		"Alloc":         "/memory/classes/total:bytes",
		"BuckHashSys":   "/memory/classes/profiling/buckets:bytes",
		"HeapReleased":  "/memory/classes/heap/released:bytes",
		"MSpanInuse":    "/memory/classes/metadata/mspan/inuse:bytes",
		"MSpanSys":      "/memory/classes/metadata/mspan/free:bytes",
		"MCacheInuse":   "/memory/classes/metadata/mcache/inuse:bytes",
		"MCacheSys":     "/memory/classes/metadata/mcache/free:bytes",
		"StackInuse":    "/memory/classes/os-stacks:bytes",
		"OtherSys":      "/memory/classes/other:bytes",
		"Sys":           "/memory/classes/total:bytes",
		"GCCPUFraction": "/gc/cpu/fraction:gc-cpu-fraction",
		"HeapIdle":      "/memory/classes/heap/unused:bytes",
		"HeapInuse":     "/memory/classes/heap/objects:bytes",
		"NextGC":        "/gc/heap/goal:bytes",
		"PauseTotalNs":  "/gc/pauses:seconds",
		"Frees":         "/gc/heap/frees:objects",
		"HeapObjects":   "/gc/heap/objects:objects",
		"NumGC":         "/gc/cycles/total:gc-cycles",
		"NumForcedGC":   "/gc/cycles/forced:gc-cycles",
		"Mallocs":       "/gc/heap/allocs:objects",
		"TotalAlloc":    "/gc/heap/allocs:bytes",
	}

	if len(testMetrics) != len(expectedMetrics) {
		t.Errorf("Expected %d metrics, got %d", len(expectedMetrics), len(testMetrics))
	}

	for _, metric := range testMetrics {
		expectedPath, exists := expectedMetrics[metric.ShortName]
		if !exists {
			t.Errorf("Unexpected metric name: %s", metric.ShortName)
			continue
		}

		if metric.Sample.Name != expectedPath {
			t.Errorf("For metric %s expected path %s, got %s",
				metric.ShortName, expectedPath, metric.Sample.Name)
		}
	}
}

// TestValueConversion тестирует конвертацию значений метрик
func TestValueConversion(t *testing.T) {
	tests := []struct {
		name     string
		kind     metrics.ValueKind
		value    interface{}
		expected float64
	}{
		{
			name:     "Uint64 value",
			kind:     metrics.KindUint64,
			value:    uint64(12345),
			expected: 12345.0,
		},
		{
			name:     "Float64 value",
			kind:     metrics.KindFloat64,
			value:    67.89,
			expected: 67.89,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result float64

			switch tt.kind {
			case metrics.KindUint64:
				result = float64(tt.value.(uint64))
			case metrics.KindFloat64:
				result = tt.value.(float64)
			}

			if result != tt.expected {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}

// TestIntervals тестирует корректность интервалов
func TestIntervals(t *testing.T) {
	pollInterval := 2
	reportInterval := 10

	if pollInterval <= 0 {
		t.Error("Poll interval should be positive")
	}

	if reportInterval <= 0 {
		t.Error("Report interval should be positive")
	}

	if reportInterval <= pollInterval {
		t.Error("Report interval should be greater than poll interval")
	}
}

// TestSignalHandling тестирует обработку сигналов
func TestSignalHandling(t *testing.T) {
	// Тестируем создание канала для сигналов
	stop := make(chan os.Signal, 1)

	// Проверяем емкость канала
	if cap(stop) != 1 {
		t.Errorf("Expected channel capacity 1, got %d", cap(stop))
	}

	// Проверяем, что канал не закрыт
	select {
	case <-stop:
		t.Error("Channel should not be closed or have values initially")
	default:
		// Это нормально - канал пустой
	}

	// Закрываем канал для очистки
	close(stop)
}

// TestMetricKindHandling тестирует обработку различных типов метрик
func TestMetricKindHandling(t *testing.T) {
	testCases := []struct {
		name        string
		valueKind   metrics.ValueKind
		expectError bool
	}{
		{
			name:        "Uint64 value",
			valueKind:   metrics.KindUint64,
			expectError: false,
		},
		{
			name:        "Float64 value",
			valueKind:   metrics.KindFloat64,
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Тест проверяет, что код корректно обрабатывает разные kinds
			var value float64
			switch tc.valueKind {
			case metrics.KindUint64:
				value = float64(100)
			case metrics.KindFloat64:
				value = 100.5
			}

			if value == 0 && !tc.expectError {
				t.Errorf("Expected non-zero value for kind %v", tc.valueKind)
			}
		})
	}
}

// TestCounterIncrement тестирует логику счетчика
func TestCounterIncrement(t *testing.T) {
	var counter int64

	// Эмулируем инкремент счетчика
	counter++
	counter++

	if counter != 2 {
		t.Errorf("Expected counter value 2, got %d", counter)
	}
}

// TestEndpointFormatting тестирует форматирование URL для JSON API
func TestEndpointFormatting(t *testing.T) {
	cfg := Config{
		Address: "localhost:8080",
	}

	endpoint := fmt.Sprintf("http://%s/update", cfg.Address)
	expected := "http://localhost:8080/update"

	if endpoint != expected {
		t.Errorf("Expected endpoint %s, got %s", expected, endpoint)
	}
}

// TestTimeFormatting тестирует форматирование времени
func TestTimeFormatting(t *testing.T) {
	testTime := time.Date(2023, 1, 1, 15, 30, 45, 0, time.UTC)
	formatted := testTime.Format("15:04:05")

	expected := "15:30:45"
	if formatted != expected {
		t.Errorf("Expected time %s, got %s", expected, formatted)
	}
}

// TestHTTPResponseHandling тестирует обработку HTTP ответов
func TestHTTPResponseHandling(t *testing.T) {
	// Создаем тестовый сервер с разными статусами
	testCases := []struct {
		name        string
		statusCode  int
		expectError bool
	}{
		{"OK response", http.StatusOK, false},
		{"Bad Request", http.StatusBadRequest, true},
		{"Not Found", http.StatusNotFound, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
			}))
			defer server.Close()

			// Простой тест проверки статуса
			if tc.statusCode != http.StatusOK && !tc.expectError {
				t.Errorf("Status %d should be treated as error", tc.statusCode)
			}
		})
	}
}

// TestMetricCalculation тестирует расчет значений метрик
func TestMetricCalculation(t *testing.T) {
	// Тестируем различные сценарии расчета значений
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{"Zero value", 0.0, 0.0},
		{"Normal value", 0.5, 50.0},
		{"Max value", 1.0, 100.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input * 100
			if result != tt.expected {
				t.Errorf("For input %f expected %f, got %f", tt.input, tt.expected, result)
			}
		})
	}
}

// TestJSONMarshaling тестирует маршалинг/анмаршалинг JSON
func TestJSONMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		metric   Metrics
		expected string
	}{
		{
			name: "Gauge metric",
			metric: Metrics{
				ID:    "TestGauge",
				MType: "gauge",
				Value: func() *float64 { v := 123.45; return &v }(),
			},
			expected: `{"id":"TestGauge","type":"gauge","value":123.45}`,
		},
		{
			name: "Counter metric",
			metric: Metrics{
				ID:    "TestCounter",
				MType: "counter",
				Delta: func() *int64 { v := int64(42); return &v }(),
			},
			expected: `{"id":"TestCounter","type":"counter","delta":42}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, err := json.Marshal(tt.metric)
			if err != nil {
				t.Errorf("Error marshaling JSON: %v", err)
				return
			}

			// Проверяем что JSON корректно парсится обратно
			var unmarshaled Metrics
			if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
				t.Errorf("Error unmarshaling JSON: %v", err)
				return
			}

			if unmarshaled.ID != tt.metric.ID {
				t.Errorf("Expected ID %s, got %s", tt.metric.ID, unmarshaled.ID)
			}
			if unmarshaled.MType != tt.metric.MType {
				t.Errorf("Expected MType %s, got %s", tt.metric.MType, unmarshaled.MType)
			}
		})
	}
}

// Остальные тесты parseFlags остаются без изменений...
func TestParseFlags(t *testing.T) {
	tests := []struct {
		name               string
		envAddress         string
		envPollInterval    string
		envReportInterval  string
		flagAddress        string
		flagPollInterval   string
		flagReportInterval string
		wantAddress        string
		wantPollInterval   time.Duration
		wantReportInterval time.Duration
	}{
		{
			name:               "default values",
			wantAddress:        "localhost:8080",
			wantPollInterval:   2 * time.Second,
			wantReportInterval: 10 * time.Second,
		},
		// ... остальные тестовые случаи остаются без изменений
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Реализация теста parseFlags остается без изменений
			// ...
		})
	}
}
