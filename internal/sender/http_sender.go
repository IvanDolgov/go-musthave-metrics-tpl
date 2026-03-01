// Package sender предоставляет реализации для отправки метрик
package sender

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/compress"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/crypto"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/hash"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"go.uber.org/zap"
)

// HTTPMetricsSender реализация отправки метрик по HTTP
type HTTPMetricsSender struct {
	config      models.Config
	publicKey   interface{} // *rsa.PublicKey
	client      *http.Client
	wg          sync.WaitGroup
	shutdown    chan struct{}
	metricsChan chan []models.Metrics
	localIP     string    // поле для хранения локального IP
	ipOnce      sync.Once // поле для однократной инициализации IP
}

// NewHTTPMetricsSender создает новый экземпляр HTTPMetricsSender
func NewHTTPMetricsSender(cfg models.Config) (*HTTPMetricsSender, error) {
	sender := &HTTPMetricsSender{
		config:      cfg,
		client:      &http.Client{Timeout: 10 * time.Second},
		shutdown:    make(chan struct{}),
		metricsChan: make(chan []models.Metrics, 100),
	}

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

// initLocalIP инициализирует локальный IP адрес
func (s *HTTPMetricsSender) initLocalIP() {
	s.ipOnce.Do(func() {
		ip, err := getLocalIP()
		if err != nil {
			logger.Log.Warn("Failed to get local IP", zap.Error(err))
			s.localIP = ""
		} else {
			s.localIP = ip
			logger.Log.Debug("Local IP detected", zap.String("ip", s.localIP))
		}
	})
}

// sendMetricsWithContext отправляет метрики на сервер с заданным контекстом
func (s *HTTPMetricsSender) sendMetricsWithContext(ctx context.Context, metrics []models.Metrics) error {
	startTime := time.Now()
	endpoint := "http://" + buildServerAddress(s.config.Server, s.config.Port) + "/updates/"

	jsonData, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}

	dataToSend := jsonData
	encryptionEnabled := false
	if s.publicKey != nil {
		pubKey := s.publicKey.(*rsa.PublicKey)
		encryptedData, err := crypto.EncryptWithHybrid(jsonData, pubKey)
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

	// Добавляем заголовок X-Real-IP с локальным IP агента
	s.initLocalIP()
	if s.localIP != "" {
		req.Header.Set("X-Real-IP", s.localIP)
	}

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
		zap.String("local_ip", s.localIP),
	)

	return nil
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
				logger.Log.Info("Metrics channel closed, stopping processor")
				return
			}
			err := s.sendMetrics(metrics)
			if err != nil {
				logger.Log.Error("Failed to send metrics batch",
					zap.Error(err),
					zap.Int("metrics_count", len(metrics)))
			}
		case <-s.shutdown:
			logger.Log.Info("Processing remaining metrics before shutdown")
			s.drainAndSend()
			return
		}
	}
}

// drainAndSend отправляет все оставшиеся метрики при завершении
func (s *HTTPMetricsSender) drainAndSend() {
	remaining := len(s.metricsChan)
	if remaining > 0 {
		logger.Log.Info("Sending remaining metrics", zap.Int("count", remaining))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		for i := 0; i < remaining; i++ {
			select {
			case metrics := <-s.metricsChan:
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

func buildServerAddress(server, port string) string {
	if strings.TrimSpace(server) == "" {
		return ":" + port
	}
	return server + ":" + port
}

// getLocalIP возвращает локальный IP адрес агента
func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		// Проверяем, что это IP адрес и не loopback
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no suitable IP address found")
}
