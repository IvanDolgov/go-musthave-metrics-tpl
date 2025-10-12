package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Функция для суммирования двух map
// func sumMaps(map1, map2 map[string]float64) map[string]int64 {
// 	result := make(map[string]int)

// 	// Добавляем значения из первого map
// 	for key, value := range map1 {
// 		result[key] += value
// 	}

// 	// Добавляем значения из второго map
// 	for key, value := range map2 {
// 		result[key] += value
// 	}

// 	return result
// }

func sendMetrics(storage *MemStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		metricType := chi.URLParam(req, "type_metric")
		name := chi.URLParam(req, "metric")

		valueMetric, exists := storage.GetMetric(name, MetricType(metricType))

		if exists {
			var message string
			if metricType == "counter" {
				// Для counter выводим как целое число
				message = fmt.Sprintf("%s: %d", name, valueMetric)
			} else {
				// Для gauge выводим как число с плавающей точкой
				message = fmt.Sprintf("%s: %f", name, valueMetric)
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

		w.Header().Set("Content-Type", "text/plain")
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
		w.Header().Set("Content-Type", "text/plain")
		// r.HeadersRegexp("Content-Type", "application/(text|json)")

		// считываем переменные из url
		// vars := mux.Vars(req)
		// metricType := vars["type_metric"]
		// name := vars["metric"]
		// valueMetric := vars["value_metric"]
		// fmt.Println(name)

		metricType := chi.URLParam(req, "type_metric")
		name := chi.URLParam(req, "metric")
		valueMetric := chi.URLParam(req, "value_metric")

		// разбираем метрики по типы
		switch metricType {
		case "gauge":
			// fmt.Println("Type gauge")
			// strconv позволяет проверть тип
			value, err := strconv.ParseFloat(valueMetric, 64)
			if err != nil {
				http.Error(w, "Invalid gauge value", http.StatusBadRequest)
				// w.WriteHeader(http.StatusBadRequest)
				return
			}
			storage.SetGauge(name, value)
			w.WriteHeader(http.StatusOK)

		case "counter":
			// fmt.Println("Type counter")
			// strconv позволяет проверть тип
			value, err := strconv.ParseInt(valueMetric, 10, 64)
			if err != nil {
				http.Error(w, "Invalid counter value", http.StatusBadRequest)
				// w.WriteHeader(http.StatusBadRequest)
				return
			}
			storage.IncrementCounter(name, value)
			w.WriteHeader(http.StatusOK)

		// case "test":
		// 	fmt.Println("Wednesday is wacky.")
		default:
			http.Error(w, "Invalid type metric", http.StatusBadRequest)
			// w.WriteHeader(http.StatusBadRequest)

		}
		// res.Write([]byte("Это страница /api."))
		fmt.Println(storage.GetAllMetrics())
	}
}

func main() {
	storage := NewMemStorage()
	router := chi.NewRouter()

	router.Get(`/`, summaryMetrics(storage))
	router.Get("/update/{type_metric}//{value_metric}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Metric name cannot be empty", http.StatusNotFound)
	})
	router.Post(`/update/{type_metric}/{metric}/{value_metric}`, getMetrics(storage))
	router.Get(`/value/{type_metric}/{metric}`, sendMetrics(storage))

	// Отключаем очистку пути. требуетя для обработки пустого name
	// router.SkipClean(true)

	// обрабатывемемли name пусто
	// router.HandleFunc("/update/{type_metric}//{value_metric}", func(w http.ResponseWriter, r *http.Request) {
	// 	http.Error(w, "Metric name cannot be empty", http.StatusNotFound)
	// })
	// // Обрабатываем случай только с пробелами
	// router.HandleFunc("/update/{type_metric}/{metric:\\S+}/{value_metric}", func(w http.ResponseWriter, r *http.Request) {
	// 	http.Error(w, "Metric name cannot contain only spaces", http.StatusNotFound)
	// })

	// router.HandleFunc(`/update/{type_metric}/{metric}/{value_metric}`, getMetrics(storage))

	// router.HandleFunc(`/`, summuryMetrics(storage))

	err := http.ListenAndServe(`:8080`, router)
	if err != nil {
		panic(err)
	}
}
