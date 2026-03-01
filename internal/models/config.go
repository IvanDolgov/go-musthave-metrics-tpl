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
	DatabaseDSN     string
	Key             string // ключ для шифрования
	RateLimit       int64  // количество потоков
	AuditFile       string // путь к файлу аудита
	AuditURL        string // URL для отправки аудита
	CryptoKey       string // путь к файлу с ключом (публичным для агента, приватным для сервера)
	TrustedSubnet   string // доверенная подсеть в формате CIDR
}
