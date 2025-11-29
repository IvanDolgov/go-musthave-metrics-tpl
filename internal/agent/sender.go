package agent

import (
	"context"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
)

// MetricsSender интерфейс для отправки метрик
type MetricsSender interface {
	SendMetricsBatch(ctx context.Context, metrics []models.Metrics) error
}
