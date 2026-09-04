package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gamegen/backend/internal/platform/config"

	_ "github.com/go-sql-driver/mysql"
)

type DB struct {
	*sql.DB
}

func Open(cfg config.DatabaseConfig) (*DB, error) {
	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	if cfg.ConnMaxLifetime != "" {
		lifetime, err := time.ParseDuration(cfg.ConnMaxLifetime)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("parse database connection lifetime: %w", err)
		}
		db.SetConnMaxLifetime(lifetime)
	}
	return &DB{DB: db}, nil
}

func (db *DB) Check(ctx context.Context) error {
	return db.PingContext(ctx)
}
