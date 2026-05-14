// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package embedding // import "miniflux.app/v2/internal/embedding"

import (
	"strings"

	"golang.org/x/net/html"
)

// StripHTML removes all HTML tags and returns the text content.
// Content inside <script> and <style> elements is excluded.
func StripHTML(s string) string {
	tokenizer := html.NewTokenizer(strings.NewReader(s))
	var b strings.Builder
	skip := 0

	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			return strings.TrimSpace(b.String())
		case html.StartTagToken:
			tn, _ := tokenizer.TagName()
			tag := string(tn)
			if tag == "script" || tag == "style" {
				skip++
			}
		case html.EndTagToken:
			tn, _ := tokenizer.TagName()
			tag := string(tn)
			if tag == "script" || tag == "style" {
				if skip > 0 {
					skip--
				}
			}
		case html.TextToken:
			if skip > 0 {
				continue
			}
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
// truncating to maxBytes to avoid sending unnecessarily large payloads.
// A maxBytes of 0 means no truncation.
func PrepareText(title, htmlContent string, maxBytes int) string {
	plain := StripHTML(htmlContent)
	var result string
	if plain == "" {
		result = title
	} else {
		result = title + " " + plain
	}

	if maxBytes > 0 && len(result) > maxBytes {
		result = truncateUTF8(result, maxBytes)
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
