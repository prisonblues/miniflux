// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetUserIDFromContext(t *testing.T) {
	h := &handler{userID: 42}

	t.Run("returns fixed userID when context has no override", func(t *testing.T) {
		ctx := context.Background()
		if got := h.getUserID(ctx); got != 42 {
			t.Errorf("getUserID() = %d, want 42", got)
		}
	})

	t.Run("returns context userID when set", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), UserIDContextKey, int64(99))
		if got := h.getUserID(ctx); got != 99 {
			t.Errorf("getUserID() = %d, want 99", got)
		}
	})

	t.Run("falls back to fixed userID when context value is zero", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), UserIDContextKey, int64(0))
		if got := h.getUserID(ctx); got != 42 {
			t.Errorf("getUserID() = %d, want 42", got)
		}
	})

	t.Run("falls back to fixed userID when context value is wrong type", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), UserIDContextKey, "not-an-int")
		if got := h.getUserID(ctx); got != 42 {
			t.Errorf("getUserID() = %d, want 42", got)
		}
	})
}

// stubHandler records whether ServeHTTP was called.
type stubHandler struct {
	called bool
	userID int64
}

func (s *stubHandler) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	s.called = true
	if id, ok := r.Context().Value(UserIDContextKey).(int64); ok {
		s.userID = id
	}
}

func TestAPIKeyAuthRejectsMissingToken(t *testing.T) {
	inner := &stubHandler{}
	// nil store is safe — apiKeyAuth rejects missing tokens before calling store.
	handler := apiKeyAuth(nil, inner)

	req := httptest.NewRequest("POST", "/mcp", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if inner.called {
		t.Error("inner handler should not have been called")
	}
}

func TestAPIKeyAuthRejectsEmptyToken(t *testing.T) {
	inner := &stubHandler{}
	handler := apiKeyAuth(nil, inner)

	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("X-Auth-Token", "")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if inner.called {
		t.Error("inner handler should not have been called")
	}
}
