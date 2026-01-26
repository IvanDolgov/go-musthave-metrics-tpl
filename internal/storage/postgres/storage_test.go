package postgres

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPostgresStorage_NewPostgresStorage тестирует создание хранилища
func TestPostgresStorage_NewPostgresStorage(t *testing.T) {
	t.Run("Successful creation", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Мокаем sql.Open
		originalSQLOpen := sqlOpen
		defer func() { sqlOpen = originalSQLOpen }()
		sqlOpen = func(driverName, dataSourceName string) (*sql.DB, error) {
			return db, nil
		}

		// Ожидаем вызовы
		mock.ExpectPing()
		mock.ExpectBegin()
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS gauge_metrics").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS counter_metrics").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		ctx := context.Background()
		storage, err := NewPostgresStorage(ctx, "test-dsn")

		require.NoError(t, err)
		require.NotNil(t, storage)
		defer storage.Close()

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Failed to open database", func(t *testing.T) {
		originalSQLOpen := sqlOpen
		defer func() { sqlOpen = originalSQLOpen }()
		sqlOpen = func(driverName, dataSourceName string) (*sql.DB, error) {
			return nil, assert.AnError
		}

		ctx := context.Background()
		storage, err := NewPostgresStorage(ctx, "test-dsn")

		assert.Error(t, err)
		assert.Nil(t, storage)
		assert.Contains(t, err.Error(), "failed to open database")
	})
}

