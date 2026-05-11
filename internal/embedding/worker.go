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
	UpdateEntryEmbedding(entryID int64, embedding model.Vector) error
}

// Worker periodically embeds entries that lack a vector.
type Worker struct {
	client    *Client
	store     EntryStore
	batchSize int
	interval  time.Duration
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

	// Process once immediately at startup for backfill.
	w.processBatch(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Embedding worker stopped")
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

func (w *Worker) processBatch(ctx context.Context) {
	entries, err := w.store.GetEntriesWithoutEmbedding(w.batchSize)
	if err != nil {
		slog.Error("Embedding worker: failed to fetch entries", slog.Any("error", err))
		return
	}

	if len(entries) == 0 {
		return
	}

	slog.Debug("Embedding worker: processing batch", slog.Int("count", len(entries)))

	texts := make([]string, len(entries))
	for i, e := range entries {
		texts[i] = PrepareText(e.Title, e.Content)
	}

	vectors, err := w.client.Embed(ctx, texts)
	if err != nil {
		slog.Error("Embedding worker: API call failed", slog.Any("error", err))
		return
	}

	for i, entry := range entries {
		if err := w.store.UpdateEntryEmbedding(entry.ID, vectors[i]); err != nil {
			slog.Error("Embedding worker: failed to store embedding",
				slog.Int64("entry_id", entry.ID),
				slog.Any("error", err),
			)
		}
	}

	slog.Debug("Embedding worker: batch complete", slog.Int("embedded", len(entries)))
}
