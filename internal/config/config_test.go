package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Определяем структуры для тестов
type testServerJSONConfig struct {
	Address       string `json:"address"`
	Restore       *bool  `json:"restore"`
	StoreInterval string `json:"store_interval"`
	StoreFile     string `json:"store_file"`
	DatabaseDSN   string `json:"database_dsn"`
	CryptoKey     string `json:"crypto_key"`
	Key           string `json:"key"`
	AuditFile     string `json:"audit_file"`
	AuditURL      string `json:"audit_url"`
	TrustedSubnet string `json:"trusted_subnet"`
}

type testAgentJSONConfig struct {
	Address        string `json:"address"`
	ReportInterval string `json:"report_interval"`
	PollInterval   string `json:"poll_interval"`
	CryptoKey      string `json:"crypto_key"`
	Key            string `json:"key"`
	RateLimit      *int64 `json:"rate_limit"`
}

func TestParseServerFlags(t *testing.T) {
	// Сохраняем оригинальные аргументы командной строки и окружение
	origArgs := os.Args
	origEnv := os.Environ()
	defer func() {
		os.Args = origArgs
		// Восстанавливаем окружение
		for _, env := range origEnv {
			parts := splitEnv(env)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()

	t.Run("default values", func(t *testing.T) {
		// Очищаем все переменные окружения, которые могут повлиять на тест
		os.Unsetenv("ADDRESS")
		os.Unsetenv("STORE_INTERVAL")
		os.Unsetenv("FILE_STORAGE_PATH")
		os.Unsetenv("RESTORE")
		os.Unsetenv("DATABASE_DSN")
		os.Unsetenv("KEY")
		os.Unsetenv("CRYPTO_KEY")
		os.Unsetenv("AUDIT_FILE")
		os.Unsetenv("AUDIT_URL")
		os.Unsetenv("TRUSTED_SUBNET")
		os.Unsetenv("CONFIG")

		os.Args = []string{"cmd"}

		cfg := ParseServerFlags()

		assert.Equal(t, "localhost:8080", cfg.Address)
		assert.Equal(t, "localhost", cfg.Server)
		assert.Equal(t, "8080", cfg.Port)
		assert.Equal(t, int64(300), cfg.StoreInterval)
		assert.Equal(t, "", cfg.FileStoragePath)
		assert.Equal(t, true, cfg.Restore)
		assert.Equal(t, "", cfg.DatabaseDSN)
		assert.Equal(t, "", cfg.Key)
		assert.Equal(t, "", cfg.CryptoKey)
		assert.Equal(t, "", cfg.AuditFile)
		assert.Equal(t, "", cfg.AuditURL)
		assert.Equal(t, "", cfg.TrustedSubnet)
	})

	t.Run("command line flags", func(t *testing.T) {
		// Очищаем переменные окружения
		os.Unsetenv("ADDRESS")
		os.Unsetenv("STORE_INTERVAL")
		os.Unsetenv("FILE_STORAGE_PATH")
		os.Unsetenv("RESTORE")
		os.Unsetenv("DATABASE_DSN")
		os.Unsetenv("KEY")
		os.Unsetenv("CRYPTO_KEY")
		os.Unsetenv("AUDIT_FILE")
		os.Unsetenv("AUDIT_URL")
		os.Unsetenv("TRUSTED_SUBNET")

		os.Args = []string{
			"cmd",
			"-a", "192.168.1.100:9090",
			"-i", "60",
			"-f", "/tmp/metrics.json",
			"-r", "false",
			"-d", "postgres://localhost:5432/metrics",
			"-k", "secret-key",
			"-crypto-key", "/path/to/private.pem",
			"-audit-file", "/tmp/audit.log",
			"-audit-url", "http://audit-server:8080",
			"-t", "192.168.1.0/24",
		}

		cfg := ParseServerFlags()

		assert.Equal(t, "192.168.1.100:9090", cfg.Address)
		assert.Equal(t, "192.168.1.100", cfg.Server)
		assert.Equal(t, "9090", cfg.Port)
		assert.Equal(t, int64(60), cfg.StoreInterval)
		assert.Equal(t, "/tmp/metrics.json", cfg.FileStoragePath)
		assert.Equal(t, false, cfg.Restore)
		assert.Equal(t, "postgres://localhost:5432/metrics", cfg.DatabaseDSN)
		assert.Equal(t, "secret-key", cfg.Key)
		assert.Equal(t, "/path/to/private.pem", cfg.CryptoKey)
		assert.Equal(t, "/tmp/audit.log", cfg.AuditFile)
		assert.Equal(t, "http://audit-server:8080", cfg.AuditURL)
		assert.Equal(t, "192.168.1.0/24", cfg.TrustedSubnet)
	})

	t.Run("environment variables override flags", func(t *testing.T) {
		os.Setenv("ADDRESS", "10.0.0.1:8080")
		os.Setenv("STORE_INTERVAL", "120")
		os.Setenv("FILE_STORAGE_PATH", "/env/metrics.json")
		os.Setenv("RESTORE", "false")
		os.Setenv("DATABASE_DSN", "postgres://env:5432/metrics")
		os.Setenv("KEY", "env-key")
		os.Setenv("CRYPTO_KEY", "/env/private.pem")
		os.Setenv("AUDIT_FILE", "/env/audit.log")
		os.Setenv("AUDIT_URL", "http://env-audit:8080")
		os.Setenv("TRUSTED_SUBNET", "10.0.0.0/8")

		os.Args = []string{
			"cmd",
			"-a", "192.168.1.100:9090", // должен быть переопределен окружением
			"-i", "60",
			"-f", "/tmp/metrics.json",
			"-r", "true",
		}

		cfg := ParseServerFlags()

		assert.Equal(t, "10.0.0.1:8080", cfg.Address)
		assert.Equal(t, "10.0.0.1", cfg.Server)
		assert.Equal(t, "8080", cfg.Port)
		assert.Equal(t, int64(120), cfg.StoreInterval)
		assert.Equal(t, "/env/metrics.json", cfg.FileStoragePath)
		assert.Equal(t, false, cfg.Restore)
		assert.Equal(t, "postgres://env:5432/metrics", cfg.DatabaseDSN)
		assert.Equal(t, "env-key", cfg.Key)
		assert.Equal(t, "/env/private.pem", cfg.CryptoKey)
		assert.Equal(t, "/env/audit.log", cfg.AuditFile)
		assert.Equal(t, "http://env-audit:8080", cfg.AuditURL)
		assert.Equal(t, "10.0.0.0/8", cfg.TrustedSubnet)
	})

	t.Run("json config file", func(t *testing.T) {
		// Создаем временный JSON файл
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "server_config.json")

		restore := false
		config := testServerJSONConfig{
			Address:       "json:8080",
			Restore:       &restore,
			StoreInterval: "45s",
			StoreFile:     "/json/metrics.json",
			DatabaseDSN:   "postgres://json:5432/metrics",
			CryptoKey:     "/json/private.pem",
			Key:           "json-key",
			AuditFile:     "/json/audit.log",
			AuditURL:      "http://json-audit:8080",
			TrustedSubnet: "192.168.2.0/24",
		}

		data, err := json.MarshalIndent(config, "", "  ")
		require.NoError(t, err)

		err = os.WriteFile(configPath, data, 0644)
		require.NoError(t, err)

		// Очищаем окружение и устанавливаем CONFIG
		os.Unsetenv("ADDRESS")
		os.Unsetenv("STORE_INTERVAL")
		os.Unsetenv("FILE_STORAGE_PATH")
		os.Unsetenv("RESTORE")
		os.Unsetenv("DATABASE_DSN")
		os.Unsetenv("KEY")
		os.Unsetenv("CRYPTO_KEY")
		os.Unsetenv("AUDIT_FILE")
		os.Unsetenv("AUDIT_URL")
		os.Unsetenv("TRUSTED_SUBNET")
		os.Setenv("CONFIG", configPath)

		os.Args = []string{"cmd"}

		cfg := ParseServerFlags()

		assert.Equal(t, "json:8080", cfg.Address)
		assert.Equal(t, "json", cfg.Server)
		assert.Equal(t, "8080", cfg.Port)
		assert.Equal(t, int64(45), cfg.StoreInterval)
		assert.Equal(t, "/json/metrics.json", cfg.FileStoragePath)
		assert.Equal(t, false, cfg.Restore)
		assert.Equal(t, "postgres://json:5432/metrics", cfg.DatabaseDSN)
		assert.Equal(t, "json-key", cfg.Key)
		assert.Equal(t, "/json/private.pem", cfg.CryptoKey)
		assert.Equal(t, "/json/audit.log", cfg.AuditFile)
		assert.Equal(t, "http://json-audit:8080", cfg.AuditURL)
		assert.Equal(t, "192.168.2.0/24", cfg.TrustedSubnet)
	})

	t.Run("priority: env > flags > json", func(t *testing.T) {
		// Создаем JSON файл
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "server_config.json")

		restore := true
		config := testServerJSONConfig{
			Address:       "json:8080",
			Restore:       &restore,
			StoreInterval: "45s",
			StoreFile:     "/json/metrics.json",
		}

		data, err := json.MarshalIndent(config, "", "  ")
		require.NoError(t, err)

		err = os.WriteFile(configPath, data, 0644)
		require.NoError(t, err)

		// Устанавливаем окружение (высший приоритет)
		os.Setenv("ADDRESS", "env:8080")
		os.Setenv("STORE_INTERVAL", "120")
		os.Setenv("FILE_STORAGE_PATH", "/env/metrics.json")
		os.Setenv("RESTORE", "false")
		os.Setenv("CONFIG", configPath)

		os.Args = []string{
			"cmd",
			"-a", "flag:8080", // средний приоритет
			"-i", "60",
			"-f", "/flag/metrics.json",
			"-r", "true",
		}

		cfg := ParseServerFlags()

		// Окружение должно победить
		assert.Equal(t, "env:8080", cfg.Address)
		assert.Equal(t, int64(120), cfg.StoreInterval)
		assert.Equal(t, "/env/metrics.json", cfg.FileStoragePath)
		assert.Equal(t, false, cfg.Restore)
	})
}

