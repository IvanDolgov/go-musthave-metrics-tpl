package config

import (
	"flag"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

// ParseFlags парсит флаги командной строки и возвращает Config
func ParseFlags() models.Config {
	var (
		address         string
		storeInterval   int64
		fileStoragePath string
		restore         bool
		pollInterval    time.Duration
		reportInterval  time.Duration
	)

	// Регистрируем флаги для сервера
	flag.StringVar(&address, "a", "localhost:8080", "server address")
	flag.Int64Var(&storeInterval, "i", 300, "store interval")
	flag.StringVar(&fileStoragePath, "f", "./.storage", "path storage file")
	flag.BoolVar(&restore, "r", true, "upload previous metrics from file")

	// Регистрируем флаги для агента
	flag.DurationVar(&pollInterval, "p", 2*time.Second, "poll interval")
	flag.DurationVar(&reportInterval, "report", 10*time.Second, "report interval")

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

	// Парсим адрес на server и port
	server, port := parseAddress(address)

	return models.Config{
		Address:         address,
		Server:          server,
		Port:            port,
		StoreInterval:   storeInterval,
		FileStoragePath: fileStoragePath,
		Restore:         restore,
		PollInterval:    pollInterval,
		ReportInterval:  reportInterval,
	}
}

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

func parseAddress(address string) (string, string) {
	parts := strings.Split(address, ":")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], "8080" // порт по умолчанию
}
