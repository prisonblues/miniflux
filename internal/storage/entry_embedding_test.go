// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"testing"
)

// An already configured vector column must never enter the transaction that
// clears all embeddings. pgvector's atttypmod is the dimension itself.
func TestConfigureEmbeddingColumnPreservesMatchingVectors(t *testing.T) {
	for _, dimensions := range []int{384, 1024, 1536} {
		t.Run(fmt.Sprint(dimensions), func(t *testing.T) {
			conn := &embeddingColumnConn{dimensions: int64(dimensions)}
			db := sql.OpenDB(conn)
			defer db.Close()
			store := NewStorage(db)
			// Repeated startup must remain idempotent.
			for range 2 {
				if err := store.ConfigureEmbeddingColumn(dimensions); err != nil {
					t.Fatal(err)
				}
			}
			if conn.indexChecks != 2 {
				t.Fatalf("index existence checked %d times, want 2", conn.indexChecks)
			}
		})
	}
}

type embeddingColumnConn struct {
	dimensions  int64
	indexChecks int
}

func (c *embeddingColumnConn) Connect(context.Context) (driver.Conn, error) { return c, nil }
func (c *embeddingColumnConn) Driver() driver.Driver                        { return c }
func (c *embeddingColumnConn) Open(string) (driver.Conn, error)             { return c, nil }
func (c *embeddingColumnConn) Close() error                                 { return nil }
func (c *embeddingColumnConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("unexpected reconfiguration transaction: existing embeddings would be cleared")
}
func (c *embeddingColumnConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("unexpected prepared statement")
}
func (c *embeddingColumnConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "SELECT atttypmod") {
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
	return &embeddingDimensionRow{dimensions: c.dimensions}, nil
}
func (c *embeddingColumnConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if !strings.Contains(query, "CREATE INDEX IF NOT EXISTS entries_embedding_idx") {
		return nil, fmt.Errorf("unexpected database mutation: %s", query)
	}
	c.indexChecks++
	return driver.RowsAffected(0), nil
}

type embeddingDimensionRow struct {
	dimensions int64
	done       bool
}

func (r *embeddingDimensionRow) Columns() []string { return []string{"atttypmod"} }
func (r *embeddingDimensionRow) Close() error      { return nil }
func (r *embeddingDimensionRow) Next(values []driver.Value) error {
	if r.done {
		return io.EOF
	}
	values[0] = r.dimensions
	r.done = true
	return nil
}
