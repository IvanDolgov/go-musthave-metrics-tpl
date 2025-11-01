package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// getMetricsWithSync возвращает обработчик с синхронным сохранением
func getMetricsWithSync(storage *MemStorage, filePath string) http.HandlerFunc {
	handler := getMetrics(storage)
	return func(w http.ResponseWriter, req *http.Request) {
		handler(w, req)
		// Синхронно сохраняем после обновления метрик
		if err := storage.SaveToFile(filePath); err != nil {
			logger.Log.Error("Failed to sync save metrics", zap.String("file", filePath), zap.Error(err))
		}
	}
}

// getJSONMetricWithSync возвращает обработчик с синхронным сохранением
func getJSONMetricWithSync(storage *MemStorage, filePath string) http.HandlerFunc {
	handler := getJSONMetric(storage)
	return func(w http.ResponseWriter, req *http.Request) {
		handler(w, req)
		// Синхронно сохраняем после обновления метрик
		if err := storage.SaveToFile(filePath); err != nil {
			logger.Log.Error("Failed to sync save metrics", zap.String("file", filePath), zap.Error(err))
		}
	}
}

func sendMetrics(storage *MemStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		metricType := chi.URLParam(req, "type_metric")
		name := chi.URLParam(req, "metric")

		valueMetric, exists := storage.GetMetric(name, models.MetricType(metricType))

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

func summaryMetrics(storage *MemStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		gaugeMetrics, counterMetrics := storage.GetAllMetrics()

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

func getMetrics(storage *MemStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
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
			storage.SetGauge(name, value)
			w.WriteHeader(http.StatusOK)

		case "counter":
			// strconv позволяет проверть тип
			value, err := strconv.ParseInt(valueMetric, 10, 64)
			if err != nil {
				http.Error(w, "Invalid counter value", http.StatusBadRequest)
				return
			}
			storage.IncrementCounter(name, value)
			w.WriteHeader(http.StatusOK)

		default:
			http.Error(w, "Invalid type metric", http.StatusBadRequest)

		}
	}
}

func getJSONMetric(storage *MemStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
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
			storage.SetGauge(metric.ID, *metric.Value)

		case "counter":
			if metric.Delta == nil {
				http.Error(w, "Missing delta for counter metric", http.StatusBadRequest)
				return
			}
			storage.IncrementCounter(metric.ID, *metric.Delta)

		default:
			http.Error(w, "Invalid metric type", http.StatusBadRequest)
			return
		}

		// установим правильный заголовок для типа данных
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// // Возвращаем обновленную метрику, чтоб понимать удалось ли опубликовать
		updatedMetric := storage.GetMetricForJSON(metric.ID, models.MetricType(metric.MType))
		json.NewEncoder(w).Encode(updatedMetric)
	}
}

func sendJSONMetric(storage *MemStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
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
		foundMetric := storage.GetMetricForJSON(metric.ID, models.MetricType(metric.MType))

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
