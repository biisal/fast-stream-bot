package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/biisal/fast-stream-bot/config"
	"github.com/biisal/fast-stream-bot/internal/bot"
	db "github.com/biisal/fast-stream-bot/internal/database/psql"
	repo "github.com/biisal/fast-stream-bot/internal/database/psql/sqlc"
	"github.com/biisal/fast-stream-bot/internal/http-server/routers"
	"github.com/biisal/fast-stream-bot/internal/http-server/shortner"
	rd "github.com/biisal/fast-stream-bot/internal/redis"
	"github.com/biisal/fast-stream-bot/internal/service/user"
	"github.com/biisal/fast-stream-bot/logger"
)

func runServer(cfg config.Config, worker *bot.Worker, redisClient rd.RedisService) error {
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	s := shortner.NewShortner(
		time.Duration(cfg.JWT_EXPIRATION)*time.Second,
		time.Duration(cfg.UUID_EXPIRATION)*time.Second,
		cfg.JWT_SECRET, redisClient, cfg.SHORTNER_URL, cfg.SHORTNER_API, cfg)

	mux := routers.SetUpRouters(worker, cfg, s)
	server := http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.HTTP_PORT),
		Handler: mux,
	}
	go func() {
		slog.Info("Runing HTTP Server", "port", cfg.HTTP_PORT)
		if err := server.ListenAndServe(); err != nil {
			slog.Error("HTTP server stopped", "error", err)
		}
	}()

	<-done
	ctx, cancle := context.WithTimeout(context.Background(), time.Second*5)
	defer cancle()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("HTTP server stopped", "error", err)
		return err
	}

	return nil
}

func mount(cfg config.Config, flags AppFlags) error {
	ctx := context.Background()
	file, err := logger.SetupSlog(cfg.ENVIRONMENT, logger.StdoutHandler)
	if err != nil {
		log.Fatal("Error setting up logger", "error", err.Error())
	}

	defer func() {
		if file != nil {
			if err = file.Close(); err != nil {
				slog.Error("Error closing file", "error", err)
			}
		}
	}()

	dbConn, err := db.CreateConn(ctx, cfg.DBSTRING, flags.InitDB)
	if err != nil {
		slog.Error("Error connecting to database", "error", err)
		return err
	}
	defer dbConn.Close()

	r := repo.New(dbConn)

	redisConn, rdNew, err := rd.New(ctx, cfg.REDIS_DBSTRING)
	if err != nil {
		slog.Error("Error connecting to redis", "error", err)
		return err
	}
	defer redisConn.Close()

	userService := user.NewService(r, rdNew, time.Minute*5)
	worker := bot.StartWorkers(&cfg, userService)
	if len(worker.Bots) <= 0 {
		errMsg := fmt.Errorf("no bots are running! returning")
		slog.Error("No bots are running", "error", errMsg)
		return errMsg
	}
	return runServer(cfg, worker, rdNew)
}
