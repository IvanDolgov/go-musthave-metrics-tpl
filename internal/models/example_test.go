package models

import (
	"encoding/json"
	"fmt"
)

// ExampleMetrics демонстрирует создание и сериализацию метрик.
func ExampleMetrics() {
	// Создаем gauge метрику
	gaugeValue := 42.5
	gaugeMetric := Metrics{
		ID:    "cpu_usage",
		MType: "gauge",
		Value: &gaugeValue,
	}

	// Создаем counter метрику
	counterDelta := int64(100)
	counterMetric := Metrics{
		ID:    "requests",
		MType: "counter",
		Delta: &counterDelta,
	}

	// Сериализуем в JSON
	gaugeJSON, _ := json.Marshal(gaugeMetric)
	counterJSON, _ := json.Marshal(counterMetric)

	fmt.Printf("Gauge JSON: %s\n", string(gaugeJSON))
	fmt.Printf("Counter JSON: %s\n", string(counterJSON))

	// Десериализуем обратно
	var parsedGauge Metrics
	json.Unmarshal(gaugeJSON, &parsedGauge)
	fmt.Printf("Parsed gauge ID: %s, Type: %s\n", parsedGauge.ID, parsedGauge.MType)

	// Вывод:
	// Gauge JSON: {"id":"cpu_usage","type":"gauge","value":42.5}
	// Counter JSON: {"id":"requests","type":"counter","delta":100}
	// Parsed gauge ID: cpu_usage, Type: gauge
}

// ExampleFileMetric демонстрирует использование FileMetric для сохранения в файл.
func ExampleFileMetric() {
	// Создаем FileMetric для gauge
	gaugeValue := 75.3
	gaugeFileMetric := FileMetric{
		ID:    "memory_usage",
		Type:  "gauge",
		Value: &gaugeValue,
	}

	// Создаем FileMetric для counter
	counterDelta := int64(50)
	counterFileMetric := FileMetric{
		ID:    "errors",
		Type:  "counter",
		Delta: &counterDelta,
	}

	// Создаем массив для сохранения в файл
	fileMetrics := []FileMetric{gaugeFileMetric, counterFileMetric}
	jsonData, _ := json.MarshalIndent(fileMetrics, "", "  ")

	fmt.Println("File metrics JSON:")
	fmt.Println(string(jsonData))

	// Вывод:
	// File metrics JSON:
	// [
	//   {
	//     "id": "memory_usage",
	//     "type": "gauge",
	//     "value": 75.3
	//   },
	//   {
	//     "id": "errors",
	//     "type": "counter",
	//     "delta": 50
	//   }
	// ]
}

// ExampleMetricType демонстрирует использование типов метрик.
func ExampleMetricType() {
	// Использование констант типов метрик
	var metricType MetricType

	// Присваиваем тип gauge
	metricType = Gauge
	fmt.Printf("Metric type: %s\n", metricType)

	// Проверка типа
	if metricType == Gauge {
		fmt.Println("This is a gauge metric")
	}

	// Присваиваем тип counter
	metricType = Counter
	fmt.Printf("Metric type: %s\n", metricType)

	// Вывод:
	// Metric type: gauge
	// This is a gauge metric
	// Metric type: counter
}
