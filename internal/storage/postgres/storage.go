package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/error/pgerrors"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/retry"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
)

// DatabaseStorage интерфейс для проверки подключения к БД
type DatabaseStorage interface {
	Ping(ctx context.Context) error
	Close() error
}

// Storage интерфейс для работы с метриками
type Storage interface {
	DatabaseStorage
	SetGauge(name string, value float64)
	IncrementCounter(name string, delta int64)
	GetMetric(name string, metricType models.MetricType) (interface{}, bool)
	GetAllMetrics() (map[string]float64, map[string]int64)
	GetMetricForJSON(name string, metricType models.MetricType) models.Metrics
	SaveToFile(filename string) error
	LoadFromFile(filename string) error
	UpdateMetricsBatch(metrics []models.Metrics) error
}

// PostgresStorage реализация Storage для PostgreSQL
type PostgresStorage struct {
	db         *sql.DB
	classifier retry.ErrorClassifier // ИЗМЕНЕНО: используем интерфейс вместо конкретного типа
}

// NewPostgresStorage создает новое подключение к PostgreSQL
func NewPostgresStorage(connectionString string) (Storage, error) {
	db, err := sql.Open("pgx", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Настраиваем пул соединений
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Проверяем подключение
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Применяем миграции
	if err := ApplyMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	return &PostgresStorage{
		db:         db,
		classifier: pgerrors.NewPostgresErrorClassifier(), // PostgresErrorClassifier реализует интерфейс ErrorClassifier
	}, nil
}

// SetGauge устанавливает значение для gauge-метрики с повторными попытками
func (s *PostgresStorage) SetGauge(name string, value float64) {
	ctx := context.Background()

	operation := func() error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		query := `
			INSERT INTO gauge_metrics (name, value, updated_at) 
			VALUES ($1, $2, $3)
			ON CONFLICT (name) 
			DO UPDATE SET value = $2, updated_at = $3
		`
		_, err := s.db.ExecContext(ctx, query, name, value, time.Now())
		if err != nil {
			logger.LogDatabaseError("set_gauge", err, query, name, value)
		}
		return err
	}

	err := retry.WithRetry(ctx, retry.DefaultRetryConfig, operation, s.classifier)
	if err != nil {
		logger.LogErrorWithContext(err, "Failed to set gauge metric after all retry attempts",
			zap.String("metric_name", name),
			zap.Float64("metric_value", value),
		)
	} else {
		logger.LogMetricUpdate("gauge", name, value, nil)
	}
}

// IncrementCounter увеличивает значение counter-метрики с повторными попытками
func (s *PostgresStorage) IncrementCounter(name string, delta int64) {
	ctx := context.Background()

	operation := func() error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		query := `
			INSERT INTO counter_metrics (name, value, updated_at) 
			VALUES ($1, $2, $3)
			ON CONFLICT (name) 
			DO UPDATE SET value = counter_metrics.value + $2, updated_at = $3
		`
		_, err := s.db.ExecContext(ctx, query, name, delta, time.Now())
		if err != nil {
			logger.LogDatabaseError("increment_counter", err, query, name, delta)
		}
		return err
	}

	err := retry.WithRetry(ctx, retry.DefaultRetryConfig, operation, s.classifier)
	if err != nil {
		logger.LogErrorWithContext(err, "Failed to increment counter metric after all retry attempts",
			zap.String("metric_name", name),
			zap.Int64("metric_delta", delta),
		)
	} else {
		logger.LogMetricUpdate("counter", name, delta, nil)
	}
}

// GetMetric возвращает метрику по имени и типу
func (s *PostgresStorage) GetMetric(name string, metricType models.MetricType) (interface{}, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch metricType {
	case models.Gauge:
		var value float64
		query := "SELECT value FROM gauge_metrics WHERE name = $1"
		err := s.db.QueryRowContext(ctx, query, name).Scan(&value)
		if err != nil {
			return nil, false
		}
		return value, true

	case models.Counter:
		var value int64
		query := "SELECT value FROM counter_metrics WHERE name = $1"
		err := s.db.QueryRowContext(ctx, query, name).Scan(&value)
		if err != nil {
			return nil, false
		}
		return value, true

	default:
		return nil, false
	}
}

