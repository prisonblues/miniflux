// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package embedding // import "miniflux.app/v2/internal/embedding"

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"miniflux.app/v2/internal/model"
)

// EntryStore is the subset of storage.Storage used by the embedding worker.
type EntryStore interface {
	GetEntriesWithoutEmbedding(limit int) ([]model.EntryForEmbedding, error)
	CountEntriesWithoutEmbedding() (int, error)
	UpdateEntryEmbedding(entryID int64, embedding model.Vector) error
	ConfigureEmbeddingColumn(dimensions int) error
}

// Worker periodically embeds entries that lack a vector.
type Worker struct {
	client       *Client
	store        EntryStore
	batchSize    int
	maxTextBytes int
	interval     time.Duration

	// configuredDimensions is the value from EMBEDDING_DIMENSIONS (0 = auto-detect).
	configuredDimensions int
	// activeDimensions is the confirmed vector size after the first batch.
	activeDimensions int

	// Cumulative stats for logging.
	totalEmbedded int64
	totalDuration time.Duration
}

// NewWorker creates a new embedding background worker.
func NewWorker(client *Client, store EntryStore, batchSize, maxTextBytes, dimensions int, interval time.Duration) *Worker {
	return &Worker{
		client:               client,
		store:                store,
		batchSize:            batchSize,
		maxTextBytes:         maxTextBytes,
		configuredDimensions: dimensions,
		interval:             interval,
	}
}

// Run starts the embedding loop. It blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	slog.Info("Starting embedding worker",
		slog.Duration("interval", w.interval),
		slog.Int("batch_size", w.batchSize),
		slog.Int("configured_dimensions", w.configuredDimensions),
	)

	// When dimensions are explicitly configured, set up the column before
	// processing any entries so the schema is ready.
	if w.configuredDimensions > 0 {
		if err := w.store.ConfigureEmbeddingColumn(w.configuredDimensions); err != nil {
			slog.Error("Embedding worker: failed to configure embedding column", slog.Any("error", err))
			return
		}
		w.activeDimensions = w.configuredDimensions
	}

	// Backfill: process continuously until caught up, then switch to interval.
	if err := w.backfill(ctx); err != nil {
		slog.Error("Embedding worker: fatal error during backfill", slog.Any("error", err))
		return
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Embedding worker stopped",
				slog.Int64("total_embedded", w.totalEmbedded),
				slog.Duration("total_api_time", w.totalDuration),
			)
			return
		case <-ticker.C:
			if _, err := w.processBatch(ctx); err != nil {
				slog.Error("Embedding worker: fatal error", slog.Any("error", err))
				return
			}
		}
	}
}

// backfill processes entries continuously (no interval delay) until there
// are no more un-embedded entries or the context is cancelled.
func (w *Worker) backfill(ctx context.Context) error {
	remaining, err := w.store.CountEntriesWithoutEmbedding()
	if err != nil {
		slog.Error("Embedding worker: failed to count pending entries", slog.Any("error", err))
		return nil // non-fatal: we'll pick them up on the next tick
	}

	if remaining == 0 {
		slog.Info("Embedding worker: no backfill needed")
		return nil
	}

	slog.Info("Embedding worker: starting backfill",
		slog.Int("entries_remaining", remaining),
	)

	for remaining > 0 {
		if ctx.Err() != nil {
			return nil
		}

		n, err := w.processBatch(ctx)
		if err != nil {
			return err
		}
		if n == 0 {
			break
		}
		remaining -= n

		slog.Info("Embedding worker: backfill progress",
			slog.Int("entries_remaining", max(remaining, 0)),
			slog.Int64("total_embedded", w.totalEmbedded),
			slog.Duration("total_api_time", w.totalDuration),
		)
	}

	slog.Info("Embedding worker: backfill complete",
		slog.Int64("total_embedded", w.totalEmbedded),
		slog.Duration("total_api_time", w.totalDuration),
	)

	return nil
}

// processBatch fetches and embeds one batch. Returns the number of entries
// successfully embedded. A non-nil error indicates a fatal condition that
// should stop the worker.
func (w *Worker) processBatch(ctx context.Context) (int, error) {
	entries, err := w.store.GetEntriesWithoutEmbedding(w.batchSize)
	if err != nil {
		slog.Error("Embedding worker: failed to fetch entries", slog.Any("error", err))
		return 0, nil
	}

	if len(entries) == 0 {
		return 0, nil
	}

	texts := make([]string, len(entries))
	for i, e := range entries {
		texts[i] = PrepareText(e.Title, e.Content, w.maxTextBytes)
	}

	start := time.Now()
	vectors, err := w.client.Embed(ctx, texts)
	apiDuration := time.Since(start)

	if err != nil {
		slog.Error("Embedding worker: API call failed",
			slog.Any("error", err),
			slog.Duration("duration", apiDuration),
			slog.Int("batch_size", len(texts)),
		)
		return 0, nil
	}

	// On the first successful batch, validate or detect vector dimensions.
	if w.activeDimensions == 0 && len(vectors) > 0 {
		detected := len(vectors[0])
		if detected == 0 {
			return 0, fmt.Errorf("API returned empty vector")
		}

		slog.Info("Embedding worker: auto-detected dimensions",
			slog.Int("dimensions", detected),
		)

		if err := w.store.ConfigureEmbeddingColumn(detected); err != nil {
			return 0, fmt.Errorf("configure embedding column: %w", err)
		}
		w.activeDimensions = detected
	} else if w.activeDimensions > 0 && len(vectors) > 0 && len(vectors[0]) != w.activeDimensions {
		return 0, fmt.Errorf("dimension mismatch: expected %d, got %d", w.activeDimensions, len(vectors[0]))
	}

	stored := 0
	for i, entry := range entries {
		if err := w.store.UpdateEntryEmbedding(entry.ID, vectors[i]); err != nil {
			slog.Error("Embedding worker: failed to store embedding",
				slog.Int64("entry_id", entry.ID),
				slog.Any("error", err),
			)
			continue
		}
		stored++
	}

	w.totalEmbedded += int64(stored)
	w.totalDuration += apiDuration

	slog.Debug("Embedding worker: batch complete",
		slog.Int("embedded", stored),
		slog.Duration("api_duration", apiDuration),
		slog.Int64("session_total", w.totalEmbedded),
	)

	return stored, nil
}
