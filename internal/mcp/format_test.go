// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"strings"
	"testing"
	"time"

	"miniflux.app/v2/internal/model"
)

func TestSnippetShortContent(t *testing.T) {
	got := snippet("plain text")
	if got != "plain text" {
		t.Errorf("expected 'plain text', got %q", got)
	}
}

func TestSnippetHTMLStripping(t *testing.T) {
	got := snippet("<p>Hello <b>world</b></p>")
	if got != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", got)
	}
}

func TestSnippetTruncation(t *testing.T) {
	long := strings.Repeat("word ", 100)
	got := snippet(long)
	if len(got) > snippetMaxLen+3 { // +3 for "..."
		t.Errorf("snippet too long: %d chars", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected truncated snippet to end with '...', got %q", got[len(got)-10:])
	}
}

func TestSnippetEmpty(t *testing.T) {
	got := snippet("")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestReadingMins(t *testing.T) {
	tests := []struct {
		minutes  int
		expected int
	}{
		{0, 0},
		{-1, 0},
		{1, 1},
		{4, 4},
		{15, 15},
	}

	for _, tt := range tests {
		got := readingMins(tt.minutes)
		if got != tt.expected {
			t.Errorf("readingMins(%d) = %d, want %d", tt.minutes, got, tt.expected)
		}
	}
}

func TestFormatEntrySummary(t *testing.T) {
	entry := &model.Entry{
		ID:          42,
		Title:       "Test Title",
		URL:         "https://example.com/article",
		Date:        time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC),
		Status:      "unread",
		ReadingTime: 4,
		Content:     "<p>Some content here</p>",
		Similarity:  0.85,
		Feed: &model.Feed{
			Title: "Test Feed",
			Category: &model.Category{
				Title: "Tech",
			},
		},
	}

	s := formatEntrySummary(entry)

	if s.ID != 42 {
		t.Errorf("ID = %d, want 42", s.ID)
	}
	if s.Title != "Test Title" {
		t.Errorf("Title = %q, want 'Test Title'", s.Title)
	}
	if s.URL != "https://example.com/article" {
		t.Errorf("URL = %q", s.URL)
	}
	if s.Feed != "Test Feed" {
		t.Errorf("Feed = %q, want 'Test Feed'", s.Feed)
	}
	if s.Category != "Tech" {
		t.Errorf("Category = %q, want 'Tech'", s.Category)
	}
	if s.Status != "unread" {
		t.Errorf("Status = %q, want 'unread'", s.Status)
	}
	if s.ReadingMins != 4 {
		t.Errorf("ReadingMins = %d, want 4", s.ReadingMins)
	}
	if s.Snippet != "Some content here" {
		t.Errorf("Snippet = %q, want 'Some content here'", s.Snippet)
	}
	if s.Similarity != 0.85 {
		t.Errorf("Similarity = %f, want 0.85", s.Similarity)
	}
	if s.Published != "2026-05-14T10:00:00Z" {
		t.Errorf("Published = %q", s.Published)
	}
}

func TestFormatEntrySummaryNilFeed(t *testing.T) {
	entry := &model.Entry{
		ID:    1,
		Title: "No Feed",
		Feed:  nil,
	}

	s := formatEntrySummary(entry)
	if s.Feed != "" {
		t.Errorf("Feed = %q, want empty", s.Feed)
	}
	if s.Category != "" {
		t.Errorf("Category = %q, want empty", s.Category)
	}
}

func TestFormatEntryDetail(t *testing.T) {
	entry := &model.Entry{
		ID:          99,
		Title:       "Detail Entry",
		URL:         "https://example.com",
		Date:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Status:      "read",
		ReadingTime: 3,
		Content:     "<p>Full HTML content</p>",
		Feed: &model.Feed{
			Title: "Feed",
			Category: &model.Category{
				Title: "Cat",
			},
		},
	}

	d := formatEntryDetail(entry)

	if d.Content != "<p>Full HTML content</p>" {
		t.Errorf("Content = %q, want raw HTML", d.Content)
	}
	if d.ReadingMins != 3 {
		t.Errorf("ReadingMins = %d, want 3", d.ReadingMins)
	}
}

func TestFormatFeedSummary(t *testing.T) {
	feed := &model.Feed{
		ID:      10,
		Title:   "My Feed",
		FeedURL: "https://example.com/feed.xml",
		SiteURL: "https://example.com",
		Category: &model.Category{
			Title: "News",
		},
	}

	s := formatFeedSummary(feed)

	if s.ID != 10 {
		t.Errorf("ID = %d, want 10", s.ID)
	}
	if s.Title != "My Feed" {
		t.Errorf("Title = %q", s.Title)
	}
	if s.Category != "News" {
		t.Errorf("Category = %q, want 'News'", s.Category)
	}
}

func TestFormatFeedSummaryNilCategory(t *testing.T) {
	feed := &model.Feed{
		ID:       1,
		Title:    "Feed",
		Category: nil,
	}

	s := formatFeedSummary(feed)
	if s.Category != "" {
		t.Errorf("Category = %q, want empty", s.Category)
	}
}