// TestPostgresStorage_SetGauge тестирует установку gauge
func TestPostgresStorage_SetGauge(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	storage := &PostgresStorage{db: db}
	ctx := context.Background()

	t.Run("Successful set", func(t *testing.T) {
		mock.ExpectExec(`INSERT INTO gauge_metrics`).
			WithArgs("test_gauge", 42.5, sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		storage.SetGauge(ctx, "test_gauge", 42.5)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("With cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		cancel()

		// Не должно быть запросов при отмененном контексте
		storage.SetGauge(ctx, "test", 1.0)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestPostgresStorage_IncrementCounter тестирует инкремент counter
func TestPostgresStorage_IncrementCounter(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	storage := &PostgresStorage{db: db}
	ctx := context.Background()

	t.Run("Successful increment", func(t *testing.T) {
		mock.ExpectExec(`INSERT INTO counter_metrics`).
			WithArgs("test_counter", int64(10), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		storage.IncrementCounter(ctx, "test_counter", 10)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestPostgresStorage_GetMetric тестирует получение метрик
func TestPostgresStorage_GetMetric(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	storage := &PostgresStorage{db: db}
	ctx := context.Background()

	t.Run("Get existing gauge", func(t *testing.T) {
		mock.ExpectQuery(`SELECT value FROM gauge_metrics WHERE name =`).
			WithArgs("cpu_usage").
			WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(75.5))

		value, exists := storage.GetMetric(ctx, "cpu_usage", models.Gauge)

		assert.True(t, exists)
		assert.Equal(t, 75.5, value)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Get non-existent gauge", func(t *testing.T) {
		mock.ExpectQuery(`SELECT value FROM gauge_metrics WHERE name =`).
			WithArgs("non_existent").
			WillReturnError(sql.ErrNoRows)

		value, exists := storage.GetMetric(ctx, "non_existent", models.Gauge)

		assert.False(t, exists)
		assert.Nil(t, value)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Get existing counter", func(t *testing.T) {
		mock.ExpectQuery(`SELECT value FROM counter_metrics WHERE name =`).
			WithArgs("requests").
			WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(int64(100)))

		value, exists := storage.GetMetric(ctx, "requests", models.Counter)

		assert.True(t, exists)
		assert.Equal(t, int64(100), value)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Get with unknown type", func(t *testing.T) {
		value, exists := storage.GetMetric(ctx, "test", models.MetricType("unknown"))

		assert.False(t, exists)
		assert.Nil(t, value)
	})
}

// TestPostgresStorage_GetAllMetrics тестирует получение всех метрик
func TestPostgresStorage_GetAllMetrics(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	storage := &PostgresStorage{db: db}
	ctx := context.Background()

	t.Run("Get all metrics", func(t *testing.T) {
		// Gauge метрики
		mock.ExpectQuery(`SELECT name, value FROM gauge_metrics`).
			WillReturnRows(sqlmock.NewRows([]string{"name", "value"}).
				AddRow("gauge1", 1.1).
				AddRow("gauge2", 2.2))

		// Counter метрики
		mock.ExpectQuery(`SELECT name, value FROM counter_metrics`).
			WillReturnRows(sqlmock.NewRows([]string{"name", "value"}).
				AddRow("counter1", int64(10)).
				AddRow("counter2", int64(20)))

		gauges, counters := storage.GetAllMetrics(ctx)

		assert.Len(t, gauges, 2)
		assert.Len(t, counters, 2)
		assert.Equal(t, 1.1, gauges["gauge1"])
		assert.Equal(t, int64(10), counters["counter1"])
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Empty result", func(t *testing.T) {
		mock.ExpectQuery(`SELECT name, value FROM gauge_metrics`).
			WillReturnRows(sqlmock.NewRows([]string{"name", "value"}))
		mock.ExpectQuery(`SELECT name, value FROM counter_metrics`).
			WillReturnRows(sqlmock.NewRows([]string{"name", "value"}))

		gauges, counters := storage.GetAllMetrics(ctx)

		assert.Empty(t, gauges)
		assert.Empty(t, counters)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestPostgresStorage_GetMetricForJSON тестирует получение метрик для JSON
func TestPostgresStorage_GetMetricForJSON(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	storage := &PostgresStorage{db: db}
	ctx := context.Background()

	t.Run("Get gauge for JSON", func(t *testing.T) {
		mock.ExpectQuery(`SELECT value FROM gauge_metrics WHERE name =`).
			WithArgs("test_gauge").
			WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(99.9))

		metric := storage.GetMetricForJSON(ctx, "test_gauge", models.Gauge)

		assert.Equal(t, "test_gauge", metric.ID)
		assert.Equal(t, "gauge", metric.MType)
		require.NotNil(t, metric.Value)
		assert.Equal(t, 99.9, *metric.Value)
		assert.Nil(t, metric.Delta)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Get non-existent metric for JSON", func(t *testing.T) {
		mock.ExpectQuery(`SELECT value FROM gauge_metrics WHERE name =`).
			WithArgs("non_existent").
			WillReturnError(sql.ErrNoRows)

		metric := storage.GetMetricForJSON(ctx, "non_existent", models.Gauge)

		assert.Empty(t, metric.ID)
		assert.Empty(t, metric.MType)
		assert.Nil(t, metric.Value)
		assert.Nil(t, metric.Delta)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestPostgresStorage_SaveToFile тестирует сохранение в файл
func TestPostgresStorage_SaveToFile(t *testing.T) {
	storage := &PostgresStorage{}
	ctx := context.Background()

	err := storage.SaveToFile(ctx, "test.json")
	assert.NoError(t, err, "SaveToFile should always return nil for PostgresStorage")
}

// TestPostgresStorage_LoadFromFile тестирует загрузку из файла
func TestPostgresStorage_LoadFromFile(t *testing.T) {
	storage := &PostgresStorage{}
	ctx := context.Background()

	err := storage.LoadFromFile(ctx, "test.json")
	assert.NoError(t, err, "LoadFromFile should always return nil for PostgresStorage")
}

// TestPostgresStorage_Ping тестирует проверку соединения
func TestPostgresStorage_Ping(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	storage := &PostgresStorage{db: db}
	ctx := context.Background()

	t.Run("Successful ping", func(t *testing.T) {
		mock.ExpectPing()

		err := storage.Ping(ctx)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Failed ping", func(t *testing.T) {
		mock.ExpectPing().WillReturnError(assert.AnError)

		err := storage.Ping(ctx)
		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestPostgresStorage_Close тестирует закрытие соединения
func TestPostgresStorage_Close(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	storage := &PostgresStorage{db: db}

	t.Run("Successful close", func(t *testing.T) {
		mock.ExpectClose()

		err := storage.Close()
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestPostgresStorage_UpdateMetricsBatch тестирует батчевое обновление
func TestPostgresStorage_UpdateMetricsBatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	storage := &PostgresStorage{db: db}
	ctx := context.Background()

	t.Run("Successful batch update", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectPrepare("INSERT INTO gauge_metrics")
		mock.ExpectPrepare("INSERT INTO counter_metrics")

		mock.ExpectExec("INSERT INTO gauge_metrics").
			WithArgs("gauge1", 1.0, sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectExec("INSERT INTO counter_metrics").
			WithArgs("counter1", int64(10), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectCommit()

		val := 1.0
		delta := int64(10)
		metrics := []models.Metrics{
			{ID: "gauge1", MType: "gauge", Value: &val},
			{ID: "counter1", MType: "counter", Delta: &delta},
		}

		err := storage.UpdateMetricsBatch(ctx, metrics)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Empty batch", func(t *testing.T) {
		err := storage.UpdateMetricsBatch(ctx, []models.Metrics{})
		assert.NoError(t, err)
	})
}

// TestPostgresStorage_InterfaceCompliance проверяет соответствие интерфейсу
func TestPostgresStorage_InterfaceCompliance(t *testing.T) {
	var storage Storage = &PostgresStorage{}
	var dbStorage DatabaseStorage = &PostgresStorage{}

	_ = storage
	_ = dbStorage

	t.Log("PostgresStorage correctly implements all interfaces")
}

// Mock для sql.Open
var sqlOpen = sql.Open
