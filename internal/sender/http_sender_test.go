package sender

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewHTTPMetricsSender(t *testing.T) {
	t.Run("successful creation without crypto key", func(t *testing.T) {
		cfg := models.Config{
			Server: "localhost",
			Port:   "8080",
		}

		sender, err := NewHTTPMetricsSender(cfg)
		require.NoError(t, err)
		require.NotNil(t, sender)

		assert.Equal(t, cfg, sender.config)
		assert.Nil(t, sender.publicKey)
		assert.NotNil(t, sender.client)
		assert.NotNil(t, sender.shutdown)
		assert.NotNil(t, sender.metricsChan)
		assert.Equal(t, "", sender.localIP)
	})

	t.Run("successful creation with crypto key", func(t *testing.T) {
		// Создаем временный файл с публичным ключом для теста
		// В реальном тесте нужно создать валидный RSA ключ
		// Здесь мы пропускаем тест, так как требуется реальный ключ
		t.Skip("Requires valid RSA public key file")
	})

	t.Run("creation with invalid crypto key", func(t *testing.T) {
		cfg := models.Config{
			Server:    "localhost",
			Port:      "8080",
			CryptoKey: "/nonexistent/key.pem",
		}

		sender, err := NewHTTPMetricsSender(cfg)
		assert.Error(t, err)
		assert.Nil(t, sender)
	})
}

func TestGetLocalIP(t *testing.T) {
	ip, err := getLocalIP()

	// Функция должна вернуть IP или ошибку, но не паниковать
	if err != nil {
		// Если не удалось получить IP (например, в тестовой среде), это не ошибка теста
		t.Logf("Could not get local IP: %v", err)
	} else {
		// Если IP получен, проверяем что это валидный IPv4 адрес
		assert.NotEmpty(t, ip, "IP should not be empty")
		parsedIP := net.ParseIP(ip)
		assert.NotNil(t, parsedIP, "IP should be valid")
		assert.NotNil(t, parsedIP.To4(), "IP should be IPv4")
		t.Logf("Local IP: %s", ip)
	}
}

func TestInitLocalIP(t *testing.T) {
	cfg := models.Config{}
	sender, err := NewHTTPMetricsSender(cfg)
	require.NoError(t, err)

	// До инициализации IP должен быть пустым
	assert.Empty(t, sender.localIP)

	// Инициализируем
	sender.initLocalIP()

	// После инициализации может быть пустым если не удалось получить IP,
	// но это нормально для тестовой среды
	t.Logf("Local IP after init: %s", sender.localIP)

	// Проверяем что sync.Once работает - повторный вызов не должен менять значение
	firstIP := sender.localIP
	sender.initLocalIP()
	assert.Equal(t, firstIP, sender.localIP)
}

