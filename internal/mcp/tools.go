// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package mcp // import "miniflux.app/v2/internal/mcp"

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"miniflux.app/v2/internal/embedding"
	"miniflux.app/v2/internal/model"
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

	// Query-embedding budget for the semantic MCP tools. The OpenRouter
	// embedding endpoint occasionally cold-starts or queues past a single short
	// timeout — most visibly during the fixed early-morning cron runs — so we
	// allow a longer per-attempt window than the previous hard 10s and retry
	// once. A slow first attempt is usually followed by a fast warm one.
	embedQueryTimeout  = 15 * time.Second
	embedQueryAttempts = 2
)

// embedQuery embeds a single query string for semantic search, retrying on
// transient failure. Returns the vector, or the last error if every attempt
// fails (or the caller's context is cancelled).
func (h *handler) embedQuery(ctx context.Context, text string) (model.Vector, error) {
	var lastErr error
	for attempt := 0; attempt < embedQueryAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, embedQueryTimeout)
		vec, err := h.embeddingClient.EmbedSingle(attemptCtx, text)
		cancel()
		if err == nil && len(vec) > 0 {
			return vec, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("embedding returned no vector")
		}
		// Stop early if the caller's context is done (parent deadline/cancel).
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt < embedQueryAttempts-1 {
			select {
			case <-time.After(500 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return nil, lastErr
}

type contextKey int

// UserIDContextKey is the context key for the authenticated MCP user ID.
// Used by the HTTP transport auth middleware to pass the resolved user to
// tool handlers. In stdio mode the handler's fixed userID field is used instead.
const UserIDContextKey contextKey = iota

// handler holds the dependencies for MCP tool handlers.
type handler struct {
	store           *storage.Storage
	userID          int64             // fixed user for stdio mode; 0 when using HTTP transport
	embeddingClient *embedding.Client // nil when embeddings disabled
}

// getUserID returns the user ID for the current request. It checks the context
// first (set by HTTP auth middleware) and falls back to the handler's fixed userID.
func (h *handler) getUserID(ctx context.Context) int64 {
	if id, ok := ctx.Value(UserIDContextKey).(int64); ok && id > 0 {
		return id
	}
	return h.userID
}

func (h *handler) searchEntries(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query is required"), nil
	}

	limit := clampLimit(req.GetInt("limit", defaultLimit))
	status := req.GetString("status", "")
	categoryID := int64(req.GetInt("category_id", 0))

	builder := storage.NewEntryQueryBuilder(h.store, h.getUserID(ctx))
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
	return mcp.NewToolResultJSON(entriesResult{Entries: results})
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

	builder := storage.NewEntryQueryBuilder(h.store, h.getUserID(ctx))
	if vec, err := h.embedQuery(ctx, query); err == nil {
		builder.WithSemanticSearch(vec)
	} else {
		// Degrade to full-text search rather than failing the whole call when
		// the embedding endpoint is briefly unavailable. Mirrors topic_scan and
		// the REST semantic path, so callers get keyword-matched results instead
		// of a hard error.
		slog.Warn("semantic_search: embedding failed, falling back to full-text search",
			slog.Any("error", err))
		builder.WithSearchQuery(query)
	}
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
	return mcp.NewToolResultJSON(entriesResult{Entries: results})
}

func (h *handler) similarEntries(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if h.embeddingClient == nil {
		return mcp.NewToolResultError("similar_entries requires EMBEDDING_ENABLED=true"), nil
	}

	entryID := int64(req.GetInt("entry_id", 0))
	if entryID == 0 {
		return mcp.NewToolResultError("entry_id is required"), nil
	}

	limit := clampLimit(req.GetInt("limit", defaultLimit))

	vec, err := h.store.GetEntryEmbedding(h.getUserID(ctx), entryID)
	if err != nil {
		return nil, fmt.Errorf("similar_entries: lookup embedding: %w", err)
	}
	if vec == nil {
		return mcp.NewToolResultError(fmt.Sprintf("entry %d has no embedding", entryID)), nil
	}

	builder := storage.NewEntryQueryBuilder(h.store, h.getUserID(ctx))
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
	return mcp.NewToolResultJSON(entriesResult{Entries: results})
}

func (h *handler) recentEntries(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	hours := req.GetInt("hours", defaultHours)
	if hours <= 0 {
		hours = defaultHours
	}
	limit := clampLimit(req.GetInt("limit", defaultLimit))
	status := req.GetString("status", "unread")

	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)

	builder := storage.NewEntryQueryBuilder(h.store, h.getUserID(ctx))
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
	return mcp.NewToolResultJSON(entriesResult{Entries: results})
}

func (h *handler) listFeeds(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	builder := storage.NewFeedQueryBuilder(h.store, h.getUserID(ctx))
	builder.WithSorting("lower(f.title)", "ASC")
	feeds, err := builder.GetFeeds()
	if err != nil {
		return nil, fmt.Errorf("list_feeds: %w", err)
	}

	results := make([]feedSummary, len(feeds))
	for i, f := range feeds {
		results[i] = formatFeedSummary(f)
	}
	return mcp.NewToolResultJSON(feedsResult{Feeds: results})
}

func (h *handler) feedEntries(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	feedID := int64(req.GetInt("feed_id", 0))
	if feedID == 0 {
		return mcp.NewToolResultError("feed_id is required"), nil
	}

	limit := clampLimit(req.GetInt("limit", defaultLimit))
	query := req.GetString("query", "")

	builder := storage.NewEntryQueryBuilder(h.store, h.getUserID(ctx))
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
	return mcp.NewToolResultJSON(entriesResult{Entries: results})
}

// MCP structuredContent must be a JSON object, never an array.
// These wrapper types ensure slice results are always nested in an object.
type entriesResult struct {
	Entries []entrySummary `json:"entries"`
}

type feedsResult struct {
	Feeds []feedSummary `json:"feeds"`
}

type topicsResult struct {
	Topics []topicResult `json:"topics"`
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
		builder := storage.NewEntryQueryBuilder(h.store, h.getUserID(ctx))
		builder.AfterPublishedDate(since)
		builder.WithLimit(limitPerTopic)

		if useSemantic && h.embeddingClient != nil {
			if vec, err := h.embedQuery(ctx, topic); err == nil {
				builder.WithSemanticSearch(vec)
			} else {
				// Fall back to FTS on embedding failure.
				slog.Warn("topic_scan: embedding failed, falling back to full-text search",
					slog.String("topic", topic), slog.Any("error", err))
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

	return mcp.NewToolResultJSON(topicsResult{Topics: results})
}

func (h *handler) getEntry(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	entryID := int64(req.GetInt("entry_id", 0))
	if entryID == 0 {
		return mcp.NewToolResultError("entry_id is required"), nil
	}

	builder := storage.NewEntryQueryBuilder(h.store, h.getUserID(ctx))
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
