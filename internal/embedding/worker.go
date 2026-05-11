// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package embedding // import "miniflux.app/v2/internal/embedding"

import (
	"context"
	"log/slog"
	"time"

	"miniflux.app/v2/internal/model"
)

// EntryStore is the subset of storage.Storage used by the embedding worker.
type EntryStore interface {
	GetEntriesWithoutEmbedding(limit int) ([]model.EntryForEmbedding, error)
	CountEntriesWithoutEmbedding() (int, error)
	UpdateEntryEmbedding(entryID int64, embedding model.Vector) error
}

// Worker periodically embeds entries that lack a vector.
type Worker struct {
	client    *Client
	store     EntryStore
	batchSize int
	interval  time.Duration

	// Cumulative stats for logging.
	totalEmbedded int64
	totalDuration time.Duration
}

// NewWorker creates a new embedding background worker.
func NewWorker(client *Client, store EntryStore, batchSize int, interval time.Duration) *Worker {
	return &Worker{
		client:    client,
		store:     store,
		batchSize: batchSize,
		interval:  interval,
	}
}

// Run starts the embedding loop. It blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	slog.Info("Starting embedding worker",
		slog.Duration("interval", w.interval),
		slog.Int("batch_size", w.batchSize),
	)

	// Backfill: process continuously until caught up, then switch to interval.
	w.backfill(ctx)

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
			w.processBatch(ctx)
		}
	}
}

// backfill processes entries continuously (no interval delay) until there
// are no more un-embedded entries or the context is cancelled.
func (w *Worker) backfill(ctx context.Context) {
	remaining, err := w.store.CountEntriesWithoutEmbedding()
	if err != nil {
		slog.Error("Embedding worker: failed to count pending entries", slog.Any("error", err))
		return
	}

	if remaining == 0 {
		slog.Info("Embedding worker: no backfill needed")
		return
	}

	slog.Info("Embedding worker: starting backfill",
		slog.Int("entries_remaining", remaining),
	)

	for remaining > 0 {
		if ctx.Err() != nil {
			return
		}

		n := w.processBatch(ctx)
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
}

// processBatch fetches and embeds one batch. Returns the number of entries
// successfully embedded.
func (w *Worker) processBatch(ctx context.Context) int {
	entries, err := w.store.GetEntriesWithoutEmbedding(w.batchSize)
	if err != nil {
		slog.Error("Embedding worker: failed to fetch entries", slog.Any("error", err))
		return 0
	}

	if len(entries) == 0 {
		return 0
	}

	texts := make([]string, len(entries))
	for i, e := range entries {
		texts[i] = PrepareText(e.Title, e.Content)
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
		return 0
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

	return stored
}
