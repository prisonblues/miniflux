// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"testing"
)

func TestVectorValueNil(t *testing.T) {
	var v Vector
	val, err := v.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != nil {
		t.Fatalf("expected nil, got %v", val)
	}
}

func TestVectorValueEmpty(t *testing.T) {
	v := Vector{}
	val, err := v.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "[]" {
		t.Fatalf("expected [], got %v", val)
	}
}

func TestVectorValueRoundTrip(t *testing.T) {
	v := Vector{0.1, 0.25, -0.5, 0}
	val, err := v.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}

	var parsed Vector
	if err := parsed.Scan(s); err != nil {
		t.Fatalf("scan error: %v", err)
	}

	if len(parsed) != len(v) {
		t.Fatalf("expected %d elements, got %d", len(v), len(parsed))
	}

	for i := range v {
		if parsed[i] != v[i] {
			t.Errorf("index %d: expected %f, got %f", i, v[i], parsed[i])
		}
	}
}

func TestVectorScanNil(t *testing.T) {
	var v Vector
	if err := v.Scan(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != nil {
		t.Fatalf("expected nil, got %v", v)
	}
}

func TestVectorScanBytes(t *testing.T) {
	var v Vector
	if err := v.Scan([]byte("[0.5,-0.3]")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(v) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(v))
	}
	if v[0] != 0.5 {
		t.Errorf("expected 0.5, got %f", v[0])
	}
	if v[1] != -0.3 {
		t.Errorf("expected -0.3, got %f", v[1])
	}
}

func TestVectorScanInvalidFormat(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"no brackets", "1,2,3"},
		{"missing close", "[1,2,3"},
		{"missing open", "1,2,3]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v Vector
			if err := v.Scan(tt.input); err == nil {
				t.Fatal("expected error for invalid format")
			}
		})
	}
}

func TestVectorScanInvalidFloat(t *testing.T) {
	var v Vector
	if err := v.Scan("[1.0,abc,3.0]"); err == nil {
		t.Fatal("expected error for invalid float")
	}
}

func TestVectorScanEmptyBrackets(t *testing.T) {
	var v Vector
	if err := v.Scan("[]"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(v) != 0 {
		t.Fatalf("expected empty vector, got %d elements", len(v))
	}
}

func TestVectorScanUnsupportedType(t *testing.T) {
	var v Vector
	if err := v.Scan(42); err == nil {
		t.Fatal("expected error for unsupported type")
	}
}
