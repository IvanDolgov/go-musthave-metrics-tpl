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

// Тест для функции Initialize
func TestInitialize(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		wantErr bool
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
			name:    "invalid level",
			level:   "invalid",
			wantErr: true,
		},
		{
			name:    "empty level",
			level:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Initialize(tt.level)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, Log)
				assert.NotEqual(t, zap.NewNop(), Log)
			}
		})
	}
}

// Тест для LogErrorWithContext
func TestLogErrorWithContext(t *testing.T) {
	// Создаем буфер для перехвата логов
	buffer := &bytes.Buffer{}

	// Создаем тестовый логгер
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	writer := zapcore.AddSync(buffer)
	core := zapcore.NewCore(encoder, writer, zapcore.DebugLevel)
	testLogger := zap.New(core)

	// Сохраняем оригинальный логгер и восстанавливаем после теста
	originalLog := Log
	defer func() { Log = originalLog }()

	Log = testLogger

	t.Run("log error with context", func(t *testing.T) {
		buffer.Reset()
		testErr := errors.New("test error")
		message := "error occurred"
		fields := []zap.Field{zap.String("key", "value")}

		LogErrorWithContext(testErr, message, fields...)

		// Проверяем что лог был записан
		logData := make(map[string]interface{})
		err := json.Unmarshal(buffer.Bytes(), &logData)
		require.NoError(t, err)

		assert.Equal(t, "error", logData["level"])
		assert.Equal(t, message, logData["msg"])
		assert.Equal(t, "test error", logData["error"])
		assert.Equal(t, "value", logData["key"])
	})

	t.Run("nil error does not log", func(t *testing.T) {
		buffer.Reset()
		LogErrorWithContext(nil, "should not appear")
		assert.Empty(t, buffer.Bytes())
	})
}

// Тест для LogRetryAttempt
func TestLogRetryAttempt(t *testing.T) {
	buffer := &bytes.Buffer{}

	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	writer := zapcore.AddSync(buffer)
	core := zapcore.NewCore(encoder, writer, zapcore.DebugLevel)
	testLogger := zap.New(core)

	originalLog := Log
	defer func() { Log = originalLog }()

	Log = testLogger

	LogRetryAttempt(2, 100*time.Millisecond, errors.New("connection failed"))

	var logData map[string]interface{}
	err := json.Unmarshal(buffer.Bytes(), &logData)
	require.NoError(t, err)

	assert.Equal(t, "warn", logData["level"])
	assert.Equal(t, float64(2), logData["attempt"])
	assert.Contains(t, logData["delay"].(string), "100ms")
	assert.Equal(t, "connection failed", logData["error"])
}

// Тест для LogRetrySuccess
func TestLogRetrySuccess(t *testing.T) {
	buffer := &bytes.Buffer{}

	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	writer := zapcore.AddSync(buffer)
	core := zapcore.NewCore(encoder, writer, zapcore.DebugLevel)
	testLogger := zap.New(core)

	originalLog := Log
	defer func() { Log = originalLog }()

	Log = testLogger

	LogRetrySuccess("db_operation", 3)

	var logData map[string]interface{}
	err := json.Unmarshal(buffer.Bytes(), &logData)
	require.NoError(t, err)

	assert.Equal(t, "info", logData["level"])
	assert.Equal(t, "db_operation", logData["operation"])
	assert.Equal(t, float64(3), logData["total_attempts"])
}

// Тест для LogRetryFailure
func TestLogRetryFailure(t *testing.T) {
	buffer := &bytes.Buffer{}

	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	writer := zapcore.AddSync(buffer)
	core := zapcore.NewCore(encoder, writer, zapcore.DebugLevel)
	testLogger := zap.New(core)

	originalLog := Log
	defer func() { Log = originalLog }()

	Log = testLogger

	finalErr := errors.New("max retries exceeded")
	LogRetryFailure("api_call", 5, finalErr)

	var logData map[string]interface{}
	err := json.Unmarshal(buffer.Bytes(), &logData)
	require.NoError(t, err)

	assert.Equal(t, "error", logData["level"])
	assert.Equal(t, "api_call", logData["operation"])
	assert.Equal(t, float64(5), logData["max_attempts"])
	assert.Equal(t, "max retries exceeded", logData["error"])
}

