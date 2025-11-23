package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/storage"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/storage/postgres"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// checkConnectDatabase проверяет подключение к базе данных
func checkConnectDatabase(dbStorage postgres.DatabaseStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if dbStorage == nil {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Database not configured - using in-memory storage"))
			return
		}

		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()

		if err := dbStorage.Ping(ctx); err != nil {
			http.Error(w, "Database connection failed", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Database connection successful"))
	}
}

// getMetricsWithSync возвращает обработчик с синхронным сохранением
func getMetricsWithSync(store storage.Storage, filePath string) http.HandlerFunc {
	handler := getMetrics(store)
	return func(w http.ResponseWriter, req *http.Request) {
		handler(w, req)
		// Синхронно сохраняем после обновления метрик
		ctx := context.Background()
		if err := store.SaveToFile(ctx, filePath); err != nil {
			logger.Log.Error("Failed to sync save metrics", zap.String("file", filePath), zap.Error(err))
		}
	}
}

// getJSONMetricWithSync возвращает обработчик с синхронным сохранением
func getJSONMetricWithSync(store storage.Storage, filePath string) http.HandlerFunc {
	handler := getJSONMetric(store)
	return func(w http.ResponseWriter, req *http.Request) {
		handler(w, req)
		// Синхронно сохраняем после обновления метрик
		ctx := context.Background()
		if err := store.SaveToFile(ctx, filePath); err != nil {
			logger.Log.Error("Failed to sync save metrics", zap.String("file", filePath), zap.Error(err))
		}
	}
}

func sendMetrics(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		metricType := chi.URLParam(req, "type_metric")
		name := chi.URLParam(req, "metric")

		valueMetric, exists := store.GetMetric(ctx, name, models.MetricType(metricType))

		if exists {
			var message string
			if metricType == "counter" {
				// Для counter выводим как целое число
				message = fmt.Sprintf("%d", valueMetric)
			} else {
				// Для gauge выводим как число с плавающей точкой
				message = fmt.Sprintf("%g", valueMetric)
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(message))
		} else {
			// нет такой переменной
			w.WriteHeader(http.StatusNotFound)
		}

	}
}

func summaryMetrics(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		gaugeMetrics, counterMetrics := store.GetAllMetrics(ctx)

		// Формируем текстовый ответ
		var response strings.Builder

		// Добавляем gauge метрики
		response.WriteString("Gauge Metrics:\n")
		for key, value := range gaugeMetrics {
			response.WriteString(fmt.Sprintf("%s: %v\n", key, value))
		}

		// Добавляем counter метрики
		response.WriteString("\nCounter Metrics:\n")
		for key, value := range counterMetrics {
			response.WriteString(fmt.Sprintf("%s: %v\n", key, value))
		}

		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(response.String()))
	}
}

func getMetrics(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if req.Method != http.MethodPost {
			// разрешаем только POST-запросы
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// установим правильный заголовок для типа данных
		w.Header().Set("Content-Type", "text/html")

		metricType := chi.URLParam(req, "type_metric")
		name := chi.URLParam(req, "metric")
		valueMetric := chi.URLParam(req, "value_metric")

		// разбираем метрики по типы
		switch metricType {
		case "gauge":
			// strconv позволяет проверть тип
			value, err := strconv.ParseFloat(valueMetric, 64)
			if err != nil {
				http.Error(w, "Invalid gauge value", http.StatusBadRequest)
				return
			}
			store.SetGauge(ctx, name, value)
			w.WriteHeader(http.StatusOK)

		case "counter":
			// strconv позволяет проверть тип
			value, err := strconv.ParseInt(valueMetric, 10, 64)
			if err != nil {
				http.Error(w, "Invalid counter value", http.StatusBadRequest)
				return
			}
			store.IncrementCounter(ctx, name, value)
			w.WriteHeader(http.StatusOK)

		default:
			http.Error(w, "Invalid type metric", http.StatusBadRequest)

		}
	}
}

func getJSONMetric(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		var metric models.Metrics
		var buf bytes.Buffer

		if req.Method != http.MethodPost {
			// разрешаем только POST-запросы
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		_, err := buf.ReadFrom(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// десериализуем JSON в Metrics
		if err = json.Unmarshal(buf.Bytes(), &metric); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Сохраняем метрику в storage в зависимости от типа
		switch metric.MType {
		case "gauge":
			if metric.Value == nil {
				http.Error(w, "Missing value for gauge metric", http.StatusBadRequest)
				return
			}
			store.SetGauge(ctx, metric.ID, *metric.Value)

		case "counter":
			if metric.Delta == nil {
				http.Error(w, "Missing delta for counter metric", http.StatusBadRequest)
				return
			}
			store.IncrementCounter(ctx, metric.ID, *metric.Delta)

		default:
			http.Error(w, "Invalid metric type", http.StatusBadRequest)
			return
		}

		// установим правильный заголовок для типа данных
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// // Возвращаем обновленную метрику, чтоб понимать удалось ли опубликовать
		updatedMetric := store.GetMetricForJSON(ctx, metric.ID, models.MetricType(metric.MType))
		json.NewEncoder(w).Encode(updatedMetric)
	}
}

func sendJSONMetric(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		var metric models.Metrics
		var buf bytes.Buffer

		if req.Method != http.MethodPost {
			// разрешаем только POST-запросы
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		_, err := buf.ReadFrom(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// десериализуем JSON в Metrics
		if err = json.Unmarshal(buf.Bytes(), &metric); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Получаем метрику из storage
		foundMetric := store.GetMetricForJSON(ctx, metric.ID, models.MetricType(metric.MType))

		// Если метрика не найдена
		if foundMetric.ID == "" {
			http.Error(w, "Metric not found", http.StatusNotFound)
			return
		}

		// установим правильный заголовок для типа данных
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(foundMetric)
	}
}

// updateMetricsBatch обрабатывает батчевое обновление метрик
func updateMetricsBatch(store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if req.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var metrics []models.Metrics
		var buf bytes.Buffer

		// Читаем тело запроса
		_, err := buf.ReadFrom(req.Body)
		if err != nil {
			http.Error(w, fmt.Errorf("failed to read request body: %w", err).Error(), http.StatusBadRequest)
			return
		}

		// Десериализуем JSON в массив метрик
		if err = json.Unmarshal(buf.Bytes(), &metrics); err != nil {
			http.Error(w, fmt.Errorf("failed to unmarshal JSON: %w", err).Error(), http.StatusBadRequest)
			return
		}

		// Проверяем, что массив не пустой
		if len(metrics) == 0 {
			http.Error(w, "empty metrics batch", http.StatusBadRequest)
			return
		}

		// Обновляем метрики батчем
		if err := store.UpdateMetricsBatch(ctx, metrics); err != nil {
			logger.Log.Error("Failed to update metrics batch", zap.Error(err))
			http.Error(w, fmt.Errorf("failed to update metrics: %w", err).Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Возвращаем успешный статус
		response := map[string]string{"status": "ok"}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Log.Error("Failed to encode response", zap.Error(err))
		}
	}
}
