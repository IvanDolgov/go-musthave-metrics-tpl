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

// ParseServerFlags парсит флаги командной строки для сервера
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
		trustedSubnet   string
		useGRPC         bool
		grpcAddress     string
	)

	// Создаем новый набор флагов для избежания конфликтов
	fs := flag.NewFlagSet("server", flag.ContinueOnError)

	// Регистрируем флаги для сервера
	fs.StringVar(&address, "a", "localhost:8080", "HTTP server address")
	fs.Int64Var(&storeInterval, "i", 300, "store interval in seconds")
	fs.StringVar(&fileStoragePath, "f", "", "path storage file")
	fs.BoolVar(&restore, "r", true, "upload previous metrics from file")
	fs.StringVar(&databaseDsn, "d", "", "database_dsn")
	fs.StringVar(&key, "k", "", "secret key for request signing")
	fs.StringVar(&cryptoKey, "crypto-key", "", "path to private key file for decryption")
	fs.StringVar(&auditFile, "audit-file", "", "path to audit log file")
	fs.StringVar(&auditURL, "audit-url", "", "URL for remote audit logging")
	fs.StringVar(&configFile, "c", "", "path to config file")
	fs.StringVar(&configFile, "config", "", "path to config file")
	fs.StringVar(&trustedSubnet, "t", "", "trusted subnet in CIDR notation")

	// Новые флаги для gRPC
	fs.BoolVar(&useGRPC, "grpc", false, "use gRPC instead of HTTP")
	fs.StringVar(&grpcAddress, "grpc-addr", "localhost:3200", "gRPC server address")

	fs.Parse(os.Args[1:])

	// Проверяем переменную окружения CONFIG
	if envConfig := os.Getenv("CONFIG"); envConfig != "" && configFile == "" {
		configFile = envConfig
	}

	// Загружаем JSON конфигурацию
	jsonConfig, _ := loadJSONConfig(configFile)

	// Применяем значения из JSON конфига (низкий приоритет)
	if jsonConfig != nil {
		applyJSONConfig(jsonConfig, &address, &storeInterval, &fileStoragePath, &restore,
			&databaseDsn, &key, &cryptoKey, &auditFile, &auditURL, &trustedSubnet,
			&useGRPC, &grpcAddress)
	}

	// Применяем переменные окружения (средний приоритет)
	applyEnvVars(&address, &storeInterval, &fileStoragePath, &restore, &databaseDsn,
		&key, &cryptoKey, &auditFile, &auditURL, &trustedSubnet, &useGRPC, &grpcAddress)

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
		TrustedSubnet:   trustedSubnet,
		UseGRPC:         useGRPC,
		GRPCAddress:     grpcAddress,
	}
}

// ParseAgentFlags парсит флаги командной строки для агента
func ParseAgentFlags() models.Config {
	var (
		address        string
		pollInterval   int64
		reportInterval int64
		key            string
		cryptoKey      string
		rateLimit      int64
		configFile     string
		useGRPC        bool
		grpcAddress    string
	)

	// Создаем новый набор флагов для избежания конфликтов
	fs := flag.NewFlagSet("agent", flag.ContinueOnError)

	// Регистрируем флаги для агента
	fs.StringVar(&address, "a", "localhost:8080", "HTTP server address")
	fs.Int64Var(&pollInterval, "p", 2, "poll interval in seconds")
	fs.Int64Var(&reportInterval, "r", 10, "report interval in seconds")
	fs.StringVar(&key, "k", "", "secret key for request signing")
	fs.StringVar(&cryptoKey, "crypto-key", "", "path to public key file for encryption")
	fs.Int64Var(&rateLimit, "l", 1, "rate limit")
	fs.StringVar(&configFile, "c", "", "path to config file")
	fs.StringVar(&configFile, "config", "", "path to config file")

	// Новые флаги для gRPC
	fs.BoolVar(&useGRPC, "grpc", false, "use gRPC instead of HTTP")
	fs.StringVar(&grpcAddress, "grpc-addr", "localhost:3200", "gRPC server address")

	fs.Parse(os.Args[1:])

	// Проверяем переменную окружения CONFIG
	if envConfig := os.Getenv("CONFIG"); envConfig != "" && configFile == "" {
		configFile = envConfig
	}

	// Загружаем JSON конфигурацию
	jsonConfig, _ := loadJSONConfig(configFile)

	// Применяем значения из JSON конфига (низкий приоритет)
	if jsonConfig != nil {
		applyAgentJSONConfig(jsonConfig, &address, &pollInterval, &reportInterval,
			&key, &cryptoKey, &rateLimit, &useGRPC, &grpcAddress)
	}

	// Применяем переменные окружения (средний приоритет)
	applyAgentEnvVars(&address, &pollInterval, &reportInterval, &key, &cryptoKey,
		&rateLimit, &useGRPC, &grpcAddress)

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
		UseGRPC:        useGRPC,
		GRPCAddress:    grpcAddress,
	}
}

