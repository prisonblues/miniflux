// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbedSingle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("expected /v1/embeddings, got %s", r.URL.Path)
		}

		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json content type, got %s", ct)
		}

		var req embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if len(req.Input) != 1 {
			t.Fatalf("expected 1 input, got %d", len(req.Input))
		}

		if req.Input[0] != "test query" {
			t.Errorf("expected input 'test query', got %q", req.Input[0])
		}

		if req.Model != "test-model" {
			t.Errorf("expected model 'test-model', got %q", req.Model)
		}

		resp := embeddingResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: []float32{0.1, 0.2, 0.3}, Index: 0},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", "test-model", 0)
	vec, err := client.EmbedSingle(context.Background(), "test query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vec) != 3 {
		t.Fatalf("expected 3 dimensions, got %d", len(vec))
	}

	if vec[0] != 0.1 || vec[1] != 0.2 || vec[2] != 0.3 {
		t.Errorf("unexpected vector values: %v", vec)
	}
}

func TestEmbedBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req embeddingRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := embeddingResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: []float32{0.3, 0.4}, Index: 1},
				{Embedding: []float32{0.1, 0.2}, Index: 0},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", "test-model", 0)
	vecs, err := client.Embed(context.Background(), []string{"first", "second"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vecs) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vecs))
	}

	// Check that index reordering works correctly
	if vecs[0][0] != 0.1 || vecs[0][1] != 0.2 {
		t.Errorf("first vector incorrect: %v", vecs[0])
	}
	if vecs[1][0] != 0.3 || vecs[1][1] != 0.4 {
		t.Errorf("second vector incorrect: %v", vecs[1])
	}
}

func TestEmbedWithAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key-123" {
			t.Errorf("expected Bearer test-key-123, got %q", auth)
		}

		resp := embeddingResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: []float32{0.1}, Index: 0},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key-123", "model", 0)
	_, err := client.EmbedSingle(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEmbedAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", "model", 0)
	_, err := client.EmbedSingle(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestEmbedCountMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := embeddingResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: []float32{0.1}, Index: 0},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", "model", 0)
	_, err := client.Embed(context.Background(), []string{"one", "two"})
	if err == nil {
		t.Fatal("expected error for count mismatch")
	}
}

func TestEmbedDuplicateIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := embeddingResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: []float32{0.1, 0.2}, Index: 0},
				{Embedding: []float32{0.3, 0.4}, Index: 0}, // duplicate index
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", "model", 0)
	_, err := client.Embed(context.Background(), []string{"first", "second"})
	if err == nil {
		t.Fatal("expected error for duplicate response index")
	}
}

func TestEmbedWithDimensionsPresent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req.Dimensions != 256 {
			t.Errorf("expected dimensions=256 in request, got %d", req.Dimensions)
		}

		resp := embeddingResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: make([]float32, 256), Index: 0},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", "model", 256)
	vec, err := client.EmbedSingle(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(vec) != 256 {
		t.Fatalf("expected 256 dimensions, got %d", len(vec))
	}
}

func TestEmbedWithDimensionsOmitted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if _, exists := raw["dimensions"]; exists {
			t.Error("expected dimensions field to be omitted from request JSON when 0")
		}

		resp := embeddingResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: []float32{0.1}, Index: 0},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", "model", 0)
	_, err := client.EmbedSingle(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
