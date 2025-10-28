package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

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
