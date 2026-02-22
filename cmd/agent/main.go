package main

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
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
	config      models.Config
	publicKey   interface{} // *rsa.PublicKey
	client      *http.Client
	wg          sync.WaitGroup
	shutdown    chan struct{}
	metricsChan chan []models.Metrics
}

// NewHTTPMetricsSender создает новый экземпляр HTTPMetricsSender
func NewHTTPMetricsSender(cfg models.Config) (*HTTPMetricsSender, error) {
	sender := &HTTPMetricsSender{
		config:      cfg,
		client:      &http.Client{Timeout: 10 * time.Second},
		shutdown:    make(chan struct{}),
		metricsChan: make(chan []models.Metrics, 100), // буферизированный канал для метрик
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

// Start запускает обработчик отправки метрик
func (s *HTTPMetricsSender) Start() {
	s.wg.Add(1)
	go s.processMetrics()
	logger.Log.Info("Metrics sender started")
}

// Stop останавливает обработчик и ожидает завершения всех отправок
func (s *HTTPMetricsSender) Stop() {
	logger.Log.Info("Stopping metrics sender, waiting for pending requests...")
	close(s.shutdown)
	s.wg.Wait()
	logger.Log.Info("Metrics sender stopped")
}

// Wait ожидает завершения всех горутин отправителя
func (s *HTTPMetricsSender) Wait() {
	s.wg.Wait()
}

// SendMetricsBatch отправляет батч метрик на сервер (неблокирующий вызов)
func (s *HTTPMetricsSender) SendMetricsBatch(ctx context.Context, metrics []models.Metrics) error {
	select {
	case s.metricsChan <- metrics:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.shutdown:
		return fmt.Errorf("sender is shutting down")
	}
}

// processMetrics обрабатывает метрики из канала и отправляет их на сервер
func (s *HTTPMetricsSender) processMetrics() {
	defer s.wg.Done()

	for {
		select {
		case metrics, ok := <-s.metricsChan:
			if !ok {
				// Канал закрыт, выходим
				logger.Log.Info("Metrics channel closed, stopping processor")
				return
			}
			// Пытаемся отправить метрики
			err := s.sendMetrics(metrics)
			if err != nil {
				logger.Log.Error("Failed to send metrics batch",
					zap.Error(err),
					zap.Int("metrics_count", len(metrics)))
			}
		case <-s.shutdown:
			// При завершении отправляем все оставшиеся метрики
			logger.Log.Info("Processing remaining metrics before shutdown")
			s.drainAndSend()
			return
		}
	}
}

// drainAndSend отправляет все оставшиеся метрики при завершении
func (s *HTTPMetricsSender) drainAndSend() {
	// Даем время на обработку метрик, которые уже в канале
	remaining := len(s.metricsChan)
	if remaining > 0 {
		logger.Log.Info("Sending remaining metrics",
			zap.Int("count", remaining))

		// Создаем контекст с таймаутом для финальной отправки
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		for i := 0; i < remaining; i++ {
			select {
			case metrics := <-s.metricsChan:
				// Отправляем метрики с увеличенным таймаутом
				err := s.sendMetricsWithContext(ctx, metrics)
				if err != nil {
					logger.Log.Error("Failed to send metrics during shutdown",
						zap.Error(err),
						zap.Int("metrics_count", len(metrics)))
				}
			case <-ctx.Done():
				logger.Log.Warn("Shutdown timeout exceeded while sending remaining metrics")
				return
			default:
				return
			}
		}
	}
}

// sendMetrics отправляет метрики на сервер
func (s *HTTPMetricsSender) sendMetrics(metrics []models.Metrics) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.sendMetricsWithContext(ctx, metrics)
}

// sendMetricsWithContext отправляет метрики на сервер с заданным контекстом
func (s *HTTPMetricsSender) sendMetricsWithContext(ctx context.Context, metrics []models.Metrics) error {
	startTime := time.Now()
	endpoint := "http://" + buildServerAddress(s.config.Server, s.config.Port) + "/updates/"

	jsonData, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}

	// Шифруем данные, если загружен публичный ключ
	dataToSend := jsonData
	encryptionEnabled := false
	if s.publicKey != nil {
		pubKey := s.publicKey.(*rsa.PublicKey)
		encryptedData, err := crypto.EncryptWithPublicKey(jsonData, pubKey)
		if err != nil {
			return fmt.Errorf("encryption error: %w", err)
		}
		dataToSend = encryptedData
		encryptionEnabled = true
	}

	hashValue := hash.ComputeHMACSHA256(dataToSend, s.config.Key)

	compressedData, err := compress.GzipCompress(dataToSend)
	if err != nil {
		return fmt.Errorf("gzip compress error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(compressedData))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")

	if encryptionEnabled {
		req.Header.Set("X-Encrypted", "true")
	}

	if hashValue != "" {
		req.Header.Set("HashSHA256", hashValue)
	}

	response, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending metrics batch: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned non-OK status: %d", response.StatusCode)
	}

	duration := time.Since(startTime)
	logger.Log.Debug("Successfully sent metrics batch",
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

	// Запускаем обработчик отправки
	sender.Start()
	defer sender.Stop()

	// Создаем агента
	metricsAgent := agent.NewMetricsAgent(cfg, sender)

	// Запускаем агента
	metricsAgent.Start()
	defer metricsAgent.Stop()

	// Ожидаем завершения по сигналу
	<-ctx.Done()
	logger.Log.Info("Context cancelled, initiating graceful shutdown...")

	// Даем время на завершение текущих операций (максимум 30 секунд)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Ожидаем завершения всех горутин
	done := make(chan struct{})
	go func() {
		// Ждем завершения агента
		metricsAgent.Wait()
		// Ждем завершения отправителя
		sender.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Log.Info("All goroutines finished successfully")
	case <-shutdownCtx.Done():
		logger.Log.Warn("Shutdown timeout exceeded, some goroutines may not have finished")
	}

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

	// Создаем контекст с возможностью отмены
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Канал для сигналов ОС
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	// Запускаем горутину для обработки сигналов
	go func() {
		sig := <-sigChan
		logger.Log.Info("Received shutdown signal", zap.String("signal", sig.String()))
		cancel() // Отменяем контекст, запуская graceful shutdown
	}()

	// Запускаем приложение с контекстом
	if err := run(ctx, cfg); err != nil {
		logger.Log.Error("Application error", zap.Error(err))
		os.Exit(1)
	}

	logger.Log.Info("Agent shutdown complete")
}
