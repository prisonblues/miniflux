// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package embedding // import "miniflux.app/v2/internal/embedding"

import (
	"strings"

	"golang.org/x/net/html"
)

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

// PrepareText concatenates the title and stripped HTML content for embedding.
func PrepareText(title, htmlContent string) string {
	plain := StripHTML(htmlContent)
	if plain == "" {
		return title
	}
	return title + " " + plain
}
