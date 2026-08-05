package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"writers.studio/internal/config"
	"writers.studio/internal/httpapi"
	"writers.studio/internal/objectstore"
	"writers.studio/internal/store"
	"writers.studio/internal/stt"
	"writers.studio/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dataStore, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer dataStore.Close()
	if err := dataStore.Migrate(ctx); err != nil {
		logger.Error("migrate database", "error", err)
		os.Exit(1)
	}
	recoveredMCPDrafts, err := dataStore.RecoverRemoteMCPDrafts(ctx)
	if err != nil {
		logger.Error("recover remote MCP drafts", "error", err)
		os.Exit(1)
	}
	if recoveredMCPDrafts > 0 {
		logger.Warn("recovered remote MCP drafts", "count", recoveredMCPDrafts)
	}
	interruptedRuns, err := dataStore.RecoverInterruptedAssistantRuns(ctx)
	if err != nil {
		logger.Error("recover interrupted assistant runs", "error", err)
		os.Exit(1)
	}
	if interruptedRuns > 0 {
		logger.Warn("recovered interrupted assistant runs", "count", interruptedRuns)
	}

	objects, err := objectstore.NewMinIO(
		ctx,
		cfg.ObjectStorageEndpoint,
		cfg.ObjectStorageAccessKey,
		cfg.ObjectStorageSecretKey,
		cfg.ObjectStorageBucket,
		cfg.ObjectStorageUseSSL,
	)
	if err != nil {
		logger.Error("initialize object storage", "error", err)
		os.Exit(1)
	}

	var transcriber stt.FinalTranscriber
	switch cfg.STTAdapter {
	case "openrouter":
		transcriber = stt.NewOpenRouter(cfg.OpenRouterAPIKey, cfg.OpenRouterSTTModel)
	case "fake":
		transcriber = stt.NewFake()
	case "disabled":
		transcriber = stt.NewUnavailable()
	default:
		logger.Error("unsupported resolved STT adapter", "adapter", cfg.STTAdapter)
		os.Exit(1)
	}
	var liveTranscriber stt.LiveTranscriber
	if cfg.LiveSTTAdapter == "deepgram" {
		liveTranscriber = stt.NewDeepgram(cfg.DeepgramAPIKey, cfg.DeepgramSTTModel)
	}

	backgroundWorker := worker.New(
		dataStore,
		objects,
		transcriber,
		cfg.LiveSTTAdapter,
		cfg.DeepgramSTTModel,
		logger,
	)
	go backgroundWorker.Run(ctx)

	api := httpapi.New(cfg, dataStore, objects, transcriber, liveTranscriber, logger)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	go func() {
		logger.Info("server started", "address", cfg.HTTPAddr, "environment", cfg.AppEnv)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped unexpectedly", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		logger.Error("graceful shutdown", "error", err)
	}
}
