// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"miniflux.app/v2/internal/embedding"
	"miniflux.app/v2/internal/storage"
)

func TestLookbackCutoff(t *testing.T) {
	now := time.Date(2026, time.March, 31, 12, 34, 56, 789, time.UTC)
	tests := []struct {
		value string
		want  time.Time
	}{
		{"24h", now.Add(-24 * time.Hour)},
		{"1 hour", now.Add(-time.Hour)},
		{"48 hours", now.Add(-48 * time.Hour)},
		{"7d", now.Add(-7 * 24 * time.Hour)},
		{"1 day", now.Add(-24 * time.Hour)},
		{" 7 DAYS ", now.Add(-7 * 24 * time.Hour)},
		{"2w", now.Add(-14 * 24 * time.Hour)},
		{"1 week", now.Add(-7 * 24 * time.Hour)},
		{"2 weeks", now.Add(-14 * 24 * time.Hour)},
		{"1mo", time.Date(2026, time.February, 28, 12, 34, 56, 789, time.UTC)},
		{"1 month", time.Date(2026, time.February, 28, 12, 34, 56, 789, time.UTC)},
		{"3 months", time.Date(2025, time.December, 31, 12, 34, 56, 789, time.UTC)},
		{"13mo", time.Date(2025, time.February, 28, 12, 34, 56, 789, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := lookbackCutoff(newRequest(map[string]any{"lookback": tt.value}), now)
			if err != nil || !got.Equal(tt.want) {
				t.Fatalf("got %v, %v; want %v", got, err, tt.want)
			}
		})
	}
	got, err := lookbackCutoff(newRequest(nil), now)
	if err != nil || !got.IsZero() {
		t.Fatalf("omitted lookback = %v, %v; want zero, nil", got, err)
	}
	// UTC conversion must happen before calendar arithmetic, including at a
	// month boundary. Locally April 1 is still March 31 in UTC.
	local := time.Date(2024, time.April, 1, 0, 30, 0, 0, time.FixedZone("UTC+2", 7200))
	got, err = lookbackCutoff(newRequest(map[string]any{"lookback": "1mo"}), local)
	want := time.Date(2024, time.February, 29, 22, 30, 0, 0, time.UTC)
	if err != nil || !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("leap-year UTC cutoff = %v, %v; want %v", got, err, want)
	}
}

func TestSemanticSearchRejectsInvalidLookbackBeforeIO(t *testing.T) {
	// A zero client and nil store ensure invalid input cannot trigger IO.
	h := &handler{embeddingClient: &embedding.Client{}}
	for _, value := range []any{nil, 24, true, "", "0h", "-1d", "1.5d", "7", "1m", "1y", "1d2h", "999999999999999999999h", "9223372036854775807w", "999999mo"} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			result, err := h.semanticSearch(context.Background(), newRequest(map[string]any{"query": "test", "lookback": value}))
			if err != nil || result == nil || !result.IsError {
				t.Fatalf("got %v, %v; want tool error", result, err)
			}
		})
	}
}

// Capture the SQL at the database boundary, then stop before needing a real
// PostgreSQL server. This exercises the handler and the real query builder.
type lookbackQueryCapture struct {
	query string
	args  []driver.NamedValue
}

var errCapturedQuery = errors.New("captured query")

func (c *lookbackQueryCapture) Connect(context.Context) (driver.Conn, error) { return c, nil }
func (c *lookbackQueryCapture) Driver() driver.Driver                        { return c }
func (c *lookbackQueryCapture) Open(string) (driver.Conn, error)             { return c, nil }
func (c *lookbackQueryCapture) Close() error                                 { return nil }
func (c *lookbackQueryCapture) Begin() (driver.Tx, error)                    { return nil, errCapturedQuery }
func (c *lookbackQueryCapture) Prepare(string) (driver.Stmt, error) {
	return nil, errCapturedQuery
}
func (c *lookbackQueryCapture) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.query, c.args = query, args
	return nil, errCapturedQuery
}

func TestSemanticSearchLookbackReachesDatabase(t *testing.T) {
	for _, tt := range []struct {
		name     string
		lookback string
		fallback bool
	}{
		{"semantic filtered", "7d", false},
		{"fallback filtered", "7d", true},
		{"all time", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.fallback {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				fmt.Fprint(w, `{"data":[{"index":0,"embedding":[0.1,0.2]}]}`)
			}))
			defer server.Close()
			capture := &lookbackQueryCapture{}
			db := sql.OpenDB(capture)
			defer db.Close()
			h := &handler{store: storage.NewStorage(db), userID: 42, embeddingClient: embedding.NewClient(server.URL, "", "test", 2)}
			args := map[string]any{"query": "AI licensing", "limit": float64(10)}
			if tt.lookback != "" {
				args["lookback"] = tt.lookback
			}
			before := time.Now().UTC().Add(-7 * 24 * time.Hour)
			_, err := h.semanticSearch(context.Background(), newRequest(args))
			after := time.Now().UTC().Add(-7 * 24 * time.Hour)
			if capture.query == "" || err == nil || !strings.Contains(err.Error(), errCapturedQuery.Error()) {
				t.Fatalf("search did not reach database: %v", err)
			}
			hasCutoff := strings.Contains(capture.query, "e.published_at > $")
			if hasCutoff != (tt.lookback != "") {
				t.Fatalf("unexpected publication filter: %s", capture.query)
			}
			if tt.lookback != "" {
				var cutoff time.Time
				for _, arg := range capture.args {
					if value, ok := arg.Value.(time.Time); ok {
						cutoff = value
					}
				}
				if cutoff.Before(before) || cutoff.After(after) {
					t.Fatalf("SQL cutoff %v outside [%v, %v]", cutoff, before, after)
				}
			}
			if strings.Contains(capture.query, "plainto_tsquery") != tt.fallback {
				t.Fatalf("unexpected search mode: %s", capture.query)
			}
			if !strings.Contains(capture.query, "LIMIT 10") {
				t.Fatalf("result limit was lost: %s", capture.query)
			}
		})
	}
}
