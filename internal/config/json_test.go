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

// Определяем структуры для тестов (копируем из json.go, но делаем их доступными)
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
}

type testAgentJSONConfig struct {
	Address        string `json:"address"`
	ReportInterval string `json:"report_interval"`
	PollInterval   string `json:"poll_interval"`
	CryptoKey      string `json:"crypto_key"`
	Key            string `json:"key"`
	RateLimit      *int64 `json:"rate_limit"`
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
		}

		data, err := json.MarshalIndent(config, "", "  ")
		require.NoError(t, err)

		err = os.WriteFile(configPath, data, 0644)
		require.NoError(t, err)

		loadedConfig, err := loadJSONConfig(configPath) // используем внутреннюю функцию
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

		loadedConfig, err := loadJSONConfig(configPath) // используем внутреннюю функцию
		require.NoError(t, err)
		require.NotNil(t, loadedConfig)

		// Проверяем значения
		assert.Equal(t, "localhost:9090", loadedConfig["address"])
		assert.Equal(t, "10s", loadedConfig["report_interval"])
		assert.Equal(t, "2s", loadedConfig["poll_interval"])
		assert.Equal(t, "/path/to/public.pem", loadedConfig["crypto_key"])
		assert.Equal(t, "secret-key", loadedConfig["key"])
		assert.Equal(t, float64(5), loadedConfig["rate_limit"]) // JSON числа парсятся как float64
	})

	t.Run("empty config file", func(t *testing.T) {
		configPath := filepath.Join(tempDir, "empty.json")
		err := os.WriteFile(configPath, []byte("{}"), 0644)
		require.NoError(t, err)

		loadedConfig, err := loadJSONConfig(configPath) // используем внутреннюю функцию
		require.NoError(t, err)
		assert.Empty(t, loadedConfig)
	})

	t.Run("non-existent file", func(t *testing.T) {
		loadedConfig, err := loadJSONConfig("/nonexistent/path/config.json") // используем внутреннюю функцию
		assert.Error(t, err)
		assert.Nil(t, loadedConfig)
	})

	t.Run("invalid json", func(t *testing.T) {
		configPath := filepath.Join(tempDir, "invalid.json")
		err := os.WriteFile(configPath, []byte("{invalid json}"), 0644)
		require.NoError(t, err)

		loadedConfig, err := loadJSONConfig(configPath) // используем внутреннюю функцию
		assert.Error(t, err)
		assert.Nil(t, loadedConfig)
	})

	t.Run("empty path", func(t *testing.T) {
		loadedConfig, err := loadJSONConfig("") // используем внутреннюю функцию
		assert.NoError(t, err)
		assert.Nil(t, loadedConfig)
	})
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDuration(tt.input, tt.defaultVal) // используем внутреннюю функцию
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
			name:       "invalid type",
			input:      "not a number",
			defaultVal: 20,
			expected:   20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseInt64(tt.input, tt.defaultVal) // используем внутреннюю функцию
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
			name:       "nil value",
			input:      nil,
			defaultVal: true,
			expected:   true,
		},
		{
			name:       "invalid type",
			input:      123,
			defaultVal: false,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseBool(tt.input, tt.defaultVal) // используем внутреннюю функцию
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
			name:       "nil value",
			input:      nil,
			defaultVal: "default",
			expected:   "default",
		},
		{
			name:       "non-string type",
			input:      123,
			defaultVal: "default",
			expected:   "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseString(tt.input, tt.defaultVal) // используем внутреннюю функцию
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Вспомогательная функция для создания указателя на bool
func boolPtr(b bool) *bool {
	return &b
}
