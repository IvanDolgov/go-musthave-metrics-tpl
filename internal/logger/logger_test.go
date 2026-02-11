package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// captureLogOutput захватывает вывод логера для тестирования
func captureLogOutput(t *testing.T, fn func()) map[string]interface{} {
	t.Helper()

	// Создаём буфер для захвата логов
	var buf bytes.Buffer

	// Создаём encoder для JSON
	encoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		MessageKey:   "msg",
		LevelKey:     "level",
		EncodeLevel:  zapcore.LowercaseLevelEncoder,
		TimeKey:      "time",
		EncodeTime:   zapcore.ISO8601TimeEncoder,
		CallerKey:    "caller",
		EncodeCaller: zapcore.ShortCallerEncoder,
	})

	// Создаём writer для буфера
	writer := zapcore.AddSync(&buf)

	// Создаём core с уровнем Debug
	core := zapcore.NewCore(encoder, writer, zapcore.DebugLevel)

	// Создаём временный логер
	tempLogger := zap.New(core)

	// Сохраняем оригинальный логер и восстанавливаем после теста
	originalLog := Log
	defer func() { Log = originalLog }()

	// Устанавливаем временный логер
	Log = tempLogger

	// Выполняем функцию
	fn()

	// Синхронизируем логер
	_ = tempLogger.Sync()

	// Парсим JSON лог
	var logEntry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logEntry)
	if err != nil {
		t.Fatalf("Failed to parse log output: %v", err)
	}

	return logEntry
}

// TestInitialize тестирует инициализацию логера
func TestInitialize(t *testing.T) {
	tests := []struct {
		name        string
		level       string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid debug level",
			level:   "debug",
			wantErr: false,
		},
		{
			name:    "valid info level",
			level:   "info",
			wantErr: false,
		},
		{
			name:    "valid warn level",
			level:   "warn",
			wantErr: false,
		},
		{
			name:    "valid error level",
			level:   "error",
			wantErr: false,
		},
		{
			name:        "invalid level",
			level:       "invalid",
			wantErr:     true,
			errContains: "unrecognized level",
		},
		{
			name:    "empty level",
			level:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Сбрасываем логер перед каждым тестом
			Log = zap.NewNop()

			err := Initialize(tt.level)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				// Проверяем что логер остался Nop
				assert.Equal(t, zap.NewNop(), Log)
			} else {
				assert.NoError(t, err)
				// Проверяем что логер не Nop
				assert.NotEqual(t, zap.NewNop(), Log)
			}
		})
	}
}

// TestLogErrorWithContext тестирует логирование ошибок с контекстом
func TestLogErrorWithContext(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		message  string
		fields   []zap.Field
		wantLog  bool
		wantMsg  string
		wantKeys []string
	}{
		{
			name:     "error with fields",
			err:      errors.New("test error"),
			message:  "operation failed",
			fields:   []zap.Field{zap.String("component", "test")},
			wantLog:  true,
			wantMsg:  "operation failed",
			wantKeys: []string{"error", "component"},
		},
		{
			name:    "nil error",
			err:     nil,
			message: "should not log",
			fields:  []zap.Field{zap.String("key", "value")},
			wantLog: false,
		},
		{
			name:     "error without additional fields",
			err:      errors.New("simple error"),
			message:  "simple operation",
			fields:   []zap.Field{},
			wantLog:  true,
			wantMsg:  "simple operation",
			wantKeys: []string{"error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantLog {
				logEntry := captureLogOutput(t, func() {
					LogErrorWithContext(tt.err, tt.message, tt.fields...)
				})

				assert.Equal(t, tt.wantMsg, logEntry["msg"])
				assert.Equal(t, "error", logEntry["level"])

				for _, key := range tt.wantKeys {
					assert.Contains(t, logEntry, key)
				}

				if tt.err != nil {
					assert.Contains(t, logEntry["error"], tt.err.Error())
				}
			} else {
				// Для nil error проверяем что ничего не залогировалось
				buf := captureLogOutput(t, func() {
					LogErrorWithContext(tt.err, tt.message, tt.fields...)
				})
				assert.Empty(t, buf)
			}
		})
	}
}

// TestLogRetryAttempt тестирует логирование попыток повтора
func TestLogRetryAttempt(t *testing.T) {
	logEntry := captureLogOutput(t, func() {
		LogRetryAttempt(3, 2*time.Second, errors.New("connection refused"))
	})

	assert.Equal(t, "Retry attempt", logEntry["msg"])
	assert.Equal(t, "warn", logEntry["level"])
	assert.Equal(t, float64(3), logEntry["attempt"])
	assert.Contains(t, logEntry["delay"], "2s")
	assert.Contains(t, logEntry["error"], "connection refused")
}

// TestLogRetrySuccess тестирует логирование успешного повтора
func TestLogRetrySuccess(t *testing.T) {
	logEntry := captureLogOutput(t, func() {
		LogRetrySuccess("update_metrics", 5)
	})

	assert.Equal(t, "Operation completed successfully after retries", logEntry["msg"])
	assert.Equal(t, "info", logEntry["level"])
	assert.Equal(t, "update_metrics", logEntry["operation"])
	assert.Equal(t, float64(5), logEntry["total_attempts"])
}