// loadJSONConfig загружает конфигурацию из JSON файла
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

// applyJSONConfig применяет значения из JSON конфига для сервера
func applyJSONConfig(jsonConfig map[string]interface{},
	address *string, storeInterval *int64, fileStoragePath *string,
	restore *bool, databaseDsn *string, key *string, cryptoKey *string,
	auditFile *string, auditURL *string, trustedSubnet *string,
	useGRPC *bool, grpcAddress *string) {
	if val, ok := jsonConfig["address"]; ok {
		if str := parseString(val, ""); str != "" && *address == "localhost:8080" {
			*address = str
		}
	}
	if val, ok := jsonConfig["restore"]; ok {
		*restore = parseBool(val, *restore)
	}
	if val, ok := jsonConfig["store_interval"]; ok {
		if str := parseString(val, ""); str != "" {
			duration := parseDuration(str, time.Duration(*storeInterval)*time.Second)
			*storeInterval = int64(duration.Seconds())
		}
	}
	if val, ok := jsonConfig["store_file"]; ok {
		if str := parseString(val, ""); str != "" && *fileStoragePath == "" {
			*fileStoragePath = str
		}
	}
	if val, ok := jsonConfig["database_dsn"]; ok {
		if str := parseString(val, ""); str != "" && *databaseDsn == "" {
			*databaseDsn = str
		}
	}
	if val, ok := jsonConfig["crypto_key"]; ok {
		if str := parseString(val, ""); str != "" && *cryptoKey == "" {
			*cryptoKey = str
		}
	}
	if val, ok := jsonConfig["key"]; ok {
		if str := parseString(val, ""); str != "" && *key == "" {
			*key = str
		}
	}
	if val, ok := jsonConfig["audit_file"]; ok {
		if str := parseString(val, ""); str != "" && *auditFile == "" {
			*auditFile = str
		}
	}
	if val, ok := jsonConfig["audit_url"]; ok {
		if str := parseString(val, ""); str != "" && *auditURL == "" {
			*auditURL = str
		}
	}
	if val, ok := jsonConfig["trusted_subnet"]; ok {
		if str := parseString(val, ""); str != "" && *trustedSubnet == "" {
			*trustedSubnet = str
		}
	}
	// Новые поля для gRPC
	if val, ok := jsonConfig["use_grpc"]; ok {
		*useGRPC = parseBool(val, *useGRPC)
	}
	if val, ok := jsonConfig["grpc_address"]; ok {
		if str := parseString(val, ""); str != "" && *grpcAddress == "localhost:3200" {
			*grpcAddress = str
		}
	}
}

// applyAgentJSONConfig применяет значения из JSON конфига для агента
func applyAgentJSONConfig(jsonConfig map[string]interface{},
	address *string, pollInterval *int64, reportInterval *int64,
	key *string, cryptoKey *string, rateLimit *int64,
	useGRPC *bool, grpcAddress *string) {
	if val, ok := jsonConfig["address"]; ok {
		if str := parseString(val, ""); str != "" && *address == "localhost:8080" {
			*address = str
		}
	}
	if val, ok := jsonConfig["report_interval"]; ok {
		if str := parseString(val, ""); str != "" {
			duration := parseDuration(str, time.Duration(*reportInterval)*time.Second)
			*reportInterval = int64(duration.Seconds())
		}
	}
	if val, ok := jsonConfig["poll_interval"]; ok {
		if str := parseString(val, ""); str != "" {
			duration := parseDuration(str, time.Duration(*pollInterval)*time.Second)
			*pollInterval = int64(duration.Seconds())
		}
	}
	if val, ok := jsonConfig["crypto_key"]; ok {
		if str := parseString(val, ""); str != "" && *cryptoKey == "" {
			*cryptoKey = str
		}
	}
	if val, ok := jsonConfig["key"]; ok {
		if str := parseString(val, ""); str != "" && *key == "" {
			*key = str
		}
	}
	if val, ok := jsonConfig["rate_limit"]; ok {
		*rateLimit = parseInt64(val, *rateLimit)
	}
	// Новые поля для gRPC
	if val, ok := jsonConfig["use_grpc"]; ok {
		*useGRPC = parseBool(val, *useGRPC)
	}
	if val, ok := jsonConfig["grpc_address"]; ok {
		if str := parseString(val, ""); str != "" && *grpcAddress == "localhost:3200" {
			*grpcAddress = str
		}
	}
}

