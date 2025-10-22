package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config содержит все параметры конфигурации приложения
type Config struct {
	Address        string
	Server         string
	Port           string
	PollInterval   time.Duration
	ReportInterval time.Duration
}

// parseFlags парсит флаги командной строки и возвращает Config
func parseFlags() Config {
	var (
		address        string
		pollInterval   int
		reportInterval int
	)

	// Регистрируем флаги
	flag.StringVar(&address, "a", "localhost:8080", "server address")
	flag.IntVar(&pollInterval, "r", 2, "update interval (sec)")
	flag.IntVar(&reportInterval, "p", 10, "push metrics interval (sec)")

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

	// Создаем и заполняем конфигурацию
	cfg := Config{
		Address:        address,
		PollInterval:   time.Duration(pollInterval) * time.Second,
		ReportInterval: time.Duration(reportInterval) * time.Second,
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