// TestLogRetryFailure тестирует логирование неудачных повторов
func TestLogRetryFailure(t *testing.T) {
	logEntry := captureLogOutput(t, func() {
		LogRetryFailure("send_data", 3, errors.New("timeout"))
	})

	assert.Equal(t, "Operation failed after all retry attempts", logEntry["msg"])
	assert.Equal(t, "error", logEntry["level"])
	assert.Equal(t, "send_data", logEntry["operation"])
	assert.Equal(t, float64(3), logEntry["max_attempts"])
	assert.Contains(t, logEntry["error"], "timeout")
}

// TestLogDatabaseError тестирует логирование ошибок БД
func TestLogDatabaseError(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		err       error
		query     string
		params    []interface{}
		wantKeys  []string
	}{
		{
			name:      "with query and params",
			operation: "select",
			err:       errors.New("table not found"),
			query:     "SELECT * FROM users",
			params:    []interface{}{"id", 123},
			wantKeys:  []string{"operation", "error", "query", "parameters"},
		},
		{
			name:      "without params",
			operation: "insert",
			err:       errors.New("duplicate key"),
			query:     "INSERT INTO metrics",
			params:    []interface{}{},
			wantKeys:  []string{"operation", "error", "query"},
		},
		{
			name:      "without query",
			operation: "delete",
			err:       errors.New("constraint violation"),
			query:     "",
			params:    []interface{}{},
			wantKeys:  []string{"operation", "error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logEntry := captureLogOutput(t, func() {
				LogDatabaseError(tt.operation, tt.err, tt.query, tt.params...)
			})

			assert.Equal(t, "Database operation failed", logEntry["msg"])
			assert.Equal(t, "error", logEntry["level"])
			assert.Equal(t, tt.operation, logEntry["operation"])
			assert.Contains(t, logEntry["error"], tt.err.Error())

			for _, key := range tt.wantKeys {
				assert.Contains(t, logEntry, key)
			}
		})
	}
}

// TestLogNetworkError тестирует логирование сетевых ошибок
func TestLogNetworkError(t *testing.T) {
	tests := []struct {
		name       string
		operation  string
		url        string
		err        error
		statusCode int
		wantKeys   []string
	}{
		{
			name:       "with status code",
			operation:  "get",
			url:        "http://example.com",
			err:        errors.New("not found"),
			statusCode: 404,
			wantKeys:   []string{"operation", "url", "error", "status_code"},
		},
		{
			name:       "without status code",
			operation:  "post",
			url:        "http://api.example.com",
			err:        errors.New("connection refused"),
			statusCode: 0,
			wantKeys:   []string{"operation", "url", "error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logEntry := captureLogOutput(t, func() {
				LogNetworkError(tt.operation, tt.url, tt.err, tt.statusCode)
			})

			assert.Equal(t, "Network operation failed", logEntry["msg"])
			assert.Equal(t, "error", logEntry["level"])
			assert.Equal(t, tt.operation, logEntry["operation"])
			assert.Equal(t, tt.url, logEntry["url"])
			assert.Contains(t, logEntry["error"], tt.err.Error())

			for _, key := range tt.wantKeys {
				assert.Contains(t, logEntry, key)
			}

			if tt.statusCode > 0 {
				assert.Equal(t, float64(tt.statusCode), logEntry["status_code"])
			}
		})
	}
}

// TestLogBatchOperation тестирует логирование батч-операций
func TestLogBatchOperation(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		batchSize int
		duration  time.Duration
		err       error
		wantLevel string
	}{
		{
			name:      "successful batch",
			operation: "save_metrics",
			batchSize: 100,
			duration:  150 * time.Millisecond,
			err:       nil,
			wantLevel: "info",
		},
		{
			name:      "failed batch",
			operation: "send_metrics",
			batchSize: 50,
			duration:  500 * time.Millisecond,
			err:       errors.New("timeout"),
			wantLevel: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logEntry := captureLogOutput(t, func() {
				LogBatchOperation(tt.operation, tt.batchSize, tt.duration, tt.err)
			})

			if tt.err == nil {
				assert.Equal(t, "Batch operation completed", logEntry["msg"])
			} else {
				assert.Equal(t, "Batch operation failed", logEntry["msg"])
			}

			assert.Equal(t, tt.wantLevel, logEntry["level"])
			assert.Equal(t, tt.operation, logEntry["operation"])
			assert.Equal(t, float64(tt.batchSize), logEntry["batch_size"])
			assert.Contains(t, logEntry["duration"], tt.duration.String())

			if tt.err != nil {
				assert.Contains(t, logEntry["error"], tt.err.Error())
			}
		})
	}
}

