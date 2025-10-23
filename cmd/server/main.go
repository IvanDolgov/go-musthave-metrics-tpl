package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
)

// run запускает приложение с переданной конфигурацией
func run(cfg Config) error {

	// создаем строку с сервером или без
	fullPathServer := buildServerAddress(cfg.Server, cfg.Port)

	// создаем хранилище
	storage := NewMemStorage()

	// создаем роутер
	router := chi.NewRouter()

	router.Get(`/`, summaryMetrics(storage))
	router.Get("/update/{type_metric}//{value_metric}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Metric name cannot be empty", http.StatusNotFound)
	})
	router.Post(`/update/{type_metric}/{metric}/{value_metric}`, getMetrics(storage))
	router.Get(`/value/{type_metric}/{metric}`, sendMetrics(storage))
	err := http.ListenAndServe(fullPathServer, router)
	if err != nil {
		panic(err)
	}

	return nil
}

func main() {
	// Получаем конфигурацию
	cfg := parseFlags()

	// Запускаем приложение
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Application error: %v\n", err)
		os.Exit(1)
	}
}

func buildServerAddress(server, port string) string {
	if strings.TrimSpace(server) == "" {
		return ":" + port
	}
	return server + ":" + port
}
