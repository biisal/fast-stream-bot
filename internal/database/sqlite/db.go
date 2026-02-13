package db

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/redis/go-redis/v9"
	_ "modernc.org/sqlite"
)

func CreateUserTable(ctx context.Context, db *sql.DB) error {
	query := `
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    total_links INTEGER NOT NULL DEFAULT 0,
    credit INTEGER NOT NULL DEFAULT 0,
    last_credit_update DATETIME DEFAULT CURRENT_TIMESTAMP,
    is_banned BOOLEAN NOT NULL DEFAULT 0,
    is_deleted BOOLEAN NOT NULL DEFAULT 0,
    is_verified BOOLEAN NOT NULL DEFAULT 0,
    is_premium BOOLEAN NOT NULL DEFAULT 0
);`
	_, err := db.ExecContext(ctx, query)
	return err
}

func ConnectToDb(ctx context.Context, connectionString string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", connectionString)
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

func CreateConn(ctx context.Context, connectionString string) (*sql.DB, error) {
	conn, err := ConnectToDb(ctx, connectionString)
	if err != nil {
		return nil, err
	}
	return conn, CreateUserTable(ctx, conn)
}

func GetRedisClient(ctx context.Context, addr string, password string) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})
	err := client.Ping(ctx).Err()
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}
	return client, nil
}
