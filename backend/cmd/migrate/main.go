package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gamegen/backend/internal/platform/config"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	command := flag.String("command", "status", "migration command: up, down-one, or status")
	path := flag.String("path", envOrDefault("MIGRATIONS_PATH", "db/migrations"), "migration directory")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if err := cfg.ValidateDatabase(); err != nil {
		log.Fatal(err)
	}

	absPath, err := filepath.Abs(*path)
	if err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("mysql", cfg.Database.DSN)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	driver, err := migratemysql.WithInstance(db, &migratemysql.Config{})
	if err != nil {
		log.Fatal(err)
	}

	migrations, err := migrate.NewWithDatabaseInstance("file://"+absPath, "mysql", driver)
	if err != nil {
		log.Fatal(err)
	}
	defer migrations.Close()

	switch *command {
	case "up":
		err = migrations.Up()
	case "down-one":
		err = migrations.Steps(-1)
	case "status":
		printVersion(migrations)
		return
	default:
		log.Fatalf("unknown migration command %q", *command)
	}

	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatal(err)
	}
	printVersion(migrations)
}

func printVersion(migrations *migrate.Migrate) {
	version, dirty, err := migrations.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		fmt.Println("migration version: none")
		return
	}
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("migration version: %d (dirty=%t)\n", version, dirty)
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