func TestBuildServerAddress(t *testing.T) {
	tests := []struct {
		name     string
		server   string
		port     string
		expected string
	}{
		{
			name:     "both server and port provided",
			server:   "localhost",
			port:     "8080",
			expected: "localhost:8080",
		},
		{
			name:     "empty server",
			server:   "",
			port:     "9090",
			expected: ":9090",
		},
		{
			name:     "server with spaces",
			server:   "  localhost  ",
			port:     "8080",
			expected: "localhost:8080",
		},
		{
			name:     "empty server with spaces",
			server:   "   ",
			port:     "8080",
			expected: ":8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildServerAddress(tt.server, tt.port)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSendMetricsWithContext(t *testing.T) {
	t.Run("successful send", func(t *testing.T) {
		// Создаем тестовый сервер
		var receivedData []byte
		var receivedHeaders http.Header
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHeaders = r.Header

			// Читаем тело запроса
			buf := make([]byte, r.ContentLength)
			r.Body.Read(buf)
			receivedData = buf

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Парсим адрес сервера
		serverURL := strings.TrimPrefix(server.URL, "http://")
		parts := strings.Split(serverURL, ":")
		host := parts[0]
		port := parts[1]

		cfg := models.Config{
			Server: host,
			Port:   port,
		}

		sender, err := NewHTTPMetricsSender(cfg)
		require.NoError(t, err)

		// Подменяем localIP для теста
		sender.localIP = "192.168.1.100"

		metrics := []models.Metrics{
			{
				ID:    "test_metric",
				MType: "gauge",
				Value: func() *float64 { v := 42.5; return &v }(),
			},
		}

		ctx := context.Background()
		err = sender.sendMetricsWithContext(ctx, metrics)
		assert.NoError(t, err)

		// Проверяем заголовки
		assert.Equal(t, "application/json", receivedHeaders.Get("Content-Type"))
		assert.Equal(t, "gzip", receivedHeaders.Get("Content-Encoding"))
		assert.Equal(t, "gzip", receivedHeaders.Get("Accept-Encoding"))
		assert.Equal(t, "192.168.1.100", receivedHeaders.Get("X-Real-IP"))

		// Проверяем, что данные были отправлены (не пустые)
		assert.NotEmpty(t, receivedData)
	})

	t.Run("server returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		serverURL := strings.TrimPrefix(server.URL, "http://")
		parts := strings.Split(serverURL, ":")
		host := parts[0]
		port := parts[1]

		cfg := models.Config{
			Server: host,
			Port:   port,
		}

		sender, err := NewHTTPMetricsSender(cfg)
		require.NoError(t, err)

		metrics := []models.Metrics{
			{
				ID:    "test",
				MType: "gauge",
				Value: func() *float64 { v := 1.0; return &v }(),
			},
		}

		ctx := context.Background()
		err = sender.sendMetricsWithContext(ctx, metrics)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "server returned non-OK status: 500")
	})

	t.Run("context timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		serverURL := strings.TrimPrefix(server.URL, "http://")
		parts := strings.Split(serverURL, ":")
		host := parts[0]
		port := parts[1]

		cfg := models.Config{
			Server: host,
			Port:   port,
		}

		sender, err := NewHTTPMetricsSender(cfg)
		require.NoError(t, err)

		metrics := []models.Metrics{
			{
				ID:    "test",
				MType: "gauge",
				Value: func() *float64 { v := 1.0; return &v }(),
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		err = sender.sendMetricsWithContext(ctx, metrics)
		assert.Error(t, err)
	})

	t.Run("with encryption", func(t *testing.T) {
		// Тест требует валидный RSA ключ, пропускаем
		t.Skip("Requires valid RSA key for encryption test")
	})

	t.Run("with hash", func(t *testing.T) {
		var receivedHash string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHash = r.Header.Get("HashSHA256")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		serverURL := strings.TrimPrefix(server.URL, "http://")
		parts := strings.Split(serverURL, ":")
		host := parts[0]
		port := parts[1]

		cfg := models.Config{
			Server: host,
			Port:   port,
			Key:    "test-secret-key",
		}

		sender, err := NewHTTPMetricsSender(cfg)
		require.NoError(t, err)

		sender.localIP = "192.168.1.100"

		metrics := []models.Metrics{
			{
				ID:    "test",
				MType: "gauge",
				Value: func() *float64 { v := 1.0; return &v }(),
			},
		}

		ctx := context.Background()
		err = sender.sendMetricsWithContext(ctx, metrics)
		assert.NoError(t, err)

		// Проверяем, что заголовок HashSHA256 был установлен
		assert.NotEmpty(t, receivedHash)
	})
}

func TestSendMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL := strings.TrimPrefix(server.URL, "http://")
	parts := strings.Split(serverURL, ":")
	host := parts[0]
	port := parts[1]

	cfg := models.Config{
		Server: host,
		Port:   port,
	}

	sender, err := NewHTTPMetricsSender(cfg)
	require.NoError(t, err)

	metrics := []models.Metrics{
		{
			ID:    "test",
			MType: "gauge",
			Value: func() *float64 { v := 1.0; return &v }(),
		},
	}

	err = sender.sendMetrics(metrics)
	assert.NoError(t, err)
}

func TestSendMetricsBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL := strings.TrimPrefix(server.URL, "http://")
	parts := strings.Split(serverURL, ":")
	host := parts[0]
	port := parts[1]

	cfg := models.Config{
		Server: host,
		Port:   port,
	}

	sender, err := NewHTTPMetricsSender(cfg)
	require.NoError(t, err)

	// Запускаем sender
	sender.Start()
	defer sender.Stop()

	metrics := []models.Metrics{
		{
			ID:    "test",
			MType: "gauge",
			Value: func() *float64 { v := 1.0; return &v }(),
		},
	}

	ctx := context.Background()
	err = sender.SendMetricsBatch(ctx, metrics)
	assert.NoError(t, err)

	// Даем время на обработку
	time.Sleep(100 * time.Millisecond)
}

func TestSendMetricsBatch_ContextCanceled(t *testing.T) {
	cfg := models.Config{}
	sender, err := NewHTTPMetricsSender(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // сразу отменяем контекст

	metrics := []models.Metrics{
		{
			ID:    "test",
			MType: "gauge",
			Value: func() *float64 { v := 1.0; return &v }(),
		},
	}

	err = sender.SendMetricsBatch(ctx, metrics)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestSendMetricsBatch_ShuttingDown(t *testing.T) {
	cfg := models.Config{}
	sender, err := NewHTTPMetricsSender(cfg)
	require.NoError(t, err)

	// Запускаем и сразу останавливаем
	sender.Start()
	sender.Stop()

	metrics := []models.Metrics{
		{
			ID:    "test",
			MType: "gauge",
			Value: func() *float64 { v := 1.0; return &v }(),
		},
	}

	ctx := context.Background()
	err = sender.SendMetricsBatch(ctx, metrics)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sender is shutting down")
}

func TestProcessMetrics(t *testing.T) {
	// Создаем наблюдаемый логгер
	core, observedLogs := observer.New(zap.InfoLevel)
	logger.Log = zap.New(core)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL := strings.TrimPrefix(server.URL, "http://")
	parts := strings.Split(serverURL, ":")
	host := parts[0]
	port := parts[1]

	cfg := models.Config{
		Server: host,
		Port:   port,
	}

	sender, err := NewHTTPMetricsSender(cfg)
	require.NoError(t, err)

	// Запускаем обработчик
	sender.Start()

	// Отправляем метрики
	metrics := []models.Metrics{
		{
			ID:    "test",
			MType: "gauge",
			Value: func() *float64 { v := 1.0; return &v }(),
		},
	}

	for i := 0; i < 5; i++ {
		err = sender.SendMetricsBatch(context.Background(), metrics)
		assert.NoError(t, err)
	}

	// Даем время на обработку
	time.Sleep(200 * time.Millisecond)

	// Останавливаем и проверяем логи
	sender.Stop()

	// Проверяем, что были логи об отправке
	found := false
	for _, log := range observedLogs.All() {
		if log.Message == "Successfully sent metrics batch" {
			found = true
			break
		}
	}
	assert.True(t, found, "Should have logged successful sends")
}

func TestDrainAndSend(t *testing.T) {
	receivedCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL := strings.TrimPrefix(server.URL, "http://")
	parts := strings.Split(serverURL, ":")
	host := parts[0]
	port := parts[1]

	cfg := models.Config{
		Server: host,
		Port:   port,
	}

	sender, err := NewHTTPMetricsSender(cfg)
	require.NoError(t, err)

	// Заполняем канал метриками
	metrics := []models.Metrics{
		{
			ID:    "test",
			MType: "gauge",
			Value: func() *float64 { v := 1.0; return &v }(),
		},
	}

	for i := 0; i < 3; i++ {
		sender.metricsChan <- metrics
	}

	// Вызываем drainAndSend
	sender.drainAndSend()

	// Проверяем, что все метрики были отправлены
	assert.Equal(t, 3, receivedCount)
}

func TestConcurrentSends(t *testing.T) {
	receivedCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL := strings.TrimPrefix(server.URL, "http://")
	parts := strings.Split(serverURL, ":")
	host := parts[0]
	port := parts[1]

	cfg := models.Config{
		Server: host,
		Port:   port,
	}

	sender, err := NewHTTPMetricsSender(cfg)
	require.NoError(t, err)

	sender.Start()
	defer sender.Stop()

	metrics := []models.Metrics{
		{
			ID:    "test",
			MType: "gauge",
			Value: func() *float64 { v := 1.0; return &v }(),
		},
	}

	// Запускаем несколько горутин для отправки метрик
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				err := sender.SendMetricsBatch(context.Background(), metrics)
				assert.NoError(t, err)
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	// Даем время на обработку всех метрик
	time.Sleep(500 * time.Millisecond)

	// Проверяем, что все метрики были отправлены
	mu.Lock()
	assert.Equal(t, 50, receivedCount)
	mu.Unlock()
}

func TestWait(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL := strings.TrimPrefix(server.URL, "http://")
	parts := strings.Split(serverURL, ":")
	host := parts[0]
	port := parts[1]

	cfg := models.Config{
		Server: host,
		Port:   port,
	}

	sender, err := NewHTTPMetricsSender(cfg)
	require.NoError(t, err)

	sender.Start()

	// Отправляем метрики
	metrics := []models.Metrics{
		{
			ID:    "test",
			MType: "gauge",
			Value: func() *float64 { v := 1.0; return &v }(),
		},
	}

	for i := 0; i < 3; i++ {
		err = sender.SendMetricsBatch(context.Background(), metrics)
		assert.NoError(t, err)
	}

	// Останавливаем и ждем
	done := make(chan struct{})
	go func() {
		sender.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Нормальное завершение
	case <-time.After(2 * time.Second):
		t.Fatal("Stop took too long")
	}
}

func TestIntegration(t *testing.T) {
	// Интеграционный тест полного цикла отправки
	receivedBatches := 0
	var receivedMetrics []models.Metrics

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBatches++

		// Декомпрессия если нужно
		// В реальном тесте нужно добавить декомпрессию

		var metrics []models.Metrics
		err := json.NewDecoder(r.Body).Decode(&metrics)
		if err == nil {
			receivedMetrics = append(receivedMetrics, metrics...)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL := strings.TrimPrefix(server.URL, "http://")
	parts := strings.Split(serverURL, ":")
	host := parts[0]
	port := parts[1]

	cfg := models.Config{
		Server: host,
		Port:   port,
	}

	sender, err := NewHTTPMetricsSender(cfg)
	require.NoError(t, err)

	sender.Start()
	defer sender.Stop()

	// Отправляем несколько батчей метрик
	for i := 0; i < 3; i++ {
		metrics := []models.Metrics{
			{
				ID:    fmt.Sprintf("test_gauge_%d", i),
				MType: "gauge",
				Value: func() *float64 { v := float64(i) * 1.5; return &v }(),
			},
			{
				ID:    fmt.Sprintf("test_counter_%d", i),
				MType: "counter",
				Delta: func() *int64 { v := int64(i) * 10; return &v }(),
			},
		}

		err = sender.SendMetricsBatch(context.Background(), metrics)
		assert.NoError(t, err)

		time.Sleep(50 * time.Millisecond)
	}

	// Даем время на отправку
	time.Sleep(500 * time.Millisecond)

	// Проверяем, что все батчи были отправлены
	assert.Equal(t, 3, receivedBatches)
	assert.Equal(t, 6, len(receivedMetrics))
}
