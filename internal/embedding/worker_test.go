// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"miniflux.app/v2/internal/model"
)

// mockStore implements EntryStore for testing.
type mockStore struct {
	mu             sync.Mutex
	entries        []model.EntryForEmbedding
	embeddings     map[int64]model.Vector
	configuredDim  int
	configureCalls int
	configureErr   error
	getEntriesErr  error
	updateErr      error
	countErr       error
}

func newMockStore(entries []model.EntryForEmbedding) *mockStore {
	return &mockStore{
		entries:    entries,
		embeddings: make(map[int64]model.Vector),
	}
}

func (m *mockStore) GetEntriesWithoutEmbedding(limit int) ([]model.EntryForEmbedding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.getEntriesErr != nil {
		return nil, m.getEntriesErr
	}

	// Return entries that don't have embeddings yet.
	var result []model.EntryForEmbedding
	for _, e := range m.entries {
		if _, ok := m.embeddings[e.ID]; !ok {
			result = append(result, e)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *mockStore) CountEntriesWithoutEmbedding() (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.countErr != nil {
		return 0, m.countErr
	}

	count := 0
	for _, e := range m.entries {
		if _, ok := m.embeddings[e.ID]; !ok {
			count++
		}
	}
	return count, nil
}

func (m *mockStore) UpdateEntryEmbedding(entryID int64, embedding model.Vector) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.updateErr != nil {
		return m.updateErr
	}

	m.embeddings[entryID] = embedding
	return nil
}

func (m *mockStore) ConfigureEmbeddingColumn(dimensions int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.configureCalls++
	if m.configureErr != nil {
		return m.configureErr
	}

	m.configuredDim = dimensions
	return nil
}

func newTestServer(dims int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req embeddingRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := embeddingResponse{}
		for i := range req.Input {
			resp.Data = append(resp.Data, struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				Embedding: make([]float32, dims),
				Index:     i,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestWorkerAutoDetectDimensions(t *testing.T) {
	server := newTestServer(384)
	defer server.Close()

	store := newMockStore([]model.EntryForEmbedding{
		{ID: 1, Title: "Test Entry", Content: "<p>content</p>"},
	})

	client := NewClient(server.URL, "", "model", 0)
	w := NewWorker(client, store, 10, 10000, 0, time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go w.Run(ctx)
	time.Sleep(500 * time.Millisecond)
	cancel()

	store.mu.Lock()
	defer store.mu.Unlock()

	if store.configuredDim != 384 {
		t.Errorf("expected auto-detected dimensions 384, got %d", store.configuredDim)
	}

	if len(store.embeddings) != 1 {
		t.Errorf("expected 1 embedding stored, got %d", len(store.embeddings))
	}

	if len(store.embeddings[1]) != 384 {
		t.Errorf("expected embedding of length 384, got %d", len(store.embeddings[1]))
	}
}

func TestWorkerExplicitDimensions(t *testing.T) {
	server := newTestServer(1024)
	defer server.Close()

	store := newMockStore([]model.EntryForEmbedding{
		{ID: 1, Title: "Entry 1", Content: ""},
		{ID: 2, Title: "Entry 2", Content: ""},
	})

	client := NewClient(server.URL, "", "model", 1024)
	w := NewWorker(client, store, 10, 10000, 1024, time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go w.Run(ctx)
	time.Sleep(500 * time.Millisecond)
	cancel()

	store.mu.Lock()
	defer store.mu.Unlock()

	if store.configureCalls < 1 {
		t.Error("expected ConfigureEmbeddingColumn to be called at startup")
	}

	if store.configuredDim != 1024 {
		t.Errorf("expected configured dimensions 1024, got %d", store.configuredDim)
	}

	if len(store.embeddings) != 2 {
		t.Errorf("expected 2 embeddings stored, got %d", len(store.embeddings))
	}
}

func TestWorkerDimensionMismatch(t *testing.T) {
	// Server returns 256-dim vectors but worker expects 384.
	server := newTestServer(256)
	defer server.Close()

	store := newMockStore([]model.EntryForEmbedding{
		{ID: 1, Title: "Entry", Content: ""},
	})

	client := NewClient(server.URL, "", "model", 384)
	w := NewWorker(client, store, 10, 10000, 384, time.Second)

	// Manually set activeDimensions to simulate having already configured.
	w.activeDimensions = 384

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go w.Run(ctx)
	time.Sleep(500 * time.Millisecond)
	cancel()

	store.mu.Lock()
	defer store.mu.Unlock()

	// No embeddings should be stored due to dimension mismatch.
	if len(store.embeddings) != 0 {
		t.Errorf("expected 0 embeddings due to dimension mismatch, got %d", len(store.embeddings))
	}
}

func TestWorkerBackfillProcessesAll(t *testing.T) {
	server := newTestServer(3)
	defer server.Close()

	entries := make([]model.EntryForEmbedding, 25)
	for i := range entries {
		entries[i] = model.EntryForEmbedding{
			ID:    int64(i + 1),
			Title: "Entry",
		}
	}

	store := newMockStore(entries)

	client := NewClient(server.URL, "", "model", 0)
	w := NewWorker(client, store, 10, 10000, 0, 10*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go w.Run(ctx)
	time.Sleep(2 * time.Second)
	cancel()

	store.mu.Lock()
	defer store.mu.Unlock()

	if len(store.embeddings) != 25 {
		t.Errorf("expected all 25 entries embedded during backfill, got %d", len(store.embeddings))
	}
}

func TestWorkerNoEntries(t *testing.T) {
	server := newTestServer(3)
	defer server.Close()

	store := newMockStore(nil)

	client := NewClient(server.URL, "", "model", 0)
	w := NewWorker(client, store, 10, 10000, 0, time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	go w.Run(ctx)
	<-ctx.Done()

	store.mu.Lock()
	defer store.mu.Unlock()

	if len(store.embeddings) != 0 {
		t.Errorf("expected 0 embeddings, got %d", len(store.embeddings))
	}

	// ConfigureEmbeddingColumn should not be called when dimensions=0 and no entries.
	if store.configureCalls != 0 {
		t.Errorf("expected 0 configure calls, got %d", store.configureCalls)
	}
}
