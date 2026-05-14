// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package mcp // import "miniflux.app/v2/internal/mcp"

import (
	"time"

	"miniflux.app/v2/internal/embedding"
	"miniflux.app/v2/internal/model"
)

const snippetMaxLen = 200

// entrySummary is the concise entry representation returned by MCP list tools.
type entrySummary struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Feed        string  `json:"feed"`
	Category    string  `json:"category"`
	Published   string  `json:"published"`
	Status      string  `json:"status"`
	ReadingMins int     `json:"reading_mins"`
	Snippet     string  `json:"snippet"`
	Similarity  float64 `json:"similarity,omitempty"`
}

// entryDetail is the full entry representation returned by get_entry.
type entryDetail struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Feed        string `json:"feed"`
	Category    string `json:"category"`
	Published   string `json:"published"`
	Status      string `json:"status"`
	ReadingMins int    `json:"reading_mins"`
	Content     string `json:"content"`
}

// feedSummary is the feed representation returned by list_feeds.
type feedSummary struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	FeedURL  string `json:"feed_url"`
	SiteURL  string `json:"site_url"`
	Category string `json:"category"`
}

func formatEntrySummary(e *model.Entry) entrySummary {
	s := entrySummary{
		ID:          e.ID,
		Title:       e.Title,
		URL:         e.URL,
		Published:   e.Date.Format(time.RFC3339),
		Status:      e.Status,
		ReadingMins: readingMins(e.ReadingTime),
		Snippet:     snippet(e.Content),
		Similarity:  e.Similarity,
	}
	if e.Feed != nil {
		s.Feed = e.Feed.Title
		if e.Feed.Category != nil {
			s.Category = e.Feed.Category.Title
		}
	}
	return s
}

func formatEntryDetail(e *model.Entry) entryDetail {
	d := entryDetail{
		ID:          e.ID,
		Title:       e.Title,
		URL:         e.URL,
		Published:   e.Date.Format(time.RFC3339),
		Status:      e.Status,
		ReadingMins: readingMins(e.ReadingTime),
		Content:     e.Content,
	}
	if e.Feed != nil {
		d.Feed = e.Feed.Title
		if e.Feed.Category != nil {
			d.Category = e.Feed.Category.Title
		}
	}
	return d
}

func formatFeedSummary(f *model.Feed) feedSummary {
	s := feedSummary{
		ID:      f.ID,
		Title:   f.Title,
		FeedURL: f.FeedURL,
		SiteURL: f.SiteURL,
	}
	if f.Category != nil {
		s.Category = f.Category.Title
	}
	return s
}

// snippet strips HTML and truncates to snippetMaxLen characters.
func snippet(htmlContent string) string {
	plain := embedding.StripHTML(htmlContent)
	if len(plain) <= snippetMaxLen {
		return plain
	}
	// Truncate at a word boundary if possible.
	cut := snippetMaxLen
	for cut > 0 && plain[cut] != ' ' {
		cut--
	}
	if cut == 0 {
		cut = snippetMaxLen
	}
	return plain[:cut] + "..."
}

// readingMins returns the reading time in minutes.
// Entry.ReadingTime is already stored in minutes by the readingtime package.
func readingMins(minutes int) int {
	if minutes < 0 {
		return 0
	}
	return minutes
}
