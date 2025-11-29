package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/agent"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/compress"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/config"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/hash"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"go.uber.org/zap"
)

// HTTPMetricsSender реализация отправки метрик по HTTP
type HTTPMetricsSender struct {
	config models.Config
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

	hashValue := hash.ComputeHMACSHA256(jsonData, s.config.Key)

	compressedData, err := compress.GzipCompress(jsonData)
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
	sender := &HTTPMetricsSender{config: cfg}

	metricsAgent := agent.NewMetricsAgent(cfg, sender)

	metricsAgent.Start()
	defer metricsAgent.Stop()

	<-ctx.Done()
	return nil
}

func main() {
	cfg := config.ParseAgentFlags()

	if err := logger.Initialize("info"); err != nil {
		fmt.Fprintf(os.Stderr, "Logger initialization error: %v\n", err)
		os.Exit(1)
	}
	defer logger.Log.Sync()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := run(ctx, cfg); err != nil {
		logger.Log.Error("Application error", zap.Error(err))
		os.Exit(1)
	}
}
