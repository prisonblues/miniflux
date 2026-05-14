// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package mcp // import "miniflux.app/v2/internal/mcp"

import (
	"context"
	"fmt"
	"time"

	"miniflux.app/v2/internal/embedding"
	"miniflux.app/v2/internal/storage"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	defaultLimit    = 20
	maxLimit        = 100
	maxTopicLimit   = 50
	maxTopics       = 20
	defaultHours    = 24
	defaultTopicLim = 5
)

// handler holds the dependencies for MCP tool handlers.
type handler struct {
	store           *storage.Storage
	userID          int64
	embeddingClient *embedding.Client // nil when embeddings disabled
}

func (h *handler) searchEntries(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query is required"), nil
	}

	limit := clampLimit(req.GetInt("limit", defaultLimit))
	status := req.GetString("status", "")
	categoryID := int64(req.GetInt("category_id", 0))

	builder := storage.NewEntryQueryBuilder(h.store, h.userID)
	builder.WithSearchQuery(query)
	builder.WithLimit(limit)
	if status != "" {
		builder.WithStatus(status)
	}
	if categoryID > 0 {
		builder.WithCategoryID(categoryID)
	}

	entries, err := builder.GetEntries()
	if err != nil {
		return nil, fmt.Errorf("search_entries: %w", err)
	}

	results := make([]entrySummary, len(entries))
	for i, e := range entries {
		results[i] = formatEntrySummary(e)
	}
	return mcp.NewToolResultJSON(results)
}

func (h *handler) semanticSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if h.embeddingClient == nil {
		return mcp.NewToolResultError("semantic search requires EMBEDDING_ENABLED=true"), nil
	}

	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query is required"), nil
	}

	limit := clampLimit(req.GetInt("limit", defaultLimit))
	status := req.GetString("status", "")
	categoryID := int64(req.GetInt("category_id", 0))

	embedCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	vec, err := h.embeddingClient.EmbedSingle(embedCtx, query)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("embedding failed: %v", err)), nil
	}

	builder := storage.NewEntryQueryBuilder(h.store, h.userID)
	builder.WithSemanticSearch(vec)
	builder.WithLimit(limit)
	if status != "" {
		builder.WithStatus(status)
	}
	if categoryID > 0 {
		builder.WithCategoryID(categoryID)
	}

	entries, err := builder.GetEntries()
	if err != nil {
		return nil, fmt.Errorf("semantic_search: %w", err)
	}

	results := make([]entrySummary, len(entries))
	for i, e := range entries {
		results[i] = formatEntrySummary(e)
	}
	return mcp.NewToolResultJSON(results)
}

func (h *handler) similarEntries(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if h.embeddingClient == nil {
		return mcp.NewToolResultError("similar_entries requires EMBEDDING_ENABLED=true"), nil
	}

	entryID := int64(req.GetInt("entry_id", 0))
	if entryID == 0 {
		return mcp.NewToolResultError("entry_id is required"), nil
	}

	limit := clampLimit(req.GetInt("limit", defaultLimit))

	vec, err := h.store.GetEntryEmbedding(h.userID, entryID)
	if err != nil {
		return nil, fmt.Errorf("similar_entries: lookup embedding: %w", err)
	}
	if vec == nil {
		return mcp.NewToolResultError(fmt.Sprintf("entry %d has no embedding", entryID)), nil
	}

	builder := storage.NewEntryQueryBuilder(h.store, h.userID)
	builder.WithSemanticSearch(vec)
	builder.WithoutEntryID(entryID)
	builder.WithLimit(limit)

	entries, err := builder.GetEntries()
	if err != nil {
		return nil, fmt.Errorf("similar_entries: %w", err)
	}

	results := make([]entrySummary, len(entries))
	for i, e := range entries {
		results[i] = formatEntrySummary(e)
	}
	return mcp.NewToolResultJSON(results)
}

func (h *handler) recentEntries(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	hours := req.GetInt("hours", defaultHours)
	if hours <= 0 {
		hours = defaultHours
	}
	limit := clampLimit(req.GetInt("limit", defaultLimit))
	status := req.GetString("status", "unread")

	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)

	builder := storage.NewEntryQueryBuilder(h.store, h.userID)
	builder.AfterPublishedDate(since)
	builder.WithLimit(limit)
	builder.WithSorting("e.published_at", "DESC")
	if status != "" {
		builder.WithStatus(status)
	}

	entries, err := builder.GetEntries()
	if err != nil {
		return nil, fmt.Errorf("recent_entries: %w", err)
	}

	results := make([]entrySummary, len(entries))
	for i, e := range entries {
		results[i] = formatEntrySummary(e)
	}
	return mcp.NewToolResultJSON(results)
}

