package main

import (
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Тесты для обработчика getJSONMetric (бывший updateJsonMetrics)
func TestGetJSONMetricHandler(t *testing.T) {
	storage := NewMemStorage()
	handler := getJSONMetric(storage)

	tests := []struct {
		name           string
		method         string
		body           string
		expectedStatus int
	}{
		{
			name:           "Valid gauge metric",
			method:         "POST",
			body:           `{"id":"temperature", "type":"gauge", "value":25.5}`,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Valid counter metric",
			method:         "POST",
			body:           `{"id":"requests", "type":"counter", "delta":10}`,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid method GET",
			method:         "GET",
			body:           `{"id":"test", "type":"gauge", "value":1.0}`,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "Invalid JSON",
			method:         "POST",
			body:           `invalid json`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Missing gauge value",
			method:         "POST",
			body:           `{"id":"temperature", "type":"gauge"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Missing counter delta",
			method:         "POST",
			body:           `{"id":"requests", "type":"counter"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid metric type",
			method:         "POST",
			body:           `{"id":"test", "type":"invalid", "value":1.0}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, "/update", strings.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("Неожиданный код ответа: получили %v хотели %v", status, tt.expectedStatus)
			}

			// Проверяем что успешные запросы возвращают JSON
			if tt.expectedStatus == http.StatusOK {
				contentType := rr.Header().Get("Content-Type")
				if !strings.Contains(contentType, "application/json") {
					t.Errorf("Expected JSON content type, got %s", contentType)
				}

				// Проверяем что ответ валидный JSON
				var response Metrics
				if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
					t.Errorf("Invalid JSON response: %v", err)
				}
			}
		})
	}
}

// Тесты для обработчика sendJSONMetric (бывший getJsonMetricValue)
func TestSendJSONMetricHandler(t *testing.T) {
	storage := NewMemStorage()

	// Добавляем тестовые данные
	storage.SetGauge("temperature", 25.5)
	storage.IncrementCounter("requests", 10)

	handler := sendJSONMetric(storage)

	tests := []struct {
		name           string
		method         string
		body           string
		expectedStatus int
		expectedValue  interface{}
	}{
		{
			name:           "Existing gauge metric",
			method:         "POST",
			body:           `{"id":"temperature", "type":"gauge"}`,
			expectedStatus: http.StatusOK,
			expectedValue:  25.5,
		},
		{
			name:           "Existing counter metric",
			method:         "POST",
			body:           `{"id":"requests", "type":"counter"}`,
			expectedStatus: http.StatusOK,
			expectedValue:  int64(10),
		},
		{
			name:           "Non-existing gauge metric",
			method:         "POST",
			body:           `{"id":"nonexistent", "type":"gauge"}`,
			expectedStatus: http.StatusNotFound,
			expectedValue:  nil,
		},
		{
			name:           "Non-existing counter metric",
			method:         "POST",
			body:           `{"id":"nonexistent", "type":"counter"}`,
			expectedStatus: http.StatusNotFound,
			expectedValue:  nil,
		},
		{
			name:           "Wrong type for existing metric",
			method:         "POST",
			body:           `{"id":"temperature", "type":"counter"}`,
			expectedStatus: http.StatusNotFound,
			expectedValue:  nil,
		},
		{
			name:           "Invalid JSON",
			method:         "POST",
			body:           `invalid json`,
			expectedStatus: http.StatusBadRequest,
			expectedValue:  nil,
		},
		{
			name:           "Invalid method GET",
			method:         "GET",
			body:           `{"id":"temperature", "type":"gauge"}`,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedValue:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, "/value", strings.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("Неожиданный код ответа: получили %v хотели %v", status, tt.expectedStatus)
			}

			// Проверяем успешные ответы
			if tt.expectedStatus == http.StatusOK {
				contentType := rr.Header().Get("Content-Type")
				if !strings.Contains(contentType, "application/json") {
					t.Errorf("Expected JSON content type, got %s", contentType)
				}

				var response Metrics
				if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
					t.Errorf("Invalid JSON response: %v", err)
				}

				// Проверяем значения
				if tt.expectedValue != nil {
					switch tt.expectedValue.(type) {
					case float64:
						if response.Value == nil || *response.Value != tt.expectedValue.(float64) {
							t.Errorf("Expected value %v, got %v", tt.expectedValue, response.Value)
						}
					case int64:
						if response.Delta == nil || *response.Delta != tt.expectedValue.(int64) {
							t.Errorf("Expected delta %v, got %v", tt.expectedValue, response.Delta)
						}
					}
				}
			}
		})
	}
}

// Тесты для обработчика sendMetrics (старый эндпоинт, оставлен для обратной совместимости)
func TestSendMetrics(t *testing.T) {
	storage := NewMemStorage()

	// Добавляем тестовые данные
	storage.SetGauge("temperature", 25.5)
	storage.IncrementCounter("requests", 10)

	tests := []struct {
		name           string
		url            string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Existing gauge metric",
			url:            "/value/gauge/temperature",
			expectedStatus: http.StatusOK,
			expectedBody:   "25.5",
		},
		{
			name:           "Existing counter metric",
			url:            "/value/counter/requests",
			expectedStatus: http.StatusOK,
			expectedBody:   "10",
		},
		{
			name:           "Non-existing gauge metric",
			url:            "/value/gauge/nonexistent",
			expectedStatus: http.StatusNotFound,
			expectedBody:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", tt.url, nil)
			if err != nil {
				t.Fatal(err)
			}

			router := chi.NewRouter()
			router.Get("/value/{type_metric}/{metric}", sendMetrics(storage))

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("Неожиданный код ответа: получили %v хотели %v", status, tt.expectedStatus)
			}

			if tt.expectedBody != "" {
				body := strings.TrimSpace(rr.Body.String())
				if body != tt.expectedBody {
					t.Errorf("Неожиданное тело ответа: получили '%v' хотели '%v'", body, tt.expectedBody)
				}
			}
		})
	}
}

// Тесты для обработчика summaryMetrics
func TestSummaryMetrics(t *testing.T) {
	storage := NewMemStorage()

	// Добавляем тестовые данные
	storage.SetGauge("temperature", 25.5)
	storage.SetGauge("memory", 1024.0)
	storage.IncrementCounter("requests", 10)
	storage.IncrementCounter("errors", 2)

	tests := []struct {
		name           string
		expectedStatus int
		expectedParts  []string
	}{
		{
			name:           "Summary with metrics",
			expectedStatus: http.StatusOK,
			expectedParts: []string{
				"Gauge Metrics:",
				"temperature: 25.5",
				"memory: 1024",
				"Counter Metrics:",
				"requests: 10",
				"errors: 2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/", nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			handler := summaryMetrics(storage)
			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("Неожиданный код ответа: получили %v хотели %v", status, tt.expectedStatus)
			}

			body := rr.Body.String()
			for _, expectedPart := range tt.expectedParts {
				if !strings.Contains(body, expectedPart) {
					t.Errorf("Ожидаемая часть '%s' не найдена в теле ответа", expectedPart)
				}
			}

			contentType := rr.Header().Get("Content-Type")
			if contentType != "text/plain" {
				t.Errorf("Неожиданный Content-Type: получили %v хотели text/plain", contentType)
			}
		})
	}
}

// Тесты для проверки инкремента счетчика через JSON API
func TestCounterIncrementJson(t *testing.T) {
	storage := NewMemStorage()
	handler := getJSONMetric(storage)

	// Первое увеличение счетчика
	req1, _ := http.NewRequest("POST", "/update", strings.NewReader(`{"id":"requests", "type":"counter", "delta":5}`))
	req1.Header.Set("Content-Type", "application/json")
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Errorf("Первый запрос не удался: получили %v хотели %v", rr1.Code, http.StatusOK)
	}

	// Второе увеличение того же счетчика
	req2, _ := http.NewRequest("POST", "/update", strings.NewReader(`{"id":"requests", "type":"counter", "delta":3}`))
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Errorf("Второй запрос не удался: получили %v хотели %v", rr2.Code, http.StatusOK)
	}

	// Проверяем итоговое значение
	_, counter := storage.GetAllMetrics()
	if counter["requests"] != 8 {
		t.Errorf("Неверное итоговое значение счетчика: получили %v хотели %v", counter["requests"], 8)
	}
}

// Тесты для проверки перезаписи gauge через JSON API
func TestGaugeOverwriteJson(t *testing.T) {
	storage := NewMemStorage()
	handler := getJSONMetric(storage)

	// Первая установка gauge
	req1, _ := http.NewRequest("POST", "/update", strings.NewReader(`{"id":"temperature", "type":"gauge", "value":25.5}`))
	req1.Header.Set("Content-Type", "application/json")
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Errorf("Первый запрос не удался: получили %v хотели %v", rr1.Code, http.StatusOK)
	}

	// Вторая установка того же gauge
	req2, _ := http.NewRequest("POST", "/update", strings.NewReader(`{"id":"temperature", "type":"gauge", "value":30.2}`))
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Errorf("Второй запрос не удался: получили %v хотели %v", rr2.Code, http.StatusOK)
	}

	// Проверяем итоговое значение
	gauge, _ := storage.GetAllMetrics()
	if gauge["temperature"] != 30.2 {
		t.Errorf("Неверное итоговое значение gauge: получили %v хотели %v", gauge["temperature"], 30.2)
	}
}

// Тест для проверки ответа после обновления метрики
func TestGetJSONMetricResponse(t *testing.T) {
	storage := NewMemStorage()
	handler := getJSONMetric(storage)

	req, _ := http.NewRequest("POST", "/update", strings.NewReader(`{"id":"test", "type":"gauge", "value":99.9}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	var response Metrics
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Errorf("Error unmarshaling response: %v", err)
	}

	if response.ID != "test" {
		t.Errorf("Expected ID 'test', got '%s'", response.ID)
	}
	if response.MType != "gauge" {
		t.Errorf("Expected type 'gauge', got '%s'", response.MType)
	}
	if response.Value == nil || *response.Value != 99.9 {
		t.Errorf("Expected value 99.9, got %v", response.Value)
	}
}

// Тесты parseFlags остаются без изменений
func TestParseFlags(t *testing.T) {
	tests := []struct {
		name        string
		envAddress  string
		flagAddress string
		wantAddress string
		wantServer  string
		wantPort    string
	}{
		{
			name:        "default values",
			wantAddress: "localhost:8080",
			wantServer:  "localhost",
			wantPort:    "8080",
		},
		{
			name:        "only flag",
			flagAddress: "127.0.0.1:9090",
			wantAddress: "127.0.0.1:9090",
			wantServer:  "127.0.0.1",
			wantPort:    "9090",
		},
		{
			name:        "only environment variable",
			envAddress:  "192.168.1.1:8080",
			wantAddress: "192.168.1.1:8080",
			wantServer:  "192.168.1.1",
			wantPort:    "8080",
		},
		{
			name:        "environment overrides flag",
			envAddress:  "env-host:8080",
			flagAddress: "flag-host:9090",
			wantAddress: "env-host:8080",
			wantServer:  "env-host",
			wantPort:    "8080",
		},
		{
			name:        "address without port",
			flagAddress: "myserver",
			wantAddress: "myserver",
			wantServer:  "myserver",
			wantPort:    "8080",
		},
		{
			name:        "empty environment uses flag",
			envAddress:  "",
			flagAddress: "flag-host:8080",
			wantAddress: "flag-host:8080",
			wantServer:  "flag-host",
			wantPort:    "8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Сохраняем оригинальные значения
			originalArgs := os.Args
			originalEnvAddress := os.Getenv("ADDRESS")

			// Восстанавливаем состояние после теста
			defer func() {
				os.Args = originalArgs
				os.Setenv("ADDRESS", originalEnvAddress)
				flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
			}()

			// Устанавливаем переменные окружения
			os.Setenv("ADDRESS", tt.envAddress)

			// Подготавливаем аргументы командной строки
			os.Args = []string{"test"}
			if tt.flagAddress != "" {
				os.Args = append(os.Args, "-a", tt.flagAddress)
			}

			// Сбрасываем флаги для нового теста
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

			// Вызываем тестируемую функцию
			config := parseFlags()

			// Проверяем результаты
			if config.Address != tt.wantAddress {
				t.Errorf("Address = %v, want %v", config.Address, tt.wantAddress)
			}
			if config.Server != tt.wantServer {
				t.Errorf("Server = %v, want %v", config.Server, tt.wantServer)
			}
			if config.Port != tt.wantPort {
				t.Errorf("Port = %v, want %v", config.Port, tt.wantPort)
			}
		})
	}
}
