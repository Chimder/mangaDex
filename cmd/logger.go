package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

const (
	Reset   = "\033[0m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"
	Gray    = "\033[90m"
)

type MinimalHandler struct {
	opts *slog.HandlerOptions
}

func (h *MinimalHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.opts.Level.Level()
}

func (h *MinimalHandler) Handle(_ context.Context, r slog.Record) error {
	timestamp := time.Now().Format("15:04:05")
	level := r.Level.String()
	message := r.Message

	var levelColor string
	switch r.Level {
	case slog.LevelDebug:
		levelColor = Cyan
	case slog.LevelInfo:
		levelColor = Green
	case slog.LevelWarn:
		levelColor = Yellow
	case slog.LevelError:
		levelColor = Red
	default:
		levelColor = White
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("%s %s%-5s%s %s%s%s",
		timestamp, levelColor, level, Reset, Gray, message, Reset))

	r.Attrs(func(a slog.Attr) bool {
		builder.WriteString(fmt.Sprintf(" %s%s%s=%s%v%s",
			Blue, a.Key, Reset, Magenta, a.Value, Reset))
		return true
	})

	builder.WriteString("\n")
	_, err := os.Stdout.WriteString(builder.String())
	return err
}

func (h *MinimalHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *MinimalHandler) WithGroup(name string) slog.Handler {
	return h
}

func LoggerInit() {
	handler := &MinimalHandler{
		opts: &slog.HandlerOptions{
			Level: slog.LevelDebug,
		},
	}
	slog.SetDefault(slog.New(handler))
}
