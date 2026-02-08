package audit

import (
	"time"
)

// Event представляет событие аудита
type Event struct {
	TS        int64    `json:"ts"`         // unix timestamp события
	Metrics   []string `json:"metrics"`    // наименование полученных метрик
	IPAddress string   `json:"ip_address"` // IP адрес входящего запроса
}

// NewEvent создает новое событие аудита
func NewEvent(metrics []string, ipAddress string) *Event {
	return &Event{
		TS:        time.Now().Unix(),
		Metrics:   metrics,
		IPAddress: ipAddress,
	}
}
