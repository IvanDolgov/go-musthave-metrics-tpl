package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

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

// Ping проверяет соединение с БД
func (s *DBStorage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close закрывает соединение с БД
func (s *DBStorage) Close() error {
	return s.db.Close()
}
