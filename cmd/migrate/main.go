package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/storage/postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	var (
		connectionString string
		rollbackVersion  int64
		rollback         bool
	)

	flag.StringVar(&connectionString, "d", "", "Database connection string")
	flag.BoolVar(&rollback, "rollback", false, "Rollback migrations")
	flag.Int64Var(&rollbackVersion, "version", 0, "Target version for rollback")
	flag.Parse()

	if connectionString == "" {
		fmt.Println("Database connection string is required")
		os.Exit(1)
	}

	// Создаем корневой контекст
	ctx := context.Background()

	db, err := sql.Open("pgx", connectionString)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Проверяем подключение
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	if rollback {
		if rollbackVersion == 0 {
			fmt.Println("Target version is required for rollback")
			os.Exit(1)
		}
		if err := postgres.RollbackMigrations(ctx, db, rollbackVersion); err != nil {
			log.Fatalf("Failed to rollback migrations: %v", err)
		}
		fmt.Printf("Successfully rolled back to version %d\n", rollbackVersion)
	} else {
		if err := postgres.ApplyMigrations(ctx, db); err != nil {
			log.Fatalf("Failed to apply migrations: %v", err)
		}
		fmt.Println("Migrations applied successfully")
	}
}