func (h *handler) listFeeds(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	builder := storage.NewFeedQueryBuilder(h.store, h.userID)
	builder.WithSorting("lower(f.title)", "ASC")
	feeds, err := builder.GetFeeds()
	if err != nil {
		return nil, fmt.Errorf("list_feeds: %w", err)
	}

	results := make([]feedSummary, len(feeds))
	for i, f := range feeds {
		results[i] = formatFeedSummary(f)
	}
	return mcp.NewToolResultJSON(results)
}

func (h *handler) feedEntries(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	feedID := int64(req.GetInt("feed_id", 0))
	if feedID == 0 {
		return mcp.NewToolResultError("feed_id is required"), nil
	}

	limit := clampLimit(req.GetInt("limit", defaultLimit))
	query := req.GetString("query", "")

	builder := storage.NewEntryQueryBuilder(h.store, h.userID)
	builder.WithFeedID(feedID)
	builder.WithLimit(limit)
	builder.WithSorting("e.published_at", "DESC")
	if query != "" {
		builder.WithSearchQuery(query)
	}

	entries, err := builder.GetEntries()
	if err != nil {
		return nil, fmt.Errorf("feed_entries: %w", err)
	}

	results := make([]entrySummary, len(entries))
	for i, e := range entries {
		results[i] = formatEntrySummary(e)
	}
	return mcp.NewToolResultJSON(results)
}

// topicResult groups search results by topic.
type topicResult struct {
	Topic   string         `json:"topic"`
	Entries []entrySummary `json:"entries"`
}

func (h *handler) topicScan(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	topicsRaw, ok := req.GetArguments()["topics"]
	if !ok {
		return mcp.NewToolResultError("topics is required"), nil
	}
	topicsSlice, ok := topicsRaw.([]any)
	if !ok || len(topicsSlice) == 0 {
		return mcp.NewToolResultError("topics must be a non-empty array of strings"), nil
	}

	topics := make([]string, 0, len(topicsSlice))
	for _, t := range topicsSlice {
		if s, ok := t.(string); ok && s != "" {
			topics = append(topics, s)
		}
	}
	if len(topics) == 0 {
		return mcp.NewToolResultError("topics must contain at least one non-empty string"), nil
	}
	if len(topics) > maxTopics {
		return mcp.NewToolResultError(fmt.Sprintf("too many topics: %d (max %d)", len(topics), maxTopics)), nil
	}

	hours := req.GetInt("hours", defaultHours)
	if hours <= 0 {
		hours = defaultHours
	}
	limitPerTopic := req.GetInt("limit_per_topic", defaultTopicLim)
	if limitPerTopic <= 0 {
		limitPerTopic = defaultTopicLim
	}
	if limitPerTopic > maxTopicLimit {
		limitPerTopic = maxTopicLimit
	}
	useSemantic := req.GetBool("use_semantic", true)

	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
	results := make([]topicResult, 0, len(topics))

	for _, topic := range topics {
		builder := storage.NewEntryQueryBuilder(h.store, h.userID)
		builder.AfterPublishedDate(since)
		builder.WithLimit(limitPerTopic)

		if useSemantic && h.embeddingClient != nil {
			embedCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			vec, err := h.embeddingClient.EmbedSingle(embedCtx, topic)
			cancel()
			if err == nil && len(vec) > 0 {
				builder.WithSemanticSearch(vec)
			} else {
				// Fall back to FTS on embedding failure.
				builder.WithSearchQuery(topic)
			}
		} else {
			builder.WithSearchQuery(topic)
		}

		entries, err := builder.GetEntries()
		if err != nil {
			return nil, fmt.Errorf("topic_scan [%s]: %w", topic, err)
		}

		summaries := make([]entrySummary, len(entries))
		for i, e := range entries {
			summaries[i] = formatEntrySummary(e)
		}
		results = append(results, topicResult{Topic: topic, Entries: summaries})
	}

	return mcp.NewToolResultJSON(results)
}

func (h *handler) getEntry(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	entryID := int64(req.GetInt("entry_id", 0))
	if entryID == 0 {
		return mcp.NewToolResultError("entry_id is required"), nil
	}

	builder := storage.NewEntryQueryBuilder(h.store, h.userID)
	builder.WithEntryID(entryID)
	entry, err := builder.GetEntry()
	if err != nil {
		return nil, fmt.Errorf("get_entry: %w", err)
	}
	if entry == nil {
		return mcp.NewToolResultError(fmt.Sprintf("entry %d not found", entryID)), nil
	}

	return mcp.NewToolResultJSON(formatEntryDetail(entry))
}

func clampLimit(v int) int {
	if v <= 0 {
		return defaultLimit
	}
	if v > maxLimit {
		return maxLimit
	}
	return v
}
