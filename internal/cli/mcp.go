// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cli // import "miniflux.app/v2/internal/cli"

import (
	"fmt"
	"log/slog"

	"miniflux.app/v2/internal/config"
	"miniflux.app/v2/internal/embedding"
	mcpserver "miniflux.app/v2/internal/mcp"
	"miniflux.app/v2/internal/storage"

	"github.com/mark3labs/mcp-go/server"
)

func startMCPServer(store *storage.Storage) {
	userID, err := resolveUser(store)
	if err != nil {
		printErrorAndExit(err)
	}

	var embeddingClient *embedding.Client
	if config.Opts.EmbeddingEnabled() {
		embeddingClient = embedding.NewClient(
			config.Opts.EmbeddingAPIURL(),
			config.Opts.EmbeddingAPIKey(),
			config.Opts.EmbeddingModel(),
			config.Opts.EmbeddingDimensions(),
		)
	}

	slog.Info("Starting MCP server", slog.Int64("user_id", userID))

	s := mcpserver.NewServer(store, userID, embeddingClient)
	if err := server.ServeStdio(s); err != nil {
		printErrorAndExit(fmt.Errorf("mcp server: %w", err))
	}
}

// resolveUser determines which Miniflux user's feeds the MCP server exposes.
//
// The MCP server runs over stdio with no network authentication — the calling
// process (e.g. Claude Desktop) spawns it directly. MCP_API_KEY is not used
// for MCP authentication; it is a regular Miniflux user API key (the same one
// generated under Settings > API Keys) used solely to identify the user whose
// data should be served.
//
// For single-user instances, the user is auto-detected and no key is needed.
func resolveUser(store *storage.Storage) (int64, error) {
	apiKey := config.Opts.MCPAPIKey()
	if apiKey != "" {
		user, err := store.UserByAPIKey(apiKey)
		if err != nil {
			return 0, fmt.Errorf("invalid MCP_API_KEY: %w", err)
		}
		return user.ID, nil
	}

	count, err := store.CountUsers()
	if err != nil {
		return 0, fmt.Errorf("unable to count users: %w", err)
	}
	if count == 1 {
		users, err := store.Users()
		if err != nil {
			return 0, fmt.Errorf("unable to list users: %w", err)
		}
		return users[0].ID, nil
	}

	return 0, fmt.Errorf("MCP_API_KEY is required when multiple users exist (%d users found); "+
		"set it to a Miniflux user API key (Settings > API Keys) to select the user", count)
}
