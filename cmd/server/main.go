package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
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
				// message = fmt.Sprintf("%s: %d", name, valueMetric)
				message = fmt.Sprintf("%d", valueMetric)
			} else {
				// Для gauge выводим как число с плавающей точкой
				// message = fmt.Sprintf("%s: %g", name, valueMetric)
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

// задаем переменные для работы с flag
var (
	address string
	server  string
	port    string
)

func parseFlags() {
	// Флаг в формате server:port
	flag.StringVar(&address, "a", "localhost:8080", "server address (short)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Supported flags:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	flag.Parse()

	// Самый простой способ - проверяем есть ли дополнительные аргументы
	if len(flag.Args()) > 0 {
		fmt.Fprintf(os.Stderr, "Error: unknown arguments: %v\n", flag.Args())
		flag.Usage()
	}

	// Парсим адрес на server и port
	parts := strings.Split(address, ":")
	if len(parts) == 2 {
		server = parts[0]
		port = parts[1]
	} else {
		server = parts[0]
		port = "8080" // порт по умолчанию
	}
}

func buildServerAddress(server, port string) string {
	if strings.TrimSpace(server) == "" {
		return ":" + port
	}
	return server + ":" + port
}

func main() {
	// считываем аргументы из аргументов
	parseFlags()

	// fmt.Printf("Server: %s\n", server)
	// fmt.Printf("Port: %s\n", port)
	// fmt.Printf("Full address: %s:%s\n", server, port)
	// создаем строку с сервером или без
	fullPathServer := buildServerAddress(server, port)

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

	err := http.ListenAndServe(fullPathServer, router)
	if err != nil {
		panic(err)
	}
}