// applyEnvVars применяет переменные окружения для сервера
func applyEnvVars(address *string, storeInterval *int64, fileStoragePath *string,
	restore *bool, databaseDsn *string, key *string, cryptoKey *string,
	auditFile *string, auditURL *string, trustedSubnet *string,
	useGRPC *bool, grpcAddress *string) {
	if envAddress := os.Getenv("ADDRESS"); envAddress != "" {
		*address = envAddress
	}
	if envVal := os.Getenv("STORE_INTERVAL"); envVal != "" {
		if val, err := strconv.ParseInt(envVal, 10, 64); err == nil {
			*storeInterval = val
		}
	}
	if envVal := os.Getenv("FILE_STORAGE_PATH"); envVal != "" {
		*fileStoragePath = envVal
	}
	if envVal := os.Getenv("RESTORE"); envVal != "" {
		*restore = getEnvBool("RESTORE", *restore)
	}
	if envVal := os.Getenv("DATABASE_DSN"); envVal != "" {
		*databaseDsn = envVal
	}
	if envVal := os.Getenv("KEY"); envVal != "" {
		*key = envVal
	}
	if envVal := os.Getenv("CRYPTO_KEY"); envVal != "" {
		*cryptoKey = envVal
	}
	if envVal := os.Getenv("AUDIT_FILE"); envVal != "" {
		*auditFile = envVal
	}
	if envVal := os.Getenv("AUDIT_URL"); envVal != "" {
		*auditURL = envVal
	}
	if envVal := os.Getenv("TRUSTED_SUBNET"); envVal != "" {
		*trustedSubnet = envVal
	}
	// Новые переменные для gRPC
	if envVal := os.Getenv("USE_GRPC"); envVal != "" {
		*useGRPC = getEnvBool("USE_GRPC", *useGRPC)
	}
	if envVal := os.Getenv("GRPC_ADDRESS"); envVal != "" {
		*grpcAddress = envVal
	}
}

// applyAgentEnvVars применяет переменные окружения для агента
func applyAgentEnvVars(address *string, pollInterval *int64, reportInterval *int64,
	key *string, cryptoKey *string, rateLimit *int64,
	useGRPC *bool, grpcAddress *string) {
	if envAddress := os.Getenv("ADDRESS"); envAddress != "" {
		*address = envAddress
	}
	if envVal := os.Getenv("POLL_INTERVAL"); envVal != "" {
		if val, err := strconv.ParseInt(envVal, 10, 64); err == nil {
			*pollInterval = val
		}
	}
	if envVal := os.Getenv("REPORT_INTERVAL"); envVal != "" {
		if val, err := strconv.ParseInt(envVal, 10, 64); err == nil {
			*reportInterval = val
		}
	}
	if envVal := os.Getenv("KEY"); envVal != "" {
		*key = envVal
	}
	if envVal := os.Getenv("CRYPTO_KEY"); envVal != "" {
		*cryptoKey = envVal
	}
	if envVal := os.Getenv("RATE_LIMIT"); envVal != "" {
		if val, err := strconv.ParseInt(envVal, 10, 64); err == nil {
			*rateLimit = val
		}
	}
	// Новые переменные для gRPC
	if envVal := os.Getenv("USE_GRPC"); envVal != "" {
		*useGRPC = getEnvBool("USE_GRPC", *useGRPC)
	}
	if envVal := os.Getenv("GRPC_ADDRESS"); envVal != "" {
		*grpcAddress = envVal
	}
}

// parseDuration парсит длительность из строки
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

// parseInt64 парсит int64 из интерфейса
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

// parseBool парсит bool из интерфейса
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

// parseString парсит строку из интерфейса
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

// parseAddress разбивает адрес на сервер и порт
func parseAddress(address string) (string, string) {
	parts := strings.Split(address, ":")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], "8080"
}
