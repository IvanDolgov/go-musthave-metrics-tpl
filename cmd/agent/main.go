package main

import (
	"bytes"
	"context"
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
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/compress"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/config"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/hash"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/retry"
	"go.uber.org/zap"
)

// Global agent instance for sending metrics
var globalAgent *agent.MetricsAgent
var agentMutex sync.RWMutex

// run запускает приложение с переданной конфигурацией
func run(ctx context.Context, cfg models.Config) error {
	// Создаем агент
	metricsAgent := agent.NewMetricsAgent(cfg)

	// Сохраняем глобальную ссылку для отправки метрик
	agentMutex.Lock()
	globalAgent = metricsAgent
	agentMutex.Unlock()

	// Переопределяем метод отправки метрик в агенте
	// Это нужно потому что функции отправки находятся в этом пакете
	// В реальном проекте лучше вынести отправку в отдельный пакет

	// Запускаем агент
	metricsAgent.Start()
	defer metricsAgent.Stop()

	// Канал для сигналов завершения
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	logger.Log.Info("Программа запущена. Нажмите Ctrl+C для остановки")

	// Ждем сигнал завершения или отмену контекста
	select {
	case <-stop:
		logger.Log.Info("Завершение программы по сигналу...")
	case <-ctx.Done():
		logger.Log.Info("Завершение программы по контексту...")
	}

	return nil
}

// sendMetricsBatch отправляет метрики батчем с повторными попытками
func sendMetricsBatch(ctx context.Context, cfg models.Config, metrics []models.Metrics) error {
	if len(metrics) == 0 {
		return nil // Не отправляем пустые батчи
	}

	operation := func() error {
		return sendMetricsBatchOnce(ctx, cfg, metrics)
	}

	// Для агента используем классификатор nil, так как он работает с HTTP, не с PostgreSQL
	err := retry.WithRetry(ctx, retry.DefaultRetryConfig, operation, nil)
	if err != nil {
		return fmt.Errorf("failed to send metrics batch after retries: %w", err)
	}

	return nil
}

// sendMetricsBatchOnce отправляет метрики батчем (одна попытка)
func sendMetricsBatchOnce(ctx context.Context, cfg models.Config, metrics []models.Metrics) error {
	startTime := time.Now()

	// Формируем полный адрес сервера
	fullPathServer := buildServerAddress(cfg.Server, cfg.Port)
	endpoint := fmt.Sprintf("http://%s/updates", fullPathServer)

	// Кодируем метрики в JSON
	jsonData, err := json.Marshal(metrics)
	if err != nil {
		logger.Log.Error("Failed to marshal metrics to JSON", zap.Error(err))
		return fmt.Errorf("error encoding JSON: %w", err)
	}

	// ВЫЧИСЛЯЕМ ХЕШ перед сжатием
	hashValue := hash.ComputeHMACSHA256(jsonData, cfg.Key)

	// Сжимаем данные
	compressedData, err := compress.GzipCompress(jsonData)
	if err != nil {
		logger.Log.Error("Failed to compress metrics data", zap.Error(err))
		return fmt.Errorf("gzip compress error: %w", err)
	}

	// Создаем клиент с таймаутом
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Создаем запрос с контекстом
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(compressedData))
	if err != nil {
		logger.Log.Error("Failed to create HTTP request", zap.Error(err))
		return fmt.Errorf("error creating request: %w", err)
	}

	// Устанавливаем заголовки
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")

	// ДОБАВЛЯЕМ ХЕШ В ЗАГОЛОВОК (если ключ установлен)
	if hashValue != "" {
		req.Header.Set("HashSHA256", hashValue)
	}

	// Отправляем запрос
	response, err := client.Do(req)
	if err != nil {
		logger.Log.Error("Error sending metrics batch",
			zap.String("endpoint", endpoint),
			zap.Error(err),
		)
		return fmt.Errorf("error sending metrics batch: %w", err)
	}
	defer response.Body.Close()

	// Проверяем статус ответа
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
	)

	return nil
}

func buildServerAddress(server, port string) string {
	if strings.TrimSpace(server) == "" {
		return ":" + port
	}
	return server + ":" + port
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

	// Переопределяем метод отправки метрик в агенте
	// Это нужно сделать до создания агента
	// В реальном проекте лучше использовать dependency injection

	// Создаем корневой контекст
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Запускаем приложение с контекстом
	if err := run(ctx, cfg); err != nil {
		logger.Log.Error("Application error", zap.Error(err))
		os.Exit(1)
	}
}

// SendMetricsBatch - экспортируемая функция для отправки метрик из пакета agent
func SendMetricsBatch(ctx context.Context, cfg models.Config, metrics []models.Metrics) error {
	return sendMetricsBatch(ctx, cfg, metrics)
}
