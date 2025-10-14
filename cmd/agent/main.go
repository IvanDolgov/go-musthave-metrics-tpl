package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"runtime/metrics"
	"strings"
	"syscall"
	"time"
)

// run запускает приложение с переданной конфигурацией
func run(cfg Config) error {
	// Канал для сигналов завершения
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	fmt.Println("Программа запущена. Нажмите Ctrl+C для остановки")

	// структура для описания метрик
	type MetricWithName struct {
		Sample    metrics.Sample
		ShortName string
	}

	// список метрик с именами
	metricsWithNames := []MetricWithName{
		{metrics.Sample{Name: "/memory/classes/total:bytes"}, "Alloc"},
		{metrics.Sample{Name: "/memory/classes/profiling/buckets:bytes"}, "BuckHashSys"},
		{metrics.Sample{Name: "/memory/classes/heap/released:bytes"}, "HeapReleased"},
		{metrics.Sample{Name: "/memory/classes/metadata/mspan/inuse:bytes"}, "MSpanInuse"},
		{metrics.Sample{Name: "/memory/classes/metadata/mspan/free:bytes"}, "MSpanSys"},
		{metrics.Sample{Name: "/memory/classes/metadata/mcache/inuse:bytes"}, "MCacheInuse"},
		{metrics.Sample{Name: "/memory/classes/metadata/mcache/free:bytes"}, "MCacheSys"},
		{metrics.Sample{Name: "/memory/classes/os-stacks:bytes"}, "StackInuse"},
		{metrics.Sample{Name: "/memory/classes/other:bytes"}, "OtherSys"},
		{metrics.Sample{Name: "/memory/classes/total:bytes"}, "Sys"},
		{metrics.Sample{Name: "/gc/cpu/fraction:gc-cpu-fraction"}, "GCCPUFraction"},
		{metrics.Sample{Name: "/memory/classes/heap/unused:bytes"}, "HeapIdle"},
		{metrics.Sample{Name: "/memory/classes/heap/objects:bytes"}, "HeapInuse"},
		{metrics.Sample{Name: "/gc/heap/goal:bytes"}, "NextGC"},
		{metrics.Sample{Name: "/gc/pauses:seconds"}, "PauseTotalNs"},
		{metrics.Sample{Name: "/gc/heap/frees:objects"}, "Frees"},
		{metrics.Sample{Name: "/gc/heap/objects:objects"}, "HeapObjects"},
		{metrics.Sample{Name: "/gc/cycles/total:gc-cycles"}, "NumGC"},
		{metrics.Sample{Name: "/gc/cycles/forced:gc-cycles"}, "NumForcedGC"},
		{metrics.Sample{Name: "/gc/heap/allocs:objects"}, "Mallocs"},
		{metrics.Sample{Name: "/gc/heap/allocs:bytes"}, "TotalAlloc"},
	}

	// Подсчет количество запусков сбора метрик
	var metricsReadCounter int

	go func() {
		for {
			metricsReadCounter++
			fmt.Println("Get metrics", time.Now().Format("15:04:05"))
			// Прочитать метрики без имен
			// metrics.Read(need_metrics)

			// Прочитать метрики с условиями, что там имена лежат отдельно
			for i := range metricsWithNames {
				metrics.Read([]metrics.Sample{metricsWithNames[i].Sample})
			}

			time.Sleep(time.Duration(cfg.PollInterval))
		}
	}()

	time.Sleep(1 * time.Second)

	go func() {
		for {
			fmt.Println("Send metrics", time.Now().Format("15:04:05"))
			// Обработать результаты
			for _, metric := range metricsWithNames {
				fmt.Printf("%s: ", metric.ShortName) // _ -> i
				var value float64
				switch metric.Sample.Value.Kind() {
				case metrics.KindUint64:
					value = float64(metric.Sample.Value.Uint64())
				case metrics.KindFloat64:
					value = metric.Sample.Value.Float64()

				}
				fmt.Printf("%f\n", value)
				sendMetric("gauge", metric.ShortName, value, cfg)

			}
			// отправляем счетчик
			fmt.Printf("%s: %d", "PollCount", metricsReadCounter)
			sendMetric("counter", "PollCount", metricsReadCounter, cfg)

			// отправляем рандомное число
			RandomValue := rand.Float64() * 100
			fmt.Printf("%s: %f", "RandomValue", RandomValue)
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
	// Формируем URL с параметрами
	fullPathServer := buildServerAddress(cfg.Server, cfg.Port)
	endpoint := fmt.Sprintf("http://%s/update/%s/%s/%v",
		fullPathServer, metricType, name, value)

	// Отправляем POST запрос
	response, err := http.Post(endpoint, "text/plain", nil)
	if err != nil {
		fmt.Println("Error sending metric:", err)
		return
	}

	defer response.Body.Close()

	// Можно добавить проверку статуса
	if response.StatusCode != http.StatusOK {
		fmt.Printf("Server returned non-OK status: %d\n", response.StatusCode)
	}
}

func buildServerAddress(server, port string) string {
	if strings.TrimSpace(server) == "" {
		return ":" + port
	}
	return server + ":" + port
}
