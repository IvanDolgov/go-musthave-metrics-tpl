package agent

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"go.uber.org/zap"
)

// BatchSender отправляет метрики батчами
type BatchSender struct {
	serverURL string
	batchSize int
	buffer    []models.Metrics
	mu        sync.Mutex
	client    *http.Client
}

// NewBatchSender создает новый BatchSender
func NewBatchSender(serverURL string, batchSize int) *BatchSender {
	return &BatchSender{
		serverURL: serverURL,
		batchSize: batchSize,
		buffer:    make([]models.Metrics, 0, batchSize),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// AddMetric добавляет метрику в буфер
func (bs *BatchSender) AddMetric(metric models.Metrics) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	bs.buffer = append(bs.buffer, metric)

	// Если буфер заполнен, отправляем батч
	if len(bs.buffer) >= bs.batchSize {
		go bs.sendBatch()
	}
}

// Flush принудительно отправляет оставшиеся метрики
func (bs *BatchSender) Flush() {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if len(bs.buffer) > 0 {
		bs.sendBatchSync()
	}
}

// sendBatchSync синхронно отправляет батч
func (bs *BatchSender) sendBatchSync() {
	if len(bs.buffer) == 0 {
		return
	}

	batch := make([]models.Metrics, len(bs.buffer))
	copy(batch, bs.buffer)
	bs.buffer = bs.buffer[:0]

	if err := bs.sendBatchToServer(batch); err != nil {
		logger.Log.Error("Failed to send batch", zap.Error(err))
		// Можно добавить retry логику здесь
	}
}

// sendBatch асинхронно отправляет батч
func (bs *BatchSender) sendBatch() {
	batch := make([]models.Metrics, len(bs.buffer))
	copy(batch, bs.buffer)
	bs.buffer = bs.buffer[:0]

	go func() {
		if err := bs.sendBatchToServer(batch); err != nil {
			logger.Log.Error("Failed to send batch", zap.Error(err))
		}
	}()
}

// sendBatchToServer отправляет батч на сервер
func (bs *BatchSender) sendBatchToServer(metrics []models.Metrics) error {
	// Сериализуем метрики в JSON
	jsonData, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	// Сжимаем данные
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(jsonData); err != nil {
		return fmt.Errorf("failed to compress data: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	// Создаем запрос
	req, err := http.NewRequest("POST", bs.serverURL+"/updates/", &buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Устанавливаем заголовки
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	// Отправляем запрос
	resp, err := bs.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status: %d", resp.StatusCode)
	}

	logger.Log.Debug("Batch sent successfully",
		zap.Int("metrics_count", len(metrics)),
		zap.String("url", bs.serverURL))

	return nil
}
