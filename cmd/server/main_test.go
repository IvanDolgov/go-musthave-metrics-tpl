package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Тесты для обработчика getMetrics
func TestGetMetricsHandler(t *testing.T) {
	storage := NewMemStorage()
	handler := getMetrics(storage)

	tests := []struct {
		name           string
		method         string
		url            string
		expectedStatus int
	}{
		{
			name:           "Valid gauge metric",
			method:         "POST",
			url:            "/update/gauge/temperature/25.5",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Valid counter metric",
			method:         "POST",
			url:            "/update/counter/requests/10",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid method GET",
			method:         "GET",
			url:            "/update/gauge/temperature/25.5",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "Invalid gauge value",
			method:         "POST",
			url:            "/update/gauge/temperature/invalid",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid counter value",
			method:         "POST",
			url:            "/update/counter/requests/invalid",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid metric type",
			method:         "POST",
			url:            "/update/invalid/metric/123",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.url, nil)
			if err != nil {
				t.Fatal(err)
			}

			router := chi.NewRouter()
			router.Post("/update/{type_metric}/{metric}/{value_metric}", handler)

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("Неожиданный код ответа: получили %v хотели %v", status, tt.expectedStatus)
			}
		})
	}
}

// Тесты для обработчика sendMetrics
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
			expectedBody:   "temperature: 25.5",
		},
		{
			name:           "Existing counter metric",
			url:            "/value/counter/requests",
			expectedStatus: http.StatusOK,
			expectedBody:   "requests: 10", // Теперь ожидаем целое число
		},
		{
			name:           "Non-existing gauge metric",
			url:            "/value/gauge/nonexistent",
			expectedStatus: http.StatusNotFound,
			expectedBody:   "",
		},
		{
			name:           "Non-existing counter metric",
			url:            "/value/counter/nonexistent",
			expectedStatus: http.StatusNotFound,
			expectedBody:   "",
		},
		{
			name:           "Wrong type for existing metric",
			url:            "/value/counter/temperature", // temperature is gauge, not counter
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
		expectedParts  []string // Части, которые должны присутствовать в ответе
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

			// Проверяем Content-Type
			contentType := rr.Header().Get("Content-Type")
			if contentType != "text/plain" {
				t.Errorf("Неожиданный Content-Type: получили %v хотели text/plain", contentType)
			}
		})
	}
}

// Тесты для пустого хранилища
func TestSummaryMetricsEmpty(t *testing.T) {
	storage := NewMemStorage()

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := summaryMetrics(storage)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Неожиданный код ответа: получили %v хотели %v", status, http.StatusOK)
	}

	body := rr.Body.String()
	expectedParts := []string{
		"Gauge Metrics:",
		"Counter Metrics:",
	}

	for _, expectedPart := range expectedParts {
		if !strings.Contains(body, expectedPart) {
			t.Errorf("Ожидаемая часть '%s' не найдена в теле ответа", expectedPart)
		}
	}
}

// Тесты для проверки инкремента счетчика
func TestCounterIncrement(t *testing.T) {
	storage := NewMemStorage()
	handler := getMetrics(storage)

	// Первое увеличение счетчика
	req1, _ := http.NewRequest("POST", "/update/counter/requests/5", nil)
	rr1 := httptest.NewRecorder()

	router := chi.NewRouter()
	router.Post("/update/{type_metric}/{metric}/{value_metric}", handler)
	router.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Errorf("Первый запрос не удался: получили %v хотели %v", rr1.Code, http.StatusOK)
	}

	// Второе увеличение того же счетчика
	req2, _ := http.NewRequest("POST", "/update/counter/requests/3", nil)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Errorf("Второй запрос не удался: получили %v хотели %v", rr2.Code, http.StatusOK)
	}

	// Проверяем итоговое значение
	gauge, counter := storage.GetAllMetrics()
	if counter["requests"] != 8 {
		t.Errorf("Неверное итоговое значение счетчика: получили %v хотели %v", counter["requests"], 8)
	}

	if len(gauge) != 0 {
		t.Errorf("Ожидалось 0 gauge метрик, получили %v", len(gauge))
	}
}

// Тесты для проверки перезаписи gauge
func TestGaugeOverwrite(t *testing.T) {
	storage := NewMemStorage()
	handler := getMetrics(storage)

	// Первая установка gauge
	req1, _ := http.NewRequest("POST", "/update/gauge/temperature/25.5", nil)
	rr1 := httptest.NewRecorder()

	router := chi.NewRouter()
	router.Post("/update/{type_metric}/{metric}/{value_metric}", handler)
	router.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Errorf("Первый запрос не удался: получили %v хотели %v", rr1.Code, http.StatusOK)
	}

	// Вторая установка того же gauge
	req2, _ := http.NewRequest("POST", "/update/gauge/temperature/30.2", nil)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Errorf("Второй запрос не удался: получили %v хотели %v", rr2.Code, http.StatusOK)
	}

	// Проверяем итоговое значение
	gauge, counter := storage.GetAllMetrics()
	if gauge["temperature"] != 30.2 {
		t.Errorf("Неверное итоговое значение gauge: получили %v хотели %v", gauge["temperature"], 30.2)
	}

	if len(counter) != 0 {
		t.Errorf("Ожидалось 0 counter метрик, получили %v", len(counter))
	}
}
