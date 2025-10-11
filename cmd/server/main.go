package main

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

func mainPage(w http.ResponseWriter, req *http.Request) {
	w.Write([]byte("Привет!"))
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
		vars := mux.Vars(req)
		metricType := vars["type_metric"]
		name := vars["metric"]
		valueMetric := vars["value_metric"]
		// fmt.Println(name)

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
	router := mux.NewRouter()
	// Отключаем очистку пути. требуетя для обработки пустого name
	router.SkipClean(true)

	// обрабатывемемли name пусто
	router.HandleFunc("/update/{type_metric}//{value_metric}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Metric name cannot be empty", http.StatusNotFound)
	})
	// // Обрабатываем случай только с пробелами
	// router.HandleFunc("/update/{type_metric}/{metric:\\S+}/{value_metric}", func(w http.ResponseWriter, r *http.Request) {
	// 	http.Error(w, "Metric name cannot contain only spaces", http.StatusNotFound)
	// })

	router.HandleFunc(`/update/{type_metric}/{metric}/{value_metric}`, getMetrics(storage))

	router.HandleFunc(`/`, mainPage)

	err := http.ListenAndServe(`:8080`, router)
	if err != nil {
		panic(err)
	}
}
