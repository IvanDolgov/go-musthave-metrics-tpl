package models

import "time"

// Config содержит все параметры конфигурации приложения
type Config struct {
	Address         string
	Server          string
	Port            string
	StoreInterval   int64
	FileStoragePath string
	Restore         bool
	PollInterval    time.Duration // для агента
	ReportInterval  time.Duration // для агента
}