// Тест для LogDatabaseError
func TestLogDatabaseError(t *testing.T) {
	buffer := &bytes.Buffer{}

	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	writer := zapcore.AddSync(buffer)
	core := zapcore.NewCore(encoder, writer, zapcore.DebugLevel)
	testLogger := zap.New(core)

	originalLog := Log
	defer func() { Log = originalLog }()

	Log = testLogger

	tests := []struct {
		name      string
		operation string
		err       error
		query     string
		params    []interface{}
	}{
		{
			name:      "error with query and params",
			operation: "select",
			err:       errors.New("connection refused"),
			query:     "SELECT * FROM users",
			params:    []interface{}{"param1", 123},
		},
		{
			name:      "error without params",
			operation: "insert",
			err:       errors.New("duplicate key"),
			query:     "INSERT INTO users",
			params:    nil,
		},
		{
			name:      "error without query",
			operation: "delete",
			err:       errors.New("table not found"),
			query:     "",
			params:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer.Reset()
			LogDatabaseError(tt.operation, tt.err, tt.query, tt.params...)

			var logData map[string]interface{}
			err := json.Unmarshal(buffer.Bytes(), &logData)
			require.NoError(t, err)

			assert.Equal(t, "error", logData["level"])
			assert.Equal(t, tt.operation, logData["operation"])
			assert.Equal(t, tt.err.Error(), logData["error"])

			if tt.query != "" {
				assert.Equal(t, tt.query, logData["query"])
			}

			if tt.params != nil {
				assert.NotNil(t, logData["parameters"])
			}
		})
	}
}

// Тест для LogNetworkError
func TestLogNetworkError(t *testing.T) {
	buffer := &bytes.Buffer{}

	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	writer := zapcore.AddSync(buffer)
	core := zapcore.NewCore(encoder, writer, zapcore.DebugLevel)
	testLogger := zap.New(core)

	originalLog := Log
	defer func() { Log = originalLog }()

	Log = testLogger

	tests := []struct {
		name       string
		operation  string
		url        string
		err        error
		statusCode int
	}{
		{
			name:       "with status code",
			operation:  "GET",
			url:        "http://example.com",
			err:        errors.New("timeout"),
			statusCode: 408,
		},
		{
			name:       "without status code",
			operation:  "POST",
			url:        "http://example.com",
			err:        errors.New("connection refused"),
			statusCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer.Reset()
			LogNetworkError(tt.operation, tt.url, tt.err, tt.statusCode)

			var logData map[string]interface{}
			err := json.Unmarshal(buffer.Bytes(), &logData)
			require.NoError(t, err)

			assert.Equal(t, "error", logData["level"])
			assert.Equal(t, tt.operation, logData["operation"])
			assert.Equal(t, tt.url, logData["url"])
			assert.Equal(t, tt.err.Error(), logData["error"])

			if tt.statusCode > 0 {
				assert.Equal(t, float64(tt.statusCode), logData["status_code"])
			}
		})
	}
}

// Тест для LogBatchOperation
func TestLogBatchOperation(t *testing.T) {
	buffer := &bytes.Buffer{}

	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	writer := zapcore.AddSync(buffer)
	core := zapcore.NewCore(encoder, writer, zapcore.DebugLevel)
	testLogger := zap.New(core)

	originalLog := Log
	defer func() { Log = originalLog }()

	Log = testLogger

	tests := []struct {
		name          string
		operation     string
		batchSize     int
		duration      time.Duration
		err           error
		expectedLevel string
	}{
		{
			name:          "successful batch",
			operation:     "insert",
			batchSize:     100,
			duration:      500 * time.Millisecond,
			err:           nil,
			expectedLevel: "info",
		},
		{
			name:          "failed batch",
			operation:     "update",
			batchSize:     50,
			duration:      1 * time.Second,
			err:           errors.New("batch failed"),
			expectedLevel: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer.Reset()
			LogBatchOperation(tt.operation, tt.batchSize, tt.duration, tt.err)

			var logData map[string]interface{}
			err := json.Unmarshal(buffer.Bytes(), &logData)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedLevel, logData["level"])
			assert.Equal(t, tt.operation, logData["operation"])
			assert.Equal(t, float64(tt.batchSize), logData["batch_size"])
			assert.Contains(t, logData["duration"].(string), "ms")

			if tt.err != nil {
				assert.Equal(t, tt.err.Error(), logData["error"])
			}
		})
	}
}

