package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

type Metrics struct {
	ID    string   `json:"id"`              // имя метрики
	MType string   `json:"type"`            // параметр, принимающий значение gauge или counter
	Delta *int64   `json:"delta,omitempty"` // значение метрики в случае передачи counter
	Value *float64 `json:"value,omitempty"` // значение метрики в случае передачи gauge
}

// run запускает приложение с переданной конфигурацией
func run(cfg Config) error {
	// Канал для сигналов завершения
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	fmt.Println("Программа запущена. Нажмите Ctrl+C для остановки")

	// Подсчет количество запусков сбора метрик
	var metricsReadCounter int64

	go func() {
		for {
			metricsReadCounter++
			fmt.Println("Get and send metrics", time.Now().Format("15:04:05"))
			fmt.Println(cfg.Address)

			// Собираем метрики через runtime.MemStats
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)

			// Отправляем gauge метрики
			gauges := map[string]float64{
				"Alloc":         float64(memStats.Alloc),
				"BuckHashSys":   float64(memStats.BuckHashSys),
				"Frees":         float64(memStats.Frees),
				"GCCPUFraction": float64(memStats.GCCPUFraction),
				"GCSys":         float64(memStats.GCSys),
				"HeapAlloc":     float64(memStats.HeapAlloc),
				"HeapIdle":      float64(memStats.HeapIdle),
				"HeapInuse":     float64(memStats.HeapInuse),
				"HeapObjects":   float64(memStats.HeapObjects),
				"HeapReleased":  float64(memStats.HeapReleased),
				"HeapSys":       float64(memStats.HeapSys),
				"LastGC":        float64(memStats.LastGC),
				"Lookups":       float64(memStats.Lookups),
				"MCacheInuse":   float64(memStats.MCacheInuse),
				"MCacheSys":     float64(memStats.MCacheSys),
				"MSpanInuse":    float64(memStats.MSpanInuse),
				"MSpanSys":      float64(memStats.MSpanSys),
				"Mallocs":       float64(memStats.Mallocs),
				"NextGC":        float64(memStats.NextGC),
				"NumForcedGC":   float64(memStats.NumForcedGC),
				"NumGC":         float64(memStats.NumGC),
				"OtherSys":      float64(memStats.OtherSys),
				"PauseTotalNs":  float64(memStats.PauseTotalNs),
				"StackInuse":    float64(memStats.StackInuse),
				"StackSys":      float64(memStats.StackSys),
				"Sys":           float64(memStats.Sys),
				"TotalAlloc":    float64(memStats.TotalAlloc),
			}

			for name, value := range gauges {
				fmt.Printf("%s: %f\n", name, value)
				sendMetric("gauge", name, value, cfg)
			}

			// отправляем счетчик
			fmt.Printf("%s: %d\n", "PollCount", metricsReadCounter)
			sendMetric("counter", "PollCount", metricsReadCounter, cfg)

			// отправляем рандомное число
			RandomValue := rand.Float64() * 100
			fmt.Printf("%s: %f\n", "RandomValue", RandomValue)
			sendMetric("gauge", "RandomValue", RandomValue, cfg)

			time.Sleep(time.Duration(cfg.ReportInterval))
		}
	}()

	// Ждем сигнал завершения
	<-stop
	fmt.Println("\nЗавершение программы...")

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

func sendMetric(metricType string, name string, value interface{}, cfg Config) {
	// Формируем полный адрес сервера
	fullPathServer := buildServerAddress(cfg.Server, cfg.Port)
	endpoint := fmt.Sprintf("http://%s/update", fullPathServer)

	// Создаем структуру для метрики с значением
	var metric Metrics

	// Заполняем метрику в зависимости от типа
	switch metricType {
	case "gauge":
		if floatValue, ok := value.(float64); ok {
			metric = Metrics{
				ID:    name,
				MType: metricType,
				Value: &floatValue,
			}
		} else {
			// Пробуем конвертировать другие числовые типы в float64
			switch v := value.(type) {
			case int:
				floatValue := float64(v)
				metric = Metrics{
					ID:    name,
					MType: metricType,
					Value: &floatValue,
				}
			case int64:
				floatValue := float64(v)
				metric = Metrics{
					ID:    name,
					MType: metricType,
					Value: &floatValue,
				}
			default:
				fmt.Printf("Invalid gauge value type: %T for metric %s\n", value, name)
				return
			}
		}
	case "counter":
		// Для counter всегда используем int64
		var intValue int64
		switch v := value.(type) {
		case int:
			intValue = int64(v)
		case int64:
			intValue = v
		case float64:
			intValue = int64(v)
		default:
			fmt.Printf("Invalid counter value type: %T for metric %s\n", value, name)
			return
		}
		metric = Metrics{
			ID:    name,
			MType: metricType,
			Delta: &intValue,
		}
	default:
		fmt.Printf("Unknown metric type: %s for metric %s\n", metricType, name)
		return
	}

	// Кодируем метрику в JSON
	jsonData, err := json.Marshal(metric)
	if err != nil {
		fmt.Printf("Error encoding JSON for metric %s: %v\n", name, err)
		return
	}

	// Отправляем POST запрос с JSON
	response, err := http.Post(endpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("Error sending metric %s: %v\n", name, err)
		return
	}

	defer response.Body.Close()

	// Проверяем статус ответа
	if response.StatusCode != http.StatusOK {
		fmt.Printf("Server returned non-OK status for metric %s: %d\n", name, response.StatusCode)
		return
	}

	// Читаем и выводим ответ (опционально)
	var responseMetric Metrics
	if err := json.NewDecoder(response.Body).Decode(&responseMetric); err != nil {
		fmt.Printf("Error decoding response for metric %s: %v\n", name, err)
		return
	}

	fmt.Printf("Successfully sent metric: %s=%v\n", name, value)
}

func buildServerAddress(server, port string) string {
	if strings.TrimSpace(server) == "" {
		return ":" + port
	}
	return server + ":" + port
}
