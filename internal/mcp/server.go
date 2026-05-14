// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package mcp // import "miniflux.app/v2/internal/mcp"

import (
	"miniflux.app/v2/internal/embedding"
	"miniflux.app/v2/internal/storage"
	"miniflux.app/v2/internal/version"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// NewServer creates a configured MCP server with all tools registered.
func NewServer(store *storage.Storage, userID int64, embeddingClient *embedding.Client) *server.MCPServer {
	s := server.NewMCPServer(
		"miniflux",
		version.Version,
		server.WithToolCapabilities(false),
	)

	h := &handler{
		store:           store,
		userID:          userID,
		embeddingClient: embeddingClient,
	}

	s.AddTool(searchEntriesTool(), h.searchEntries)
	s.AddTool(semanticSearchTool(), h.semanticSearch)
	s.AddTool(similarEntriesTool(), h.similarEntries)
	s.AddTool(recentEntriesTool(), h.recentEntries)
	s.AddTool(listFeedsTool(), h.listFeeds)
	s.AddTool(feedEntriesTool(), h.feedEntries)
	s.AddTool(topicScanTool(), h.topicScan)
	s.AddTool(getEntryTool(), h.getEntry)

	return s
}

func searchEntriesTool() mcp.Tool {
	return mcp.NewTool("search_entries",
		mcp.WithDescription("Full-text search across all RSS entries using PostgreSQL tsvector"),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 20, max 100)")),
		mcp.WithString("status", mcp.Description("Filter by status"), mcp.Enum("unread", "read")),
		mcp.WithNumber("category_id", mcp.Description("Filter by category ID")),
	)
}

func semanticSearchTool() mcp.Tool {
	return mcp.NewTool("semantic_search",
		mcp.WithDescription("Vector similarity search using embeddings (requires EMBEDDING_ENABLED=true)"),
		mcp.WithString("query", mcp.Required(), mcp.Description("Natural language search query")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 20, max 100)")),
		mcp.WithString("status", mcp.Description("Filter by status"), mcp.Enum("unread", "read")),
		mcp.WithNumber("category_id", mcp.Description("Filter by category ID")),
	)
}

func similarEntriesTool() mcp.Tool {
	return mcp.NewTool("similar_entries",
		mcp.WithDescription("Find entries similar to a given entry using vector similarity (requires EMBEDDING_ENABLED=true)"),
		mcp.WithNumber("entry_id", mcp.Required(), mcp.Description("Reference entry ID")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 20, max 100)")),
	)
}

func recentEntriesTool() mcp.Tool {
	return mcp.NewTool("recent_entries",
		mcp.WithDescription("Get entries published in the last N hours"),
		mcp.WithNumber("hours", mcp.Description("Lookback window in hours (default 24)")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 20, max 100)")),
		mcp.WithString("status", mcp.Description("Filter by status (default unread)"), mcp.Enum("unread", "read")),
	)
}

func listFeedsTool() mcp.Tool {
	return mcp.NewTool("list_feeds",
		mcp.WithDescription("List all subscribed feeds with their categories"),
	)
}

func feedEntriesTool() mcp.Tool {
	return mcp.NewTool("feed_entries",
		mcp.WithDescription("Get entries from a specific feed"),
		mcp.WithNumber("feed_id", mcp.Required(), mcp.Description("Feed ID")),
		mcp.WithString("query", mcp.Description("Optional full-text search within this feed")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 20, max 100)")),
	)
}

func topicScanTool() mcp.Tool {
	return mcp.NewTool("topic_scan",
		mcp.WithDescription("Search multiple topics in one call. Uses semantic search when embeddings are enabled, falls back to full-text search."),
		mcp.WithArray("topics", mcp.Required(), mcp.Description("List of topic strings to search"),
			mcp.WithStringItems()),
		mcp.WithNumber("hours", mcp.Description("Lookback window in hours (default 24)")),
		mcp.WithNumber("limit_per_topic", mcp.Description("Max results per topic (default 5, max 50)")),
		mcp.WithBoolean("use_semantic", mcp.Description("Use semantic search when available (default true)")),
	)
}

func getEntryTool() mcp.Tool {
	return mcp.NewTool("get_entry",
		mcp.WithDescription("Get the full content of a single entry"),
		mcp.WithNumber("entry_id", mcp.Required(), mcp.Description("Entry ID")),
	)
}
