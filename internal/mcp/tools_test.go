// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"testing"

	gomcp "github.com/mark3labs/mcp-go/mcp"
)

// newRequest builds a CallToolRequest with the given arguments map.
func newRequest(args map[string]any) gomcp.CallToolRequest {
	return gomcp.CallToolRequest{
		Params: gomcp.CallToolParams{
			Arguments: args,
		},
	}
}

func TestClampLimit(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{0, defaultLimit},
		{-1, defaultLimit},
		{1, 1},
		{20, 20},
		{100, 100},
		{101, maxLimit},
		{999, maxLimit},
	}

	for _, tt := range tests {
		got := clampLimit(tt.input)
		if got != tt.expected {
			t.Errorf("clampLimit(%d) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestSemanticSearchRequiresEmbeddingClient(t *testing.T) {
	h := &handler{
		store:           nil,
		userID:          1,
		embeddingClient: nil,
	}

	result, err := h.semanticSearch(context.Background(), newRequest(map[string]any{"query": "test"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result when embedding client is nil")
	}
}

func TestSimilarEntriesRequiresEmbeddingClient(t *testing.T) {
	h := &handler{
		store:           nil,
		userID:          1,
		embeddingClient: nil,
	}

	result, err := h.similarEntries(context.Background(), newRequest(map[string]any{"entry_id": float64(42)}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result when embedding client is nil")
	}
}

func TestSearchEntriesRequiresQuery(t *testing.T) {
	h := &handler{
		store:  nil,
		userID: 1,
	}

	result, err := h.searchEntries(context.Background(), newRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result when query is missing")
	}
}

func TestGetEntryRequiresEntryID(t *testing.T) {
	h := &handler{
		store:  nil,
		userID: 1,
	}

	result, err := h.getEntry(context.Background(), newRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result when entry_id is missing")
	}
}

func TestFeedEntriesRequiresFeedID(t *testing.T) {
	h := &handler{
		store:  nil,
		userID: 1,
	}

	result, err := h.feedEntries(context.Background(), newRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result when feed_id is missing")
	}
}

func TestTopicScanRequiresTopics(t *testing.T) {
	h := &handler{
		store:  nil,
		userID: 1,
	}

	// Missing topics
	result, err := h.topicScan(context.Background(), newRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result when topics is missing")
	}

	// Empty topics array
	result, err = h.topicScan(context.Background(), newRequest(map[string]any{"topics": []any{}}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result when topics is empty")
	}

	// Topics with only empty strings
	result, err = h.topicScan(context.Background(), newRequest(map[string]any{"topics": []any{""}}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result when topics contains only empty strings")
	}
}

func TestSimilarEntriesRequiresEntryID(t *testing.T) {
	h := &handler{
		store:           nil,
		userID:          1,
		embeddingClient: nil,
	}

	// entry_id defaults to 0 when not provided, which triggers the embedding client check first
	// but with no client, it should error about embeddings
	result, err := h.similarEntries(context.Background(), newRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result")
	}
}

func TestToolDefinitions(t *testing.T) {
	tools := []struct {
		name string
		fn   func() gomcp.Tool
	}{
		{"search_entries", searchEntriesTool},
		{"semantic_search", semanticSearchTool},
		{"similar_entries", similarEntriesTool},
		{"recent_entries", recentEntriesTool},
		{"list_feeds", listFeedsTool},
		{"feed_entries", feedEntriesTool},
		{"topic_scan", topicScanTool},
		{"get_entry", getEntryTool},
	}

	for _, tt := range tools {
		tool := tt.fn()
		if got := tool.GetName(); got != tt.name {
			t.Errorf("tool name = %q, want %q", got, tt.name)
		}
	}
}

func TestNewServerRegistersAllTools(t *testing.T) {
	// NewServer should not panic with nil store/client (tools are just registered, not called)
	s := NewServer(nil, 1, nil)
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
}