// TestLogMetricUpdate тестирует логирование обновлений метрик
func TestLogMetricUpdate(t *testing.T) {
	tests := []struct {
		name       string
		metricType string
		metricName string
		value      interface{}
		err        error
		wantLevel  string
	}{
		{
			name:       "successful gauge update",
			metricType: "gauge",
			metricName: "cpu_usage",
			value:      45.6,
			err:        nil,
			wantLevel:  "debug",
		},
		{
			name:       "successful counter update",
			metricType: "counter",
			metricName: "requests_total",
			value:      123,
			err:        nil,
			wantLevel:  "debug",
		},
		{
			name:       "failed metric update",
			metricType: "gauge",
			metricName: "memory_usage",
			value:      1024,
			err:        errors.New("invalid value"),
			wantLevel:  "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logEntry := captureLogOutput(t, func() {
				LogMetricUpdate(tt.metricType, tt.metricName, tt.value, tt.err)
			})

			if tt.err == nil {
				assert.Equal(t, "Metric updated successfully", logEntry["msg"])
			} else {
				assert.Equal(t, "Metric update failed", logEntry["msg"])
			}

			assert.Equal(t, tt.wantLevel, logEntry["level"])
			assert.Equal(t, tt.metricType, logEntry["metric_type"])
			assert.Equal(t, tt.metricName, logEntry["metric_name"])

			// Проверяем значение в зависимости от типа
			switch v := tt.value.(type) {
			case float64:
				assert.Equal(t, v, logEntry["value"])
			case int:
				assert.Equal(t, float64(v), logEntry["value"])
			default:
				assert.Equal(t, tt.value, logEntry["value"])
			}

			if tt.err != nil {
				assert.Contains(t, logEntry["error"], tt.err.Error())
			}
		})
	}
}

// TestWithFunctions тестирует функции добавления контекстных полей
func TestWithFunctions(t *testing.T) {
	tests := []struct {
		name    string
		fn      func() []zap.Field
		wantKey string
		wantVal interface{}
		wantLen int
	}{
		{
			name:    "WithRequestID",
			fn:      func() []zap.Field { return WithRequestID("req-123") },
			wantKey: "request_id",
			wantVal: "req-123",
			wantLen: 1,
		},
		{
			name:    "WithComponent",
			fn:      func() []zap.Field { return WithComponent("agent") },
			wantKey: "component",
			wantVal: "agent",
			wantLen: 1,
		},
		{
			name:    "WithUserID",
			fn:      func() []zap.Field { return WithUserID("user-456") },
			wantKey: "user_id",
			wantVal: "user-456",
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := tt.fn()

			assert.Len(t, fields, tt.wantLen)

			// Проверяем что поле правильное через логирование
			logEntry := captureLogOutput(t, func() {
				Log.Error("test message", fields...)
			})

			assert.Contains(t, logEntry, tt.wantKey)
			assert.Equal(t, tt.wantVal, logEntry[tt.wantKey])
		})
	}
}

// TestLogLevels тестирует фильтрацию по уровням логирования
func TestLogLevels(t *testing.T) {
	tests := []struct {
		name      string
		setLevel  string
		logFunc   func()
		shouldLog bool
	}{
		{
			name:     "debug level - should log debug",
			setLevel: "debug",
			logFunc: func() {
				LogMetricUpdate("gauge", "test", 123, nil)
			},
			shouldLog: true,
		},
		{
			name:     "info level - should not log debug",
			setLevel: "info",
			logFunc: func() {
				LogMetricUpdate("gauge", "test", 123, nil)
			},
			shouldLog: false,
		},
		{
			name:     "info level - should log info",
			setLevel: "info",
			logFunc: func() {
				LogRetrySuccess("test", 1)
			},
			shouldLog: true,
		},
		{
			name:     "error level - should log error",
			setLevel: "error",
			logFunc: func() {
				LogErrorWithContext(errors.New("test"), "test error")
			},
			shouldLog: true,
		},
		{
			name:     "error level - should not log warn",
			setLevel: "error",
			logFunc: func() {
				LogRetryAttempt(1, time.Second, errors.New("test"))
			},
			shouldLog: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Инициализируем логер с нужным уровнем
			err := Initialize(tt.setLevel)
			require.NoError(t, err)

			logEntry := captureLogOutput(t, tt.logFunc)

			if tt.shouldLog {
				assert.NotEmpty(t, logEntry)
			} else {
				assert.Empty(t, logEntry)
			}
		})
	}
}

// BenchmarkLogFunctions тестирует производительность функций логирования
func BenchmarkLogFunctions(b *testing.B) {
	// Инициализируем логер с уровнем error для минимизации влияния на производительность
	_ = Initialize("error")
	err := errors.New("test error")

	b.Run("LogErrorWithContext", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			LogErrorWithContext(err, "test message", zap.String("key", "value"))
		}
	})

	b.Run("LogRetryAttempt", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			LogRetryAttempt(i, time.Second, err)
		}
	})

	b.Run("LogMetricUpdate", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			LogMetricUpdate("gauge", "cpu", 45.6, nil)
		}
	})

	b.Run("WithFunctions", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = WithRequestID("test-id")
			_ = WithComponent("test-component")
			_ = WithUserID("test-user")
		}
	})
}
