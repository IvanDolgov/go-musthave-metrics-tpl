package config

import (
	"encoding/json"
	"flag"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

// ParseServerFlags парсит флаги командной строки для сервера с поддержкой JSON конфига
func ParseServerFlags() models.Config {
	var (
		address         string
		storeInterval   int64
		fileStoragePath string
		restore         bool
		databaseDsn     string
		key             string
		cryptoKey       string
		auditFile       string
		auditURL        string
		configFile      string
	)

	// Регистрируем флаги для сервера
	flag.StringVar(&address, "a", "localhost:8080", "server address")
	flag.Int64Var(&storeInterval, "i", 300, "store interval in seconds")
	flag.StringVar(&fileStoragePath, "f", "", "path storage file")
	flag.BoolVar(&restore, "r", true, "upload previous metrics from file")
	flag.StringVar(&databaseDsn, "d", "", "database_dsn")
	flag.StringVar(&key, "k", "", "secret key for request signing")
	flag.StringVar(&cryptoKey, "crypto-key", "", "path to private key file for decryption")
	flag.StringVar(&auditFile, "audit-file", "", "path to audit log file")
	flag.StringVar(&auditURL, "audit-url", "", "URL for remote audit logging")
	flag.StringVar(&configFile, "c", "", "path to config file")
	flag.StringVar(&configFile, "config", "", "path to config file")

	flag.Parse()

	// Проверяем переменную окружения CONFIG
	if envConfig := os.Getenv("CONFIG"); envConfig != "" && configFile == "" {
		configFile = envConfig
	}

	// Загружаем JSON конфигурацию, если указан файл
	jsonConfig, _ := loadJSONConfig(configFile) // игнорируем ошибку, используем значения по умолчанию

	// Применяем значения из JSON конфига (низкий приоритет)
	if jsonConfig != nil {
		if val, ok := jsonConfig["address"]; ok {
			if str := parseString(val, ""); str != "" && address == "localhost:8080" {
				address = str
			}
		}
		if val, ok := jsonConfig["restore"]; ok {
			restore = parseBool(val, restore)
		}
		if val, ok := jsonConfig["store_interval"]; ok {
			if str := parseString(val, ""); str != "" {
				// Парсим длительность из строки (например "300s")
				duration := parseDuration(str, time.Duration(storeInterval)*time.Second)
				storeInterval = int64(duration.Seconds())
			}
		}
		if val, ok := jsonConfig["store_file"]; ok {
			if str := parseString(val, ""); str != "" && fileStoragePath == "" {
				fileStoragePath = str
			}
		}
		if val, ok := jsonConfig["database_dsn"]; ok {
			if str := parseString(val, ""); str != "" && databaseDsn == "" {
				databaseDsn = str
			}
		}
		if val, ok := jsonConfig["crypto_key"]; ok {
			if str := parseString(val, ""); str != "" && cryptoKey == "" {
				cryptoKey = str
			}
		}
		if val, ok := jsonConfig["key"]; ok {
			if str := parseString(val, ""); str != "" && key == "" {
				key = str
			}
		}
		if val, ok := jsonConfig["audit_file"]; ok {
			if str := parseString(val, ""); str != "" && auditFile == "" {
				auditFile = str
			}
		}
		if val, ok := jsonConfig["audit_url"]; ok {
			if str := parseString(val, ""); str != "" && auditURL == "" {
				auditURL = str
			}
		}
	}

	// Применяем приоритеты параметров (env vars имеют приоритет над флагами и JSON)
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

	if envCryptoKey := os.Getenv("CRYPTO_KEY"); envCryptoKey != "" {
		cryptoKey = envCryptoKey
	}

	if envAuditFile := os.Getenv("AUDIT_FILE"); envAuditFile != "" {
		auditFile = envAuditFile
	}

	if envAuditURL := os.Getenv("AUDIT_URL"); envAuditURL != "" {
		auditURL = envAuditURL
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
		CryptoKey:       cryptoKey,
		AuditFile:       auditFile,
		AuditURL:        auditURL,
	}
}

// ParseAgentFlags парсит флаги командной строки для агента с поддержкой JSON конфига
func ParseAgentFlags() models.Config {
	var (
		address        string
		pollInterval   int64
		reportInterval int64
		key            string
		cryptoKey      string
		rateLimit      int64
		configFile     string
	)

	// Регистрируем флаги для агента
	flag.StringVar(&address, "a", "localhost:8080", "server address")
	flag.Int64Var(&pollInterval, "p", 2, "poll interval in seconds")
	flag.Int64Var(&reportInterval, "r", 10, "report interval in seconds")
	flag.StringVar(&key, "k", "", "secret key for request signing")
	flag.StringVar(&cryptoKey, "crypto-key", "", "path to public key file for encryption")
	flag.Int64Var(&rateLimit, "l", 1, "rate limit")
	flag.StringVar(&configFile, "c", "", "path to config file")
	flag.StringVar(&configFile, "config", "", "path to config file")

	flag.Parse()

	// Проверяем переменную окружения CONFIG
	if envConfig := os.Getenv("CONFIG"); envConfig != "" && configFile == "" {
		configFile = envConfig
	}

	// Загружаем JSON конфигурацию, если указан файл
	jsonConfig, _ := loadJSONConfig(configFile) // игнорируем ошибку, используем значения по умолчанию

	// Применяем значения из JSON конфига (низкий приоритет)
	if jsonConfig != nil {
		if val, ok := jsonConfig["address"]; ok {
			if str := parseString(val, ""); str != "" && address == "localhost:8080" {
				address = str
			}
		}
		if val, ok := jsonConfig["report_interval"]; ok {
			if str := parseString(val, ""); str != "" {
				duration := parseDuration(str, time.Duration(reportInterval)*time.Second)
				reportInterval = int64(duration.Seconds())
			}
		}
		if val, ok := jsonConfig["poll_interval"]; ok {
			if str := parseString(val, ""); str != "" {
				duration := parseDuration(str, time.Duration(pollInterval)*time.Second)
				pollInterval = int64(duration.Seconds())
			}
		}
		if val, ok := jsonConfig["crypto_key"]; ok {
			if str := parseString(val, ""); str != "" && cryptoKey == "" {
				cryptoKey = str
			}
		}
		if val, ok := jsonConfig["key"]; ok {
			if str := parseString(val, ""); str != "" && key == "" {
				key = str
			}
		}
		if val, ok := jsonConfig["rate_limit"]; ok {
			rateLimit = parseInt64(val, rateLimit)
		}
	}

	// Применяем приоритеты параметров (env vars имеют приоритет над флагами и JSON)
	if envAddress := os.Getenv("ADDRESS"); envAddress != "" {
		address = envAddress
	}

	pollInterval = getEnvInt64("POLL_INTERVAL", pollInterval)
	reportInterval = getEnvInt64("REPORT_INTERVAL", reportInterval)

	if envKey := os.Getenv("KEY"); envKey != "" {
		key = envKey
	}

	if envCryptoKey := os.Getenv("CRYPTO_KEY"); envCryptoKey != "" {
		cryptoKey = envCryptoKey
	}

	if envRateLimit := os.Getenv("RATE_LIMIT"); envRateLimit != "" {
		if val, err := strconv.ParseInt(envRateLimit, 10, 64); err == nil {
			rateLimit = val
		}
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
		CryptoKey:      cryptoKey,
		RateLimit:      rateLimit,
	}
}

// loadJSONConfig загружает конфигурацию из JSON файла (внутренняя функция)
func loadJSONConfig(path string) (map[string]interface{}, error) {
	if path == "" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return config, nil
}

// parseDuration парсит длительность из строки (внутренняя функция)
func parseDuration(s string, defaultVal time.Duration) time.Duration {
	if s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultVal
	}
	return d
}

// parseInt64 парсит int64 из интерфейса (внутренняя функция)
func parseInt64(v interface{}, defaultVal int64) int64 {
	if v == nil {
		return defaultVal
	}
	switch val := v.(type) {
	case float64:
		return int64(val)
	case int64:
		return val
	case int:
		return int64(val)
	default:
		return defaultVal
	}
}

// parseBool парсит bool из интерфейса (внутренняя функция)
func parseBool(v interface{}, defaultVal bool) bool {
	if v == nil {
		return defaultVal
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		if val == "true" {
			return true
		}
		if val == "false" {
			return false
		}
	default:
		return defaultVal
	}
	return defaultVal
}

// parseString парсит строку из интерфейса (внутренняя функция)
func parseString(v interface{}, defaultVal string) string {
	if v == nil {
		return defaultVal
	}
	if s, ok := v.(string); ok {
		return s
	}
	return defaultVal
}

// getEnvBool получает значение bool из переменной окружения
func getEnvBool(key string, defaultVal bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultVal
}

// getEnvInt64 получает значение int64 из переменной окружения
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
	return parts[0], "8080"
}
