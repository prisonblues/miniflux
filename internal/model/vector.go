// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model // import "miniflux.app/v2/internal/model"

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
)

// Vector is a float32 slice that serializes to/from pgvector's text format.
type Vector []float32

// Value implements driver.Valuer, encoding the vector as "[0.1,0.2,0.3]".
func (v Vector) Value() (driver.Value, error) {
	if v == nil {
		return nil, nil
	}

	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String(), nil
}

// Scan implements sql.Scanner, parsing pgvector's text representation.
func (v *Vector) Scan(src any) error {
	if src == nil {
		*v = nil
		return nil
	}

	s, ok := src.(string)
	if !ok {
		if b, ok2 := src.([]byte); ok2 {
			s = string(b)
		} else {
			return fmt.Errorf("vector: cannot scan %T", src)
		}
	}

	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return fmt.Errorf("vector: invalid format %q", s)
	}

	inner := s[1 : len(s)-1]
	if inner == "" {
		*v = Vector{}
		return nil
	}

	parts := strings.Split(inner, ",")
	result := make(Vector, len(parts))
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 32)
		if err != nil {
			return fmt.Errorf("vector: invalid float at index %d: %w", i, err)
		}
		result[i] = float32(f)
	}

	*v = result
	return nil
}