// GetAllMetrics возвращает все метрики
func (s *PostgresStorage) GetAllMetrics() (map[string]float64, map[string]int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gauges := make(map[string]float64)
	counters := make(map[string]int64)

	// Получаем gauge метрики
	rows, err := s.db.QueryContext(ctx, "SELECT name, value FROM gauge_metrics")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			var value float64
			if err := rows.Scan(&name, &value); err == nil {
				gauges[name] = value
			}
		}
		// Проверяем ошибки после итерации
		if err := rows.Err(); err != nil {
			fmt.Printf("Error reading gauge metrics: %v\n", err)
		}
	}

	// Получаем counter метрики
	rows, err = s.db.QueryContext(ctx, "SELECT name, value FROM counter_metrics")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			var value int64
			if err := rows.Scan(&name, &value); err == nil {
				counters[name] = value
			}
		}
		// Проверяем ошибки после итерации
		if err := rows.Err(); err != nil {
			fmt.Printf("Error reading counter metrics: %v\n", err)
		}
	}

	return gauges, counters
}

// GetMetricForJSON возвращает метрику в формате для JSON
func (s *PostgresStorage) GetMetricForJSON(name string, metricType models.MetricType) models.Metrics {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch metricType {
	case models.Gauge:
		var value float64
		query := "SELECT value FROM gauge_metrics WHERE name = $1"
		err := s.db.QueryRowContext(ctx, query, name).Scan(&value)
		if err == nil {
			return models.Metrics{
				ID:    name,
				MType: "gauge",
				Value: &value,
			}
		}

	case models.Counter:
		var value int64
		query := "SELECT value FROM counter_metrics WHERE name = $1"
		err := s.db.QueryRowContext(ctx, query, name).Scan(&value)
		if err == nil {
			return models.Metrics{
				ID:    name,
				MType: "counter",
				Delta: &value,
			}
		}
	}

	return models.Metrics{}
}

// SaveToFile - для совместимости с интерфейсом
func (s *PostgresStorage) SaveToFile(filename string) error {
	return nil
}

// LoadFromFile - для совместимости с интерфейсом
func (s *PostgresStorage) LoadFromFile(filename string) error {
	return nil
}

// Close закрывает соединение с БД
func (s *PostgresStorage) Close() error {
	return s.db.Close()
}

// Ping проверяет соединение с БД
func (s *PostgresStorage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// UpdateMetricsBatch обновляет метрики батчем в транзакции с повторными попытками
func (s *PostgresStorage) UpdateMetricsBatch(metrics []models.Metrics) error {
	ctx := context.Background()

	operation := func() error {
		return s.updateMetricsBatchTx(ctx, metrics)
	}

	return retry.WithRetry(ctx, retry.DefaultRetryConfig, operation, s.classifier)
}

// updateMetricsBatchTx внутренняя функция для обновления метрик в транзакции
func (s *PostgresStorage) updateMetricsBatchTx(ctx context.Context, metrics []models.Metrics) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Начинаем транзакцию
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Подготавливаем запросы для gauge и counter
	gaugeStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO gauge_metrics (name, value, updated_at) 
		VALUES ($1, $2, $3)
		ON CONFLICT (name) 
		DO UPDATE SET value = $2, updated_at = $3
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare gauge statement: %w", err)
	}
	defer gaugeStmt.Close()

	counterStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO counter_metrics (name, value, updated_at) 
		VALUES ($1, $2, $3)
		ON CONFLICT (name) 
		DO UPDATE SET value = counter_metrics.value + $2, updated_at = $3
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare counter statement: %w", err)
	}
	defer counterStmt.Close()

	// Обрабатываем каждую метрику
	for _, metric := range metrics {
		switch metric.MType {
		case "gauge":
			if metric.Value == nil {
				continue
			}
			_, err := gaugeStmt.ExecContext(ctx, metric.ID, *metric.Value, time.Now())
			if err != nil {
				return fmt.Errorf("failed to update gauge metric %s: %w", metric.ID, err)
			}

		case "counter":
			if metric.Delta == nil {
				continue
			}
			_, err := counterStmt.ExecContext(ctx, metric.ID, *metric.Delta, time.Now())
			if err != nil {
				return fmt.Errorf("failed to update counter metric %s: %w", metric.ID, err)
			}
		}
	}

	// Коммитим транзакцию
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
