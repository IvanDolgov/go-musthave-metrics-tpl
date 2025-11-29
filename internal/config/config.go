package config

import (
	"flag"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

// ParseServerFlags парсит флаги командной строки для сервера
func ParseServerFlags() models.Config {
	var (
		address         string
		storeInterval   int64
		fileStoragePath string
		restore         bool
		databaseDsn     string
		key             string // секретный ключ
	)

	// Регистрируем флаги для сервера
	flag.StringVar(&address, "a", "localhost:8080", "server address")
	flag.Int64Var(&storeInterval, "i", 300, "store interval in seconds")
	flag.StringVar(&fileStoragePath, "f", "", "path storage file")
	flag.BoolVar(&restore, "r", true, "upload previous metrics from file")
	flag.StringVar(&databaseDsn, "d", "", "database_dsn")
	flag.StringVar(&key, "k", "", "secret key for request signing")

	flag.Parse()

	// Применяем приоритеты параметров (env vars имеют приоритет над флагами)
	if envAddress := os.Getenv("ADDRESS"); envAddress != "" {
		address = envAddress
	}

	storeInterval = getEnvInt64("STORE_INTERVAL", storeInterval)

	if envFileStoragePath := os.Getenv("FILE_STORAGE_PATH"); envFileStoragePath != "" {
		fileStoragePath = envFileStoragePath
	}

	restore = getEnvBool("RESTORE", restore)

	if envDatabaseDSN := os.Getenv("DATABASE_DSN"); envDatabaseDSN != "" {
		databaseDsn = envDatabaseDSN
	}

	if envKey := os.Getenv("KEY"); envKey != "" {
		key = envKey
	}

	// Парсим адрес на server и port
	server, port := parseAddress(address)

	return models.Config{
		Address:         address,
		Server:          server,
		Port:            port,
		StoreInterval:   storeInterval,
		FileStoragePath: fileStoragePath,
		Restore:         restore,
		DatabaseDSN:     databaseDsn,
		Key:             key,
	}
}

// ParseAgentFlags парсит флаги командной строки для агента
func ParseAgentFlags() models.Config {
	var (
		address        string
		pollInterval   int64  // в секундах для совместимости с тестами
		reportInterval int64  // в секундах для совместимости с тестами
		key            string // секретный ключ
		rateLimit      int64  // rate limit
	)

	// Регистрируем флаги для агента
	flag.StringVar(&address, "a", "localhost:8080", "server address")
	flag.Int64Var(&pollInterval, "p", 2, "poll interval in seconds")
	flag.Int64Var(&reportInterval, "r", 10, "report interval in seconds")
	flag.StringVar(&key, "k", "", "secret key for request signing")
	flag.Int64Var(&rateLimit, "l", 1, "rate limit")

	flag.Parse()

	// Применяем приоритеты параметров (env vars имеют приоритет над флагами)
	if envAddress := os.Getenv("ADDRESS"); envAddress != "" {
		address = envAddress
	}

	pollInterval = getEnvInt64("POLL_INTERVAL", pollInterval)
	reportInterval = getEnvInt64("REPORT_INTERVAL", reportInterval)

	if envKey := os.Getenv("KEY"); envKey != "" {
		key = envKey
	}

	if envRateLimit := os.Getenv("RATE_LIMIT"); envRateLimit != "" {
		key = envRateLimit
	}

	// Парсим адрес на server и port
	server, port := parseAddress(address)

	return models.Config{
		Address:        address,
		Server:         server,
		Port:           port,
		PollInterval:   time.Duration(pollInterval) * time.Second,
		ReportInterval: time.Duration(reportInterval) * time.Second,
		Key:            key,
		RateLimit:      rateLimit,
	}
}

// остальные функции остаются без изменений...
func getEnvBool(key string, defaultVal bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultVal
}

func getEnvInt64(key string, defaultVal int64) int64 {
	if value, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultVal
}

// parseAddress разбивает адрес на сервер и порт
func parseAddress(address string) (string, string) {
	parts := strings.Split(address, ":")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], "8080" // порт по умолчанию
}
