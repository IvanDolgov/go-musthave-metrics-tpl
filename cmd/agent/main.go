package main

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/agent"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/buildinfo"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/compress"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/config"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/crypto"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/hash"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"go.uber.org/zap"
)

// Глобальные переменные для версии сборки
var (
	buildVersion string
	buildDate    string
	buildCommit  string
)

// HTTPMetricsSender реализация отправки метрик по HTTP
type HTTPMetricsSender struct {
	config    models.Config
	publicKey interface{} // *rsa.PublicKey
}

// NewHTTPMetricsSender создает новый экземпляр HTTPMetricsSender
func NewHTTPMetricsSender(cfg models.Config) (*HTTPMetricsSender, error) {
	sender := &HTTPMetricsSender{
		config: cfg,
	}

	// Загружаем публичный ключ, если указан путь
	if cfg.CryptoKey != "" {
		pubKey, err := crypto.LoadPublicKey(cfg.CryptoKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load public key: %w", err)
		}
		sender.publicKey = pubKey
		logger.Log.Info("Public key loaded for encryption", zap.String("path", cfg.CryptoKey))
	}

	return sender, nil
}

// SendMetricsBatch отправляет батч метрик на сервер
func (s *HTTPMetricsSender) SendMetricsBatch(ctx context.Context, metrics []models.Metrics) error {
	startTime := time.Now()
	endpoint := "http://" + buildServerAddress(s.config.Server, s.config.Port) + "/updates/"

	jsonData, err := json.Marshal(metrics)
	if err != nil {
		logger.Log.Error("Failed to marshal metrics to JSON", zap.Error(err))
		return fmt.Errorf("error encoding JSON: %w", err)
	}

	// Шифруем данные, если загружен публичный ключ
	dataToSend := jsonData
	encryptionEnabled := false
	if s.publicKey != nil {
		pubKey := s.publicKey.(*rsa.PublicKey)
		encryptedData, err := crypto.EncryptWithPublicKey(jsonData, pubKey)
		if err != nil {
			logger.Log.Error("Failed to encrypt metrics data", zap.Error(err))
			return fmt.Errorf("encryption error: %w", err)
		}
		dataToSend = encryptedData
		encryptionEnabled = true
		logger.Log.Debug("Metrics encrypted", zap.Int("original_size", len(jsonData)), zap.Int("encrypted_size", len(encryptedData)))
	}

	hashValue := hash.ComputeHMACSHA256(dataToSend, s.config.Key)

	compressedData, err := compress.GzipCompress(dataToSend)
	if err != nil {
		logger.Log.Error("Failed to compress metrics data", zap.Error(err))
		return fmt.Errorf("gzip compress error: %w", err)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(compressedData))
	if err != nil {
		logger.Log.Error("Failed to create HTTP request", zap.Error(err))
		return fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")

	// Добавляем заголовок, указывающий, что данные зашифрованы
	if encryptionEnabled {
		req.Header.Set("X-Encrypted", "true")
	}

	if hashValue != "" {
		req.Header.Set("HashSHA256", hashValue)
	}

	response, err := client.Do(req)
	if err != nil {
		logger.Log.Error("Error sending metrics batch",
			zap.String("endpoint", endpoint),
			zap.Error(err),
		)
		return fmt.Errorf("error sending metrics batch: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		err := fmt.Errorf("server returned non-OK status: %d", response.StatusCode)
		logger.Log.Error("Server returned error",
			zap.String("endpoint", endpoint),
			zap.Int("status_code", response.StatusCode),
		)
		return err
	}

	duration := time.Since(startTime)
	logger.Log.Info("Successfully sent metrics batch",
		zap.Int("metrics_count", len(metrics)),
		zap.Duration("duration", duration),
		zap.Bool("encrypted", encryptionEnabled),
	)

	return nil
}

func buildServerAddress(server, port string) string {
	if strings.TrimSpace(server) == "" {
		return ":" + port
	}
	return server + ":" + port
}

func run(ctx context.Context, cfg models.Config) error {
	// Вывод информации о сборке
	buildinfo.Print()

	sender, err := NewHTTPMetricsSender(cfg)
	if err != nil {
		return fmt.Errorf("failed to create metrics sender: %w", err)
	}

	metricsAgent := agent.NewMetricsAgent(cfg, sender)

	metricsAgent.Start()
	defer metricsAgent.Stop()

	<-ctx.Done()
	return nil
}

func main() {
	// Получаем конфигурацию
	cfg := config.ParseAgentFlags()

	// Инициализируем логер
	if err := logger.Initialize("info"); err != nil {
		fmt.Fprintf(os.Stderr, "Logger initialization error: %v\n", err)
		os.Exit(1)
	}
	defer logger.Log.Sync()

	// Создаем корневой контекст
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Запускаем приложение с контекстом
	if err := run(ctx, cfg); err != nil {
		logger.Log.Error("Application error", zap.Error(err))
		os.Exit(1)
	}
}
