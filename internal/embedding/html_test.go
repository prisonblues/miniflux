// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package embedding

import (
	"testing"
)

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "simple tags",
			input:    "<p>hello</p> <b>world</b>",
			expected: "hello world",
		},
		{
			name:     "nested tags",
			input:    "<div><p>hello <strong>world</strong></p></div>",
			expected: "hello world",
		},
		{
			name:     "links",
			input:    `<a href="https://example.com">click here</a>`,
			expected: "click here",
		},
		{
			name:     "script and style",
			input:    `<p>text</p><script>alert('xss')</script><style>.x{}</style><p>more</p>`,
			expected: "text alert('xss') .x{} more",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   \n\t  ",
			expected: "",
		},
		{
			name:     "br tags",
			input:    "line1<br>line2<br/>line3",
			expected: "line1 line2 line3",
		},
		{
			name:     "entities",
			input:    "<p>A &amp; B &lt; C</p>",
			expected: "A & B < C",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripHTML(tt.input)
			if result != tt.expected {
				t.Errorf("StripHTML(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestPrepareText(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		htmlContent string
		expected    string
	}{
		{
			name:        "title and content",
			title:       "My Article",
			htmlContent: "<p>The body of the article.</p>",
			expected:    "My Article The body of the article.",
		},
		{
			name:        "title only",
			title:       "My Article",
			htmlContent: "",
			expected:    "My Article",
		},
		{
			name:        "title with empty HTML",
			title:       "My Article",
			htmlContent: "<div>   </div>",
			expected:    "My Article",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PrepareText(tt.title, tt.htmlContent)
			if result != tt.expected {
				t.Errorf("PrepareText(%q, %q) = %q, want %q", tt.title, tt.htmlContent, result, tt.expected)
			}
		})
	}
}