func TestParseAgentFlags(t *testing.T) {
	// Сохраняем оригинальные аргументы командной строки и окружение
	origArgs := os.Args
	origEnv := os.Environ()
	defer func() {
		os.Args = origArgs
		// Восстанавливаем окружение
		for _, env := range origEnv {
			parts := splitEnv(env)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()

	t.Run("default values", func(t *testing.T) {
		// Очищаем переменные окружения
		os.Unsetenv("ADDRESS")
		os.Unsetenv("POLL_INTERVAL")
		os.Unsetenv("REPORT_INTERVAL")
		os.Unsetenv("KEY")
		os.Unsetenv("CRYPTO_KEY")
		os.Unsetenv("RATE_LIMIT")
		os.Unsetenv("CONFIG")

		os.Args = []string{"cmd"}

		cfg := ParseAgentFlags()

		assert.Equal(t, "localhost:8080", cfg.Address)
		assert.Equal(t, "localhost", cfg.Server)
		assert.Equal(t, "8080", cfg.Port)
		assert.Equal(t, 2*time.Second, cfg.PollInterval)
		assert.Equal(t, 10*time.Second, cfg.ReportInterval)
		assert.Equal(t, "", cfg.Key)
		assert.Equal(t, "", cfg.CryptoKey)
		assert.Equal(t, int64(1), cfg.RateLimit)
	})

	t.Run("command line flags", func(t *testing.T) {
		// Очищаем переменные окружения
		os.Unsetenv("ADDRESS")
		os.Unsetenv("POLL_INTERVAL")
		os.Unsetenv("REPORT_INTERVAL")
		os.Unsetenv("KEY")
		os.Unsetenv("CRYPTO_KEY")
		os.Unsetenv("RATE_LIMIT")

		os.Args = []string{
			"cmd",
			"-a", "192.168.1.100:9090",
			"-p", "5",
			"-r", "20",
			"-k", "secret-key",
			"-crypto-key", "/path/to/public.pem",
			"-l", "10",
		}

		cfg := ParseAgentFlags()

		assert.Equal(t, "192.168.1.100:9090", cfg.Address)
		assert.Equal(t, "192.168.1.100", cfg.Server)
		assert.Equal(t, "9090", cfg.Port)
		assert.Equal(t, 5*time.Second, cfg.PollInterval)
		assert.Equal(t, 20*time.Second, cfg.ReportInterval)
		assert.Equal(t, "secret-key", cfg.Key)
		assert.Equal(t, "/path/to/public.pem", cfg.CryptoKey)
		assert.Equal(t, int64(10), cfg.RateLimit)
	})

	t.Run("environment variables override flags", func(t *testing.T) {
		os.Setenv("ADDRESS", "10.0.0.1:8080")
		os.Setenv("POLL_INTERVAL", "3")
		os.Setenv("REPORT_INTERVAL", "15")
		os.Setenv("KEY", "env-key")
		os.Setenv("CRYPTO_KEY", "/env/public.pem")
		os.Setenv("RATE_LIMIT", "5")

		os.Args = []string{
			"cmd",
			"-a", "192.168.1.100:9090",
			"-p", "2",
			"-r", "10",
			"-l", "1",
		}

		cfg := ParseAgentFlags()

		assert.Equal(t, "10.0.0.1:8080", cfg.Address)
		assert.Equal(t, "10.0.0.1", cfg.Server)
		assert.Equal(t, "8080", cfg.Port)
		assert.Equal(t, 3*time.Second, cfg.PollInterval)
		assert.Equal(t, 15*time.Second, cfg.ReportInterval)
		assert.Equal(t, "env-key", cfg.Key)
		assert.Equal(t, "/env/public.pem", cfg.CryptoKey)
		assert.Equal(t, int64(5), cfg.RateLimit)
	})

	t.Run("json config file", func(t *testing.T) {
		// Создаем временный JSON файл
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "agent_config.json")

		rateLimit := int64(8)
		config := testAgentJSONConfig{
			Address:        "json:8080",
			ReportInterval: "25s",
			PollInterval:   "4s",
			CryptoKey:      "/json/public.pem",
			Key:            "json-key",
			RateLimit:      &rateLimit,
		}

		data, err := json.MarshalIndent(config, "", "  ")
		require.NoError(t, err)

		err = os.WriteFile(configPath, data, 0644)
		require.NoError(t, err)

		// Очищаем окружение и устанавливаем CONFIG
		os.Unsetenv("ADDRESS")
		os.Unsetenv("POLL_INTERVAL")
		os.Unsetenv("REPORT_INTERVAL")
		os.Unsetenv("KEY")
		os.Unsetenv("CRYPTO_KEY")
		os.Unsetenv("RATE_LIMIT")
		os.Setenv("CONFIG", configPath)

		os.Args = []string{"cmd"}

		cfg := ParseAgentFlags()

		assert.Equal(t, "json:8080", cfg.Address)
		assert.Equal(t, "json", cfg.Server)
		assert.Equal(t, "8080", cfg.Port)
		assert.Equal(t, 4*time.Second, cfg.PollInterval)
		assert.Equal(t, 25*time.Second, cfg.ReportInterval)
		assert.Equal(t, "json-key", cfg.Key)
		assert.Equal(t, "/json/public.pem", cfg.CryptoKey)
		assert.Equal(t, int64(8), cfg.RateLimit)
	})

	t.Run("priority: env > flags > json", func(t *testing.T) {
		// Создаем JSON файл
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "agent_config.json")

		rateLimit := int64(8)
		config := testAgentJSONConfig{
			Address:        "json:8080",
			ReportInterval: "25s",
			PollInterval:   "4s",
			RateLimit:      &rateLimit,
		}

		data, err := json.MarshalIndent(config, "", "  ")
		require.NoError(t, err)

		err = os.WriteFile(configPath, data, 0644)
		require.NoError(t, err)

		// Устанавливаем окружение (высший приоритет)
		os.Setenv("ADDRESS", "env:8080")
		os.Setenv("POLL_INTERVAL", "3")
		os.Setenv("REPORT_INTERVAL", "15")
		os.Setenv("RATE_LIMIT", "5")
		os.Setenv("CONFIG", configPath)

		os.Args = []string{
			"cmd",
			"-a", "flag:8080",
			"-p", "2",
			"-r", "10",
			"-l", "1",
		}

		cfg := ParseAgentFlags()

		// Окружение должно победить
		assert.Equal(t, "env:8080", cfg.Address)
		assert.Equal(t, 3*time.Second, cfg.PollInterval)
		assert.Equal(t, 15*time.Second, cfg.ReportInterval)
		assert.Equal(t, int64(5), cfg.RateLimit)
	})
}

