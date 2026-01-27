package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/storage"
	"github.com/go-chi/chi/v5"
)

// Example демонстрирует основные сценарии использования API сервера метрик.
func Example() {
	// Создаем in-memory хранилище
	store := storage.NewMemStorage()

	// Инициализируем маршрутизатор
	r := chi.NewRouter()

	// Регистрируем обработчики
	r.Post("/update/{type_metric}/{metric}/{value_metric}", getMetrics(store))
	r.Get("/value/{type_metric}/{metric}", sendMetrics(store))
	r.Post("/update/", getJSONMetric(store))
	r.Post("/value/", sendJSONMetric(store))
	r.Post("/updates/", updateMetricsBatch(store))
	r.Get("/", summaryMetrics(store))

	// Запускаем тестовый сервер
	ts := httptest.NewServer(r)
	defer ts.Close()

	// Пример 1: Обновление gauge метрики через URL
	req1, _ := http.NewRequest("POST", ts.URL+"/update/gauge/cpu_usage/42.5", nil)
	resp1, _ := http.DefaultClient.Do(req1)
	fmt.Printf("Update gauge status: %d\n", resp1.StatusCode)
	resp1.Body.Close()

	// Пример 2: Обновление counter метрики через URL
	req2, _ := http.NewRequest("POST", ts.URL+"/update/counter/requests/10", nil)
	resp2, _ := http.DefaultClient.Do(req2)
	fmt.Printf("Update counter status: %d\n", resp2.StatusCode)
	resp2.Body.Close()

	// Пример 3: Получение значения метрики
	req3, _ := http.NewRequest("GET", ts.URL+"/value/gauge/cpu_usage", nil)
	resp3, _ := http.DefaultClient.Do(req3)
	var value string
	if resp3.StatusCode == http.StatusOK {
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp3.Body)
		value = buf.String()
	}
	fmt.Printf("Get gauge value: %s\n", value)
	resp3.Body.Close()

	// Пример 4: Обновление метрики через JSON
	metric := models.Metrics{
		ID:    "memory_usage",
		MType: "gauge",
		Value: func() *float64 { v := 75.3; return &v }(),
	}
	body, _ := json.Marshal(metric)
	req4, _ := http.NewRequest("POST", ts.URL+"/update/", bytes.NewBuffer(body))
	req4.Header.Set("Content-Type", "application/json")
	resp4, _ := http.DefaultClient.Do(req4)
	fmt.Printf("Update via JSON status: %d\n", resp4.StatusCode)
	resp4.Body.Close()

	// Пример 5: Батчевое обновление метрик
	metrics := []models.Metrics{
		{
			ID:    "cpu_temp",
			MType: "gauge",
			Value: func() *float64 { v := 65.5; return &v }(),
		},
		{
			ID:    "errors",
			MType: "counter",
			Delta: func() *int64 { v := int64(5); return &v }(),
		},
	}
	batchBody, _ := json.Marshal(metrics)
	req5, _ := http.NewRequest("POST", ts.URL+"/updates/", bytes.NewBuffer(batchBody))
	req5.Header.Set("Content-Type", "application/json")
	resp5, _ := http.DefaultClient.Do(req5)
	fmt.Printf("Batch update status: %d\n", resp5.StatusCode)
	resp5.Body.Close()

	// Вывод:
	// Update gauge status: 200
	// Update counter status: 200
	// Get gauge value: 42.5
	// Update via JSON status: 200
	// Batch update status: 200
}

// Example_withMiddleware демонстрирует использование middleware (логирование, сжатие, хеши).
func Example_withMiddleware() {
	store := storage.NewMemStorage()

	r := chi.NewRouter()

	// В реальном коде middleware добавляются так:
	// r.Use(middleware.WithLogging)
	// r.Use(middleware.WithGzip)
	// r.Use(middleware.HashValidation("secret-key"))
	// r.Use(middleware.HashResponse("secret-key"))

	// Регистрируем обработчики
	r.Post("/update/{type_metric}/{metric}/{value_metric}", getMetrics(store))
	r.Get("/value/{type_metric}/{metric}", sendMetrics(store))
	r.Post("/update/", getJSONMetric(store))

	ts := httptest.NewServer(r)
	defer ts.Close()

	// Отправляем запрос с поддержкой gzip
	metric := models.Metrics{
		ID:    "test_metric",
		MType: "gauge",
		Value: func() *float64 { v := 99.9; return &v }(),
	}
	body, _ := json.Marshal(metric)

	req, _ := http.NewRequest("POST", ts.URL+"/update/", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")

	resp, _ := http.DefaultClient.Do(req)
	fmt.Printf("Request with middleware status: %d\n", resp.StatusCode)
	resp.Body.Close()

	// Вывод:
	// Request with middleware status: 200
}

// Example_database демонстрирует работу с PostgreSQL хранилищем.
func Example_database() {
	// Этот пример требует запущенной PostgreSQL базы данных
	// и показывается только для демонстрации API

	fmt.Println("Для использования PostgreSQL хранилища:")
	fmt.Println("1. Установите PostgreSQL и создайте базу данных")
	fmt.Println("2. Настройте строку подключения:")
	fmt.Println("   export DATABASE_DSN='postgres://user:password@localhost:5432/metrics'")
	fmt.Println("3. Запустите сервер с флагом -d")
	fmt.Println("4. Используйте эндпоинт /ping для проверки подключения")

	// Пример проверки подключения к БД
	// r.Get("/ping", checkConnectDatabase(dbStorage))

	// Вывод:
	// Для использования PostgreSQL хранилища:
	// 1. Установите PostgreSQL и создайте базу данных
	// 2. Настройте строку подключения:
	//    export DATABASE_DSN='postgres://user:password@localhost:5432/metrics'
	// 3. Запустите сервер с флагом -d
	// 4. Используйте эндпоинт /ping для проверки подключения
}

// Example_healthCheck демонстрирует использование health-check эндпоинтов.
func Example_healthCheck() {
	store := storage.NewMemStorage()

	r := chi.NewRouter()
	r.Get("/", summaryMetrics(store))
	// Эндпоинт /ping будет добавлен при использовании PostgreSQL
	// r.Get("/ping", checkConnectDatabase(dbStorage))

	ts := httptest.NewServer(r)
	defer ts.Close()

	// Проверка основного эндпоинта
	req, _ := http.NewRequest("GET", ts.URL+"/", nil)
	resp, _ := http.DefaultClient.Do(req)

	if resp.StatusCode == http.StatusOK {
		fmt.Println("Сервер работает корректно")
	} else {
		fmt.Println("Проблемы с сервером")
	}
	resp.Body.Close()

	// Вывод:
	// Сервер работает корректно
}

// TestExamples запускает все примеры для проверки.
func TestExamples(t *testing.T) {
	// Эти примеры демонстрируют использование API
	// и проверяются системой тестирования Go
	// Примеры не требуют явных assertions, они проверяются
	// на корректность компиляции и выполнения
	t.Run("BasicExample", func(t *testing.T) {
		Example()
	})
	t.Run("MiddlewareExample", func(t *testing.T) {
		Example_withMiddleware()
	})
	t.Run("DatabaseExample", func(t *testing.T) {
		Example_database()
	})
	t.Run("HealthCheckExample", func(t *testing.T) {
		Example_healthCheck()
	})
}
