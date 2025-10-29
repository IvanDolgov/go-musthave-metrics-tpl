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
	"sync"
	"syscall"
	"time"
)

type Metrics struct {
	ID    string   `json:"id"`              // имя метрики
	MType string   `json:"type"`            // параметр, принимающий значение gauge или counter
	Delta *int64   `json:"delta,omitempty"` // значение метрики в случае передачи counter
	Value *float64 `json:"value,omitempty"` // значение метрики в случае передачи gauge
}

// Структура для хранения текущих метрик
type CurrentMetrics struct {
	MemStats    runtime.MemStats
	PollCount   int64
	RandomValue float64
	mu          sync.RWMutex
}

// run запускает приложение с переданной конфигурацией
func run(cfg Config) error {
	// Канал для сигналов завершения
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	fmt.Println("Программа запущена. Нажмите Ctrl+C для остановки")

	// Ждем немного чтобы сервер успел запуститься
	time.Sleep(2 * time.Second)

	// Текущие метрики
	currentMetrics := &CurrentMetrics{}

	// ГОРУТИНА СБОРА МЕТРИК (с интервалом PollInterval)
	go func() {
		pollTicker := time.NewTicker(time.Duration(cfg.PollInterval))
		defer pollTicker.Stop()

		for range pollTicker.C {
			currentMetrics.mu.Lock()
			currentMetrics.PollCount++
			currentMetrics.RandomValue = rand.Float64() * 100
			runtime.ReadMemStats(&currentMetrics.MemStats)
			currentMetrics.mu.Unlock()

			fmt.Printf("Collected metrics batch #%d\n", currentMetrics.PollCount)
		}
	}()

	// ГОРУТИНА ОТПРАВКИ МЕТРИК (с интервалом ReportInterval)
	go func() {
		reportTicker := time.NewTicker(time.Duration(cfg.ReportInterval))
		defer reportTicker.Stop()

		for range reportTicker.C {
			currentMetrics.mu.RLock()
			pollCount := currentMetrics.PollCount
			randomValue := currentMetrics.RandomValue
			memStats := currentMetrics.MemStats
			currentMetrics.mu.RUnlock()

			fmt.Printf("Sending metrics batch #%d to %s\n", pollCount, cfg.Address)
			sendRuntimeMetrics(cfg, pollCount, randomValue, memStats)
		}
	}()

	// Ждем сигнал завершения
	<-stop
	fmt.Println("\nЗавершение программы...")
	return nil
}

func sendRuntimeMetrics(cfg Config, pollCount int64, randomValue float64, memStats runtime.MemStats) {
	// Отправляем ВСЕ необходимые метрики из автотеста
	metricsToSend := map[string]float64{
		// Runtime метрики из memStats
		"Alloc":         float64(memStats.Alloc),
		"BuckHashSys":   float64(memStats.BuckHashSys),
		"Frees":         float64(memStats.Frees),
		"GCCPUFraction": memStats.GCCPUFraction,
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

		// Кастомные метрики
		"RandomValue": randomValue,
	}

	// Отправляем gauge метрики
	for name, value := range metricsToSend {
		sendMetric("gauge", name, value, cfg)
	}

	// Отправляем counter метрику
	sendMetric("counter", "PollCount", pollCount, cfg)
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
			fmt.Printf("Invalid gauge value type: %T\n", value)
			return
		}
	case "counter":
		var intValue int64
		switch v := value.(type) {
		case int64:
			intValue = v
		case int:
			intValue = int64(v)
		default:
			fmt.Printf("Invalid counter value type: %T\n", value)
			return
		}
		metric = Metrics{
			ID:    name,
			MType: metricType,
			Delta: &intValue,
		}
	default:
		fmt.Printf("Unknown metric type: %s\n", metricType)
		return
	}

	// Кодируем метрику в JSON
	jsonData, err := json.Marshal(metric)
	if err != nil {
		fmt.Println("Error encoding JSON:", err)
		return
	}

	// Сжимаем данные
	compressedData, err := GzipCompress(jsonData)
	if err != nil {
		fmt.Println("gzip compress error: %w", err)
		return
	}

	// Создаем клиент с таймаутом
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Создаем запрос
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(compressedData))
	if err != nil {
		fmt.Printf("Error creating request for %s: %v\n", name, err)
		return
	}

	// Устанавливаем правильные заголовки для REQUEST
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")

	// Отправляем запрос
	response, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error sending metric %s: %v\n", name, err)
		return
	}
	defer response.Body.Close()

	// Проверяем статус ответа
	if response.StatusCode != http.StatusOK {
		fmt.Printf("Server returned non-OK status for %s: %d\n", name, response.StatusCode)
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

func main() {
	// Получаем конфигурацию
	cfg := parseFlags()

	// Запускаем приложение
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Application error: %v\n", err)
		os.Exit(1)
	}
}
