package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

// getEnvBool получает булево значение из environment variable
func getEnvBool(key string, defaultVal bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultVal
}

// getEnvInt64 получает int64 значение из environment variable
func getEnvInt64(key string, defaultVal int64) int64 {
	if value, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultVal
}

// parseFlags парсит флаги командной строки и возвращает Config
func parseFlags() models.Config {
	var (
		address         string
		storeInterval   int64
		fileStoragePath string
		restore         bool
	)

	// Регистрируем флаги
	flag.StringVar(&address, "a", "localhost:8080", "server address")

	flag.Int64Var(&storeInterval, "i", 300, "store interval")
	flag.StringVar(&fileStoragePath, "f", "./file.storage", "path storage file")
	flag.BoolVar(&restore, "r", true, "upload previos metrics from file")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Supported flags:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	flag.Parse()

	// Проверяем есть ли дополнительные аргументы
	if len(flag.Args()) > 0 {
		fmt.Fprintf(os.Stderr, "Error: unknown arguments: %v\n", flag.Args())
		flag.Usage()
	}

	// берем перменные из env, а если не задано, то используем флаги. Приоритет у енвов
	if envAddress := os.Getenv("ADDRESS"); envAddress != "" {
		address = envAddress
	}

	// запускаем функцию в которой проверяем енв, а если нет то берет ту переменную которые из аргументов
	storeInterval = getEnvInt64("STORE_INTERVAL", storeInterval)

	if envFileStoragePath := os.Getenv("FILE_STORAGE_PATH"); envFileStoragePath != "" {
		fileStoragePath = envFileStoragePath
	}

	restore = getEnvBool("RESTORE", restore)

	// Создаем и заполняем конфигурацию
	cfg := models.Config{
		Address:         address,
		StoreInterval:   storeInterval,
		FileStoragePath: fileStoragePath,
		Restore:         restore,
	}

	// Парсим адрес на server и port
	parts := strings.Split(address, ":")
	if len(parts) == 2 {
		cfg.Server = parts[0]
		cfg.Port = parts[1]
	} else {
		cfg.Server = parts[0]
		cfg.Port = "8080" // порт по умолчанию
	}

	return cfg
}
