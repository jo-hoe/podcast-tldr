package main

import (
	"log/slog"
	"os"
	"strings"

	"github.com/jo-hoe/git-archive-backup/internal/config"
	"github.com/jo-hoe/git-archive-backup/internal/gitrepo"
	"github.com/jo-hoe/git-archive-backup/internal/service"
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

	svc := service.New(cfg, openRepository, os.Getenv("EPISODE_ID"))
	if err := svc.Run(); err != nil {
		slog.Error("backup stage failed", "err", err)
		os.Exit(1)
	}
}

// openRepository adapts gitrepo.Open to the service.Opener signature.
func openRepository(opts gitrepo.Options) (gitrepo.Repository, error) {
	return gitrepo.Open(opts)
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
