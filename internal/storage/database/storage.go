package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	_ "github.com/jackc/pgx/v5/stdlib" // драйвер PostgreSQL
)

// DatabaseStorage интерфейс для работы с базой данных
//
//go:generate mockgen -destination=../mocks/mock_database_storage.go -package=mocks github.com/IvanDolgov/go-musthave-metrics-tpl/internal/storage/database DatabaseStorage
type DatabaseStorage interface {
	// Ping проверяет соединение с БД
	Ping(ctx context.Context) error
	// Close закрывает соединение с БД
	Close() error
}

// DBStorage - хранилище для работы с PostgreSQL
type DBStorage struct {
	db *sql.DB
}

// NewDBStorage создает новое подключение к PostgreSQL
func NewDBStorage(connectionString string) (*DBStorage, error) {
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

	return &DBStorage{db: db}, nil
}

// SetGauge устанавливает значение для gauge-метрики
func (s *DBStorage) SetGauge(name string, value float64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
        INSERT INTO gauge_metrics (name, value, updated_at) 
        VALUES ($1, $2, $3)
        ON CONFLICT (name) 
        DO UPDATE SET value = $2, updated_at = $3
    `
	_, err := s.db.ExecContext(ctx, query, name, value, time.Now())
	if err != nil {
		// Логируем ошибку, но не паникуем
		fmt.Printf("Failed to set gauge metric: %v\n", err)
	}
}

// IncrementCounter увеличивает значение counter-метрики
func (s *DBStorage) IncrementCounter(name string, delta int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
        INSERT INTO counter_metrics (name, value, updated_at) 
        VALUES ($1, $2, $3)
        ON CONFLICT (name) 
        DO UPDATE SET value = counter_metrics.value + $2, updated_at = $3
    `
	_, err := s.db.ExecContext(ctx, query, name, delta, time.Now())
	if err != nil {
		fmt.Printf("Failed to increment counter metric: %v\n", err)
	}
}

// GetMetric возвращает метрику по имени и типу
func (s *DBStorage) GetMetric(name string, metricType models.MetricType) (interface{}, bool) {
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
func (s *DBStorage) GetAllMetrics() (map[string]float64, map[string]int64) {
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
	}

	return gauges, counters
}

// GetMetricForJSON возвращает метрику в формате для JSON
func (s *DBStorage) GetMetricForJSON(name string, metricType models.MetricType) models.Metrics {
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

// SaveToFile - для совместимости с интерфейсом (не используется для PostgreSQL)
func (s *DBStorage) SaveToFile(filename string) error {
	// Для PostgreSQL сохранение в файл не требуется
	return nil
}

// LoadFromFile - для совместимости с интерфейсом (не используется для PostgreSQL)
func (s *DBStorage) LoadFromFile(filename string) error {
	// Для PostgreSQL загрузка из файла не требуется
	return nil
}

// Ping проверяет соединение с БД
func (s *DBStorage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close закрывает соединение с БД
func (s *DBStorage) Close() error {
	return s.db.Close()
}
