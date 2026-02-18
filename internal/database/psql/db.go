// Package db contains database related functions
package db

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func initDBTable(ctx context.Context, db *pgxpool.Pool) error {
	slog.Info("Initializing database")
	query := `
CREATE TABLE IF NOT EXISTS users (
    id BIGINT PRIMARY KEY,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    total_links INTEGER NOT NULL DEFAULT 0,
    credit INTEGER NOT NULL DEFAULT 0,
    last_credit_update TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    is_banned BOOLEAN NOT NULL DEFAULT false,
    is_deleted BOOLEAN NOT NULL DEFAULT false,
    is_verified BOOLEAN NOT NULL DEFAULT false,
    is_premium BOOLEAN NOT NULL DEFAULT false
);
	`
	_, err := db.Exec(ctx, query)
	return err
}

func CreateConn(ctx context.Context, connectionString string, initDB bool) (*pgxpool.Pool, error) {
	conn, err := pgxpool.New(ctx, connectionString)
	if err != nil {
		return nil, err
	}
	if initDB {
		if err := initDBTable(ctx, conn); err != nil {
			return nil, err
		}
	}
	return conn, nil
}

func GetRedisClient(ctx context.Context, addr string, password string) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})
	err := client.Ping(ctx).Err()
	if err != nil {
		return nil, err
	}
	return client, nil
}
