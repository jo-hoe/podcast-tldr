package main

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/jo-hoe/llm-summarizer/internal/config"
	"github.com/jo-hoe/llm-summarizer/internal/llm"
	"github.com/jo-hoe/llm-summarizer/internal/prompt"
	"github.com/jo-hoe/llm-summarizer/internal/service"
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

	template, err := prompt.Load(cfg.PromptPath)
	if err != nil {
		slog.Error("failed to load prompt template", "path", cfg.PromptPath, "err", err)
		os.Exit(1)
	}

	client := llm.NewOpenAIClient(cfg.BaseURL, cfg.APIKey())
	svc := service.New(cfg, client, template)
	if err := svc.Run(context.Background()); err != nil {
		slog.Error("summarize stage failed", "err", err)
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
