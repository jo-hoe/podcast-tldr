package main

import (
	"log/slog"
	"os"
	"strings"

	"github.com/jo-hoe/artifact-zipper/internal/archive"
	"github.com/jo-hoe/artifact-zipper/internal/config"
	"github.com/jo-hoe/artifact-zipper/internal/service"
)

func main() {
	initLogger(slog.LevelInfo)

	configPath, err := config.ResolvePath()
	if err != nil {
		slog.Error("failed to resolve config path", "err", err)
		os.Exit(1)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("failed to load config", "path", configPath, "err", err)
		os.Exit(1)
	}
	initLogger(parseLogLevel(cfg.LogLevel))

	svc := service.New(cfg, archive.NewZipWriter(), os.Getenv("EPISODE_ID"))
	if err := svc.Run(); err != nil {
		slog.Error("zip stage failed", "err", err)
		os.Exit(1)
	}
}

func initLogger(level slog.Level) {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
}

func parseLogLevel(lvl string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(lvl)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