// Тест для LogMetricUpdate
func TestLogMetricUpdate(t *testing.T) {
	buffer := &bytes.Buffer{}

	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	writer := zapcore.AddSync(buffer)
	core := zapcore.NewCore(encoder, writer, zapcore.DebugLevel)
	testLogger := zap.New(core)

	originalLog := Log
	defer func() { Log = originalLog }()

	Log = testLogger

	tests := []struct {
		name          string
		metricType    string
		metricName    string
		value         interface{}
		err           error
		expectedLevel string
	}{
		{
			name:          "successful gauge update",
			metricType:    "gauge",
			metricName:    "temperature",
			value:         23.5,
			err:           nil,
			expectedLevel: "debug",
		},
		{
			name:          "successful counter update",
			metricType:    "counter",
			metricName:    "requests",
			value:         42,
			err:           nil,
			expectedLevel: "debug",
		},
		{
			name:          "failed metric update",
			metricType:    "gauge",
			metricName:    "memory",
			value:         1024,
			err:           errors.New("invalid metric"),
			expectedLevel: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer.Reset()
			LogMetricUpdate(tt.metricType, tt.metricName, tt.value, tt.err)

			var logData map[string]interface{}
			err := json.Unmarshal(buffer.Bytes(), &logData)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedLevel, logData["level"])
			assert.Equal(t, tt.metricType, logData["metric_type"])
			assert.Equal(t, tt.metricName, logData["metric_name"])

			if tt.err != nil {
				assert.Equal(t, tt.err.Error(), logData["error"])
			}
		})
	}
}

// Тест для функций с контекстом
func TestContextFunctions(t *testing.T) {
	t.Run("WithRequestID", func(t *testing.T) {
		fields := WithRequestID("req-123")
		assert.Len(t, fields, 1)
		assert.Equal(t, "request_id", fields[0].Key)
		assert.Equal(t, "req-123", fields[0].String)
	})

	t.Run("WithComponent", func(t *testing.T) {
		fields := WithComponent("api")
		assert.Len(t, fields, 1)
		assert.Equal(t, "component", fields[0].Key)
		assert.Equal(t, "api", fields[0].String)
	})

	t.Run("WithUserID", func(t *testing.T) {
		fields := WithUserID("user-456")
		assert.Len(t, fields, 1)
		assert.Equal(t, "user_id", fields[0].Key)
		assert.Equal(t, "user-456", fields[0].String)
	})
}

// Тест для проверки что логгер инициализируется с NOP по умолчанию
func TestDefaultLoggerIsNop(t *testing.T) {
	assert.Equal(t, zap.NewNop(), Log)
}

// Интеграционный тест для полного цикла логирования
func TestFullLoggingCycle(t *testing.T) {
	buffer := &bytes.Buffer{}

	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	writer := zapcore.AddSync(buffer)
	core := zapcore.NewCore(encoder, writer, zapcore.DebugLevel)
	testLogger := zap.New(core)

	originalLog := Log
	defer func() { Log = originalLog }()

	Log = testLogger

	// Симулируем полный цикл операции с ретраями
	err := errors.New("temporary failure")

	LogRetryAttempt(1, 100*time.Millisecond, err)
	LogRetryAttempt(2, 200*time.Millisecond, err)
	LogRetryAttempt(3, 400*time.Millisecond, err)
	LogRetryFailure("api_call", 3, err)

	// Проверяем что все 4 лога были записаны
	lines := bytes.Split(bytes.TrimSpace(buffer.Bytes()), []byte("\n"))
	assert.Len(t, lines, 4)
}
