package main

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/jo-hoe/rss-audio-downloader/internal/config"
	"github.com/jo-hoe/rss-audio-downloader/internal/download"
	"github.com/jo-hoe/rss-audio-downloader/internal/feed"
	"github.com/jo-hoe/rss-audio-downloader/internal/service"
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

	svc := service.New(cfg, feed.NewGofeedParser(), download.NewHTTPDownloader())
	if err := svc.Run(context.Background()); err != nil {
		slog.Error("download stage failed", "err", err)
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
