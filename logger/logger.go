// Package logger provides a logger that writes to both stdout and a file.
package logger

import (
	"context"
	"io"
	"log/slog"
	"os"

	"github.com/biisal/fast-stream-bot/config"
)

type multiHandler struct {
	handlers []slog.Handler
}

func (m multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		_ = h.Handle(ctx, r)
	}
	return nil
}

func (m multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	copies := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		copies[i] = h.WithAttrs(attrs)
	}
	return multiHandler{handlers: copies}
}

func (m multiHandler) WithGroup(name string) slog.Handler {
	copies := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		copies[i] = h.WithGroup(name)
	}
	return multiHandler{handlers: copies}
}

func Setup(env string) (io.Closer, error) {
	stdoutLevel := slog.LevelInfo
	if env == config.ENVIRONMENT_LOCAL {
		stdoutLevel = slog.LevelDebug
	}

	stdoutOpts := &slog.HandlerOptions{Level: stdoutLevel}
	fileOpts := &slog.HandlerOptions{Level: slog.LevelError}

	file, err := os.OpenFile("fsb.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	fileHandler := slog.NewTextHandler(file, fileOpts)
	stdoutHandler := slog.NewTextHandler(os.Stdout, stdoutOpts)

	handler := multiHandler{handlers: []slog.Handler{fileHandler, stdoutHandler}}
	slog.SetDefault(slog.New(handler))

	return file, nil
}