// Вспомогательная функция для разбиения переменной окружения
func splitEnv(env string) []string {
	for i := 0; i < len(env); i++ {
		if env[i] == '=' {
			return []string{env[:i], env[i+1:]}
		}
	}
	return []string{env}
}

func TestLoadJSONConfig(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("valid server config", func(t *testing.T) {
		configPath := filepath.Join(tempDir, "server_config.json")

		restore := false
		config := testServerJSONConfig{
			Address:       "localhost:9090",
			Restore:       &restore,
			StoreInterval: "30s",
			StoreFile:     "/tmp/metrics.json",
			DatabaseDSN:   "postgres://localhost:5432/metrics",
			CryptoKey:     "/path/to/private.pem",
			Key:           "secret-key",
			AuditFile:     "/tmp/audit.log",
			AuditURL:      "http://audit-server:8080",
			TrustedSubnet: "192.168.1.0/24",
		}

		data, err := json.MarshalIndent(config, "", "  ")
		require.NoError(t, err)

		err = os.WriteFile(configPath, data, 0644)
		require.NoError(t, err)

		loadedConfig, err := loadJSONConfig(configPath)
		require.NoError(t, err)
		require.NotNil(t, loadedConfig)

		// Проверяем значения
		assert.Equal(t, "localhost:9090", loadedConfig["address"])
		assert.Equal(t, false, loadedConfig["restore"])
		assert.Equal(t, "30s", loadedConfig["store_interval"])
		assert.Equal(t, "/tmp/metrics.json", loadedConfig["store_file"])
		assert.Equal(t, "postgres://localhost:5432/metrics", loadedConfig["database_dsn"])
		assert.Equal(t, "/path/to/private.pem", loadedConfig["crypto_key"])
		assert.Equal(t, "secret-key", loadedConfig["key"])
		assert.Equal(t, "/tmp/audit.log", loadedConfig["audit_file"])
		assert.Equal(t, "http://audit-server:8080", loadedConfig["audit_url"])
		assert.Equal(t, "192.168.1.0/24", loadedConfig["trusted_subnet"])
	})

	t.Run("valid agent config", func(t *testing.T) {
		configPath := filepath.Join(tempDir, "agent_config.json")

		rateLimit := int64(5)
		config := testAgentJSONConfig{
			Address:        "localhost:9090",
			ReportInterval: "10s",
			PollInterval:   "2s",
			CryptoKey:      "/path/to/public.pem",
			Key:            "secret-key",
			RateLimit:      &rateLimit,
		}

		data, err := json.MarshalIndent(config, "", "  ")
		require.NoError(t, err)

		err = os.WriteFile(configPath, data, 0644)
		require.NoError(t, err)

		loadedConfig, err := loadJSONConfig(configPath)
		require.NoError(t, err)
		require.NotNil(t, loadedConfig)

		// Проверяем значения
		assert.Equal(t, "localhost:9090", loadedConfig["address"])
		assert.Equal(t, "10s", loadedConfig["report_interval"])
		assert.Equal(t, "2s", loadedConfig["poll_interval"])
		assert.Equal(t, "/path/to/public.pem", loadedConfig["crypto_key"])
		assert.Equal(t, "secret-key", loadedConfig["key"])
		assert.Equal(t, float64(5), loadedConfig["rate_limit"])
	})

	t.Run("empty config file", func(t *testing.T) {
		configPath := filepath.Join(tempDir, "empty.json")
		err := os.WriteFile(configPath, []byte("{}"), 0644)
		require.NoError(t, err)

		loadedConfig, err := loadJSONConfig(configPath)
		require.NoError(t, err)
		assert.Empty(t, loadedConfig)
	})

	t.Run("non-existent file", func(t *testing.T) {
		loadedConfig, err := loadJSONConfig("/nonexistent/path/config.json")
		assert.Error(t, err)
		assert.Nil(t, loadedConfig)
	})

	t.Run("invalid json", func(t *testing.T) {
		configPath := filepath.Join(tempDir, "invalid.json")
		err := os.WriteFile(configPath, []byte("{invalid json}"), 0644)
		require.NoError(t, err)

		loadedConfig, err := loadJSONConfig(configPath)
		assert.Error(t, err)
		assert.Nil(t, loadedConfig)
	})

	t.Run("empty path", func(t *testing.T) {
		loadedConfig, err := loadJSONConfig("")
		assert.NoError(t, err)
		assert.Nil(t, loadedConfig)
	})
}

