// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package embedding // import "miniflux.app/v2/internal/embedding"

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"miniflux.app/v2/internal/model"
)

// Client calls an OpenAI-compatible /v1/embeddings endpoint.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
	dimensions int
}

// NewClient creates a new embedding API client.
// When dimensions > 0, the value is sent in the request so compatible models
// can truncate their output (Matryoshka / OpenAI dimensions parameter).
func NewClient(baseURL, apiKey, modelName string, dimensions int) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 120 * time.Second},
		baseURL:    baseURL,
		apiKey:     apiKey,
		model:      modelName,
		dimensions: dimensions,
	}
}

type embeddingRequest struct {
	Input      []string `json:"input"`
	Model      string   `json:"model"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

// Embed returns embeddings for the given texts. The returned slice is
// positionally aligned with the input.
func (c *Client) Embed(ctx context.Context, texts []string) ([]model.Vector, error) {
	apiReq := embeddingRequest{
		Input: texts,
		Model: c.model,
	}
	if c.dimensions > 0 {
		apiReq.Dimensions = c.dimensions
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("embedding: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedding: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("embedding: API returned %d: %s", resp.StatusCode, respBody)
	}

	var result embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("embedding: decode response: %w", err)
	}

	if len(result.Data) != len(texts) {
		return nil, fmt.Errorf("embedding: expected %d embeddings, got %d", len(texts), len(result.Data))
	}

	vectors := make([]model.Vector, len(texts))
	for _, d := range result.Data {
		if d.Index < 0 || d.Index >= len(texts) {
			return nil, fmt.Errorf("embedding: response index %d out of range", d.Index)
		}
		if vectors[d.Index] != nil {
			return nil, fmt.Errorf("embedding: duplicate response index %d", d.Index)
		}
		vectors[d.Index] = model.Vector(d.Embedding)
	}

	return vectors, nil
}

// EmbedSingle returns the embedding for a single text string.
func (c *Client) EmbedSingle(ctx context.Context, text string) (model.Vector, error) {
	vectors, err := c.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}
