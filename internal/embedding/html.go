// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package embedding // import "miniflux.app/v2/internal/embedding"

import (
	"strings"

	"golang.org/x/net/html"
)

// maxTextBytes caps the plain text sent to the embedding API. Most embedding
// models (all-MiniLM-L6-v2, nomic-embed-text, etc.) have a 256–512 token
// context window. ~2000 chars covers that comfortably without sending entire
// articles that the model would silently truncate anyway.
const maxTextBytes = 2000

// StripHTML removes all HTML tags and returns the text content.
func StripHTML(s string) string {
	tokenizer := html.NewTokenizer(strings.NewReader(s))
	var b strings.Builder

	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			return strings.TrimSpace(b.String())
		case html.TextToken:
			text := strings.TrimSpace(tokenizer.Token().Data)
			if text != "" {
				if b.Len() > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(text)
			}
		}
	}
}

// PrepareText concatenates the title and stripped HTML content for embedding,
// truncating to maxTextBytes to avoid sending large payloads that the model
// would silently truncate anyway.
func PrepareText(title, htmlContent string) string {
	plain := StripHTML(htmlContent)
	var result string
	if plain == "" {
		result = title
	} else {
		result = title + " " + plain
	}

	if len(result) > maxTextBytes {
		result = truncateUTF8(result, maxTextBytes)
	}
	return result
}

// truncateUTF8 truncates s to at most maxBytes without breaking UTF-8.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Walk backwards from the limit to find a valid UTF-8 boundary.
	for i := maxBytes; i > 0; i-- {
		if s[i]&0xC0 != 0x80 {
			return s[:i]
		}
	}
	return ""
}