func TestGetEnvBool(t *testing.T) {
	// Сохраняем текущее окружение
	oldEnv := os.Getenv("TEST_BOOL")
	defer os.Setenv("TEST_BOOL", oldEnv)

	tests := []struct {
		name       string
		envValue   string
		defaultVal bool
		expected   bool
	}{
		{
			name:       "Environment variable set to true",
			envValue:   "true",
			defaultVal: false,
			expected:   true,
		},
		{
			name:       "Environment variable set to false",
			envValue:   "false",
			defaultVal: true,
			expected:   false,
		},
		{
			name:       "Environment variable set to 1",
			envValue:   "1",
			defaultVal: false,
			expected:   true,
		},
		{
			name:       "Environment variable set to 0",
			envValue:   "0",
			defaultVal: true,
			expected:   false,
		},
		{
			name:       "Environment variable not set",
			envValue:   "",
			defaultVal: true,
			expected:   true,
		},
		{
			name:       "Invalid environment value",
			envValue:   "invalid",
			defaultVal: true,
			expected:   true,
		},
		{
			name:       "Empty string with default false",
			envValue:   "",
			defaultVal: false,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("TEST_BOOL", tt.envValue)
			} else {
				os.Unsetenv("TEST_BOOL")
			}

			result := getEnvBool("TEST_BOOL", tt.defaultVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseAddress(t *testing.T) {
	tests := []struct {
		name           string
		address        string
		expectedServer string
		expectedPort   string
	}{
		{
			name:           "Full address with port",
			address:        "localhost:8080",
			expectedServer: "localhost",
			expectedPort:   "8080",
		},
		{
			name:           "IP address with port",
			address:        "192.168.1.100:9090",
			expectedServer: "192.168.1.100",
			expectedPort:   "9090",
		},
		{
			name:           "Hostname with port",
			address:        "example.com:3000",
			expectedServer: "example.com",
			expectedPort:   "3000",
		},
		{
			name:           "Only hostname, no port",
			address:        "localhost",
			expectedServer: "localhost",
			expectedPort:   "8080",
		},
		{
			name:           "Only IP, no port",
			address:        "192.168.1.100",
			expectedServer: "192.168.1.100",
			expectedPort:   "8080",
		},
		{
			name:           "Empty address",
			address:        "",
			expectedServer: "",
			expectedPort:   "8080",
		},
		{
			name:           "IPv6 address with port",
			address:        "[::1]:8080",
			expectedServer: "[::1]",
			expectedPort:   "8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, port := parseAddress(tt.address)
			assert.Equal(t, tt.expectedServer, server)
			assert.Equal(t, tt.expectedPort, port)
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultVal time.Duration
		expected   time.Duration
	}{
		{
			name:       "valid seconds",
			input:      "30s",
			defaultVal: 10 * time.Second,
			expected:   30 * time.Second,
		},
		{
			name:       "valid minutes",
			input:      "5m",
			defaultVal: 1 * time.Minute,
			expected:   5 * time.Minute,
		},
		{
			name:       "valid milliseconds",
			input:      "500ms",
			defaultVal: 1 * time.Second,
			expected:   500 * time.Millisecond,
		},
		{
			name:       "empty string",
			input:      "",
			defaultVal: 10 * time.Second,
			expected:   10 * time.Second,
		},
		{
			name:       "invalid format",
			input:      "invalid",
			defaultVal: 10 * time.Second,
			expected:   10 * time.Second,
		},
		{
			name:       "negative duration",
			input:      "-10s",
			defaultVal: 5 * time.Second,
			expected:   -10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDuration(tt.input, tt.defaultVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseInt64(t *testing.T) {
	tests := []struct {
		name       string
		input      interface{}
		defaultVal int64
		expected   int64
	}{
		{
			name:       "from float64",
			input:      float64(42),
			defaultVal: 0,
			expected:   42,
		},
		{
			name:       "from float64 with decimal",
			input:      float64(42.7),
			defaultVal: 0,
			expected:   42,
		},
		{
			name:       "from int64",
			input:      int64(100),
			defaultVal: 0,
			expected:   100,
		},
		{
			name:       "from int",
			input:      50,
			defaultVal: 0,
			expected:   50,
		},
		{
			name:       "nil value",
			input:      nil,
			defaultVal: 10,
			expected:   10,
		},
		{
			name:       "invalid type - string",
			input:      "not a number",
			defaultVal: 20,
			expected:   20,
		},
		{
			name:       "invalid type - bool",
			input:      true,
			defaultVal: 30,
			expected:   30,
		},
		{
			name:       "negative int64",
			input:      int64(-50),
			defaultVal: 0,
			expected:   -50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseInt64(tt.input, tt.defaultVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		name       string
		input      interface{}
		defaultVal bool
		expected   bool
	}{
		{
			name:       "from bool true",
			input:      true,
			defaultVal: false,
			expected:   true,
		},
		{
			name:       "from bool false",
			input:      false,
			defaultVal: true,
			expected:   false,
		},
		{
			name:       "from string true",
			input:      "true",
			defaultVal: false,
			expected:   true,
		},
		{
			name:       "from string false",
			input:      "false",
			defaultVal: true,
			expected:   false,
		},
		{
			name:       "from string TRUE (uppercase)",
			input:      "TRUE",
			defaultVal: false,
			expected:   true,
		},
		{
			name:       "from string FALSE (uppercase)",
			input:      "FALSE",
			defaultVal: true,
			expected:   false,
		},
		{
			name:       "nil value",
			input:      nil,
			defaultVal: true,
			expected:   true,
		},
		{
			name:       "invalid type - int",
			input:      123,
			defaultVal: false,
			expected:   false,
		},
		{
			name:       "invalid type - float",
			input:      1.0,
			defaultVal: true,
			expected:   true,
		},
		{
			name:       "invalid string",
			input:      "invalid",
			defaultVal: true,
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseBool(tt.input, tt.defaultVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseString(t *testing.T) {
	tests := []struct {
		name       string
		input      interface{}
		defaultVal string
		expected   string
	}{
		{
			name:       "valid string",
			input:      "test value",
			defaultVal: "default",
			expected:   "test value",
		},
		{
			name:       "empty string",
			input:      "",
			defaultVal: "default",
			expected:   "",
		},
		{
			name:       "nil value",
			input:      nil,
			defaultVal: "default",
			expected:   "default",
		},
		{
			name:       "non-string type - int",
			input:      123,
			defaultVal: "default",
			expected:   "default",
		},
		{
			name:       "non-string type - bool",
			input:      true,
			defaultVal: "default",
			expected:   "default",
		},
		{
			name:       "string with spaces",
			input:      "  spaced  ",
			defaultVal: "default",
			expected:   "  spaced  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseString(tt.input, tt.defaultVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Вспомогательная функция для создания указателя на bool
func boolPtr(b bool) *bool {
	return &b
}
