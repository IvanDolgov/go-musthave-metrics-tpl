package models

// MetricType представляет тип метрики
type MetricType string

const (
	Gauge   MetricType = "gauge"
	Counter MetricType = "counter"
)

// Metrics представляет метрику для JSON API
type Metrics struct {
	ID    string   `json:"id"`              // имя метрики
	MType string   `json:"type"`            // параметр, принимающий значение gauge или counter
	Delta *int64   `json:"delta,omitempty"` // значение метрики в случае передачи counter
	Value *float64 `json:"value,omitempty"` // значение метрики в случае передачи gauge
}

// FileMetric представляет метрику для сохранения в файл
type FileMetric struct {
	ID    string   `json:"id"`
	Type  string   `json:"type"`
	Value *float64 `json:"value,omitempty"`
	Delta *int64   `json:"delta,omitempty"`
}
