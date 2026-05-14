// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cli // import "miniflux.app/v2/internal/cli"

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"miniflux.app/v2/internal/config"
	"miniflux.app/v2/internal/embedding"
	"miniflux.app/v2/internal/http/server"
	mcpserver "miniflux.app/v2/internal/mcp"
	"miniflux.app/v2/internal/metric"
	"miniflux.app/v2/internal/storage"
	"miniflux.app/v2/internal/systemd"
	"miniflux.app/v2/internal/worker"
)

func startDaemon(store *storage.Storage) {
	slog.Debug("Starting daemon...")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	signal.Notify(stop, syscall.SIGTERM)

	pool := worker.NewPool(store, config.Opts.WorkerPoolSize())

	if config.Opts.HasSchedulerService() && !config.Opts.HasMaintenanceMode() {
		runScheduler(store, pool)
	}

	var httpServers []*http.Server
	if config.Opts.HasHTTPService() {
		httpServers = server.StartWebServer(store, pool)
	}

	var embeddingClient *embedding.Client
	embeddingCtx, cancelEmbedding := context.WithCancel(context.Background())
	if config.Opts.EmbeddingEnabled() {
		embeddingClient = embedding.NewClient(
			config.Opts.EmbeddingAPIURL(),
			config.Opts.EmbeddingAPIKey(),
			config.Opts.EmbeddingModel(),
			config.Opts.EmbeddingDimensions(),
		)
		embeddingWorker := embedding.NewWorker(
			embeddingClient,
			store,
			config.Opts.EmbeddingBatchSize(),
			config.Opts.EmbeddingMaxTextBytes(),
			config.Opts.EmbeddingDimensions(),
			config.Opts.EmbeddingInterval(),
		)
		go embeddingWorker.Run(embeddingCtx)
	}

	var mcpHTTPServer *http.Server
	if config.Opts.MCPEnabled() {
		mcpHTTPServer = mcpserver.StartHTTPServer(store, embeddingClient, config.Opts.MCPHTTPAddr())
	}

	metricsCtx, cancelMetrics := context.WithCancel(context.Background())
	if config.Opts.HasMetricsCollector() {
		collector := metric.NewCollector(store, config.Opts.MetricsRefreshInterval())
		go collector.GatherStorageMetrics(metricsCtx)
	}

	if systemd.HasNotifySocket() {
		slog.Debug("Sending readiness notification to Systemd")

		if err := systemd.SdNotify(systemd.SdNotifyReady); err != nil {
			slog.Error("Unable to send readiness notification to systemd", slog.Any("error", err))
		}

		if config.Opts.HasWatchdog() && systemd.HasSystemdWatchdog() {
			slog.Debug("Activating Systemd watchdog")

			go func() {
				interval, err := systemd.WatchdogInterval()
				if err != nil {
					slog.Error("Unable to get watchdog interval from systemd", slog.Any("error", err))
					return
				}

				for {
					if err := store.Ping(); err != nil {
						slog.Error("Unable to ping database", slog.Any("error", err))
					} else {
						systemd.SdNotify(systemd.SdNotifyWatchdog)
					}

					time.Sleep(interval / 3)
				}
			}()
		}
	}

	<-stop
	slog.Debug("Shutting down the process")
	cancelEmbedding()
	cancelMetrics()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if mcpHTTPServer != nil {
		slog.Debug("Shutting down MCP HTTP server...")
		if err := mcpHTTPServer.Shutdown(ctx); err != nil {
			slog.Error("MCP HTTP server shutdown error", slog.Any("error", err))
		}
	}

	if len(httpServers) > 0 {
		slog.Debug("Shutting down HTTP servers...")
		for _, server := range httpServers {
			if server != nil {
				if err := server.Shutdown(ctx); err != nil {
					slog.Error("HTTP server shutdown error", slog.Any("error", err), slog.String("addr", server.Addr))
				}
			}
		}
		slog.Debug("All HTTP servers shut down.")
	} else {
		slog.Debug("No HTTP servers to shut down.")
	}

	slog.Debug("Shutting down worker pool...")
	pool.Shutdown()
	slog.Debug("Worker pool shut down.")

	slog.Debug("Process gracefully stopped")
}
