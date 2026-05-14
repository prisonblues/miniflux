// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package mcp // import "miniflux.app/v2/internal/mcp"

import (
	"context"
	"log/slog"
	"net/http"

	"miniflux.app/v2/internal/embedding"
	"miniflux.app/v2/internal/storage"

	"github.com/mark3labs/mcp-go/server"
)

// StartHTTPServer creates and starts an MCP HTTP server using the streamable
// HTTP transport from the mcp-go SDK. Requests are authenticated via the
// X-Auth-Token header (same as the REST API), and the resolved user ID is
// injected into the request context for tool handlers.
//
// The returned *http.Server can be used for graceful shutdown.
func StartHTTPServer(store *storage.Storage, embeddingClient *embedding.Client, addr string) *http.Server {
	mcpServer := NewServer(store, 0, embeddingClient)

	streamServer := server.NewStreamableHTTPServer(mcpServer,
		server.WithStateLess(true),
	)

	mux := http.NewServeMux()
	mux.Handle("/mcp", apiKeyAuth(store, streamServer))

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		slog.Info("Starting MCP HTTP server", slog.String("listen_address", addr))
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("MCP HTTP server failed", slog.Any("error", err))
		}
	}()

	return srv
}

// apiKeyAuth validates the X-Auth-Token header against the Miniflux user
// database. On success the resolved user ID is stored in the request context
// under UserIDContextKey so that tool handlers can retrieve it.
func apiKeyAuth(store *storage.Storage, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Auth-Token")
		if token == "" {
			http.Error(w, "X-Auth-Token header is required", http.StatusUnauthorized)
			return
		}

		user, err := store.UserByAPIKey(token)
		if err != nil {
			slog.Error("[MCP] API key lookup failed",
				slog.Any("error", err),
				slog.String("request_uri", r.RequestURI),
			)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if user == nil {
			slog.Warn("[MCP] Invalid API key",
				slog.String("request_uri", r.RequestURI),
			)
			http.Error(w, "invalid API key", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), UserIDContextKey, user.ID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
