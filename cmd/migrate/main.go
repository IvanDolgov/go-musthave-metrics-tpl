package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/storage/postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	var (
		dsn           string
		rollback      bool
		targetVersion int64
	)

	flag.StringVar(&dsn, "d", "", "Database DSN")
	flag.BoolVar(&rollback, "rollback", false, "Rollback migrations")
	flag.Int64Var(&targetVersion, "target", 0, "Target version for rollback")
	flag.Parse()

	if dsn == "" {
		dsn = os.Getenv("DATABASE_DSN")
	}

	if dsn == "" {
		log.Fatal("Database DSN is required. Use -d flag or DATABASE_DSN environment variable")
	}

	// Подключаемся к БД
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Проверяем подключение
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	if rollback {
		// Откатываем миграции
		if targetVersion == 0 {
			log.Fatal("Target version is required for rollback. Use -target flag")
		}

		if err := postgres.RollbackMigrations(db, targetVersion); err != nil {
			log.Fatalf("Failed to rollback migrations: %v", err)
		}
		fmt.Printf("Migrations rolled back to version %d\n", targetVersion)
	} else {
		// Применяем миграции
		if err := postgres.ApplyMigrations(db); err != nil {
			log.Fatalf("Failed to apply migrations: %v", err)
		}
		fmt.Println("Migrations applied successfully")
	}
}
