package retry

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/error/pgerrors"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"go.uber.org/zap"
)

// RetryConfig конфигурация для повторных попыток
type RetryConfig struct {
	MaxAttempts int
	Delays      []time.Duration
}

// DefaultRetryConfig стандартная конфигурация (1s, 3s, 5s)
var DefaultRetryConfig = RetryConfig{
	MaxAttempts: 3,
	Delays:      []time.Duration{1 * time.Second, 3 * time.Second, 5 * time.Second},
}

// RetriableFunc функция, которую нужно выполнить с повторными попытками
type RetriableFunc func() error

// WithRetry выполняет функцию с повторными попытками для retriable-ошибок
func WithRetry(ctx context.Context, config RetryConfig, fn RetriableFunc, classifier *pgerrors.PostgresErrorClassifier) error {
	var lastErr error

	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		// Выполняем функцию
		err := fn()
		if err == nil {
			if attempt > 0 {
				logger.LogRetrySuccess("operation", attempt+1)
			}
			return nil // Успех
		}

		lastErr = err

		// Проверяем, является ли ошибка retriable
		isRetriable := false
		if classifier != nil {
			classification := classifier.Classify(err)
			isRetriable = (classification == pgerrors.Retriable)
		} else {
			// Если классификатор не передан, считаем все сетевые ошибки retriable
			isRetriable = isNetworkError(err)
		}

		if !isRetriable {
			logger.Log.Error("Non-retriable error encountered", zap.Error(err))
			return fmt.Errorf("non-retriable error: %w", err)
		}

		// Логируем попытку повтора
		delay := getDelay(config, attempt)
		logger.LogRetryAttempt(attempt+1, delay, err)

		// Если это последняя попытка, выходим
		if attempt == config.MaxAttempts-1 {
			break
		}

		// Ждём перед следующей попыткой
		select {
		case <-ctx.Done():
			logger.Log.Error("Retry cancelled by context", zap.Error(ctx.Err()))
			return fmt.Errorf("retry cancelled: %w", ctx.Err())
		case <-time.After(delay):
			// Продолжаем
		}
	}

	logger.LogRetryFailure("operation", config.MaxAttempts, lastErr)
	return fmt.Errorf("operation failed after %d attempts: %w", config.MaxAttempts, lastErr)
}

// getDelay возвращает задержку для текущей попытки
func getDelay(config RetryConfig, attempt int) time.Duration {
	if attempt < len(config.Delays) {
		return config.Delays[attempt]
	}
	// Если попыток больше, чем задержек, используем последнюю задержку
	return config.Delays[len(config.Delays)-1]
}

// isNetworkError проверяет, является ли ошибка сетевой
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	errorStr := err.Error()
	networkErrors := []string{
		"connection refused",
		"connection reset",
		"network is unreachable",
		"timeout",
		"deadline exceeded",
		"no such host",
		"temporary failure",
		"dial tcp",
	}

	for _, networkError := range networkErrors {
		if containsIgnoreCase(errorStr, networkError) {
			return true
		}
	}

	return false
}

// containsIgnoreCase проверяет наличие подстроки без учета регистра
func containsIgnoreCase(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}

	sLower := strings.ToLower(s)
	substrLower := strings.ToLower(substr)
	return strings.Contains(sLower, substrLower)
}
