package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// проверка главной страницы
func TestMainPage(t *testing.T) {
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(mainPage)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Код статус: получили %v зотели %v", status, http.StatusOK)
	}
}

// тесты получения метрик
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

			// Запускаем роутер
			router := mux.NewRouter()
			router.HandleFunc(`/update/{type_metric}/{metric}/{value_metric}`, handler)

			rr := httptest.NewRecorder()

			// Выдергиваем переменные из урла сплитом
			vars := map[string]string{
				"type_metric":  strings.Split(tt.url, "/")[2],
				"metric":       strings.Split(tt.url, "/")[3],
				"value_metric": strings.Split(tt.url, "/")[4],
			}

			req = mux.SetURLVars(req, vars)
			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("Неожиданный код ответа: получили %v хотели %v", status, tt.expectedStatus)
			}
		})
	}
}
